package catalog

import (
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
)

func TestListEnvironmentsReturnsStableSortedView(t *testing.T) {
	environments := ListEnvironments(manifest.Catalog{
		Environments: []domain.Environment{
			{Name: "prod-usw2", AccountRef: "prod-core", Region: "us-west-2", NetworkRef: "prod-west"},
			{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2", NetworkRef: "prod-main"},
		},
	})

	if len(environments) != 2 {
		t.Fatalf("expected two environments, got %+v", environments)
	}
	if environments[0].Name != "prod-apne2" || environments[1].Name != "prod-usw2" {
		t.Fatalf("expected sorted environment names, got %+v", environments)
	}
	if environments[0].AccountRef != "prod-core" || environments[0].Region != "ap-northeast-2" {
		t.Fatalf("unexpected environment view: %+v", environments[0])
	}
}
