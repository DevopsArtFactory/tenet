package spec

import "testing"

func TestValidateAccountSpecAcceptsMinimalMultiAccountConfiguration(t *testing.T) {
	account := AccountSpec{
		Name:           "prod-core",
		AWSAccountID:   "123456789012",
		RoleARN:        "arn:aws:iam::123456789012:role/TenetDeployerRole",
		AllowedRegions: []string{"ap-northeast-2"},
	}

	if err := ValidateAccount(account); err != nil {
		t.Fatalf("ValidateAccount returned error: %v", err)
	}
}

func TestValidateAccountSpecRejectsInvalidAccountID(t *testing.T) {
	account := AccountSpec{Name: "prod-core", AWSAccountID: "123", RoleARN: "arn:aws:iam::123456789012:role/TenetDeployerRole"}
	if err := ValidateAccount(account); err == nil {
		t.Fatalf("expected invalid account ID error")
	}
}

func TestValidateEnvironmentSpecRequiresPlacementFields(t *testing.T) {
	environment := EnvironmentSpec{Name: "prod-apne2"}
	if err := ValidateEnvironment(environment); err == nil {
		t.Fatalf("expected missing placement error")
	}
}

func TestValidateDeploymentSpecAcceptsEC2BlueGreen(t *testing.T) {
	deployment := DeploymentSpec{
		Name:           "payments-api-prod",
		AppRef:         "payments-api",
		EnvironmentRef: "prod-apne2",
		Target:         TargetSpec{Type: TargetTypeEC2},
		Strategy:       StrategySpec{Type: StrategyTypeBlueGreen, WarmupSeconds: 60, MinHealthyPercent: 100},
		Traffic:        TrafficSpec{Port: 443, Hostname: "api.example.com"},
		Capacity:       CapacitySpec{Desired: 4, Min: 2, Max: 8},
		Binary:         "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64",
	}

	if err := ValidateDeployment(deployment); err != nil {
		t.Fatalf("ValidateDeployment returned error: %v", err)
	}
}

func TestValidateDeploymentSpecAcceptsGoployerStyleEC2Manifest(t *testing.T) {
	deployment := DeploymentSpec{
		Name:     "payments-api-prod",
		Binary:   "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64",
		UserData: UserDataSpec{Type: "git", Path: "userdata/payments-api.sh"},
		Stacks: []StackSpec{{
			Stack:           "payments-api-prod",
			Account:         "prod-core",
			Env:             "prod-apne2",
			ReplacementType: "blueGreen",
			Capacity:        CapacitySpec{Desired: 3, Min: 2, Max: 6},
			Regions:         []RegionSpec{{Region: "ap-northeast-2", InstanceType: "t3.medium", AMIID: "ami-1234567890abcdef0"}},
		}},
	}

	if err := ValidateDeployment(deployment); err != nil {
		t.Fatalf("ValidateDeployment returned error for goployer-style manifest: %v", err)
	}
}

func TestValidateDeploymentSpecAcceptsECSImageManifest(t *testing.T) {
	deployment := DeploymentSpec{
		Name:   "payments-api-staging",
		Image:  "ghcr.io/devopsartfactory/payments-api:staging-20260415",
		Target: TargetSpec{Type: TargetTypeECS},
		Stacks: []StackSpec{{
			Stack:           "payments-api-staging",
			Account:         "staging-core",
			Env:             "staging-apne2",
			ReplacementType: "rolling",
			Capacity:        CapacitySpec{Desired: 2, Min: 1, Max: 3},
			Regions:         []RegionSpec{{Region: "ap-northeast-2"}},
		}},
	}

	if err := ValidateDeployment(deployment); err != nil {
		t.Fatalf("ValidateDeployment returned error for ECS image manifest: %v", err)
	}
}

