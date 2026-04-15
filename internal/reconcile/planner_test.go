package reconcile

import (
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
)

func TestPlannerBuildsCandidatePlanForOutOfSyncDeployment(t *testing.T) {
	planner := Planner{}
	deployment := domain.Deployment{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", Generation: 2}
	status := domain.DeploymentStatus{ObservedGeneration: 1, Sync: domain.Sync{State: domain.SyncStateOutOfSync}}

	plan := planner.Build(deployment, status)
	if len(plan.Steps) == 0 {
		t.Fatalf("expected planner to emit steps")
	}

	if plan.Steps[0].Kind != StepResolvePlacement {
		t.Fatalf("expected first step to resolve placement, got %s", plan.Steps[0].Kind)
	}
}

func TestPlannerReturnsCurrentStateRepairForSameGenerationNonSyncedDeployment(t *testing.T) {
	planner := Planner{}
	deployment := domain.Deployment{Generation: 3}
	status := domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateDrifted}}

	plan := planner.Build(deployment, status)
	if len(plan.Steps) != 3 {
		t.Fatalf("expected three repair steps, got %+v", plan.Steps)
	}
	if plan.Steps[2].Kind != StepReconcileCurrentState {
		t.Fatalf("expected current-state reconciliation step, got %s", plan.Steps[2].Kind)
	}
}

func TestPlannerReturnsMarkSyncedForHealthyDeployment(t *testing.T) {
	planner := Planner{}
	deployment := domain.Deployment{Generation: 3}
	status := domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced}}

	plan := planner.Build(deployment, status)
	if len(plan.Steps) != 1 || plan.Steps[0].Kind != StepMarkSynced {
		t.Fatalf("expected single mark-synced step, got %+v", plan.Steps)
	}
}

func TestPlannerDoesNotReconcilePausedDeployment(t *testing.T) {
	planner := Planner{}
	deployment := domain.Deployment{Generation: 3}
	status := domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStatePaused}}

	plan := planner.Build(deployment, status)
	if len(plan.Steps) != 2 {
		t.Fatalf("expected observation-only plan, got %+v", plan.Steps)
	}
	if plan.Steps[0].Kind != StepResolvePlacement || plan.Steps[1].Kind != StepObserveCurrentState {
		t.Fatalf("unexpected paused plan: %+v", plan.Steps)
	}
}

func TestPlannerContinuesReconcilingErroredDeployment(t *testing.T) {
	planner := Planner{}
	deployment := domain.Deployment{Generation: 3}
	status := domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateError}}

	plan := planner.Build(deployment, status)
	if len(plan.Steps) != 3 {
		t.Fatalf("expected repair plan for errored deployment, got %+v", plan.Steps)
	}
	if plan.Steps[2].Kind != StepReconcileCurrentState {
		t.Fatalf("expected current-state reconciliation for errored deployment, got %+v", plan.Steps)
	}
}

func TestPlannerWaitsForFreshDesiredWhenStatusIsAhead(t *testing.T) {
	planner := Planner{}
	deployment := domain.Deployment{Generation: 2}
	status := domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateReconciling}}

	plan := planner.Build(deployment, status)
	if len(plan.Steps) != 3 {
		t.Fatalf("expected await-fresh-desired plan, got %+v", plan.Steps)
	}
	if plan.Steps[2].Kind != StepAwaitFreshDesired {
		t.Fatalf("expected await-fresh-desired step, got %+v", plan.Steps)
	}
}
