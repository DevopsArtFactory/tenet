package applyrequest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

var ErrRecoveryUnavailable = errors.New("ECS recovery is unavailable")
var ErrFailedIntentHeld = errors.New("failed ECS intent requires an explicit retry approval")

// ECSRecovery binds a separate recovery Apply to immutable forward-failure evidence.
type ECSRecovery struct {
	ApplyID                  string                      `json:"applyId"`
	ForwardIntent            *domain.ECSDeploymentIntent `json:"forwardIntent"`
	ForwardTaskDefinitionARN string                      `json:"forwardTaskDefinitionArn"`
	ForwardDeploymentID      string                      `json:"forwardDeploymentId"`
	PreviousService          ECSServiceSnapshot          `json:"previousService"`
	Target                   *domain.ECSDeploymentIntent `json:"target"`
	Automatic                bool                        `json:"automatic,omitempty"`
}

func (r *ECSRecovery) Clone() *ECSRecovery {
	if r == nil {
		return nil
	}
	v := *r
	v.ForwardIntent = r.ForwardIntent.Clone()
	v.Target = r.Target.Clone()
	return &v
}

func recoveryEvidence(parent Record) (*ECSRecovery, error) {
	if parent.Status != StatusFailed || parent.Recovery != nil || parent.Principal == nil || len(parent.Operations) != 3 || parent.Operations[0].ECS == nil || parent.Operations[0].ECS.DeploymentStrategy != "ROLLING" {
		return nil, ErrRecoveryUnavailable
	}
	register, update := parent.Operations[0].ECSCheckpoint, parent.Operations[1].ECSCheckpoint
	if register == nil || update == nil || register.State != "complete" || update.Result.PreviousService == nil || update.Result.PreviousService.TaskDefinitionARN == "" || update.Result.TaskDefinitionARN == "" || register.Result.TaskDefinitionARN != update.Result.TaskDefinitionARN {
		return nil, fmt.Errorf("%w: durable registration and pre-update service baseline are required", ErrRecoveryUnavailable)
	}
	baseline := *update.Result.PreviousService
	if baseline.DesiredCount < 0 || baseline.MinimumHealthyPercent < 0 || baseline.MinimumHealthyPercent > 100 || baseline.MaximumPercent < 100 || baseline.MaximumPercent > 200 {
		return nil, fmt.Errorf("%w: invalid previous service configuration", ErrRecoveryUnavailable)
	}
	deploymentID := update.Result.ServiceDeploymentID
	if deploymentID == "" && parent.ECSRuntime != nil {
		deploymentID = parent.ECSRuntime.DeploymentID
	}
	return &ECSRecovery{ApplyID: parent.ID, ForwardIntent: parent.Operations[0].ECS.Clone(), ForwardTaskDefinitionARN: register.Result.TaskDefinitionARN, ForwardDeploymentID: deploymentID, PreviousService: baseline}, nil
}

func (s Service) RequestRollback(ctx context.Context, parentID string) (CreateResult, error) {
	if !s.EnableAWSApply || s.AuthorizeCreate == nil || s.RollbackTarget == nil {
		return CreateResult{}, ErrRecoveryUnavailable
	}
	parent, err := s.Get(ctx, parentID)
	if err != nil {
		return CreateResult{}, err
	}
	recovery, err := recoveryEvidence(parent)
	if err != nil {
		return CreateResult{}, err
	}
	approval := parent
	approval.Recovery = recovery
	principal, err := s.AuthorizeCreate(ctx, approval)
	if err != nil {
		return CreateResult{}, err
	}
	if principal == nil || !principal.Identity.Authenticated {
		return CreateResult{}, ErrExecutionUnauthorized
	}
	store, ok := s.Store.(ECSExecutionStore)
	if !ok {
		return CreateResult{}, ErrRecoveryUnavailable
	}
	unlock, err := store.LockExecution(ctx, parent.DeploymentName, recovery.ForwardIntent.ServiceARN)
	if err != nil {
		return CreateResult{}, err
	}
	defer unlock()
	return s.createRollback(ctx, parent, principal, false)
}