func TestValidateDeploymentSpecRejectsUnknownTargetType(t *testing.T) {
	deployment := DeploymentSpec{
		Name:           "payments-api-prod",
		AppRef:         "payments-api",
		EnvironmentRef: "prod-apne2",
		Target:         TargetSpec{Type: TargetType("lambda")},
		Strategy:       StrategySpec{Type: StrategyTypeBlueGreen},
		Traffic:        TrafficSpec{Port: 443},
		Capacity:       CapacitySpec{Desired: 1, Min: 1, Max: 1},
	}

	if err := ValidateDeployment(deployment); err == nil {
		t.Fatalf("expected invalid target type error")
	}
}

func TestValidateDeploymentSpecRejectsUnknownStrategyType(t *testing.T) {
	deployment := DeploymentSpec{
		Name:           "payments-api-prod",
		AppRef:         "payments-api",
		EnvironmentRef: "prod-apne2",
		Target:         TargetSpec{Type: TargetTypeEC2},
		Strategy:       StrategySpec{Type: StrategyType("mystery")},
		Traffic:        TrafficSpec{Port: 443},
		Capacity:       CapacitySpec{Desired: 1, Min: 1, Max: 1},
	}

	if err := ValidateDeployment(deployment); err == nil {
		t.Fatalf("expected invalid strategy error")
	}
}

func TestValidateDeploymentSpecRejectsInvalidCapacityBounds(t *testing.T) {
	deployment := DeploymentSpec{
		Name:           "payments-api-prod",
		AppRef:         "payments-api",
		EnvironmentRef: "prod-apne2",
		Target:         TargetSpec{Type: TargetTypeEC2},
		Strategy:       StrategySpec{Type: StrategyTypeBlueGreen},
		Traffic:        TrafficSpec{Port: 443},
		Capacity:       CapacitySpec{Desired: 5, Min: 2, Max: 4},
	}

	if err := ValidateDeployment(deployment); err == nil {
		t.Fatalf("expected invalid capacity error")
	}
}

func TestValidateDeploymentSpecRejectsInvalidPort(t *testing.T) {
	deployment := DeploymentSpec{
		Name:           "payments-api-prod",
		AppRef:         "payments-api",
		EnvironmentRef: "prod-apne2",
		Target:         TargetSpec{Type: TargetTypeEC2},
		Strategy:       StrategySpec{Type: StrategyTypeBlueGreen},
		Traffic:        TrafficSpec{Port: 70000},
		Capacity:       CapacitySpec{Desired: 1, Min: 1, Max: 1},
	}

	if err := ValidateDeployment(deployment); err == nil {
		t.Fatalf("expected invalid port error")
	}
}

func TestValidateDeploymentSpecRejectsInvalidWarmup(t *testing.T) {
	deployment := DeploymentSpec{
		Name:           "payments-api-prod",
		AppRef:         "payments-api",
		EnvironmentRef: "prod-apne2",
		Target:         TargetSpec{Type: TargetTypeECS},
		Strategy:       StrategySpec{Type: StrategyTypeRolling, WarmupSeconds: -1, MinHealthyPercent: 50},
		Traffic:        TrafficSpec{Port: 8080},
		Capacity:       CapacitySpec{Desired: 2, Min: 1, Max: 3},
	}

	if err := ValidateDeployment(deployment); err == nil {
		t.Fatalf("expected invalid warmup error")
	}
}

func TestValidateDeploymentSpecRejectsInvalidHealthyPercent(t *testing.T) {
	deployment := DeploymentSpec{
		Name:           "payments-api-prod",
		AppRef:         "payments-api",
		EnvironmentRef: "prod-apne2",
		Target:         TargetSpec{Type: TargetTypeECS},
		Strategy:       StrategySpec{Type: StrategyTypeRolling, WarmupSeconds: 0, MinHealthyPercent: 101},
		Traffic:        TrafficSpec{Port: 8080},
		Capacity:       CapacitySpec{Desired: 2, Min: 1, Max: 3},
	}

	if err := ValidateDeployment(deployment); err == nil {
		t.Fatalf("expected invalid healthy percent error")
	}
}
