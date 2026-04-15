package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

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
		plan := planner.Build(deployment, status)
		status.Rollout = deriveRolloutSummary(deployment, status, plan)
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

func syncStateLabel(state domain.SyncState) string {
	if state == "" {
		return "Unknown"
	}
	return string(state)
}

func deriveRolloutSummary(deployment domain.Deployment, status domain.DeploymentStatus, plan reconcile.Plan) *domain.RolloutSummary {
	steps := make([]domain.RolloutStepSummary, 0, len(plan.Steps))
	currentStep := ""
	if len(plan.Steps) > 0 {
		currentStep = string(plan.Steps[len(plan.Steps)-1].Kind)
	}
	for index, step := range plan.Steps {
		steps = append(steps, domain.RolloutStepSummary{
			Kind:    string(step.Kind),
			Current: index == len(plan.Steps)-1,
		})
	}

	return &domain.RolloutSummary{
		DesiredGeneration:  deployment.Generation,
		ObservedGeneration: status.ObservedGeneration,
		Phase:              rolloutPhaseForPlan(plan),
		CurrentStep:        currentStep,
		Summary:            rolloutSummaryForPlan(status, plan),
		Steps:              steps,
	}
}

func rolloutPhaseForPlan(plan reconcile.Plan) domain.RolloutPhase {
	if len(plan.Steps) == 0 {
		return domain.RolloutPhasePending
	}

	switch plan.Steps[len(plan.Steps)-1].Kind {
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

func rolloutSummaryForPlan(status domain.DeploymentStatus, plan reconcile.Plan) string {
	if len(plan.Steps) == 0 {
		return "Rollout pending"
	}

	switch plan.Steps[len(plan.Steps)-1].Kind {
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
		if planContainsStep(plan, reconcile.StepCreateCodeDeployDeployment) {
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
	if planContainsStep(plan, reconcile.StepCreateCodeDeployDeployment) {
		return "Shifting traffic with ECS CodeDeploy"
	}
	return "Shifting traffic through weighted target groups"
}

func planContainsStep(plan reconcile.Plan, kind reconcile.StepKind) bool {
	for _, step := range plan.Steps {
		if step.Kind == kind {
			return true
		}
	}
	return false
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
