package applyrequest

import (
	"context"
	"fmt"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const NativeRollbackOperationKind = "StopECSServiceDeployment"
const NativeCapacityRestoreOperationKind = "RestoreECSServiceCapacity"

type ECSNativeRollback struct {
	Automatic                bool   `json:"automatic,omitempty"`
	ParentApplyID            string `json:"parentApplyId"`
	DeploymentARN            string `json:"deploymentArn"`
	ForwardRevisionARN       string `json:"forwardRevisionArn"`
	SourceRevisionARN        string `json:"sourceRevisionArn"`
	ForwardTaskDefinitionARN string `json:"forwardTaskDefinitionArn"`
	SourceTaskDefinitionARN  string `json:"sourceTaskDefinitionArn"`
	DesiredCount             int32  `json:"desiredCount"`
}

func NativeRollbackEvidence(parent Record) (*ECSNativeRollback, error) {
	var empty *ECSNativeRollback
	if parent.ID == "" || parent.Principal == nil || !parent.Principal.Identity.Authenticated || parent.Recovery != nil || (parent.Status != StatusRunning && parent.Status != StatusFailed && !(parent.Status == StatusCancelled && parent.NativeRollbackRequestID != "")) || len(parent.Operations) != 3 {
		return empty, fmt.Errorf("owned running/failed native forward apply required")
	}
	kinds := []reconcile.StepKind{reconcile.StepRegisterECSTaskDefinition, reconcile.StepUpdateECSService, reconcile.StepVerifyServiceStability}
	for i, operation := range parent.Operations {
		if operation.Kind != string(kinds[i]) || !reflect.DeepEqual(operation.ECS, parent.Operations[0].ECS) {
			return empty, fmt.Errorf("unaltered native forward operations required")
		}
	}
	first, update := parent.Operations[0], parent.Operations[1]
	if first.ECS == nil || update.ECS == nil || first.ECS.DeploymentStrategy == "ROLLING" || first.ECSCheckpoint == nil || first.ECSCheckpoint.State != "complete" || update.ECSCheckpoint == nil || update.ECSCheckpoint.State != "complete" || update.ECSCheckpoint.MutationAuthorizedAt == "" {
		return empty, fmt.Errorf("durable owned registration and update checkpoints required")
	}
	if _, err := time.Parse(time.RFC3339Nano, update.ECSCheckpoint.MutationAuthorizedAt); err != nil {
		return empty, fmt.Errorf("valid forward mutation authorization timestamp required")
	}
	result := update.ECSCheckpoint.Result
	if result.NativeService == nil || result.NativeService.NoOpDeploymentARN != "" || result.PreviousService == nil || first.ECSCheckpoint.Result.TaskDefinitionARN != result.TaskDefinitionARN {
		return empty, fmt.Errorf("adopted or incomplete deployment cannot authorize rollback")
	}
	target := &ECSNativeRollback{ParentApplyID: parent.ID, DeploymentARN: result.ServiceDeploymentARN, ForwardRevisionARN: result.ServiceRevisionARN, SourceRevisionARN: result.PreviousServiceRevisionARN, ForwardTaskDefinitionARN: result.TaskDefinitionARN, SourceTaskDefinitionARN: result.PreviousTaskDefinitionARN, DesiredCount: result.PreviousService.DesiredCount}
	known := false
	for _, source := range result.NativeService.SourceRevisions {
		known = known || source.ARN == target.SourceRevisionARN && source.TaskDefinitionARN == target.SourceTaskDefinitionARN
	}
	if !known {
		return empty, fmt.Errorf("rollback requires the actual source captured before the forward write")
	}
	if err := target.Validate(*first.ECS); err != nil {
		return empty, err
	}
	return target, nil
}

func (t ECSNativeRollback) Clone() *ECSNativeRollback { copy := t; return &copy }

func (t ECSNativeRollback) Validate(intent domain.ECSDeploymentIntent) error {
	if t.Automatic && !intent.RollbackOnFailure {
		return fmt.Errorf("automatic native recovery requires reviewed rollbackOnFailure")
	}
	service, err := arn.Parse(intent.ServiceARN)
	base, baseErr := arn.Parse(intent.BaseTaskDefinitionARN)
	if err != nil || baseErr != nil || service.Service != "ecs" || !strings.HasPrefix(service.Resource, "service/") || intent.DeploymentController != "ECS" || intent.Advanced == nil || (intent.DeploymentStrategy != "BLUE_GREEN" && intent.DeploymentStrategy != "CANARY" && intent.DeploymentStrategy != "LINEAR") {
		return fmt.Errorf("explicit native advanced intent required")
	}
	scoped := func(value, kind string) bool {
		a, e := arn.Parse(value)
		prefix := kind + "/" + strings.TrimPrefix(service.Resource, "service/") + "/"
		suffix := strings.TrimPrefix(a.Resource, prefix)
		return e == nil && a.Partition == service.Partition && a.Service == "ecs" && a.Region == intent.Region && a.AccountID == intent.AWSAccountID && strings.HasPrefix(a.Resource, prefix) && suffix != "" && !strings.ContainsAny(suffix, "/*?")
	}
	family, _, _ := strings.Cut(strings.TrimPrefix(base.Resource, "task-definition/"), ":")
	sameTaskFamily := func(value string) bool {
		a, e := arn.Parse(value)
		f, revision, found := strings.Cut(strings.TrimPrefix(a.Resource, "task-definition/"), ":")
		n, nErr := strconv.Atoi(revision)
		return e == nil && found && nErr == nil && n > 0 && a.Partition == service.Partition && a.Service == "ecs" && a.Region == intent.Region && a.AccountID == intent.AWSAccountID && strings.HasPrefix(a.Resource, "task-definition/") && f == family
	}
	if t.ParentApplyID == "" || t.DesiredCount < 0 || !scoped(t.DeploymentARN, "service-deployment") || !scoped(t.ForwardRevisionARN, "service-revision") || !scoped(t.SourceRevisionARN, "service-revision") || t.ForwardRevisionARN == t.SourceRevisionARN || !sameTaskFamily(t.ForwardTaskDefinitionARN) || !sameTaskFamily(t.SourceTaskDefinitionARN) {
		return fmt.Errorf("fixed native rollback identities and captured capacity required")
	}
	return nil
}

func (s Service) requestNativeRollback(ctx context.Context, parent Record) (CreateResult, error) {
	if s.PrepareNativeRollback == nil {
		return CreateResult{}, ErrRecoveryUnavailable
	}
	store, ok := s.Store.(ECSExecutionStore)
	if !ok || len(parent.Operations) == 0 || parent.Operations[0].ECS == nil {
		return CreateResult{}, ErrRecoveryUnavailable
	}
	unlock, err := store.LockExecution(ctx, parent.DeploymentName, parent.Operations[0].ECS.ServiceARN)
	if err != nil {
		return CreateResult{}, err
	}
	defer unlock()
	parent, err = s.Get(ctx, parent.ID)
	if err != nil {
		return CreateResult{}, err
	}
	_, err = NativeRollbackEvidence(parent)
	if err != nil {
		return CreateResult{}, fmt.Errorf("%w: %v", ErrRecoveryUnavailable, err)
	}
	principal, err := s.AuthorizeCreate(ctx, parent)
	if err != nil {
		return CreateResult{}, err
	}
	if principal == nil || !principal.Identity.Authenticated || (principal.Identity.ID == "" && principal.Identity.Email == "") || principal.Automatic {
		return CreateResult{}, ErrExecutionUnauthorized
	}
	return s.createNativeRollback(ctx, parent, principal, false)
}

// Caller holds the physical-service lock and has freshly authorized approval.
func (s Service) createNativeRollback(ctx context.Context, parent Record, principal *ExecutionPrincipal, automatic bool) (CreateResult, error) {
	store, ok := s.Store.(ECSExecutionStore)
	if !ok || !s.EnableAWSApply || s.PrepareNativeRollback == nil {
		return CreateResult{}, ErrRecoveryUnavailable
	}
	target, err := NativeRollbackEvidence(parent)
	if err != nil {
		return CreateResult{}, err
	}
	if automatic && (!nativeAutomaticFailure(parent) || !reflect.DeepEqual(principal, parent.Principal)) {
		return CreateResult{}, ErrExecutionUnauthorized
	}
	target.Automatic = automatic
	records, err := store.List(ctx)
	if err != nil {
		return CreateResult{}, err
	}
	for _, existing := range records {
		if existing.RetryOf == parent.ID && !existing.Status.IsTerminal() {
			return CreateResult{}, fmt.Errorf("%w: explicit retry already active", ErrRecoveryUnavailable)
		}
		if existing.NativeRollback != nil && existing.NativeRollback.ParentApplyID == parent.ID {
			if err := nativeRollbackMatchesParent(existing, parent); err != nil {
				return CreateResult{}, err
			}
			if err := s.handoffNativeRollback(ctx, existing); err != nil {
				return CreateResult{}, err
			}
			return CreateResult{Record: NormalizeOperationIDs(existing)}, nil
		}
	}
	if parent.NativeRollbackRequestID != "" {
		return CreateResult{}, fmt.Errorf("%w: linked recovery record is missing", ErrRecoveryUnavailable)
	}
	if err := s.PrepareNativeRollback(ctx, parent, *target); err != nil {
		return CreateResult{}, fmt.Errorf("%w: %v", ErrRecoveryUnavailable, err)
	}
	child := Record{PlanID: parent.PlanID, DeploymentName: parent.DeploymentName, Source: parent.Source, Status: StatusQueued, CreatedAt: s.now().UTC().Format(time.RFC3339), Actor: requestactor.FromContext(ctx), Principal: principal.Clone(), NativeRollback: target.Clone()}
	if automatic {
		child.Actor = parent.Actor
	}
	child.Operations = nativeRecoveryOperations(parent, *target)
	child.Details = []string{"Native recovery of Apply " + parent.ID + "; the original forward evidence is preserved."}
	if automatic {
		child.Details = []string{"Automatic native recovery of failed Apply " + parent.ID + "; inherited reviewed rollbackOnFailure approval and original principal."}
	}
	child.Lifecycle = initialLifecycle(child)
	child.ID, err = stableRecordID(child)
	if err != nil {
		return CreateResult{}, err
	}
	if err := nativeRollbackMatchesParent(child, parent); err != nil {
		return CreateResult{}, err
	}
	child = NormalizeOperationIDs(child)
	// Save the recovery intent first. Both the request path and executor repair the
	// parent link under this same service lock after an interruption between files.
	if err := store.Save(ctx, child); err != nil {
		return CreateResult{}, err
	}
	if err := s.handoffNativeRollback(ctx, child); err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Record: child, Created: true}, nil
}

