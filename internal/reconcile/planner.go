package reconcile

import "github.com/DevopsArtFactory/tenet/internal/domain"

// StepKind identifies the planner actions required to reconcile a deployment.
type StepKind string

const (
	StepResolvePlacement           StepKind = "ResolvePlacement"
	StepObserveCurrentState        StepKind = "ObserveCurrentState"
	StepReconcileCurrentState      StepKind = "ReconcileCurrentState"
	StepPlanCandidateGeneration    StepKind = "PlanCandidateGeneration"
	StepCreateCandidateCapacity    StepKind = "CreateCandidateCapacity"
	StepVerifyCandidateCapacity    StepKind = "VerifyCandidateCapacity"
	StepShiftTrafficCanary         StepKind = "ShiftTrafficCanary"
	StepShiftTrafficLinear         StepKind = "ShiftTrafficLinear"
	StepShiftTrafficFull           StepKind = "ShiftTrafficFull"
	StepPromoteCandidateCapacity   StepKind = "PromoteCandidateCapacity"
	StepStartInstanceRefresh       StepKind = "StartInstanceRefresh"
	StepVerifyReplacementCapacity  StepKind = "VerifyReplacementCapacity"
	StepUpdateECSService           StepKind = "UpdateECSService"
	StepVerifyServiceStability     StepKind = "VerifyServiceStability"
	StepCreateCodeDeployDeployment StepKind = "CreateCodeDeployDeployment"
	StepMarkSynced                 StepKind = "MarkSynced"
	StepAwaitFreshDesired          StepKind = "AwaitFreshDesired"
)

// Step is an abstract reconciliation action.
type Step struct {
	Kind StepKind
}

// Plan is an ordered list of reconciliation steps.
type Plan struct {
	Steps []Step
}

// Planner turns desired deployment and observed status into an abstract plan.
type Planner struct{}

// Build returns a minimal reconciliation plan without side effects.
func (Planner) Build(deployment domain.Deployment, status domain.DeploymentStatus) Plan {
	if deployment.Generation < status.ObservedGeneration {
		return Plan{Steps: []Step{
			{Kind: StepResolvePlacement},
			{Kind: StepObserveCurrentState},
			{Kind: StepAwaitFreshDesired},
		}}
	}

	if deployment.Generation == status.ObservedGeneration {
		if status.IsSynced() {
			return Plan{Steps: []Step{{Kind: StepMarkSynced}}}
		}
		if status.Sync.State == domain.SyncStatePaused {
			return Plan{Steps: []Step{
				{Kind: StepResolvePlacement},
				{Kind: StepObserveCurrentState},
			}}
		}

		return Plan{Steps: []Step{
			{Kind: StepResolvePlacement},
			{Kind: StepObserveCurrentState},
			{Kind: StepReconcileCurrentState},
		}}
	}

	return Plan{Steps: rolloutStepsFor(deployment)}
}

func rolloutStepsFor(deployment domain.Deployment) []Step {
	switch deployment.ExecutionMode {
	case "asgWeightedTargetGroups":
		return []Step{
			{Kind: StepResolvePlacement},
			{Kind: StepObserveCurrentState},
			{Kind: StepCreateCandidateCapacity},
			{Kind: StepVerifyCandidateCapacity},
			{Kind: trafficShiftStep(deployment.Strategy)},
			{Kind: StepPromoteCandidateCapacity},
		}
	case "asgInstanceRefresh":
		return []Step{
			{Kind: StepResolvePlacement},
			{Kind: StepObserveCurrentState},
			{Kind: StepStartInstanceRefresh},
			{Kind: StepVerifyReplacementCapacity},
		}
	case "ecsRolling":
		return []Step{
			{Kind: StepResolvePlacement},
			{Kind: StepObserveCurrentState},
			{Kind: StepUpdateECSService},
			{Kind: StepVerifyServiceStability},
		}
	case "ecsCodeDeploy":
		return []Step{
			{Kind: StepResolvePlacement},
			{Kind: StepObserveCurrentState},
			{Kind: StepCreateCodeDeployDeployment},
			{Kind: trafficShiftStep(deployment.Strategy)},
			{Kind: StepVerifyServiceStability},
		}
	default:
		return []Step{
			{Kind: StepResolvePlacement},
			{Kind: StepObserveCurrentState},
			{Kind: StepPlanCandidateGeneration},
		}
	}
}

func trafficShiftStep(strategy string) StepKind {
	switch strategy {
	case "canary":
		return StepShiftTrafficCanary
	case "linear":
		return StepShiftTrafficLinear
	default:
		return StepShiftTrafficFull
	}
}
