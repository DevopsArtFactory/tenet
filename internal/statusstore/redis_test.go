package statusstore

import (
	"context"
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
	if _, err := store.Get(context.Background(), "missing"); err == nil {
		t.Fatalf("expected not found error")
	}
}
