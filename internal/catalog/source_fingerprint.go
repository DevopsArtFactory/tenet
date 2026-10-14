package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
)

// DeriveDeploymentSourceFingerprint returns the deterministic deployment-detail fingerprint
// used by overview detail routes and plan preview optimistic matching.
func DeriveDeploymentSourceFingerprint(detail DeploymentOverviewDetail) (string, error) {
	payload := struct {
		Git                *domain.GitSource               `json:"git,omitempty"`
		ECS                *domain.ECSDeploymentIntent     `json:"ecs,omitempty"`
		DeploymentName     string                          `json:"deploymentName"`
		TargetType         string                          `json:"targetType,omitempty"`
		Strategy           string                          `json:"strategy,omitempty"`
		ExecutionMode      string                          `json:"executionMode,omitempty"`
		DesiredArtifact    domain.DeploymentArtifact       `json:"desiredArtifact,omitempty"`
		DeployedArtifact   domain.DeploymentArtifact       `json:"deployedArtifact,omitempty"`
		RuntimeTimestamp   string                          `json:"runtimeTimestamp,omitempty"`
		RuntimeSource      string                          `json:"runtimeSource,omitempty"`
		CurrentStep        string                          `json:"currentStep,omitempty"`
		DesiredGeneration  int64                           `json:"desiredGeneration,omitempty"`
		ObservedGeneration int64                           `json:"observedGeneration,omitempty"`
		Sync               domain.Sync                     `json:"sync"`
		Health             domain.Health                   `json:"health"`
		Conditions         []domain.Condition              `json:"conditions,omitempty"`
		RuntimeChanges     []string                        `json:"runtimeChanges,omitempty"`
		RuntimeDetails     []string                        `json:"runtimeChangeDetails,omitempty"`
		RolloutSteps       []domain.RolloutStepSummary     `json:"rolloutSteps,omitempty"`
		RolloutExecution   []domain.RolloutExecutionDetail `json:"rolloutExecution,omitempty"`
	}{
		Git:                detail.Git.Clone(),
		ECS:                detail.ECS,
		DeploymentName:     detail.Name,
		TargetType:         detail.TargetType,
		Strategy:           detail.Strategy,
		ExecutionMode:      detail.ExecutionMode,
		DesiredArtifact:    detail.DesiredArtifact,
		DeployedArtifact:   sourceFingerprintDeployedArtifact(detail),
		RuntimeTimestamp:   sourceFingerprintRuntimeTimestamp(detail),
		RuntimeSource:      sourceFingerprintRuntimeSource(detail),
		CurrentStep:        sourceFingerprintCurrentStep(detail),
		DesiredGeneration:  sourceFingerprintDesiredGeneration(detail),
		ObservedGeneration: sourceFingerprintObservedGeneration(detail),
		Sync:               detail.Sync,
		Health:             detail.Health,
		Conditions:         append([]domain.Condition(nil), detail.Conditions...),
		RuntimeChanges:     sourceFingerprintRuntimeChanges(detail),
		RuntimeDetails:     sourceFingerprintRuntimeChangeDetails(detail),
		RolloutSteps:       sourceFingerprintRolloutSteps(detail),
		RolloutExecution:   sourceFingerprintExecutionResources(detail),
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode deployment source fingerprint: %w", err)
	}
	sum := sha256.Sum256(content)
	return "plan_" + hex.EncodeToString(sum[:8]), nil
}

func sourceFingerprintRolloutSteps(detail DeploymentOverviewDetail) []domain.RolloutStepSummary {
	if detail.Rollout != nil && len(detail.Rollout.Steps) > 0 {
		return append([]domain.RolloutStepSummary(nil), detail.Rollout.Steps...)
	}
	plan := reconcile.Planner{}.Build(domain.Deployment{
		Name:            detail.Name,
		AppRef:          detail.AppRef,
		EnvironmentRef:  detail.EnvironmentRef,
		Generation:      sourceFingerprintDesiredGeneration(detail),
		TargetType:      detail.TargetType,
		Strategy:        detail.Strategy,
		ExecutionMode:   detail.ExecutionMode,
		DesiredArtifact: detail.DesiredArtifact,
		UserData:        detail.UserData,
		RolloutConfig:   detail.RolloutConfig,
	}, domain.DeploymentStatus{
		ObservedGeneration: sourceFingerprintObservedGeneration(detail),
		Sync:               detail.Sync,
		Health:             detail.Health,
		Conditions:         append([]domain.Condition(nil), detail.Conditions...),
		History:            append([]domain.StatusHistoryEntry(nil), detail.History...),
		DeployedArtifact:   sourceFingerprintDeployedArtifact(detail),
		Rollout:            detail.Rollout,
	})
	steps := make([]domain.RolloutStepSummary, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		steps = append(steps, domain.RolloutStepSummary{Kind: string(step.Kind)})
	}
	return steps
}

