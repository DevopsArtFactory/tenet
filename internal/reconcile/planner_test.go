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
