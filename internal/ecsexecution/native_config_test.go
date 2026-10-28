package ecsexecution

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func TestNativeConfigExplicitlyClearsAlarmAndHookPolicyInSDKPayload(t *testing.T) {
	_, _, r := nativeFixture(t, "CANARY")
	r.Intent.Advanced.Alarms = nil
	r.Intent.Advanced.LifecycleHooks = nil
	config, err := advancedAWSConfig(r.Intent)
	if err != nil {
		t.Fatal(err)
	}
	if config.Alarms == nil || config.Alarms.Enable || config.Alarms.Rollback || config.Alarms.AlarmNames == nil || config.LifecycleHooks == nil {
		t.Fatal("omitted policy would preserve unreviewed alarms/hooks in UpdateService")
	}
	client, requests := nativeSDKFixture(t, func(string) string { return `{"service":{}}` })
	if _, err := client.UpdateService(context.Background(), &ecs.UpdateServiceInput{Cluster: aws.String(r.Intent.ClusterARN), Service: aws.String(r.Intent.ServiceARN), DeploymentConfiguration: config}); err != nil {
		t.Fatal(err)
	}
	wire := <-requests
	value := wire.body["deploymentConfiguration"].(map[string]any)
	alarms := value["alarms"].(map[string]any)
	if alarms["enable"] != false || alarms["rollback"] != false || len(alarms["alarmNames"].([]any)) != 0 || len(value["lifecycleHooks"].([]any)) != 0 {
		t.Fatal("SDK dropped explicit empty/disabled advanced policy")
	}
}
func TestNativeConfigNormalizationOwnsSlicesAndIgnoresAWSOrdering(t *testing.T) {
	_, _, r := nativeFixture(t, "BLUE_GREEN")
	r.Intent.Advanced.Alarms.Names = []string{"b", "a"}
	r.Intent.Advanced.LifecycleHooks[0].Stages = []string{"POST_SCALE_UP", "PRE_SCALE_UP"}
	config, err := advancedAWSConfig(r.Intent)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]string(nil), config.Alarms.AlarmNames...)
	slices.Reverse(config.Alarms.AlarmNames)
	slices.Reverse(config.LifecycleHooks[0].LifecycleStages)
	if !advancedConfigMatches(config, r.Intent) {
		t.Fatal("AWS stage/alarm ordering changed reviewed semantics")
	}
	normalized, err := normalizedAdvancedConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	normalized.Alarms.AlarmNames[0] = "mutated"
	normalized.LifecycleHooks[0].LifecycleStages[0] = types.DeploymentLifecycleHookStageReconcileService
	if reflect.DeepEqual(config.Alarms.AlarmNames, normalized.Alarms.AlarmNames) || r.Intent.Advanced.Alarms.Names[0] != before[0] || r.Intent.Advanced.LifecycleHooks[0].Stages[0] != "POST_SCALE_UP" {
		t.Fatal("config normalization mutated reviewed/current policy")
	}
}

func TestNativeConfigRejectsCountsOutsideTheECSIntegerRange(t *testing.T) {
	_, _, r := nativeFixture(t, "CANARY")
	for _, count := range []int{-1, 0, 1 << 31, 1<<32 + 2} {
		r.Intent.DesiredCount = count
		if _, err := advancedAWSConfig(r.Intent); err == nil {
			t.Fatal("invalid or wrapping count accepted", count)
		}
	}
}
