package spec

import (
	"encoding/json"
	"gopkg.in/yaml.v3"
	"math"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
)

func advancedContract(t *testing.T, strategy StrategyType) DeploymentSpec {
	t.Helper()
	d := ecsContractSpec(t)
	d.Strategy.Type = strategy
	zero := int32(0)
	percent := 5.5
	step := 12.5
	prefix := "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:"
	d.ECS.Advanced = &ECSAdvancedSpec{BakeTimeInMinutes: &zero, Routing: domain.ECSALBRouting{TargetGroupARN: prefix + "targetgroup/worker-blue/1111111111111111", AlternateTargetGroupARN: prefix + "targetgroup/worker-green/2222222222222222", ProductionListenerRuleARN: prefix + "listener-rule/app/worker/3333333333333333/4444444444444444/5555555555555555", TestListenerRuleARN: prefix + "listener-rule/app/worker/3333333333333333/6666666666666666/7777777777777777", InfrastructureRoleARN: "arn:aws:iam::123456789012:role/tenet/ecs-routing", ContainerPort: 8080}, Alarms: &domain.ECSDeploymentAlarms{Enable: true, Rollback: true, Names: []string{"worker-health"}}, LifecycleHooks: []domain.ECSLambdaHook{{FunctionARN: "arn:aws:lambda:ap-northeast-2:123456789012:function:validate-worker:3", RoleARN: "arn:aws:iam::123456789012:role/ecs-hook", Stages: []string{"POST_SCALE_UP"}}}}
	if strategy == StrategyTypeCanary {
		d.ECS.Advanced.Canary = &domain.ECSCanaryConfiguration{Percent: &percent, BakeTimeInMinutes: &zero}
	}
	if strategy == StrategyTypeLinear {
		d.ECS.Advanced.Linear = &domain.ECSLinearConfiguration{StepPercent: &step, StepBakeTimeInMinutes: &zero}
	}
	return d
}
func TestNativeAdvancedInputsSupportExplicitFractionalAndZeroSettings(t *testing.T) {
	for _, strategy := range []StrategyType{StrategyTypeBlueGreen, StrategyTypeCanary, StrategyTypeLinear} {
		d := advancedContract(t, strategy)
		if err := ValidateDeployment(d); err != nil {
			t.Fatalf("%s: %v", strategy, err)
		}
		// Previous intent-only previews remain compatible, without enabling execution.
		d.ECS.Advanced = nil
		if err := ValidateDeployment(d); err != nil {
			t.Fatal(err)
		}
	}
}
func TestNativeAdvancedInputsRejectAmbiguousOrUnsupportedConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*DeploymentSpec)
	}{
		{"Rolling", func(d *DeploymentSpec) { d.Strategy.Type = StrategyTypeRolling }},
		{"missing bake", func(d *DeploymentSpec) { d.ECS.Advanced.BakeTimeInMinutes = nil }},
		{"negative bake", func(d *DeploymentSpec) { *d.ECS.Advanced.BakeTimeInMinutes = -1 }},
		{"long bake", func(d *DeploymentSpec) { *d.ECS.Advanced.BakeTimeInMinutes = 1441 }},
		{"missing canary", func(d *DeploymentSpec) { d.ECS.Advanced.Canary = nil }},
		{"wrong strategy settings", func(d *DeploymentSpec) { d.Strategy.Type = StrategyTypeBlueGreen }},
		{"mixed linear", func(d *DeploymentSpec) { d.ECS.Advanced.Linear = &domain.ECSLinearConfiguration{} }},
		{"missing percent", func(d *DeploymentSpec) { d.ECS.Advanced.Canary.Percent = nil }},
		{"tiny percent", func(d *DeploymentSpec) { *d.ECS.Advanced.Canary.Percent = 0.01 }},
		{"too large percent", func(d *DeploymentSpec) { *d.ECS.Advanced.Canary.Percent = 100.1 }},
		{"fraction finer than tenths", func(d *DeploymentSpec) { *d.ECS.Advanced.Canary.Percent = 5.55 }},
		{"NaN percent", func(d *DeploymentSpec) { *d.ECS.Advanced.Canary.Percent = math.NaN() }},
		{"infinite percent", func(d *DeploymentSpec) { *d.ECS.Advanced.Canary.Percent = math.Inf(1) }},
		{"missing phase bake", func(d *DeploymentSpec) { d.ECS.Advanced.Canary.BakeTimeInMinutes = nil }},
		{"total schedule over 24h", func(d *DeploymentSpec) {
			*d.ECS.Advanced.Canary.BakeTimeInMinutes = 1440
			post := int32(1)
			d.ECS.Advanced.BakeTimeInMinutes = &post
		}},
		{"Rolling percentages", func(d *DeploymentSpec) { value := 100; d.ECS.MinimumHealthyPercent = &value }},
		{"one target", func(d *DeploymentSpec) {
			d.ECS.Advanced.Routing.AlternateTargetGroupARN = d.ECS.Advanced.Routing.TargetGroupARN
		}},
		{"cross account target", func(d *DeploymentSpec) {
			d.ECS.Advanced.Routing.TargetGroupARN = strings.ReplaceAll(d.ECS.Advanced.Routing.TargetGroupARN, "123456789012", "999999999999")
		}},
		{"cross region rule", func(d *DeploymentSpec) {
			d.ECS.Advanced.Routing.ProductionListenerRuleARN = strings.ReplaceAll(d.ECS.Advanced.Routing.ProductionListenerRuleARN, "ap-northeast-2", "us-east-1")
		}},
		{"NLB listener", func(d *DeploymentSpec) {
			d.ECS.Advanced.Routing.ProductionListenerRuleARN = "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:listener/net/worker/3333333333333333/4444444444444444"
		}},
		{"wildcard routing role", func(d *DeploymentSpec) {
			d.ECS.Advanced.Routing.InfrastructureRoleARN = "arn:aws:iam::123456789012:role/*"
		}},
		{"different test ALB", func(d *DeploymentSpec) {
			d.ECS.Advanced.Routing.TestListenerRuleARN = strings.ReplaceAll(d.ECS.Advanced.Routing.TestListenerRuleARN, "app/worker/", "app/other/")
		}},
		{"same test rule", func(d *DeploymentSpec) {
			d.ECS.Advanced.Routing.TestListenerRuleARN = d.ECS.Advanced.Routing.ProductionListenerRuleARN
		}},
		{"missing port", func(d *DeploymentSpec) { d.ECS.Advanced.Routing.ContainerPort = 0 }},
		{"disabled alarm block", func(d *DeploymentSpec) { d.ECS.Advanced.Alarms.Enable = false }},
		{"duplicate alarm", func(d *DeploymentSpec) { d.ECS.Advanced.Alarms.Names = []string{"worker-health", "worker-health"} }},
		{"empty alarm", func(d *DeploymentSpec) { d.ECS.Advanced.Alarms.Names = []string{""} }},
		{"unversioned hook", func(d *DeploymentSpec) {
			d.ECS.Advanced.LifecycleHooks[0].FunctionARN = "arn:aws:lambda:ap-northeast-2:123456789012:function:validate-worker"
		}},
		{"cross account hook role", func(d *DeploymentSpec) {
			d.ECS.Advanced.LifecycleHooks[0].RoleARN = "arn:aws:iam::999999999999:role/ecs-hook"
		}},
		{"unknown hook stage", func(d *DeploymentSpec) { d.ECS.Advanced.LifecycleHooks[0].Stages = []string{"PAUSE"} }},
		{"duplicate hook stage", func(d *DeploymentSpec) {
			d.ECS.Advanced.LifecycleHooks = append(d.ECS.Advanced.LifecycleHooks, d.ECS.Advanced.LifecycleHooks[0])
		}},
		{"test hook without route", func(d *DeploymentSpec) {
			d.ECS.Advanced.Routing.TestListenerRuleARN = ""
			d.ECS.Advanced.LifecycleHooks[0].Stages = []string{"TEST_TRAFFIC_SHIFT"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := advancedContract(t, StrategyTypeCanary)
			tc.change(&d)
			if err := ValidateDeployment(d); err == nil {
				t.Fatal("unsupported advanced configuration accepted")
			}
		})
	}
	linear := advancedContract(t, StrategyTypeLinear)
	*linear.ECS.Advanced.Linear.StepPercent = 2.9
	if err := ValidateDeployment(linear); err == nil {
		t.Fatal("Linear step below minimum accepted")
	}
	*linear.ECS.Advanced.Linear.StepPercent = 3
	*linear.ECS.Advanced.Linear.StepBakeTimeInMinutes = 60
	if err := ValidateDeployment(linear); err == nil {
		t.Fatal("multi-day Linear schedule accepted")
	}
}