func nativeRollbackMatchesParent(child, parent Record) error {
	target, err := NativeRollbackEvidence(parent)
	if err != nil {
		return err
	}
	if child.NativeRollback != nil && child.NativeRollback.Automatic {
		if !nativeAutomaticFailure(parent) || !reflect.DeepEqual(child.Principal, parent.Principal) {
			return fmt.Errorf("automatic native recovery lost reviewed failure approval")
		}
		target.Automatic = true
	}
	expected := nativeRecoveryOperations(parent, *target)
	id, idErr := stableRecordID(child)
	if idErr != nil || id != child.ID || child.Recovery != nil || child.RetryOf != "" || child.Principal == nil || !child.Principal.Identity.Authenticated || (child.Principal.Identity.ID == "" && child.Principal.Identity.Email == "") || (child.Principal.Automatic && !target.Automatic) || !reflect.DeepEqual(child.NativeRollback, target) || child.PlanID != parent.PlanID || child.DeploymentName != parent.DeploymentName || !reflect.DeepEqual(child.Source, parent.Source) || len(child.Operations) != len(expected) {
		return fmt.Errorf("altered native recovery approval/source")
	}
	for i, op := range child.Operations {
		original := expected[i]
		if op.ID != original.ID || op.Kind != original.Kind || op.Target != original.Target || !reflect.DeepEqual(op.ECS, original.ECS) {
			return fmt.Errorf("altered native recovery operations")
		}
	}
	if parent.NativeRollbackRequestID != "" && parent.NativeRollbackRequestID != child.ID {
		return fmt.Errorf("native recovery already belongs to a different apply")
	}
	return nil
}

