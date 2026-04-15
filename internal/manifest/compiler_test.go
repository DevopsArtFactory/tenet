package manifest

import (
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/spec"
)

func TestCompileReturnsDomainCatalog(t *testing.T) {
	catalog, err := Compile(CatalogSpec{
		Accounts: []spec.AccountSpec{{
			Name:           "prod-core",
			AWSAccountID:   "123456789012",
			RoleARN:        "arn:aws:iam::123456789012:role/TenetDeployerRole",
			AllowedRegions: []string{"ap-northeast-2"},
		}},
		Environments: []spec.EnvironmentSpec{{
			Name:       "prod-apne2",
			AccountRef: "prod-core",
			Region:     "ap-northeast-2",
			NetworkRef: "prod-main",
		}},
		Deployments: []spec.DeploymentSpec{{
			Name:           "payments-api-prod",
			AppRef:         "payments-api",
			EnvironmentRef: "prod-apne2",
			Target:         spec.TargetSpec{Type: spec.TargetTypeEC2},
			Strategy:       spec.StrategySpec{Type: spec.StrategyTypeBlueGreen, WarmupSeconds: 60, MinHealthyPercent: 100},
			Traffic:        spec.TrafficSpec{Hostname: "api.example.com", Port: 443},
			Capacity:       spec.CapacitySpec{Desired: 4, Min: 2, Max: 8},
			Binary:         "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64",
		}},
	})
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	if len(catalog.Accounts) != 1 || len(catalog.Environments) != 1 || len(catalog.Deployments) != 1 {
		t.Fatalf("unexpected catalog counts: %+v", catalog)
	}
	if catalog.Deployments[0].AppRef != "payments-api" {
		t.Fatalf("unexpected deployment appRef: %s", catalog.Deployments[0].AppRef)
	}
	if catalog.Deployments[0].DesiredArtifact.Binary != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64" {
		t.Fatalf("unexpected deployment binary: %+v", catalog.Deployments[0].DesiredArtifact)
	}
}

