package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

const (
	runtimeHistorySourceController = "controller"
	runtimeHistoryMaxEntries       = 10
)

var runtimeHistoryNow = time.Now

func emitRuntimePlans(ctx context.Context, logger *log.Logger, config startupConfig, store statusstore.Store) error {
	if config.RepoRoot == "" {
		logger.Printf("controller runtime disabled reason=no_repo_root")
		return nil
	}

	compiled, err := catalog.LoadFromRepository(config.RepoRoot)
	if err != nil {
		return fmt.Errorf("load runtime catalog: %w", err)
	}

	if store == nil {
		runtimeStore, err := openRuntimeStatusStore(ctx, logger, config)
		if err != nil {
			return err
		}
		if runtimeStore != nil {
			store = runtimeStore
			defer func() {
				_ = runtimeStore.Close()
			}()
		}
	}

	statuses := map[string]domain.DeploymentStatus{}
	if store != nil {
		if err := statusstore.SeedFromFileIfEmpty(ctx, store, config.StatusSeedPath); err != nil {
			return fmt.Errorf("seed runtime statuses: %w", err)
		}
		snapshots, err := store.List(ctx)
		if err != nil {
			return fmt.Errorf("load runtime statuses: %w", err)
		}
		for _, snapshot := range snapshots {
			statuses[snapshot.DeploymentName] = snapshot.Status
		}
	}

	currentStatuses := make(map[string]domain.DeploymentStatus, len(compiled.Deployments))
	if len(compiled.Deployments) == 0 {
		if store != nil {
			if err := store.Save(ctx, nil); err != nil {
				return fmt.Errorf("save runtime statuses: %w", err)
			}
		}
		logger.Printf("controller runtime found no deployments")
		return nil
	}

	planner := reconcile.Planner{}
	for _, deployment := range compiled.Deployments {
		status := statuses[deployment.Name]
		previousStatus := status
		plan := planner.Build(deployment, status)
		status.Rollout = deriveRolloutSummary(deployment, status, plan)
		status.History = appendRuntimeHistory(previousStatus, status)
		currentStatuses[deployment.Name] = status
		logger.Printf(
			"controller reconcile deployment=%s strategy=%s execution_mode=%s desired_generation=%d observed_generation=%d sync_state=%s steps=%s",
			deployment.Name,
			deployment.Strategy,
			deployment.ExecutionMode,
			deployment.Generation,
			status.ObservedGeneration,
			syncStateLabel(status.Sync.State),
			stepSummary(plan),
		)
	}

	if store != nil {
		snapshots := make([]statusstore.Snapshot, 0, len(currentStatuses))
		for deploymentName, status := range currentStatuses {
			snapshots = append(snapshots, statusstore.Snapshot{DeploymentName: deploymentName, Status: status})
		}
		sort.Slice(snapshots, func(i, j int) bool {
			return snapshots[i].DeploymentName < snapshots[j].DeploymentName
		})
		if err := store.Save(ctx, snapshots); err != nil {
			return fmt.Errorf("save runtime statuses: %w", err)
		}
	}

	return nil
}

func appendRuntimeHistory(previous domain.DeploymentStatus, current domain.DeploymentStatus) []domain.StatusHistoryEntry {
	history := append([]domain.StatusHistoryEntry(nil), current.History...)
	entry := deriveRuntimeHistoryEntry(current)
	if len(history) == 0 {
		entry.Timestamp = runtimeHistoryNow().UTC().Format(time.RFC3339)
		return []domain.StatusHistoryEntry{entry}
	}

	latest := deriveRuntimeHistoryEntry(previous)
	if len(history) > 0 {
		latest = history[len(history)-1]
	}
	if !runtimeHistoryMeaningfullyChanged(latest, entry) {
		return capRuntimeHistory(history)
	}

	entry.Timestamp = runtimeHistoryNow().UTC().Format(time.RFC3339)
	history = append(history, entry)
	return capRuntimeHistory(history)
}

func capRuntimeHistory(history []domain.StatusHistoryEntry) []domain.StatusHistoryEntry {
	if len(history) <= runtimeHistoryMaxEntries {
		return history
	}
	return append([]domain.StatusHistoryEntry(nil), history[len(history)-runtimeHistoryMaxEntries:]...)
}

