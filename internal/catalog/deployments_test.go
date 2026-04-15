package catalog

import (
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
)

func TestListDeploymentsReturnsStableSortedView(t *testing.T) {
	deployments := ListDeployments(manifest.Catalog{
		Deployments: []domain.Deployment{
			{Name: "worker-prod", AppRef: "payments-worker", EnvironmentRef: "prod-apne2", TargetType: "ecs", DesiredArtifact: domain.DeploymentArtifact{Image: "ghcr.io/devopsartfactory/payments-worker:1.2.3"}},
			{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", TargetType: "ec2", DesiredArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64"}},
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
	if deployments[0].Binary == "" || deployments[1].Image == "" {
		t.Fatalf("expected artifact metadata in deployment views, got %+v", deployments)
	}
}
