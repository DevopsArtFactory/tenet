package catalog

import (
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
)

func TestListPlacementsReturnsStableSortedView(t *testing.T) {
	placements, err := ListPlacements(manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}}},
		Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2", NetworkRef: "prod-main"}},
		Deployments: []domain.Deployment{
			{Name: "worker-prod", AppRef: "payments-worker", EnvironmentRef: "prod-apne2"},
			{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"},
		},
	})
	if err != nil {
		t.Fatalf("ListPlacements returned error: %v", err)
	}
	if len(placements) != 2 {
		t.Fatalf("expected two placements, got %+v", placements)
	}
	if placements[0].DeploymentName != "api-prod" || placements[1].DeploymentName != "worker-prod" {
		t.Fatalf("expected sorted placement list, got %+v", placements)
	}
	if placements[0].AccountName != "prod-core" || placements[0].Region != "ap-northeast-2" {
		t.Fatalf("unexpected placement view: %+v", placements[0])
	}
}

func TestListPlacementsPropagatesResolutionErrors(t *testing.T) {
	_, err := ListPlacements(manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"us-west-2"}}},
		Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments:  []domain.Deployment{{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
	})
	if err == nil {
		t.Fatalf("expected placement resolution error")
	}
	if !strings.Contains(err.Error(), "resolve placement for deployment \"api-prod\"") {
		t.Fatalf("unexpected error: %v", err)
	}
}
