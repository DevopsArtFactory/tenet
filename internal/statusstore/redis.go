package statusstore

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/redis/go-redis/v9"
)

const defaultRedisKeyPrefix = "tenet:status"

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

func (s RedisStore) indexKey() string {
	return s.keyPrefix + ":deployments"
}

func (s RedisStore) snapshotKey(name string) string {
	return fmt.Sprintf("%s:deployment:%s", s.keyPrefix, name)
}

// List returns all snapshots sorted by deployment name.
func (s RedisStore) List(ctx context.Context) ([]Snapshot, error) {
	names, err := s.client.SMembers(ctx, s.indexKey()).Result()
	if err != nil {
		return nil, fmt.Errorf("list redis status names: %w", err)
	}
	if len(names) == 0 {
		return []Snapshot{}, nil
	}
	sort.Strings(names)
	snapshots := make([]Snapshot, 0, len(names))
	for _, name := range names {
		snapshot, err := s.Get(ctx, name)
		if err != nil {
			return nil, err
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

// Save replaces the full snapshot set in Redis.
func (s RedisStore) Save(ctx context.Context, snapshots []Snapshot) error {
	existingNames, err := s.client.SMembers(ctx, s.indexKey()).Result()
	if err != nil {
		return fmt.Errorf("list existing redis status names: %w", err)
	}
	current := make(map[string]struct{}, len(snapshots))
	pipe := s.client.TxPipeline()
	for _, snapshot := range snapshots {
		payload, err := json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("encode redis status snapshot: %w", err)
		}
		current[snapshot.DeploymentName] = struct{}{}
		pipe.Set(ctx, s.snapshotKey(snapshot.DeploymentName), payload, 0)
		pipe.SAdd(ctx, s.indexKey(), snapshot.DeploymentName)
	}
	for _, name := range existingNames {
		if _, ok := current[name]; !ok {
			pipe.Del(ctx, s.snapshotKey(name))
			pipe.SRem(ctx, s.indexKey(), name)
		}
	}
	if len(snapshots) == 0 {
		pipe.Del(ctx, s.indexKey())
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("write redis status snapshots: %w", err)
	}
	return nil
}
