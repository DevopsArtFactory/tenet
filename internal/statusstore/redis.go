package statusstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const defaultRedisKeyPrefix = "tenet:status"

var redisListSnapshotsScript = redis.NewScript(`
local names = redis.call("SMEMBERS", KEYS[1])
if #names == 0 then
  return {}
end

table.sort(names)
local payloads = {}
for _, name in ipairs(names) do
  local payload = redis.call("GET", ARGV[1] .. name)
  if not payload then
    return redis.error_reply("missing snapshot for deployment " .. name)
  end
  table.insert(payloads, payload)
end
return payloads
`)

var redisSaveSnapshotsScript = redis.NewScript(`
local indexKey = KEYS[1]
local snapshotPrefix = ARGV[1]
local desiredCount = tonumber(ARGV[2])
local desiredNames = {}
local offset = 3
for i = 1, desiredCount do
  local name = ARGV[offset]
  local payload = ARGV[offset + 1]
  desiredNames[name] = true
  redis.call("SET", snapshotPrefix .. name, payload)
  redis.call("SADD", indexKey, name)
  offset = offset + 2
end

local existingNames = redis.call("SMEMBERS", indexKey)
for _, name in ipairs(existingNames) do
  if not desiredNames[name] then
    redis.call("DEL", snapshotPrefix .. name)
    redis.call("SREM", indexKey, name)
  end
end
if desiredCount == 0 then
  redis.call("DEL", indexKey)
end
return desiredCount
`)

// RedisStore persists deployment status snapshots in Redis.
type RedisStore struct {
	client    redis.UniversalClient
	keyPrefix string
}

// NewRedisStore returns a Redis-backed status store.
func NewRedisStore(client redis.UniversalClient, keyPrefix string) RedisStore {
	if keyPrefix == "" {
		keyPrefix = defaultRedisKeyPrefix
	}
	return RedisStore{client: client, keyPrefix: keyPrefix}
}

func (s RedisStore) Client() redis.UniversalClient {
	return s.client
}

func (s RedisStore) indexKey() string {
	return s.keyPrefix + ":deployments"
}

func (s RedisStore) snapshotKey(name string) string {
	return fmt.Sprintf("%s:deployment:%s", s.keyPrefix, name)
}

// List returns all snapshots sorted by deployment name from one Redis batch read.
func (s RedisStore) List(ctx context.Context) ([]Snapshot, error) {
	result, err := redisListSnapshotsScript.Run(ctx, s.client, []string{s.indexKey()}, s.keyPrefix+":deployment:").Result()
	if err != nil {
		return nil, fmt.Errorf("read redis status batch: %w", err)
	}
	payloadValues, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("read redis status batch: unexpected script result type %T", result)
	}
	if len(payloadValues) == 0 {
		return []Snapshot{}, nil
	}
	snapshots := make([]Snapshot, 0, len(payloadValues))
	for _, payloadValue := range payloadValues {
		payload, ok := payloadValue.(string)
		if !ok {
			return nil, fmt.Errorf("read redis status batch: unexpected payload type %T", payloadValue)
		}
		var snapshot Snapshot
		if err := json.Unmarshal([]byte(payload), &snapshot); err != nil {
			return nil, fmt.Errorf("decode redis status snapshot: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

// Get returns one snapshot by deployment name.
func (s RedisStore) Get(ctx context.Context, deploymentName string) (Snapshot, error) {
	payload, err := s.client.Get(ctx, s.snapshotKey(deploymentName)).Result()
	if err != nil {
		if err == redis.Nil {
			return Snapshot{}, fmt.Errorf("%w: %s", ErrNotFound, deploymentName)
		}
		return Snapshot{}, fmt.Errorf("read redis status snapshot: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal([]byte(payload), &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode redis status snapshot: %w", err)
	}
	return snapshot, nil
}

// Save replaces the full snapshot set in Redis through one server-side batch.
func (s RedisStore) Save(ctx context.Context, snapshots []Snapshot) error {
	args := make([]interface{}, 0, 2+len(snapshots)*2)
	args = append(args, s.keyPrefix+":deployment:", len(snapshots))
	for _, snapshot := range snapshots {
		payload, err := json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("encode redis status snapshot: %w", err)
		}
		args = append(args, snapshot.DeploymentName, string(payload))
	}
	if _, err := redisSaveSnapshotsScript.Run(ctx, s.client, []string{s.indexKey()}, args...).Result(); err != nil {
		return fmt.Errorf("write redis status snapshots: %w", err)
	}
	return nil
}

// Upsert writes one snapshot into Redis and updates the deployment index.
func (s RedisStore) Upsert(ctx context.Context, snapshot Snapshot) error {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode redis status snapshot: %w", err)
	}
	pipe := s.client.TxPipeline()
	pipe.Set(ctx, s.snapshotKey(snapshot.DeploymentName), payload, 0)
	pipe.SAdd(ctx, s.indexKey(), snapshot.DeploymentName)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("write redis status snapshot: %w", err)
	}
	return nil
}

// Delete removes one snapshot from Redis and updates the deployment index.
func (s RedisStore) Delete(ctx context.Context, deploymentName string) error {
	pipe := s.client.TxPipeline()
	pipe.Del(ctx, s.snapshotKey(deploymentName))
	pipe.SRem(ctx, s.indexKey(), deploymentName)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("delete redis status snapshot: %w", err)
	}
	return nil
}

// Apply updates and deletes snapshots in one Redis transaction.
func (s RedisStore) Apply(ctx context.Context, upserts []Snapshot, deletes []string) error {
	pipe := s.client.TxPipeline()
	for _, snapshot := range upserts {
		payload, err := json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("encode redis status snapshot: %w", err)
		}
		pipe.Set(ctx, s.snapshotKey(snapshot.DeploymentName), payload, 0)
		pipe.SAdd(ctx, s.indexKey(), snapshot.DeploymentName)
	}
	for _, deploymentName := range deletes {
		pipe.Del(ctx, s.snapshotKey(deploymentName))
		pipe.SRem(ctx, s.indexKey(), deploymentName)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("write redis status batch: %w", err)
	}
	return nil
}