// Caller holds the same deployment/service lock as forward execution and has
// freshly authorized the principal. Never rewrite the failed forward record.
func (s Service) createRollback(ctx context.Context, parent Record, principal *ExecutionPrincipal, automatic bool) (CreateResult, error) {
	recovery, err := recoveryEvidence(parent)
	if err != nil {
		return CreateResult{}, err
	}
	records, err := s.Store.List(ctx)
	if err != nil {
		return CreateResult{}, err
	}
	for _, existing := range records {
		if existing.RetryOf == parent.ID && !existing.Status.IsTerminal() {
			return CreateResult{}, fmt.Errorf("%w: an explicit retry is already active", ErrRecoveryUnavailable)
		}
		if existing.Recovery != nil && existing.Recovery.ApplyID == parent.ID {
			return CreateResult{Record: NormalizeOperationIDs(existing)}, nil
		}
	}
	if s.RollbackTarget == nil {
		return CreateResult{}, ErrRecoveryUnavailable
	}
	target, err := s.RollbackTarget(ctx, parent)
	if err != nil {
		return CreateResult{}, err
	}
	recovery.Target = target.Clone()
	recovery.Automatic = automatic
	if err := validateRecoveryTarget(recovery); err != nil {
		return CreateResult{}, err
	}
	record := Record{PlanID: parent.PlanID, DeploymentName: parent.DeploymentName, Source: parent.Source, Status: StatusQueued, CreatedAt: s.now().UTC().Format(time.RFC3339), Actor: requestactor.FromContext(ctx), Principal: principal.Clone(), Recovery: recovery}
	if automatic {
		record.Actor = parent.Actor
	}
	record.Operations = cloneOperations(parent.Operations)
	for i := range record.Operations {
		op := &record.Operations[i]
		op.ECS = target.Clone()
		op.Status = OperationStatusQueued
		op.ECSCheckpoint = nil
		op.ExecutionAttempt = nil
		op.Target = target.ServiceARN
		if i == 0 {
			op.Target = target.BaseTaskDefinitionARN
			op.Summary = "Reuse the exact previous ACTIVE task definition; never register a rollback replacement."
		}
		if i == 1 {
			op.Summary = "Restore the previous task-definition/count/rolling configuration, fenced to the failed deployment identity."
		}
		if i == 2 {
			op.Summary = "Verify the previous revision stabilizes; recovery success does not change the failed forward result."
		}
	}
	record.Details = []string{"Separate Rolling recovery of failed Apply " + parent.ID + "; the failed forward result is preserved."}
	record.Lifecycle = initialLifecycle(record)
	record.ID, err = stableRecordID(record)
	if err != nil {
		return CreateResult{}, err
	}
	readiness := deriveExecutionReadiness(record)
	record.ExecutionReadiness = &readiness
	if err := s.Store.Save(ctx, record); err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Record: NormalizeOperationIDs(record), Created: true}, nil
}

func validateRecoveryTarget(r *ECSRecovery) error {
	if r == nil || r.ForwardIntent == nil || r.Target == nil || r.Target.Image == "" {
		return ErrRecoveryUnavailable
	}
	expected := r.ForwardIntent.Clone()
	expected.BaseTaskDefinitionARN = r.PreviousService.TaskDefinitionARN
	expected.Image = r.Target.Image
	expected.DesiredCount = int(r.PreviousService.DesiredCount)
	expected.MinimumHealthyPercent = int(r.PreviousService.MinimumHealthyPercent)
	expected.MaximumPercent = int(r.PreviousService.MaximumPercent)
	expected.RollbackOnFailure = false
	if !reflect.DeepEqual(expected, r.Target) {
		return fmt.Errorf("%w: target differs from the persisted previous service", ErrRecoveryUnavailable)
	}
	return nil
}

func reviewedECSOperationMatches(record Record, op Operation, reviewed planpreview.Operation) bool {
	if record.Recovery == nil {
		return op.Target == reviewed.Target && reflect.DeepEqual(op.ECS, reviewed.ECS)
	}
	target := record.Recovery.Target.ServiceARN
	if op.Kind == "RegisterECSTaskDefinition" {
		target = record.Recovery.Target.BaseTaskDefinitionARN
	}
	return op.Target == target && reflect.DeepEqual(op.ECS, record.Recovery.Target) && reflect.DeepEqual(reviewed.ECS, record.Recovery.ForwardIntent)
}

func (s Service) validateECSRecovery(ctx context.Context, record Record, plan planpreview.Record) error {
	if record.Recovery == nil {
		return nil
	}
	parent, err := s.Get(ctx, record.Recovery.ApplyID)
	if err != nil {
		return err
	}
	expected, err := recoveryEvidence(parent)
	if err != nil {
		return err
	}
	expected.Target = record.Recovery.Target.Clone()
	expected.Automatic = record.Recovery.Automatic
	if parent.PlanID != plan.ID || parent.DeploymentName != record.DeploymentName || !reflect.DeepEqual(expected, record.Recovery) {
		return fmt.Errorf("%w: altered forward failure evidence", ErrRecoveryUnavailable)
	}
	return validateRecoveryTarget(record.Recovery)
}