func TestCompileAcceptsGoployerStyleDeploymentManifest(t *testing.T) {
	catalog, err := Compile(CatalogSpec{
		Accounts: []spec.AccountSpec{{
			Name:           "prod-core",
			AWSAccountID:   "123456789012",
			RoleARN:        "arn:aws:iam::123456789012:role/TenetDeployerRole",
			AllowedRegions: []string{"ap-northeast-2"},
		}},
		Environments: []spec.EnvironmentSpec{{
			Name:       "prod-apne2",
			AccountRef: "prod-core",
			Region:     "ap-northeast-2",
			NetworkRef: "prod-main",
		}},
		Deployments: []spec.DeploymentSpec{{
			Name:   "payments-api-prod",
			Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64",
			Stacks: []spec.StackSpec{{
				Stack:           "payments-api-prod",
				Account:         "prod-core",
				Env:             "prod-apne2",
				ReplacementType: "blueGreen",
				Capacity:        spec.CapacitySpec{Desired: 3, Min: 2, Max: 6},
				Regions:         []spec.RegionSpec{{Region: "ap-northeast-2", InstanceType: "t3.medium"}},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Compile returned error for goployer-style deployment: %v", err)
	}
	if got := catalog.Deployments[0]; got.AppRef != "payments-api-prod" || got.EnvironmentRef != "prod-apne2" {
		t.Fatalf("unexpected compiled deployment: %+v", got)
	}
}

func TestCompileRejectsGoployerStackAccountMismatch(t *testing.T) {
	_, err := Compile(CatalogSpec{
		Accounts: []spec.AccountSpec{
			{Name: "prod-core", AWSAccountID: "123456789012", RoleARN: "arn:aws:iam::123456789012:role/TenetDeployerRole", AllowedRegions: []string{"ap-northeast-2"}},
			{Name: "other-core", AWSAccountID: "123456789013", RoleARN: "arn:aws:iam::123456789013:role/TenetDeployerRole", AllowedRegions: []string{"ap-northeast-2"}},
		},
		Environments: []spec.EnvironmentSpec{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments: []spec.DeploymentSpec{{
			Name:   "payments-api-prod",
			Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64",
			Stacks: []spec.StackSpec{{Stack: "payments-api-prod", Account: "other-core", Env: "prod-apne2", ReplacementType: "blueGreen", Capacity: spec.CapacitySpec{Desired: 3, Min: 2, Max: 6}, Regions: []spec.RegionSpec{{Region: "ap-northeast-2"}}}},
		}},
	})
	if err == nil {
		t.Fatalf("expected stack account mismatch error")
	}
}

func TestCompileRejectsGoployerStackRegionMismatch(t *testing.T) {
	_, err := Compile(CatalogSpec{
		Accounts:     []spec.AccountSpec{{Name: "prod-core", AWSAccountID: "123456789012", RoleARN: "arn:aws:iam::123456789012:role/TenetDeployerRole", AllowedRegions: []string{"ap-northeast-2", "us-west-2"}}},
		Environments: []spec.EnvironmentSpec{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments: []spec.DeploymentSpec{{
			Name:   "payments-api-prod",
			Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64",
			Stacks: []spec.StackSpec{{Stack: "payments-api-prod", Account: "prod-core", Env: "prod-apne2", ReplacementType: "blueGreen", Capacity: spec.CapacitySpec{Desired: 3, Min: 2, Max: 6}, Regions: []spec.RegionSpec{{Region: "us-west-2"}}}},
		}},
	})
	if err == nil {
		t.Fatalf("expected stack region mismatch error")
	}
}

func TestCompileRejectsEnvironmentReferencingUnknownAccount(t *testing.T) {
	_, err := Compile(CatalogSpec{
		Accounts: []spec.AccountSpec{{
			Name:         "prod-core",
			AWSAccountID: "123456789012",
			RoleARN:      "arn:aws:iam::123456789012:role/TenetDeployerRole",
		}},
		Environments: []spec.EnvironmentSpec{{
			Name:       "prod-apne2",
			AccountRef: "missing",
			Region:     "ap-northeast-2",
		}},
	})
	if err == nil {
		t.Fatalf("expected unknown account error")
	}
}

func TestCompileRejectsDuplicateAccountNames(t *testing.T) {
	_, err := Compile(CatalogSpec{
		Accounts: []spec.AccountSpec{
			{Name: "prod-core", AWSAccountID: "123456789012", RoleARN: "arn:aws:iam::123456789012:role/TenetDeployerRole"},
			{Name: "prod-core", AWSAccountID: "123456789013", RoleARN: "arn:aws:iam::123456789013:role/TenetDeployerRole"},
		},
	})
	if err == nil {
		t.Fatalf("expected duplicate account error")
	}
}

func TestCompileRejectsDuplicateEnvironmentNames(t *testing.T) {
	_, err := Compile(CatalogSpec{
		Accounts: []spec.AccountSpec{{
			Name:         "prod-core",
			AWSAccountID: "123456789012",
			RoleARN:      "arn:aws:iam::123456789012:role/TenetDeployerRole",
		}},
		Environments: []spec.EnvironmentSpec{
			{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"},
			{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"},
		},
	})
	if err == nil {
		t.Fatalf("expected duplicate environment error")
	}
}

func TestCompileRejectsDuplicateDeploymentNames(t *testing.T) {
	_, err := Compile(CatalogSpec{
		Accounts: []spec.AccountSpec{{
			Name:         "prod-core",
			AWSAccountID: "123456789012",
			RoleARN:      "arn:aws:iam::123456789012:role/TenetDeployerRole",
		}},
		Environments: []spec.EnvironmentSpec{{
			Name:       "prod-apne2",
			AccountRef: "prod-core",
			Region:     "ap-northeast-2",
		}},
		Deployments: []spec.DeploymentSpec{
			{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", Target: spec.TargetSpec{Type: spec.TargetTypeEC2}, Strategy: spec.StrategySpec{Type: spec.StrategyTypeBlueGreen}, Traffic: spec.TrafficSpec{Port: 443}, Capacity: spec.CapacitySpec{Desired: 1, Min: 1, Max: 1}},
			{Name: "payments-api-prod", AppRef: "payments-worker", EnvironmentRef: "prod-apne2", Target: spec.TargetSpec{Type: spec.TargetTypeECS}, Strategy: spec.StrategySpec{Type: spec.StrategyTypeRolling}, Traffic: spec.TrafficSpec{Port: 8080}, Capacity: spec.CapacitySpec{Desired: 1, Min: 1, Max: 1}},
		},
	})
	if err == nil {
		t.Fatalf("expected duplicate deployment error")
	}
}

func TestCompileRejectsDeploymentReferencingUnknownEnvironment(t *testing.T) {
	_, err := Compile(CatalogSpec{
		Accounts: []spec.AccountSpec{{
			Name:         "prod-core",
			AWSAccountID: "123456789012",
			RoleARN:      "arn:aws:iam::123456789012:role/TenetDeployerRole",
		}},
		Environments: []spec.EnvironmentSpec{{
			Name:       "prod-apne2",
			AccountRef: "prod-core",
			Region:     "ap-northeast-2",
		}},
		Deployments: []spec.DeploymentSpec{{
			Name:           "payments-api-prod",
			AppRef:         "payments-api",
			EnvironmentRef: "missing",
			Target:         spec.TargetSpec{Type: spec.TargetTypeEC2},
			Strategy:       spec.StrategySpec{Type: spec.StrategyTypeBlueGreen},
			Traffic:        spec.TrafficSpec{Port: 443},
			Capacity:       spec.CapacitySpec{Desired: 1, Min: 1, Max: 1},
		}},
	})
	if err == nil {
		t.Fatalf("expected unknown environment error")
	}
}
