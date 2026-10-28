package spec

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/manifestcodec"
)

const ecsContractJSON = `{"name":"worker","environmentRef":"prod","target":{"type":"ecs"},"strategy":{"type":"rolling"},"image":"registry.example.com/team/worker:v2","capacity":{"desired":2,"min":1,"max":4},"ecs":{"deploymentController":"ECS","clusterArn":"arn:aws:ecs:ap-northeast-2:123456789012:cluster/production","serviceArn":"arn:aws:ecs:ap-northeast-2:123456789012:service/production/worker","baseTaskDefinitionArn":"arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:17","containerName":"worker"}}`

func ecsContractSpec(t *testing.T) DeploymentSpec {
	t.Helper()
	var d DeploymentSpec
	if err := json.Unmarshal([]byte(ecsContractJSON), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestECSExecutionInputValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*DeploymentSpec)
	}{
		{"wrong target", func(d *DeploymentSpec) { d.Target.Type = TargetTypeEC2; d.Image = "" }},
		{"missing controller", func(d *DeploymentSpec) { d.ECS.DeploymentController = "" }},
		{"legacy controller", func(d *DeploymentSpec) { d.ECS.DeploymentController = "CODE_DEPLOY" }},
		{"missing cluster", func(d *DeploymentSpec) { d.ECS.ClusterARN = "" }},
		{"short service ARN", func(d *DeploymentSpec) { d.ECS.ServiceARN = "arn:aws:ecs:ap-northeast-2:123456789012:service/worker" }},
		{"wrong cluster", func(d *DeploymentSpec) {
			d.ECS.ServiceARN = strings.ReplaceAll(d.ECS.ServiceARN, "production/", "other/")
		}},
		{"cross account", func(d *DeploymentSpec) {
			d.ECS.ServiceARN = strings.ReplaceAll(d.ECS.ServiceARN, "123456789012", "999999999999")
		}},
		{"cross region", func(d *DeploymentSpec) {
			d.ECS.BaseTaskDefinitionARN = strings.ReplaceAll(d.ECS.BaseTaskDefinitionARN, "ap-northeast-2", "us-east-1")
		}},
		{"cross partition", func(d *DeploymentSpec) {
			d.ECS.ServiceARN = strings.Replace(d.ECS.ServiceARN, "arn:aws:", "arn:aws-cn:", 1)
		}},
		{"unpinned revision", func(d *DeploymentSpec) {
			d.ECS.BaseTaskDefinitionARN = strings.TrimSuffix(d.ECS.BaseTaskDefinitionARN, ":17")
		}},
		{"zero revision", func(d *DeploymentSpec) {
			d.ECS.BaseTaskDefinitionARN = strings.ReplaceAll(d.ECS.BaseTaskDefinitionARN, ":17", ":0")
		}},
		{"revision overflow", func(d *DeploymentSpec) {
			d.ECS.BaseTaskDefinitionARN = strings.ReplaceAll(d.ECS.BaseTaskDefinitionARN, ":17", ":9999999999999999")
		}},
		{"missing container", func(d *DeploymentSpec) { d.ECS.ContainerName = "" }},
		{"invalid container", func(d *DeploymentSpec) { d.ECS.ContainerName = "worker/other" }},
		{"missing image", func(d *DeploymentSpec) { d.Image = "" }},
		{"credential URL", func(d *DeploymentSpec) { d.Image = "https://user:password@example.com/worker" }},
		{"malformed digest", func(d *DeploymentSpec) { d.Image = "worker@sha256:abc" }},
		{"duplicate digest", func(d *DeploymentSpec) { d.Image = "worker@bad@sha256:" + strings.Repeat("a", 64) }},
		{"bad minimum", func(d *DeploymentSpec) { d.ECS.MinimumHealthyPercent = new(int); *d.ECS.MinimumHealthyPercent = -1 }},
		{"no progress", func(d *DeploymentSpec) { d.ECS.MaximumPercent = new(int); *d.ECS.MaximumPercent = 100 }},
		{"rounding prevents progress", func(d *DeploymentSpec) {
			d.Capacity.Desired = 1
			d.ECS.MinimumHealthyPercent = new(int)
			*d.ECS.MinimumHealthyPercent = 50
			d.ECS.MaximumPercent = new(int)
			*d.ECS.MaximumPercent = 100
		}},
		{"conflicting minimum", func(d *DeploymentSpec) { d.Strategy.MinHealthyPercent = 75; d.ECS.MinimumHealthyPercent = new(int) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := ecsContractSpec(t)
			tc.change(&d)
			if err := ValidateDeployment(d); err == nil {
				t.Fatal("accepted invalid ECS execution inputs")
			}
		})
	}
	d := ecsContractSpec(t)
	if err := ValidateDeployment(d); err != nil {
		t.Fatal(err)
	}
	if err := ValidateECSPlacement(d, "999999999999", "ap-northeast-2"); err == nil {
		t.Fatal("accepted placement outside environment")
	}
	if err := ValidateECSPlacement(d, "123456789012", "us-east-1"); err == nil {
		t.Fatal("accepted wrong environment region")
	}
	d.ECS.MinimumHealthyPercent = new(int)
	if err := ValidateDeployment(d); err != nil {
		t.Fatal(err)
	}
	if min, max := ECSRollingPercentages(d); min != 0 || max != 200 {
		t.Fatal("explicit zero minimum lost")
	}
	d.ECS = nil
	if err := ValidateDeployment(d); err != nil {
		t.Fatalf("legacy read-only manifest no longer accepted: %v", err)
	}
}

