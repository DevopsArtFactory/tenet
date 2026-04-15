package catalog

import (
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
)

func TestListAccountsReturnsStableSortedView(t *testing.T) {
	accounts := ListAccounts(manifest.Catalog{
		Accounts: []domain.Account{
			{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2", "us-west-2"}},
			{Name: "dev-core", AWSAccountID: "123456789013", AllowedRegions: []string{"ap-northeast-2"}},
		},
	})

	if len(accounts) != 2 {
		t.Fatalf("expected two accounts, got %+v", accounts)
	}
	if accounts[0].Name != "dev-core" || accounts[1].Name != "prod-core" {
		t.Fatalf("expected sorted account names, got %+v", accounts)
	}
	if accounts[1].AWSAccountID != "123456789012" || len(accounts[1].AllowedRegions) != 2 {
		t.Fatalf("unexpected account view: %+v", accounts[1])
	}
}
