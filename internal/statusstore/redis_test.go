package statusstore

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisStoreSaveListAndGet(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer server.Close()

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	store := NewRedisStore(client, "tenet:test")
	ctx := context.Background()
	input := []Snapshot{
		{DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 2, Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "warming"}}},
		{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "stable"}, DeployedArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64"}}},
	}

	if err := store.Save(ctx, input); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 2 || listed[0].DeploymentName != "api-prod" || listed[1].DeploymentName != "worker-prod" {
		t.Fatalf("unexpected listed snapshots: %+v", listed)
	}
	got, err := store.Get(ctx, "api-prod")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Status.DeployedArtifact.Binary == "" {
		t.Fatalf("expected artifact metadata, got %+v", got)
	}
}

func TestRedisStoreGetReturnsNotFound(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer server.Close()

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	store := NewRedisStore(client, "tenet:test")
	if _, err := store.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRedisStoreUpsertCreatesAndReplacesSnapshot(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer server.Close()

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	store := NewRedisStore(client, "tenet:test")
	ctx := context.Background()

	if err := store.Upsert(ctx, Snapshot{DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 2, Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "warming"}}}); err != nil {
		t.Fatalf("first Upsert returned error: %v", err)
	}
	if err := store.Upsert(ctx, Snapshot{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "stable"}}}); err != nil {
		t.Fatalf("second Upsert returned error: %v", err)
	}
	if err := store.Upsert(ctx, Snapshot{DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 4, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "rolled-forward"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "all checks passed"}}}); err != nil {
		t.Fatalf("replacement Upsert returned error: %v", err)
	}

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 2 || listed[0].DeploymentName != "api-prod" || listed[1].DeploymentName != "worker-prod" {
		t.Fatalf("unexpected listed snapshots: %+v", listed)
	}
	if listed[1].Status.ObservedGeneration != 4 || listed[1].Status.Sync.Reason != "rolled-forward" || listed[1].Status.Health.Overall != domain.HealthHealthy {
		t.Fatalf("expected replaced worker snapshot, got %+v", listed[1])
	}

	members, err := server.Members("tenet:test:deployments")
	if err != nil {
		t.Fatalf("Members returned error: %v", err)
	}
	if len(members) != 2 || members[0] != "api-prod" || members[1] != "worker-prod" {
		t.Fatalf("unexpected redis deployment index members: %+v", members)
	}
}

func TestRedisStoreDeleteRemovesSnapshotAndMissingIsIgnored(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer server.Close()

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	store := NewRedisStore(client, "tenet:test")
	ctx := context.Background()

	if err := store.Save(ctx, []Snapshot{
		{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3}},
		{DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 2}},
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if err := store.Delete(ctx, "worker-prod"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if err := store.Delete(ctx, "missing"); err != nil {
		t.Fatalf("Delete for missing snapshot returned error: %v", err)
	}

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 1 || listed[0].DeploymentName != "api-prod" {
		t.Fatalf("unexpected listed snapshots after delete: %+v", listed)
	}
	if _, err := store.Get(ctx, "worker-prod"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected worker-prod to be deleted with ErrNotFound, got %v", err)
	}
	members, err := server.Members("tenet:test:deployments")
	if err != nil {
		t.Fatalf("Members returned error: %v", err)
	}
	if len(members) != 1 || members[0] != "api-prod" {
		t.Fatalf("unexpected redis deployment index members after delete: %+v", members)
	}
}

func TestRedisStoreListRejectsIndexEntriesMissingSnapshots(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer server.Close()

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	store := NewRedisStore(client, "tenet:test")
	if _, err := server.SAdd("tenet:test:deployments", "ghost-prod"); err != nil {
		t.Fatalf("SAdd returned error: %v", err)
	}

	_, err = store.List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "read redis status batch") {
		t.Fatalf("expected batch read error for missing indexed snapshot, got %v", err)
	}
}

func TestRedisStoreApplyBatchesUpdatesAndDeletes(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer server.Close()

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	store := NewRedisStore(client, "tenet:test")
	ctx := context.Background()
	if err := store.Save(ctx, []Snapshot{
		{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 1}},
		{DeploymentName: "legacy-prod", Status: domain.DeploymentStatus{ObservedGeneration: 9}},
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	if err := store.Apply(ctx, []Snapshot{
		{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "stable"}}},
		{DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 2}},
	}, []string{"legacy-prod"}); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 2 || listed[0].DeploymentName != "api-prod" || listed[1].DeploymentName != "worker-prod" {
		t.Fatalf("unexpected listed snapshots after apply: %+v", listed)
	}
	if listed[0].Status.ObservedGeneration != 3 || listed[0].Status.Sync.Reason != "stable" {
		t.Fatalf("expected api-prod to be updated in apply batch, got %+v", listed[0])
	}
	if _, err := store.Get(ctx, "legacy-prod"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected legacy-prod to be removed by apply batch, got %v", err)
	}
}
