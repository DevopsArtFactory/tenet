package catalog

import (
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
)

func TestBuildTopologyReturnsSummaryAndStableViews(t *testing.T) {
	topology, err := BuildTopology(manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}}},
		Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2", NetworkRef: "prod-main"}},
		Deployments:  []domain.Deployment{{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
	})
	if err != nil {
		t.Fatalf("BuildTopology returned error: %v", err)
	}
	if topology.Summary.AccountCount != 1 || topology.Summary.EnvironmentCount != 1 || topology.Summary.DeploymentCount != 1 {
		t.Fatalf("unexpected summary: %+v", topology.Summary)
	}
	if len(topology.Accounts) != 1 || len(topology.Environments) != 1 || len(topology.Deployments) != 1 || len(topology.Placements) != 1 {
		t.Fatalf("unexpected topology: %+v", topology)
	}
	if topology.Placements[0].DeploymentName != "api-prod" || topology.Placements[0].AccountName != "prod-core" {
		t.Fatalf("unexpected placement: %+v", topology.Placements[0])
	}
}

func TestBuildTopologyPropagatesPlacementErrors(t *testing.T) {
	_, err := BuildTopology(manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"us-west-2"}}},
		Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments:  []domain.Deployment{{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
	})
	if err == nil {
		t.Fatalf("expected topology build error")
	}
	if !strings.Contains(err.Error(), "resolve placement for deployment \"api-prod\"") {
		t.Fatalf("unexpected error: %v", err)
	}
}
