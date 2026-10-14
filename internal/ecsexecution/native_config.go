package ecsexecution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/spec"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func validateAdvancedIntent(intent domain.ECSDeploymentIntent) error {
	if intent.Advanced == nil || intent.DeploymentController != "ECS" {
		return fmt.Errorf("reviewed native advanced intent is required")
	}
	strategy := map[string]spec.StrategyType{"BLUE_GREEN": spec.StrategyTypeBlueGreen, "CANARY": spec.StrategyTypeCanary, "LINEAR": spec.StrategyTypeLinear}[intent.DeploymentStrategy]
	if strategy == "" {
		return fmt.Errorf("unsupported native advanced strategy")
	}
	contract := spec.DeploymentSpec{Name: "native-check", EnvironmentRef: "reviewed", Target: spec.TargetSpec{Type: spec.TargetTypeECS}, Strategy: spec.StrategySpec{Type: strategy}, Image: intent.Image, Capacity: spec.CapacitySpec{Desired: intent.DesiredCount, Min: intent.DesiredCount, Max: intent.DesiredCount}, ECS: &spec.ECSDeploymentSpec{DeploymentController: "ECS", ClusterARN: intent.ClusterARN, ServiceARN: intent.ServiceARN, BaseTaskDefinitionARN: intent.BaseTaskDefinitionARN, ContainerName: intent.ContainerName, Advanced: (*spec.ECSAdvancedSpec)(intent.Advanced)}}
	if err := spec.ValidateDeployment(contract); err != nil {
		return err
	}
	return spec.ValidateECSPlacement(contract, intent.AWSAccountID, intent.Region)
}
func verifiedNativeRole(intent domain.ECSDeploymentIntent, role applyrequest.DeployerRoleReadiness) error {
	if err := validateAdvancedIntent(intent); err != nil {
		return err
	}
	placement, err := arn.Parse(intent.ServiceARN)
	principal, roleErr := arn.Parse(role.RoleARN)
	if err != nil || roleErr != nil || principal.Service != "iam" || principal.Region != "" || !strings.HasPrefix(principal.Resource, "role/") || principal.AccountID != intent.AWSAccountID || principal.Partition != placement.Partition || !role.Configured || !role.Ready || !role.CheckedWithAWS || role.AWSAccountID != intent.AWSAccountID || role.Region != intent.Region {
		return fmt.Errorf("native execution requires verified service placement and deployer role")
	}
	return nil
}
func advancedAWSConfig(intent domain.ECSDeploymentIntent) (*types.DeploymentConfiguration, error) {
	if err := validateAdvancedIntent(intent); err != nil {
		return nil, err
	}
	c := intent.Advanced
	config := &types.DeploymentConfiguration{Strategy: types.DeploymentStrategy(intent.DeploymentStrategy), BakeTimeInMinutes: aws.Int32(*c.BakeTimeInMinutes), Alarms: &types.DeploymentAlarms{AlarmNames: []string{}}, LifecycleHooks: []types.DeploymentLifecycleHook{}}
	if c.Canary != nil {
		config.CanaryConfiguration = &types.CanaryConfiguration{CanaryPercent: aws.Float64(*c.Canary.Percent), CanaryBakeTimeInMinutes: aws.Int32(*c.Canary.BakeTimeInMinutes)}
	}
	if c.Linear != nil {
		config.LinearConfiguration = &types.LinearConfiguration{StepPercent: aws.Float64(*c.Linear.StepPercent), StepBakeTimeInMinutes: aws.Int32(*c.Linear.StepBakeTimeInMinutes)}
	}
	if c.Alarms != nil {
		config.Alarms = &types.DeploymentAlarms{Enable: true, Rollback: c.Alarms.Rollback, AlarmNames: append([]string(nil), c.Alarms.Names...)}
	}
	for _, hook := range c.LifecycleHooks {
		stages := make([]types.DeploymentLifecycleHookStage, len(hook.Stages))
		for i, stage := range hook.Stages {
			stages[i] = types.DeploymentLifecycleHookStage(stage)
		}
		config.LifecycleHooks = append(config.LifecycleHooks, types.DeploymentLifecycleHook{HookTargetArn: aws.String(hook.FunctionARN), RoleArn: aws.String(hook.RoleARN), LifecycleStages: stages})
	}
	return config, nil
}

// Normalize only fields managed by the advanced contract. AWS may reorder alarm
// names/stages; disabled alarms and absent hook slices are equivalent to none.
func normalizedAdvancedConfig(config *types.DeploymentConfiguration) (*types.DeploymentConfiguration, error) {
	if config == nil {
		return nil, fmt.Errorf("native service configuration unavailable")
	}
	copy := &types.DeploymentConfiguration{Strategy: config.Strategy, BakeTimeInMinutes: config.BakeTimeInMinutes, CanaryConfiguration: config.CanaryConfiguration, LinearConfiguration: config.LinearConfiguration}
	if config.DeploymentCircuitBreaker != nil && config.DeploymentCircuitBreaker.Enable {
		return nil, fmt.Errorf("Rolling circuit breaker cannot be inherited by native advanced execution")
	}
	if config.Alarms != nil && config.Alarms.Enable {
		alarms := *config.Alarms
		alarms.AlarmNames = append([]string(nil), alarms.AlarmNames...)
		slices.Sort(alarms.AlarmNames)
		copy.Alarms = &alarms
	}
	for _, hook := range config.LifecycleHooks {
		if hook.HookDetails != nil || aws.ToString(hook.HookTargetArn) == "" || aws.ToString(hook.RoleArn) == "" {
			return nil, fmt.Errorf("unsupported native lifecycle hook payload/type")
		}
		hook.LifecycleStages = append([]types.DeploymentLifecycleHookStage(nil), hook.LifecycleStages...)
		slices.Sort(hook.LifecycleStages)
		copy.LifecycleHooks = append(copy.LifecycleHooks, hook)
	}
	slices.SortFunc(copy.LifecycleHooks, func(a, b types.DeploymentLifecycleHook) int {
		aJSON, _ := json.Marshal(a)
		bJSON, _ := json.Marshal(b)
		return strings.Compare(string(aJSON), string(bJSON))
	})
	return copy, nil
}
func advancedConfigMatches(observed *types.DeploymentConfiguration, intent domain.ECSDeploymentIntent) bool {
	expected, err := advancedAWSConfig(intent)
	if err != nil {
		return false
	}
	expected, err = normalizedAdvancedConfig(expected)
	if err != nil {
		return false
	}
	actual, err := normalizedAdvancedConfig(observed)
	return err == nil && reflect.DeepEqual(actual, expected)
}
func nativeConfigurationHash(config *types.DeploymentConfiguration) (string, error) {
	normalized, err := normalizedAdvancedConfig(config)
	if err != nil {
		return "", err
	}
	normalized.MinimumHealthyPercent = config.MinimumHealthyPercent
	normalized.MaximumPercent = config.MaximumPercent
	content, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}