func TestAdvancedDecoderRejectsUnsupportedFieldsInJSONAndYAML(t *testing.T) {
	for _, content := range []string{
		`{"bakeTimeInMinutes":0,"unknownSetting":true}`,
		`{"canary":{"percent":5,"bakeTimeInMinutes":0,"unknownSetting":true}}`,
		`{"routing":{"type":"nlb"}}`,
		`{"lifecycleHooks":[{"targetType":"PAUSE"}]}`,
	} {
		var d ECSAdvancedSpec
		if err := json.Unmarshal([]byte(content), &d); err == nil {
			t.Fatal("unsupported JSON traffic policy silently ignored")
		}
	}
	var d ECSAdvancedSpec
	if err := yaml.Unmarshal([]byte("bakeTimeInMinutes: 0\ncanary:\n  percent: 5\n  bakeTimeInMinutes: 0\n  unknownSetting: true\n"), &d); err == nil {
		t.Fatal("unsupported YAML traffic policy silently ignored")
	}
}

func TestAdvancedAutomaticSelfHealRequiresReviewedTrafficAndAutomaticMode(t *testing.T) {
	for _, strategy := range []StrategyType{StrategyTypeBlueGreen, StrategyTypeCanary, StrategyTypeLinear} {
		t.Run(string(strategy), func(t *testing.T) {
			d := advancedContract(t, strategy)
			d.ECS.SyncPolicy = &ECSSyncPolicy{Mode: "automatic"}
			if err := ValidateDeployment(d); err != nil {
				t.Fatal("reviewed advanced automatic sync rejected", err)
			}
			d.ECS.SyncPolicy.SelfHeal = true
			if err := ValidateDeployment(d); err != nil {
				t.Fatal("advanced drift self-heal rejected", err)
			}
			d.ECS.SyncPolicy.Mode = "manual"
			if err := ValidateDeployment(d); err == nil {
				t.Fatal("self-heal accepted without automatic mode")
			}
			d.ECS.SyncPolicy.Mode = "automatic"
			d.ECS.SyncPolicy.SelfHeal = false
			d.ECS.Advanced = nil
			if err := ValidateDeployment(d); err == nil {
				t.Fatal("intent-only legacy preview authorized automatic traffic changes")
			}
		})
	}
}