func (e Executor) automaticECSRecovery(ctx context.Context, parent Record) error {
	if parent.Status != StatusFailed || parent.Recovery != nil || parent.Principal == nil || len(parent.Operations) != 3 || parent.Operations[0].ECS == nil || !parent.Operations[0].ECS.RollbackOnFailure || parent.ECSRuntime == nil {
		return nil
	}
	// Read unavailability, timeout and external supersession never authorize a rollback.
	if parent.ECSRuntime.Reason != "rollout_failed" && parent.ECSRuntime.Reason != "rollback_observed" {
		return nil
	}
	records, err := e.Service.Store.List(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if (record.Recovery != nil && record.Recovery.ApplyID == parent.ID) || record.RetryOf == parent.ID {
			return nil
		}
	}
	if err := e.ECS.Authorize(ctx, parent); err != nil {
		return err
	}
	current, err := e.ECS.CurrentIntent(ctx, parent.DeploymentName)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, parent.Operations[0].ECS) {
		return nil
	}
	_, err = e.Service.createRollback(ctx, parent, parent.Principal, true)
	return err
}

func (s Service) checkFailedECSIntent(ctx context.Context, candidate Record) error {
	if len(candidate.Operations) == 0 || candidate.Operations[0].ECS == nil {
		return nil
	}
	records, err := s.Store.List(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Recovery != nil || record.Status != StatusFailed || record.Principal == nil || len(record.Operations) == 0 || record.Operations[0].ECS == nil {
			continue
		}
		if reflect.DeepEqual(record.Operations[0].ECS, candidate.Operations[0].ECS) {
			if candidate.RetryOf == record.ID {
				return nil
			}
			// A retry may supersede an earlier failure in the same retry chain.
			if candidate.RetryOf != "" {
				parent, err := s.Get(ctx, candidate.RetryOf)
				if err != nil {
					return err
				}
				if parent.Status == StatusFailed && parent.Recovery == nil && len(parent.Operations) > 0 && reflect.DeepEqual(parent.Operations[0].ECS, candidate.Operations[0].ECS) {
					return nil
				}
			}
			return ErrFailedIntentHeld
		}
	}
	return nil
}

func (s Service) Retry(ctx context.Context, parentID string) (CreateResult, error) {
	if !s.EnableAWSApply || s.AuthorizeCreate == nil || s.CreateRetryPlan == nil {
		return CreateResult{}, ErrRecoveryUnavailable
	}
	parent, err := s.Get(ctx, parentID)
	if err != nil {
		return CreateResult{}, err
	}
	if parent.Status != StatusFailed || parent.Recovery != nil || parent.Principal == nil || len(parent.Operations) != 3 || parent.Operations[0].ECS == nil {
		return CreateResult{}, ErrRecoveryUnavailable
	}
	if _, err := s.AuthorizeCreate(ctx, parent); err != nil {
		return CreateResult{}, err
	}
	store, ok := s.Store.(ECSExecutionStore)
	if !ok {
		return CreateResult{}, ErrRecoveryUnavailable
	}
	unlock, err := store.LockExecution(ctx, parent.DeploymentName, parent.Operations[0].ECS.ServiceARN)
	if err != nil {
		return CreateResult{}, err
	}
	defer unlock()
	records, err := s.Store.List(ctx)
	if err != nil {
		return CreateResult{}, err
	}
	for _, record := range records {
		if record.Recovery != nil && record.Recovery.ApplyID == parent.ID && !record.Status.IsTerminal() {
			return CreateResult{}, fmt.Errorf("%w: rollback is still active", ErrRecoveryUnavailable)
		}
		if record.RetryOf == parent.ID {
			return CreateResult{Record: NormalizeOperationIDs(record)}, nil
		}
	}

	plan, err := s.CreateRetryPlan(ctx, parent.DeploymentName)
	if err != nil {
		return CreateResult{}, err
	}
	if len(plan.Operations) != 3 || !reflect.DeepEqual(plan.Operations[0].ECS, parent.Operations[0].ECS) {
		return CreateResult{}, ErrSourceFingerprintConflict
	}
	return s.create(ctx, plan.ID, parent.ID)
}

func failedDeploymentID(record Record) string {
	if len(record.Operations) > 1 && record.Operations[1].ECSCheckpoint != nil && record.Operations[1].ECSCheckpoint.Result.ServiceDeploymentID != "" {
		return record.Operations[1].ECSCheckpoint.Result.ServiceDeploymentID
	}
	if record.ECSRuntime != nil {
		return record.ECSRuntime.DeploymentID
	}
	return ""
}
func (s Service) validateECSRetry(ctx context.Context, record Record) error {
	if record.RetryOf == "" {
		if record.RetryDeploymentID != "" {
			return ErrRecoveryUnavailable
		}
		return nil
	}
	parent, err := s.Get(ctx, record.RetryOf)
	if err != nil {
		return err
	}
	if parent.Status != StatusFailed || parent.Recovery != nil || record.Recovery != nil || parent.DeploymentName != record.DeploymentName || len(parent.Operations) != 3 || len(record.Operations) != 3 || !reflect.DeepEqual(parent.Operations[0].ECS, record.Operations[0].ECS) || record.RetryDeploymentID != failedDeploymentID(parent) {
		return ErrRecoveryUnavailable
	}
	return nil
}
