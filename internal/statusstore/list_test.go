package statusstore

import (
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