func sourceFingerprintExecutionResources(detail DeploymentOverviewDetail) []domain.RolloutExecutionDetail {
	if detail.ECS != nil {
		return []domain.RolloutExecutionDetail{{Resource: "ecsService", Name: detail.ECS.ServiceARN}}
	}
	if detail.Rollout != nil && len(detail.Rollout.Execution) > 0 {
		return append([]domain.RolloutExecutionDetail(nil), detail.Rollout.Execution...)
	}
	switch detail.ExecutionMode {
	case "ecsCodeDeploy":
		return []domain.RolloutExecutionDetail{
			{Resource: "ecsService", Name: detail.Name + "-service"},
			{Resource: "codeDeployApplication", Name: detail.Name + "-codedeploy-app"},
			{Resource: "codeDeployDeploymentGroup", Name: detail.Name + "-codedeploy-group"},
			{Resource: "productionListener", Name: detail.Name + "-prod-listener"},
			{Resource: "testListener", Name: detail.Name + "-test-listener"},
			{Resource: "blueTaskSetTargetGroup", Name: detail.Name + "-blue"},
			{Resource: "greenTaskSetTargetGroup", Name: detail.Name + "-green"},
		}
	case "ecsRolling":
		return []domain.RolloutExecutionDetail{{Resource: "ecsService", Name: detail.Name + "-service"}}
	case "asgWeightedTargetGroups":
		return []domain.RolloutExecutionDetail{
			{Resource: "candidateAutoScalingGroup", Name: detail.Name + "-candidate"},
			{Resource: "activeTargetGroup", Name: detail.Name + "-active"},
			{Resource: "candidateTargetGroup", Name: detail.Name + "-candidate"},
			{Resource: "productionListenerRule", Name: detail.Name + "-prod-listener"},
			{Resource: "trafficShiftPlan", Name: detail.Name + "-weighted-routing"},
			{Resource: "userDataBootstrapSource", Name: sourceFingerprintUserDataBootstrapSource(detail.UserData)},
		}
	case "asgInstanceRefresh":
		return []domain.RolloutExecutionDetail{
			{Resource: "serviceAutoScalingGroup", Name: detail.Name + "-service"},
			{Resource: "userDataBootstrapSource", Name: sourceFingerprintUserDataBootstrapSource(detail.UserData)},
		}
	default:
		return nil
	}
}

func sourceFingerprintUserDataBootstrapSource(userData domain.DeploymentUserData) string {
	if userData.Type == "" && userData.Path == "" {
		return "unspecified"
	}
	if userData.Path == "" {
		return userData.Type
	}
	if userData.Type == "" {
		return userData.Path
	}
	return userData.Type + ":" + userData.Path
}

func sourceFingerprintDesiredGeneration(detail DeploymentOverviewDetail) int64 {
	switch {
	case detail.Rollout != nil && detail.Rollout.DesiredGeneration > 0:
		return detail.Rollout.DesiredGeneration
	case detail.RuntimeSummary != nil && detail.RuntimeSummary.DesiredGeneration > 0:
		return detail.RuntimeSummary.DesiredGeneration
	case sourceFingerprintObservedGeneration(detail) > 0 && detail.Sync.State == domain.SyncStateSynced && detail.Health.Overall.IsHealthy():
		return sourceFingerprintObservedGeneration(detail)
	case sourceFingerprintObservedGeneration(detail) > 0:
		return sourceFingerprintObservedGeneration(detail) + 1
	default:
		return 1
	}
}

func sourceFingerprintObservedGeneration(detail DeploymentOverviewDetail) int64 {
	if detail.RuntimeSummary != nil && detail.RuntimeSummary.ObservedGeneration > 0 {
		return detail.RuntimeSummary.ObservedGeneration
	}
	return detail.ObservedGeneration
}

func sourceFingerprintRuntimeTimestamp(detail DeploymentOverviewDetail) string {
	if detail.RuntimeSummary != nil {
		return detail.RuntimeSummary.Timestamp
	}
	return ""
}

func sourceFingerprintRuntimeSource(detail DeploymentOverviewDetail) string {
	if detail.RuntimeSummary != nil {
		return detail.RuntimeSummary.Source
	}
	return ""
}

func sourceFingerprintRuntimeChanges(detail DeploymentOverviewDetail) []string {
	if detail.RuntimeSummary != nil {
		return append([]string(nil), detail.RuntimeSummary.Changes...)
	}
	return nil
}

func sourceFingerprintRuntimeChangeDetails(detail DeploymentOverviewDetail) []string {
	if detail.RuntimeSummary != nil {
		return append([]string(nil), detail.RuntimeSummary.ChangeDetails...)
	}
	return nil
}

func sourceFingerprintCurrentStep(detail DeploymentOverviewDetail) string {
	if detail.Rollout != nil && detail.Rollout.CurrentStep != "" {
		return detail.Rollout.CurrentStep
	}
	if detail.RuntimeSummary != nil {
		return detail.RuntimeSummary.CurrentStep
	}
	return ""
}

func sourceFingerprintDeployedArtifact(detail DeploymentOverviewDetail) domain.DeploymentArtifact {
	if detail.DeployedArtifact != (domain.DeploymentArtifact{}) {
		return detail.DeployedArtifact
	}
	if detail.RuntimeSummary != nil {
		return detail.RuntimeSummary.DeployedArtifact
	}
	return domain.DeploymentArtifact{}
}