func deriveRuntimeHistoryEntry(status domain.DeploymentStatus) domain.StatusHistoryEntry {
	entry := domain.StatusHistoryEntry{
		Source:  runtimeHistorySourceController,
		Summary: "Runtime status updated",
		Sync:    status.Sync,
		Health:  status.Health,
	}
	if status.Rollout != nil {
		entry.Summary = status.Rollout.Summary
		entry.RolloutPhase = status.Rollout.Phase
		entry.CurrentStep = status.Rollout.CurrentStep
	}
	return entry
}

func runtimeHistoryMeaningfullyChanged(previous domain.StatusHistoryEntry, current domain.StatusHistoryEntry) bool {
	return previous.Source != current.Source ||
		previous.Summary != current.Summary ||
		previous.Sync != current.Sync ||
		previous.Health != current.Health ||
		previous.RolloutPhase != current.RolloutPhase ||
		previous.CurrentStep != current.CurrentStep
}

func syncStateLabel(state domain.SyncState) string {
	if state == "" {
		return "Unknown"
	}
	return string(state)
}

func deriveRolloutSummary(deployment domain.Deployment, status domain.DeploymentStatus, plan reconcile.Plan) *domain.RolloutSummary {
	steps := make([]domain.RolloutStepSummary, 0, len(plan.Steps))
	currentIndex := currentRolloutStepIndex(plan)
	completed := completedRolloutStep(plan, currentIndex)
	currentStep := ""
	if currentIndex >= 0 && !completed {
		currentStep = string(plan.Steps[currentIndex].Kind)
	}
	for index, step := range plan.Steps {
		steps = append(steps, domain.RolloutStepSummary{
			Kind:    string(step.Kind),
			Detail:  rolloutStepDetail(deployment, plan, step.Kind),
			Current: index == currentIndex && !completed,
			Status:  rolloutStepStatus(index, currentIndex, completed),
		})
	}

	return &domain.RolloutSummary{
		DesiredGeneration:  deployment.Generation,
		ObservedGeneration: status.ObservedGeneration,
		Phase:              rolloutPhaseForStep(plan, currentIndex),
		CurrentStep:        currentStep,
		Summary:            rolloutSummaryForStep(status, plan, currentIndex),
		Steps:              steps,
		Execution:          deriveRolloutExecutionDetails(deployment),
	}
}

func currentRolloutStepIndex(plan reconcile.Plan) int {
	for index, step := range plan.Steps {
		if step.Kind == reconcile.StepResolvePlacement || step.Kind == reconcile.StepObserveCurrentState {
			continue
		}
		return index
	}
	return -1
}

func completedRolloutStep(plan reconcile.Plan, currentIndex int) bool {
	if currentIndex < 0 || currentIndex >= len(plan.Steps) {
		return false
	}
	return plan.Steps[currentIndex].Kind == reconcile.StepMarkSynced
}

func rolloutStepStatus(index int, currentIndex int, completed bool) domain.RolloutStepStatus {
	if completed {
		return domain.RolloutStepStatusCompleted
	}
	if currentIndex >= 0 {
		switch {
		case index < currentIndex:
			return domain.RolloutStepStatusCompleted
		case index == currentIndex:
			return domain.RolloutStepStatusCurrent
		default:
			return domain.RolloutStepStatusPending
		}
	}
	return domain.RolloutStepStatusPending
}

