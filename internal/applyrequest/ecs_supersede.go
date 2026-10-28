package applyrequest

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

// CanSupersedeECS proves that no service update can have been sent. Registration
// may have left a reusable/orphan revision; it cannot have changed service traffic.
func CanSupersedeECS(record Record) bool {
	if record.Status.IsTerminal() || record.Recovery != nil || record.NativeRollback != nil || record.NativeRollbackRequestID != "" || !nativeForwardOperations(record) {
		return false
	}
	cp := record.Operations[1].ECSCheckpoint
	return cp == nil || cp.State == "retryable_rejection" || (cp.State == "in_flight" && cp.MutationGuarded && cp.MutationAuthorizedAt == "")
}

// Recovery approvals and partial/unknown operation contracts are never superseded.
func nativeForwardOperations(record Record) bool {
	kinds := []string{"RegisterECSTaskDefinition", "UpdateECSService", "VerifyServiceStability"}
	if len(record.Operations) != len(kinds) || record.Operations[0].ECS == nil {
		return false
	}
	intent := record.Operations[0].ECS
	if intent.DeploymentController != "ECS" {
		return false
	}
	switch intent.DeploymentStrategy {
	case "ROLLING":
	case "BLUE_GREEN", "CANARY", "LINEAR":
		if intent.Advanced == nil {
			return false
		}
	default:
		return false
	}
	for i, op := range record.Operations {
		if op.Kind != kinds[i] || !reflect.DeepEqual(op.ECS, intent) {
			return false
		}
	}
	return true
}

func expectedECSIntent(record Record) *domain.ECSDeploymentIntent {
	if record.Recovery != nil {
		return record.Recovery.ForwardIntent
	}
	if len(record.Operations) > 0 {
		return record.Operations[0].ECS
	}
	return nil
}

func ECSReplacementNeeded(replacement, previous Record) bool {
	if replacement.ID == previous.ID || !CanSupersedeECS(previous) {
		return false
	}
	return !reflect.DeepEqual(expectedECSIntent(replacement), expectedECSIntent(previous)) || (!hasECSCheckpoints(previous) && replacement.Source.Fingerprint != previous.Source.Fingerprint)
}

