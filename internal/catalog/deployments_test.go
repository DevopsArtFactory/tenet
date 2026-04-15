package catalog

import (
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
)

func TestListDeploymentsReturnsStableSortedView(t *testing.T) {
	deployments := ListDeployments(manifest.Catalog{
		Deployments: []domain.Deployment{
			{Name: "worker-prod", AppRef: "payments-worker", EnvironmentRef: "prod-apne2"},
			{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"},
		},
	})

	if len(deployments) != 2 {
		t.Fatalf("expected two deployments, got %+v", deployments)
	}
	if deployments[0].Name != "api-prod" || deployments[1].Name != "worker-prod" {
		t.Fatalf("expected sorted deployment names, got %+v", deployments)
	}
	if deployments[0].AppRef != "payments-api" || deployments[0].EnvironmentRef != "prod-apne2" {
		t.Fatalf("unexpected deployment view: %+v", deployments[0])
	}
}
