package catalog

import (
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
)

func TestSummarizeCountsCatalogResources(t *testing.T) {
	summary := Summarize(manifest.Catalog{
		Accounts:     []domain.Account{{}, {}},
		Environments: []domain.Environment{{}, {}, {}},
		Deployments:  []domain.Deployment{{}, {}, {}, {}},
	})
	if summary.AccountCount != 2 || summary.EnvironmentCount != 3 || summary.DeploymentCount != 4 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}