// SupersedeECS cancels only an unsent service update after a newly approved Apply
// is stored. Callers hold the deployment/service execution lock. The atomic state
// check also fences an older worker's final mutation guard.
func (s Service) SupersedeECS(ctx context.Context, previousID, replacementID string) (Record, error) {
	if s.AuthorizeCreate == nil || s.LoadPlan == nil || !s.EnableAWSApply {
		return Record{}, ErrExecutionUnauthorized
	}
	replacement, err := s.Get(ctx, replacementID)
	if err != nil {
		return Record{}, err
	}
	if replacement.Principal == nil || (replacement.Status.IsTerminal() && (replacement.Status != StatusSucceeded || replacement.ECSRuntime == nil || replacement.ECSRuntime.State != "converged")) || replacement.Recovery != nil || replacement.NativeRollback != nil || replacement.NativeRollbackRequestID != "" || !nativeForwardOperations(replacement) {
		return Record{}, ErrInvalidState
	}
	expected, err := stableRecordID(replacement)
	if err != nil || expected != replacement.ID {
		return Record{}, ErrInvalidState
	}
	plan, err := s.LoadPlan(ctx, replacement.PlanID)
	if err != nil {
		return Record{}, err
	}
	plan = planpreview.NormalizeOperationIDs(plan)
	if plan.Status != planpreview.StatusReady || plan.DeploymentName != replacement.DeploymentName || len(plan.Operations) != 3 || !reviewedDriftLink(replacement, plan) || replacement.Source.Fingerprint != plan.Source.Fingerprint || !reflect.DeepEqual(replacement.Source.Git, plan.Source.Git) {
		return Record{}, ErrSourceFingerprintConflict
	}
	if err := s.validateECSRecovery(ctx, replacement, plan); err != nil {
		return Record{}, err
	}
	if err := s.validateECSRetry(ctx, replacement); err != nil {
		return Record{}, err
	}
	for i, operation := range replacement.Operations {
		if operation.ID != plan.Operations[i].ID || operation.Kind != plan.Operations[i].Kind || !reviewedECSOperationMatches(replacement, operation, plan.Operations[i]) {
			return Record{}, ErrInvalidState
		}
	}
	if err := s.validateCurrentFingerprint(ctx, plan); err != nil {
		return Record{}, err
	}
	if _, err := s.AuthorizeCreate(ctx, replacement); err != nil {
		return Record{}, err
	}
	previous, err := s.Get(ctx, previousID)
	if err != nil {
		return Record{}, err
	}
	if previous.DeploymentName != replacement.DeploymentName && (len(previous.Operations) == 0 || previous.Operations[0].ECS == nil || len(replacement.Operations) == 0 || replacement.Operations[0].ECS == nil || previous.Operations[0].ECS.ServiceARN != replacement.Operations[0].ECS.ServiceARN) {
		return Record{}, ErrInvalidState
	}
	// Cancelling a positively unsent previous approval requires current operator
	// scope, not permission to automatically execute its old manual sync policy.
	if _, err := s.AuthorizeCreate(context.WithValue(ctx, automaticApprovalKey{}, false), previous); err != nil {
		return Record{}, err
	}
	store, ok := s.Store.(MutationStore)
	if !ok {
		return Record{}, ErrInvalidState
	}
	return store.Mutate(ctx, previous.ID, func(current Record) (Record, error) {
		if !ECSReplacementNeeded(replacement, current) {
			return Record{}, ErrInvalidState
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		oldStatus := current.Status
		current.Status = StatusCancelled
		current.Error = ""
		current.Details = []string{"Superseded before service mutation by reviewed Apply " + replacement.ID + ". Any registered revision is retained; no service update was authorized."}
		current.Operations = transitionOperations(current.Operations, StatusCancelled, current.Lifecycle.CurrentStep)
		current.Lifecycle = transitionLifecycle(current.Lifecycle, len(current.Operations), TransitionRequest{Status: StatusCancelled, Step: current.Lifecycle.CurrentStep}, s.now())
		current.Transitions = appendTransition(current.Transitions, Transition{TransitionedAt: now, PreviousStatus: oldStatus, Status: StatusCancelled, Step: current.Lifecycle.CurrentStep, Actor: requestactor.FromContext(ctx), Details: append([]string(nil), current.Details...), Lifecycle: current.Lifecycle})
		readiness := deriveExecutionReadiness(current)
		current.ExecutionReadiness = &readiness
		return current, nil
	})
}

func principalContext(ctx context.Context, principal *ExecutionPrincipal) context.Context {
	if principal == nil {
		return ctx
	}
	ctx = context.WithValue(ctx, automaticApprovalKey{}, principal.Automatic)
	ctx = requestactor.WithAuthenticatedIdentityContext(ctx, principal.Identity)
	ctx = requestactor.WithAuthenticatedGroupsContext(ctx, principal.Groups)
	return requestactor.WithContext(ctx, requestactor.Actor{Available: true, ID: principal.Identity.ID, Name: principal.Identity.Name, Email: principal.Identity.Email})
}

func (e Executor) supersedeECSConflict(ctx context.Context, replacement, previous Record) error {
	if replacement.Principal == nil || !ECSReplacementNeeded(replacement, previous) {
		return fmt.Errorf("%w: older service execution must finish", ErrExecutionBusy)
	}
	if _, err := e.validateECS(ctx, replacement, true); err != nil {
		return err
	}
	_, err := e.Service.SupersedeECS(principalContext(ctx, replacement.Principal), previous.ID, replacement.ID)
	return err
}

// AutomaticApprovalContext is internal control-plane attribution, never decoded
// from a client request. Authorization still requires a configured RBAC subject.
type automaticApprovalKey struct{}

func WithAutomaticApproval(ctx context.Context) context.Context {
	return context.WithValue(ctx, automaticApprovalKey{}, true)
}
func AutomaticApprovalFromContext(ctx context.Context) bool {
	value, _ := ctx.Value(automaticApprovalKey{}).(bool)
	return value
}

func reviewedDriftLink(record Record, plan planpreview.Record) bool {
	expected := ""
	if plan.Source.ManagedDrift != nil {
		expected = plan.Source.ManagedDrift.LastVerifiedApplyID
	}
	return record.Source.LastVerifiedApplyID == expected
}
