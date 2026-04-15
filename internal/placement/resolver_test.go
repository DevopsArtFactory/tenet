package placement

import (
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
)

func TestResolveReturnsPlacement(t *testing.T) {
	resolver := Resolver{}
	accounts := []domain.Account{{
		Name:           "prod-core",
		AWSAccountID:   "123456789012",
		AllowedRegions: []string{"ap-northeast-2"},
	}}
	environments := []domain.Environment{{
		Name:       "prod-apne2",
		AccountRef: "prod-core",
		Region:     "ap-northeast-2",
		NetworkRef: "prod-main",
	}}
	deployment := domain.Deployment{AppRef: "payments-api", EnvironmentRef: "prod-apne2"}

	placement, err := resolver.Resolve(accounts, environments, deployment)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if placement.AccountName != "prod-core" {
		t.Fatalf("unexpected account name: %s", placement.AccountName)
	}
}

func TestResolveFailsForMissingAccount(t *testing.T) {
	resolver := Resolver{}
	_, err := resolver.Resolve(nil, []domain.Environment{{Name: "prod", AccountRef: "missing", Region: "ap-northeast-2"}}, domain.Deployment{EnvironmentRef: "prod"})
	if err == nil {
		t.Fatalf("expected missing account error")
	}
}

func TestResolveFailsForDisallowedRegion(t *testing.T) {
	resolver := Resolver{}
	accounts := []domain.Account{{Name: "prod-core", AllowedRegions: []string{"us-east-1"}}}
	environments := []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}}

	_, err := resolver.Resolve(accounts, environments, domain.Deployment{EnvironmentRef: "prod-apne2"})
	if err == nil {
		t.Fatalf("expected disallowed region error")
	}
}
