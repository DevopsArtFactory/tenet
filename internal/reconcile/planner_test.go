package reconcile

import (
	"reflect"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
)

func TestPlannerBuildsStrategyAwareCandidatePlans(t *testing.T) {
	planner := Planner{}
	status := domain.DeploymentStatus{ObservedGeneration: 1, Sync: domain.Sync{State: domain.SyncStateOutOfSync}}

	tests := []struct {
		name       string
		deployment domain.Deployment
		want       []StepKind
	}{
		{
			name:       "ec2 canary uses weighted target group steps",
			deployment: domain.Deployment{Generation: 2, TargetType: "ec2", Strategy: "canary", ExecutionMode: "asgWeightedTargetGroups"},
			want:       []StepKind{StepResolvePlacement, StepObserveCurrentState, StepCreateCandidateCapacity, StepVerifyCandidateCapacity, StepShiftTrafficCanary, StepPromoteCandidateCapacity},
		},
		{
			name:       "ec2 linear uses weighted target group steps",
			deployment: domain.Deployment{Generation: 2, TargetType: "ec2", Strategy: "linear", ExecutionMode: "asgWeightedTargetGroups"},
			want:       []StepKind{StepResolvePlacement, StepObserveCurrentState, StepCreateCandidateCapacity, StepVerifyCandidateCapacity, StepShiftTrafficLinear, StepPromoteCandidateCapacity},
		},
		{
			name:       "ec2 blue green uses weighted target group steps",
			deployment: domain.Deployment{Generation: 2, TargetType: "ec2", Strategy: "blueGreen", ExecutionMode: "asgWeightedTargetGroups"},
			want:       []StepKind{StepResolvePlacement, StepObserveCurrentState, StepCreateCandidateCapacity, StepVerifyCandidateCapacity, StepShiftTrafficFull, StepPromoteCandidateCapacity},
		},
		{
			name:       "ec2 rolling uses instance refresh steps",
			deployment: domain.Deployment{Generation: 2, TargetType: "ec2", Strategy: "rolling", ExecutionMode: "asgInstanceRefresh"},
			want:       []StepKind{StepResolvePlacement, StepObserveCurrentState, StepStartInstanceRefresh, StepVerifyReplacementCapacity},
		},
		{
			name:       "ecs rolling uses native rolling steps",
			deployment: domain.Deployment{Generation: 2, TargetType: "ecs", Strategy: "rolling", ExecutionMode: "ecsRolling"},
			want:       []StepKind{StepResolvePlacement, StepObserveCurrentState, StepUpdateECSService, StepVerifyServiceStability},
		},
		{
			name:       "ecs blue green uses codedeploy steps",
			deployment: domain.Deployment{Generation: 2, TargetType: "ecs", Strategy: "blueGreen", ExecutionMode: "ecsCodeDeploy"},
			want:       []StepKind{StepResolvePlacement, StepObserveCurrentState, StepCreateCodeDeployDeployment, StepShiftTrafficFull, StepVerifyServiceStability},
		},
		{
			name:       "ecs canary uses codedeploy steps",
			deployment: domain.Deployment{Generation: 2, TargetType: "ecs", Strategy: "canary", ExecutionMode: "ecsCodeDeploy"},
			want:       []StepKind{StepResolvePlacement, StepObserveCurrentState, StepCreateCodeDeployDeployment, StepShiftTrafficCanary, StepVerifyServiceStability},
		},
		{
			name:       "ecs linear uses codedeploy steps",
			deployment: domain.Deployment{Generation: 2, TargetType: "ecs", Strategy: "linear", ExecutionMode: "ecsCodeDeploy"},
			want:       []StepKind{StepResolvePlacement, StepObserveCurrentState, StepCreateCodeDeployDeployment, StepShiftTrafficLinear, StepVerifyServiceStability},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := planner.Build(tt.deployment, status)
			if got := stepKinds(plan.Steps); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("unexpected plan steps: got %v want %v", got, tt.want)
			}
		})
	}
}

func TestPlannerReturnsCurrentStateRepairForSameGenerationNonSyncedDeployment(t *testing.T) {
	planner := Planner{}
	deployment := domain.Deployment{Generation: 3}
	status := domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateDrifted}}

	plan := planner.Build(deployment, status)
	want := []StepKind{StepResolvePlacement, StepObserveCurrentState, StepReconcileCurrentState}
	if got := stepKinds(plan.Steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected repair plan: got %v want %v", got, want)
	}
}

func TestPlannerReturnsMarkSyncedForHealthyDeployment(t *testing.T) {
	planner := Planner{}
	deployment := domain.Deployment{Generation: 3}
	status := domain.DeploymentStatus{
		ObservedGeneration: 3,
		Sync:               domain.Sync{State: domain.SyncStateSynced},
		Health:             domain.Health{Overall: domain.HealthHealthy},
	}

	plan := planner.Build(deployment, status)
	want := []StepKind{StepMarkSynced}
	if got := stepKinds(plan.Steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected synced plan: got %v want %v", got, want)
	}
}

func TestPlannerKeepsReconcilingSameGenerationSyncedDeploymentUntilHealthy(t *testing.T) {
	planner := Planner{}
	deployment := domain.Deployment{Generation: 3}

	for _, health := range []domain.HealthStatus{domain.HealthProgressing, domain.HealthUnhealthy, domain.HealthUnknown} {
		t.Run(string(health), func(t *testing.T) {
			status := domain.DeploymentStatus{
				ObservedGeneration: 3,
				Sync:               domain.Sync{State: domain.SyncStateSynced},
				Health:             domain.Health{Overall: health},
			}

			plan := planner.Build(deployment, status)
			want := []StepKind{StepResolvePlacement, StepObserveCurrentState, StepReconcileCurrentState}
			if got := stepKinds(plan.Steps); !reflect.DeepEqual(got, want) {
				t.Fatalf("unexpected non-healthy synced plan: got %v want %v", got, want)
			}
		})
	}
}

func TestPlannerDoesNotReconcilePausedDeployment(t *testing.T) {
	planner := Planner{}
	deployment := domain.Deployment{Generation: 3}
	status := domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStatePaused}}

	plan := planner.Build(deployment, status)
	want := []StepKind{StepResolvePlacement, StepObserveCurrentState}
	if got := stepKinds(plan.Steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected paused plan: got %v want %v", got, want)
	}
}

func TestPlannerContinuesReconcilingErroredDeployment(t *testing.T) {
	planner := Planner{}
	deployment := domain.Deployment{Generation: 3}
	status := domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateError}}

	plan := planner.Build(deployment, status)
	want := []StepKind{StepResolvePlacement, StepObserveCurrentState, StepReconcileCurrentState}
	if got := stepKinds(plan.Steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected errored plan: got %v want %v", got, want)
	}
}

func TestPlannerWaitsForFreshDesiredWhenStatusIsAhead(t *testing.T) {
	planner := Planner{}
	deployment := domain.Deployment{Generation: 2}
	status := domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateReconciling}}

	plan := planner.Build(deployment, status)
	want := []StepKind{StepResolvePlacement, StepObserveCurrentState, StepAwaitFreshDesired}
	if got := stepKinds(plan.Steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected await-fresh-desired plan: got %v want %v", got, want)
	}
}

func stepKinds(steps []Step) []StepKind {
	kinds := make([]StepKind, 0, len(steps))
	for _, step := range steps {
		kinds = append(kinds, step.Kind)
	}
	return kinds
}