func (s Service) validateNativeRollback(ctx context.Context, child Record, plan planpreview.Record) error {
	if child.NativeRollback == nil {
		return nil
	}
	parent, err := s.Get(ctx, child.NativeRollback.ParentApplyID)
	if err != nil {
		return err
	}
	if err := nativeRollbackMatchesParent(child, parent); err != nil {
		return err
	}
	if parent.PlanID != plan.ID || len(plan.Operations) != 3 {
		return fmt.Errorf("native recovery original plan mismatch")
	}
	for i, op := range parent.Operations {
		reviewed := plan.Operations[i]
		if op.ID != reviewed.ID || op.Kind != reviewed.Kind || op.Target != reviewed.Target || !reflect.DeepEqual(op.ECS, reviewed.ECS) {
			return fmt.Errorf("native recovery parent differs from reviewed plan")
		}
	}
	return nil
}

func (s Service) handoffNativeRollback(ctx context.Context, child Record) error {
	store, ok := s.Store.(MutationStore)
	if !ok {
		return ErrRecoveryUnavailable
	}
	_, err := store.Mutate(ctx, child.NativeRollback.ParentApplyID, func(parent Record) (Record, error) {
		if err := nativeRollbackMatchesParent(child, parent); err != nil {
			return Record{}, err
		}
		if parent.NativeRollbackRequestID == child.ID {
			return parent, nil
		}
		parent.NativeRollbackRequestID = child.ID
		note := "Local forward tracking handed to native recovery Apply " + child.ID + "; AWS rollback outcome is not yet verified."
		parent.Details = append(parent.Details, note)
		if parent.Status == StatusRunning {
			previous := parent.Status
			parent.Status = StatusCancelled
			parent.Claim = nil
			now := s.now()
			parent.Lifecycle = transitionLifecycle(parent.Lifecycle, len(parent.Operations), TransitionRequest{Status: StatusCancelled}, now)
			parent.Transitions = append(parent.Transitions, Transition{TransitionedAt: now.UTC().Format(time.RFC3339Nano), PreviousStatus: previous, Status: StatusCancelled, Actor: child.Actor, Details: []string{note}, Lifecycle: parent.Lifecycle})
		}
		return parent, nil
	})
	return err
}