func rolloutStepDetail(deployment domain.Deployment, plan reconcile.Plan, kind reconcile.StepKind) string {
	switch kind {
	case reconcile.StepCreateCandidateCapacity:
		return "Create a fresh candidate Auto Scaling Group bootstrapped from userdata before attaching it to the candidate target group."
	case reconcile.StepShiftTrafficCanary:
		if plan.ContainsStep(reconcile.StepCreateCodeDeployDeployment) {
			return "Shift a small canary slice of ECS service traffic to the replacement task set through CodeDeploy before the full cutover."
		}
		return "Shift a small canary slice of production traffic to the candidate target group before full promotion."
	case reconcile.StepShiftTrafficLinear:
		if plan.ContainsStep(reconcile.StepCreateCodeDeployDeployment) {
			return "Advance ECS traffic in linear increments through CodeDeploy until the replacement task set serves all traffic."
		}
		return "Advance ALB traffic in linear increments toward the candidate target group until promotion is safe."
	case reconcile.StepShiftTrafficFull:
		if plan.ContainsStep(reconcile.StepCreateCodeDeployDeployment) {
			return "Shift all ECS service traffic to the replacement task set through CodeDeploy in a single blue/green cutover."
		}
		return "Shift all production traffic from the active target group to the candidate target group in one blue/green cutover."
	case reconcile.StepUpdateECSService:
		return "Update the ECS service to start a native rolling deployment that replaces running tasks in place."
	case reconcile.StepCreateCodeDeployDeployment:
		if deployment.ExecutionMode == "ecsCodeDeploy" {
			return "Create a CodeDeploy deployment to coordinate blue/green ECS traffic shifting for the replacement task set."
		}
	case reconcile.StepVerifyServiceStability:
		if plan.ContainsStep(reconcile.StepCreateCodeDeployDeployment) {
			return "Wait for the ECS service to stabilize after CodeDeploy finishes shifting traffic to the replacement task set."
		}
		return "Wait for the ECS service to report a stable native rolling deployment after replacing tasks."
	}
	return ""
}

func deriveRolloutExecutionDetails(deployment domain.Deployment) []domain.RolloutExecutionDetail {
	switch deployment.ExecutionMode {
	case "asgWeightedTargetGroups":
		return []domain.RolloutExecutionDetail{{
			Resource:    "candidateAutoScalingGroup",
			Name:        deployment.Name + "-candidate",
			Description: "Candidate Auto Scaling Group Tenet will create for weighted target group rollout",
		}, {
			Resource:    "activeTargetGroup",
			Name:        deployment.Name + "-active",
			Description: "Currently active target group receiving production traffic",
		}, {
			Resource:    "candidateTargetGroup",
			Name:        deployment.Name + "-candidate",
			Description: "Candidate target group Tenet will register and warm before traffic shifting",
		}, {
			Resource:    "productionListenerRule",
			Name:        deployment.Name + "-prod-listener",
			Description: "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout",
		}, {
			Resource:    "trafficShiftPlan",
			Name:        deployment.Name + "-weighted-routing",
			Description: "Weighted routing plan Tenet follows to move production traffic from the active target group to the candidate target group",
		}, {
			Resource:    "userDataBootstrapSource",
			Name:        userDataBootstrapSource(deployment.UserData),
			Description: "Normalized userdata bootstrap source for candidate EC2 capacity",
		}}
	case "asgInstanceRefresh":
		return []domain.RolloutExecutionDetail{{
			Resource:    "serviceAutoScalingGroup",
			Name:        deployment.Name + "-service",
			Description: "Service Auto Scaling Group Tenet refreshes in place",
		}, {
			Resource:    "userDataBootstrapSource",
			Name:        userDataBootstrapSource(deployment.UserData),
			Description: "Normalized userdata bootstrap source for refreshed EC2 instances",
		}}
	case "ecsCodeDeploy":
		return []domain.RolloutExecutionDetail{{
			Resource:    "ecsService",
			Name:        deployment.Name + "-service",
			Description: "ECS service Tenet updates during deployment",
		}, {
			Resource:    "codeDeployApplication",
			Name:        deployment.Name + "-codedeploy-app",
			Description: "CodeDeploy application coordinating ECS traffic shifting",
		}, {
			Resource:    "codeDeployDeploymentGroup",
			Name:        deployment.Name + "-codedeploy-group",
			Description: "CodeDeploy deployment group Tenet targets for ECS rollout",
		}, {
			Resource:    "productionListener",
			Name:        deployment.Name + "-prod-listener",
			Description: "Production listener CodeDeploy shifts during ECS blue/green rollout",
		}, {
			Resource:    "testListener",
			Name:        deployment.Name + "-test-listener",
			Description: "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion",
		}, {
			Resource:    "blueTaskSetTargetGroup",
			Name:        deployment.Name + "-blue",
			Description: "Blue task set target group CodeDeploy treats as the live ECS traffic destination",
		}, {
			Resource:    "greenTaskSetTargetGroup",
			Name:        deployment.Name + "-green",
			Description: "Green task set target group CodeDeploy warms before shifting ECS traffic",
		}}
	case "ecsRolling":
		return []domain.RolloutExecutionDetail{{
			Resource:    "ecsService",
			Name:        deployment.Name + "-service",
			Description: "ECS service Tenet updates with native rolling deployment",
		}}
	default:
		return nil
	}
}