func TestECSExecutionYAMLAndJSONAgree(t *testing.T) {
	var fromJSON, fromYAML DeploymentSpec
	if err := manifestcodec.Decode("worker.json", []byte(ecsContractJSON), &fromJSON); err != nil {
		t.Fatal(err)
	}
	yaml := `name: worker
environmentRef: prod
target: {type: ecs}
strategy: {type: rolling}
image: registry.example.com/team/worker:v2
capacity: {desired: 2, min: 1, max: 4}
ecs:
  deploymentController: ECS
  clusterArn: arn:aws:ecs:ap-northeast-2:123456789012:cluster/production
  serviceArn: arn:aws:ecs:ap-northeast-2:123456789012:service/production/worker
  baseTaskDefinitionArn: arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:17
  containerName: worker
`
	if err := manifestcodec.Decode("worker.yaml", []byte(yaml), &fromYAML); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(fromJSON)
	b, _ := json.Marshal(fromYAML)
	if string(a) != string(b) {
		t.Fatalf("JSON/YAML differ: %s vs %s", a, b)
	}
}

func TestAutomaticECSRecoveryRequiresExplicitOptInAndReviewedAdvancedInputs(t *testing.T) {
	d := ecsContractSpec(t)
	if d.ECS.RollbackOnFailure {
		t.Fatal("automatic rollback unexpectedly enabled by default")
	}
	d.ECS.RollbackOnFailure = true
	if err := validateECSDeployment(d); err != nil {
		t.Fatal(err)
	}
	d.Strategy.Type = StrategyTypeCanary
	if err := validateECSDeployment(d); err == nil {
		t.Fatal("advanced recovery accepted missing traffic inputs")
	}
}

func TestECSSyncPolicyDefaultsAndValidation(t *testing.T) {
	for _, tc := range []struct {
		mode  string
		heal  bool
		valid bool
	}{{"", false, true}, {"manual", false, true}, {"automatic", false, true}, {"automatic", true, true}, {"", true, false}, {"manual", true, false}, {"auto", false, false}} {
		d := ecsContractSpec(t)
		d.ECS.SyncPolicy = &ECSSyncPolicy{Mode: tc.mode, SelfHeal: tc.heal}
		if err := ValidateDeployment(d); (err == nil) != tc.valid {
			t.Fatalf("mode=%q selfHeal=%v: %v", tc.mode, tc.heal, err)
		}
	}
}

func TestAutomaticNativeRecoverySupportsAllReviewedStrategies(t *testing.T) {
	for _, strategy := range []StrategyType{StrategyTypeBlueGreen, StrategyTypeCanary, StrategyTypeLinear} {
		d := advancedContract(t, strategy)
		if d.ECS.RollbackOnFailure {
			t.Fatal("native rollback unexpectedly defaulted on")
		}
		d.ECS.RollbackOnFailure = true
		if err := ValidateDeployment(d); err != nil {
			t.Fatal(strategy, err)
		}
	}
}