func (s Service) hasPendingNativeRollback(ctx context.Context, parent Record) (bool, error) {
	records, err := s.Store.List(ctx)
	if err != nil {
		return false, err
	}
	for _, child := range records {
		if child.NativeRollback != nil && child.NativeRollback.ParentApplyID == parent.ID {
			if err := nativeRollbackMatchesParent(child, parent); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	if parent.NativeRollbackRequestID != "" {
		return false, fmt.Errorf("linked native recovery is missing; forward tracking cannot resume")
	}
	return false, nil
}

func nativeRollbackKinds(record Record) []string {
	kinds := []string{NativeRollbackOperationKind}
	if record.NativeRollback != nil && len(record.Operations) > 0 && record.Operations[0].ECS != nil && int64(record.NativeRollback.DesiredCount) != int64(record.Operations[0].ECS.DesiredCount) {
		kinds = append(kinds, NativeCapacityRestoreOperationKind)
	}
	return append(kinds, string(reconcile.StepVerifyServiceStability))
}

func nativeRecoveryOperations(parent Record, target ECSNativeRollback) []Operation {
	operations := cloneOperations(parent.Operations[1:])
	if int64(target.DesiredCount) != int64(parent.Operations[0].ECS.DesiredCount) {
		restore := cloneOperations(parent.Operations[1:2])[0]
		restore.ID += ":restore-capacity"
		restore.Kind = NativeCapacityRestoreOperationKind
		restore.Summary = fmt.Sprintf("After source rollback, restore captured desired count %d without starting another deployment.", target.DesiredCount)
		operations = []Operation{operations[0], restore, operations[1]}
	}
	for i := range operations {
		op := &operations[i]
		op.Status = OperationStatusQueued
		op.ECSCheckpoint = nil
		op.ExecutionAttempt = nil
		op.Target = op.ECS.ServiceARN
		op.Phase = "Recovering"
		if i == 0 {
			op.Kind = NativeRollbackOperationKind
			op.Target = target.DeploymentARN
			op.Summary = "Request ECS native rollback of the exact owned deployment to its captured source revision."
		} else if i == len(operations)-1 {
			op.Summary = "Verify captured source capacity, traffic, tasks and ALB health; stop acceptance alone is not recovery."
		}
	}
	return operations
}
