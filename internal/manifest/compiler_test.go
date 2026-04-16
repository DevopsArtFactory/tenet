package manifest

import (
	"strings"
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

func TestCompileDerivesExecutionModesForEC2AndECS(t *testing.T) {
	catalog, err := Compile(CatalogSpec{
		Accounts: []spec.AccountSpec{
			{Name: "prod-core", AWSAccountID: "123456789012", RoleARN: "arn:aws:iam::123456789012:role/TenetDeployerRole", AllowedRegions: []string{"ap-northeast-2"}},
			{Name: "staging-core", AWSAccountID: "123456789013", RoleARN: "arn:aws:iam::123456789013:role/TenetDeployerRole", AllowedRegions: []string{"ap-northeast-2"}},
		},
		Environments: []spec.EnvironmentSpec{
			{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"},
			{Name: "staging-apne2", AccountRef: "staging-core", Region: "ap-northeast-2"},
		},
		Deployments: []spec.DeploymentSpec{
			{
				Name:   "payments-api-prod",
				Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64",
				Stacks: []spec.StackSpec{{
					Stack:           "payments-api-prod",
					Account:         "prod-core",
					Env:             "prod-apne2",
					ReplacementType: "canary",
					Capacity:        spec.CapacitySpec{Desired: 3, Min: 2, Max: 6},
					Regions:         []spec.RegionSpec{{Region: "ap-northeast-2"}},
				}},
			},
			{
				Name:   "payments-api-staging",
				Image:  "ghcr.io/devopsartfactory/payments-api:staging-20260415",
				Target: spec.TargetSpec{Type: spec.TargetTypeECS},
				Stacks: []spec.StackSpec{{
					Stack:           "payments-api-staging",
					Account:         "staging-core",
					Env:             "staging-apne2",
					ReplacementType: "linear",
					Capacity:        spec.CapacitySpec{Desired: 2, Min: 1, Max: 3},
					Regions:         []spec.RegionSpec{{Region: "ap-northeast-2"}},
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	if got := catalog.Deployments[0]; got.Strategy != "canary" || got.ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("unexpected EC2 rollout mapping: %+v", got)
	}
	if got := catalog.Deployments[1]; got.Strategy != "linear" || got.ExecutionMode != "ecsCodeDeploy" {
		t.Fatalf("unexpected ECS rollout mapping: %+v", got)
	}
}

func TestCompilePreservesNormalizedTopLevelUserDataForEC2(t *testing.T) {
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
		}},
		Deployments: []spec.DeploymentSpec{{
			Name:           "payments-api-prod",
			AppRef:         "payments-api",
			EnvironmentRef: "prod-apne2",
			Target:         spec.TargetSpec{Type: spec.TargetTypeEC2},
			Strategy:       spec.StrategySpec{Type: spec.StrategyTypeBlueGreen},
			Traffic:        spec.TrafficSpec{Port: 443},
			Capacity:       spec.CapacitySpec{Desired: 2, Min: 1, Max: 3},
			Binary:         "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64",
			UserData:       spec.UserDataSpec{Type: " file ", Path: " ./userdata/bootstrap/../payments-api-prod.sh "},
		}},
	})
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	if got := catalog.Deployments[0].UserData; got.Type != "file" || got.Path != "userdata/payments-api-prod.sh" {
		t.Fatalf("expected normalized EC2 userData, got %+v", got)
	}
}

func TestCompilePreservesNormalizedTopLevelUserDataForEC2Rolling(t *testing.T) {
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
		}},
		Deployments: []spec.DeploymentSpec{{
			Name:           "payments-api-prod",
			AppRef:         "payments-api",
			EnvironmentRef: "prod-apne2",
			Target:         spec.TargetSpec{Type: spec.TargetTypeEC2},
			Strategy:       spec.StrategySpec{Type: spec.StrategyTypeRolling},
			Traffic:        spec.TrafficSpec{Port: 443},
			Capacity:       spec.CapacitySpec{Desired: 2, Min: 1, Max: 3},
			Binary:         "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64",
			UserData:       spec.UserDataSpec{Type: " file ", Path: " ./userdata/refresh/../payments-api-refresh.sh "},
		}},
	})
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	if got := catalog.Deployments[0]; got.ExecutionMode != "asgInstanceRefresh" || got.UserData.Type != "file" || got.UserData.Path != "userdata/payments-api-refresh.sh" {
		t.Fatalf("expected rolling EC2 deployment to keep normalized userData, got %+v", got)
	}
}

