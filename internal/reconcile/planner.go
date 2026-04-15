package reconcile

import "github.com/DevopsArtFactory/tenet/internal/domain"

// StepKind identifies the planner actions required to reconcile a deployment.
type StepKind string

const (
	StepResolvePlacement        StepKind = "ResolvePlacement"
	StepObserveCurrentState     StepKind = "ObserveCurrentState"
	StepReconcileCurrentState   StepKind = "ReconcileCurrentState"
	StepPlanCandidateGeneration StepKind = "PlanCandidateGeneration"
	StepMarkSynced              StepKind = "MarkSynced"
	StepAwaitFreshDesired       StepKind = "AwaitFreshDesired"
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

	return Plan{Steps: []Step{
		{Kind: StepResolvePlacement},
		{Kind: StepObserveCurrentState},
		{Kind: StepPlanCandidateGeneration},
	}}
}
