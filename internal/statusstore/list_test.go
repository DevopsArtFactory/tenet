package statusstore

import (
	"errors"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
)

func TestListReturnsStableSortedCopy(t *testing.T) {
	input := []Snapshot{
		{DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 2}},
		{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3}},
	}
	listed := List(input)
	if listed[0].DeploymentName != "api-prod" || listed[1].DeploymentName != "worker-prod" {
		t.Fatalf("expected sorted snapshots, got %+v", listed)
	}
	input[0].DeploymentName = "zzz"
	if listed[1].DeploymentName != "worker-prod" {
		t.Fatalf("expected defensive copy, got %+v", listed)
	}
}

func TestGetReturnsNamedSnapshot(t *testing.T) {
	snapshot, err := Get([]Snapshot{{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3}}}, "api-prod")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if snapshot.DeploymentName != "api-prod" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestGetReturnsNotFound(t *testing.T) {
	_, err := Get(nil, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}