func userDataBootstrapSource(userData domain.DeploymentUserData) string {
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

func rolloutPhaseForStep(plan reconcile.Plan, currentIndex int) domain.RolloutPhase {
	if currentIndex < 0 || len(plan.Steps) == 0 {
		return domain.RolloutPhasePending
	}

	switch plan.Steps[currentIndex].Kind {
	case reconcile.StepPlanCandidateGeneration:
		return domain.RolloutPhasePlanning
	case reconcile.StepReconcileCurrentState:
		return domain.RolloutPhaseProvisioning
	case reconcile.StepCreateCandidateCapacity, reconcile.StepStartInstanceRefresh, reconcile.StepCreateCodeDeployDeployment, reconcile.StepUpdateECSService:
		return domain.RolloutPhaseProvisioning
	case reconcile.StepVerifyCandidateCapacity:
		return domain.RolloutPhaseHealthChecking
	case reconcile.StepShiftTrafficCanary, reconcile.StepShiftTrafficLinear, reconcile.StepShiftTrafficFull, reconcile.StepPromoteCandidateCapacity:
		return domain.RolloutPhasePromoting
	case reconcile.StepVerifyReplacementCapacity, reconcile.StepVerifyServiceStability:
		return domain.RolloutPhaseStabilizing
	case reconcile.StepMarkSynced:
		return domain.RolloutPhaseCompleted
	case reconcile.StepAwaitFreshDesired:
		return domain.RolloutPhaseAccepted
	default:
		return domain.RolloutPhaseAccepted
	}
}

func rolloutSummaryForStep(status domain.DeploymentStatus, plan reconcile.Plan, currentIndex int) string {
	if currentIndex < 0 || len(plan.Steps) == 0 {
		return "Rollout pending"
	}

	switch plan.Steps[currentIndex].Kind {
	case reconcile.StepPlanCandidateGeneration:
		return "Planning candidate generation"
	case reconcile.StepReconcileCurrentState:
		return "Reconciling current state"
	case reconcile.StepCreateCandidateCapacity:
		return "Creating candidate capacity for weighted target groups"
	case reconcile.StepVerifyCandidateCapacity:
		return "Verifying candidate capacity before traffic shifting"
	case reconcile.StepShiftTrafficCanary:
		return "Shifting canary traffic through weighted target groups"
	case reconcile.StepShiftTrafficLinear:
		return trafficShiftSummary(plan)
	case reconcile.StepShiftTrafficFull:
		return trafficShiftSummary(plan)
	case reconcile.StepPromoteCandidateCapacity:
		return "Promoting candidate capacity through weighted target groups"
	case reconcile.StepStartInstanceRefresh:
		return "Starting EC2 instance refresh"
	case reconcile.StepVerifyReplacementCapacity:
		return "Verifying replacement capacity during instance refresh"
	case reconcile.StepUpdateECSService:
		return "Updating ECS service deployment"
	case reconcile.StepCreateCodeDeployDeployment:
		return "Creating CodeDeploy traffic shift deployment"
	case reconcile.StepVerifyServiceStability:
		if plan.ContainsStep(reconcile.StepCreateCodeDeployDeployment) {
			return "Stabilizing ECS service after CodeDeploy traffic shift"
		}
		return "Stabilizing ECS service after rolling update"
	case reconcile.StepMarkSynced:
		if status.Sync.State == domain.SyncStateSynced && status.Health.Overall == domain.HealthHealthy {
			return "Deployment is synced and healthy"
		}
		return "Deployment is synced"
	case reconcile.StepAwaitFreshDesired:
		return "Waiting for fresher desired state"
	default:
		return "Observing current state"
	}
}

func trafficShiftSummary(plan reconcile.Plan) string {
	if plan.ContainsStep(reconcile.StepCreateCodeDeployDeployment) {
		return "Shifting traffic with ECS CodeDeploy"
	}
	return "Shifting traffic through weighted target groups"
}

func stepSummary(plan reconcile.Plan) string {
	if len(plan.Steps) == 0 {
		return "none"
	}
	steps := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		steps = append(steps, string(step.Kind))
	}
	return strings.Join(steps, ",")
}