func TestCompilePreservesNormalizedStackUserDataFallbackForEC2(t *testing.T) {
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
		}},
		Deployments: []spec.DeploymentSpec{{
			Name:   "payments-api-prod",
			Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64",
			Stacks: []spec.StackSpec{{
				Stack:           "payments-api-prod",
				Account:         "prod-core",
				Env:             "prod-apne2",
				ReplacementType: "blueGreen",
				UserData:        spec.UserDataSpec{Type: " git ", Path: " ./userdata/bootstrap/../payments-api-prod.sh "},
				Capacity:        spec.CapacitySpec{Desired: 2, Min: 1, Max: 3},
				Regions:         []spec.RegionSpec{{Region: "ap-northeast-2"}},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	if got := catalog.Deployments[0].UserData; got.Type != "git" || got.Path != "userdata/payments-api-prod.sh" {
		t.Fatalf("expected normalized stack userData fallback, got %+v", got)
	}
}

func TestCompileAssignsStableNonZeroDesiredGeneration(t *testing.T) {
	input := testCompileGenerationCatalogSpec()

	first, err := Compile(input)
	if err != nil {
		t.Fatalf("first Compile returned error: %v", err)
	}
	second, err := Compile(input)
	if err != nil {
		t.Fatalf("second Compile returned error: %v", err)
	}

	if first.Deployments[0].Generation == 0 {
		t.Fatalf("expected non-zero desired generation, got %+v", first.Deployments[0])
	}
	if first.Deployments[0].Generation != second.Deployments[0].Generation {
		t.Fatalf("expected stable desired generation, got %d and %d", first.Deployments[0].Generation, second.Deployments[0].Generation)
	}

	input.Deployments[0].Binary = "s3://releases.example.com/payments-api/prod/payments-api-linux-arm64"
	third, err := Compile(input)
	if err != nil {
		t.Fatalf("third Compile returned error: %v", err)
	}
	if third.Deployments[0].Generation == first.Deployments[0].Generation {
		t.Fatalf("expected desired generation to change when desired artifact changes, got %d", third.Deployments[0].Generation)
	}
}

func TestCompileChangesDesiredGenerationWhenCapacityChanges(t *testing.T) {
	input := testCompileGenerationCatalogSpec()

	first, err := Compile(input)
	if err != nil {
		t.Fatalf("first Compile returned error: %v", err)
	}

	input.Deployments[0].Capacity.Desired = 5
	second, err := Compile(input)
	if err != nil {
		t.Fatalf("second Compile returned error: %v", err)
	}

	if second.Deployments[0].Generation == first.Deployments[0].Generation {
		t.Fatalf("expected desired generation to change when capacity changes, got %d", second.Deployments[0].Generation)
	}
}

func TestCompileChangesDesiredGenerationWhenTagsChange(t *testing.T) {
	input := testCompileGenerationCatalogSpec()

	first, err := Compile(input)
	if err != nil {
		t.Fatalf("first Compile returned error: %v", err)
	}

	input.Deployments[0].Tags = append(input.Deployments[0].Tags, "compliance:pci")
	second, err := Compile(input)
	if err != nil {
		t.Fatalf("second Compile returned error: %v", err)
	}

	if second.Deployments[0].Generation == first.Deployments[0].Generation {
		t.Fatalf("expected desired generation to change when tags change, got %d", second.Deployments[0].Generation)
	}
}

func TestCompileChangesDesiredGenerationWhenReferencedEnvironmentPlacementChanges(t *testing.T) {
	input := testCompileGenerationCatalogSpec()

	first, err := Compile(input)
	if err != nil {
		t.Fatalf("first Compile returned error: %v", err)
	}

	input.Environments[0].NetworkRef = "prod-dr"
	second, err := Compile(input)
	if err != nil {
		t.Fatalf("second Compile returned error: %v", err)
	}

	if second.Deployments[0].Generation == first.Deployments[0].Generation {
		t.Fatalf("expected desired generation to change when referenced environment placement changes, got %d", second.Deployments[0].Generation)
	}
}

func TestCompileChangesDesiredGenerationWhenReferencedEnvironmentPolicyChanges(t *testing.T) {
	input := testCompileGenerationCatalogSpec()

	first, err := Compile(input)
	if err != nil {
		t.Fatalf("first Compile returned error: %v", err)
	}

	input.Environments[0].PolicyRef = "prod-strict"
	second, err := Compile(input)
	if err != nil {
		t.Fatalf("second Compile returned error: %v", err)
	}

	if second.Deployments[0].Generation == first.Deployments[0].Generation {
		t.Fatalf("expected desired generation to change when referenced environment policy changes, got %d", second.Deployments[0].Generation)
	}
}

func TestCompileChangesDesiredGenerationWhenReferencedAccountIdentityChanges(t *testing.T) {
	input := testCompileGenerationCatalogSpec()

	first, err := Compile(input)
	if err != nil {
		t.Fatalf("first Compile returned error: %v", err)
	}

	input.Accounts[0].AWSAccountID = "210987654321"
	second, err := Compile(input)
	if err != nil {
		t.Fatalf("second Compile returned error: %v", err)
	}

	if second.Deployments[0].Generation == first.Deployments[0].Generation {
		t.Fatalf("expected desired generation to change when referenced account identity changes, got %d", second.Deployments[0].Generation)
	}
}

func TestCompileChangesDesiredGenerationWhenReferencedAccountPlacementChanges(t *testing.T) {
	input := testCompileGenerationCatalogSpec()

	first, err := Compile(input)
	if err != nil {
		t.Fatalf("first Compile returned error: %v", err)
	}

	input.Accounts[0].AllowedRegions = []string{"ap-northeast-2", "us-west-2"}
	second, err := Compile(input)
	if err != nil {
		t.Fatalf("second Compile returned error: %v", err)
	}

	if second.Deployments[0].Generation == first.Deployments[0].Generation {
		t.Fatalf("expected desired generation to change when referenced account placement changes, got %d", second.Deployments[0].Generation)
	}
}

func testCompileGenerationCatalogSpec() CatalogSpec {
	return CatalogSpec{
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
			UserData:       spec.UserDataSpec{Type: "file", Path: "userdata/payments-api-prod.sh"},
			Tags:           []string{"team:payments", "tier:prod"},
			Stacks: []spec.StackSpec{{
				Stack:                      "payments-api-prod",
				Account:                    "prod-core",
				Env:                        "prod-apne2",
				ReplacementType:            "blueGreen",
				TerminationDelayRate:       30,
				RollingUpdateInstanceCount: 1,
				UserData:                   spec.UserDataSpec{Type: "file", Path: "userdata/payments-api-prod.sh"},
				Capacity:                   spec.CapacitySpec{Desired: 4, Min: 2, Max: 8},
				Regions:                    []spec.RegionSpec{{Region: "ap-northeast-2", InstanceType: "t3.medium", AMIID: "ami-1234567890abcdef0"}},
			}},
		}},
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

func TestCompileRejectsDuplicateAWSAccountID(t *testing.T) {
	_, err := Compile(CatalogSpec{
		Accounts: []spec.AccountSpec{
			{Name: "prod-core", AWSAccountID: "123456789012", RoleARN: "arn:aws:iam::123456789012:role/TenetDeployerRole"},
			{Name: "staging-core", AWSAccountID: "123456789012", RoleARN: "arn:aws:iam::123456789012:role/TenetDeployerRole"},
		},
	})
	if err == nil {
		t.Fatalf("expected duplicate AWS account ID error")
	}
	if !strings.Contains(err.Error(), `duplicate AWS account ID "123456789012"`) || !strings.Contains(err.Error(), `prod-core`) || !strings.Contains(err.Error(), `staging-core`) {
		t.Fatalf("expected clear duplicate AWS account ID error, got %v", err)
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
