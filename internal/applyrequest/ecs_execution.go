package applyrequest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
)

// ECSCheckpoint is durable evidence for one reviewed operation. In-flight means
// the call may have reached AWS; recovery must reconcile before any retry.
type ECSCheckpoint struct {
	RequestID  string             `json:"requestId"`
	State      string             `json:"state"`
	Attempts   int                `json:"attempts"`
	UpdatedAt  string             `json:"updatedAt"`
	RetryAfter string             `json:"retryAfter,omitempty"`
	Error      string             `json:"error,omitempty"`
	Result     ECSOperationResult `json:"result,omitempty"`
}

// ECSServiceSnapshot records fields changed by a Rolling update before AWS mutation.
type ECSServiceSnapshot struct {
	TaskDefinitionARN     string `json:"taskDefinitionArn"`
	DesiredCount          int32  `json:"desiredCount"`
	MinimumHealthyPercent int32  `json:"minimumHealthyPercent"`
	MaximumPercent        int32  `json:"maximumPercent"`
	PrimaryDeploymentID   string `json:"primaryDeploymentId"`
}

// ECSOperationResult contains identifiers and rollback fields, never task bodies or secrets.
type ECSOperationResult struct {
	PreviousService           *ECSServiceSnapshot `json:"previousService,omitempty"`
	ServiceDeploymentID       string              `json:"serviceDeploymentId,omitempty"`
	TaskDefinitionRevision    int32               `json:"taskDefinitionRevision,omitempty"`
	TaskDefinitionIntent      string              `json:"taskDefinitionIntent,omitempty"`
	TaskDefinitionARN         string              `json:"taskDefinitionArn,omitempty"`
	PreviousTaskDefinitionARN string              `json:"previousTaskDefinitionArn,omitempty"`
	ServiceDeploymentARN      string              `json:"serviceDeploymentArn,omitempty"`
	AWSRequestID              string              `json:"awsRequestId,omitempty"`
}

// ECSOperationRequest binds an injected handler to reviewed intent and durable identity.
type ECSOperationRequest struct {
	ApplyID        string
	OperationID    string
	RequestID      string
	Intent         domain.ECSDeploymentIntent
	Role           DeployerRoleReadiness
	PriorResults   []ECSOperationResult
	PreparedResult ECSOperationResult
}

// ECSReconciliation reports confirmed completion, confirmed absence (safe to
// retry), or uncertainty (keep ownership and reconcile again without writing).
type ECSReconciliation struct {
	Failure  string
	Complete bool
	Absent   bool
	Result   ECSOperationResult
}

// ECSOperationHandler implementations must use assumed deployer-role clients.
// Preflight is read-only and checks concrete inputs/actions, including scoped
// iam:PassRole for inherited task roles. IAM simulation alone is not authority.
// Execute must issue at most one mutation; disable blind SDK mutation retries.
// Reconcile must never mutate and may report Absent only with positive evidence.
type ECSOperationHandler interface {
	Preflight(context.Context, ECSOperationRequest) error
	Execute(context.Context, ECSOperationRequest) (ECSOperationResult, error)
	Reconcile(context.Context, ECSOperationRequest) (ECSReconciliation, error)
}

// ECSOperationPreparer captures read-only rollback/correlation evidence before
// the executor persists an in-flight checkpoint and permits the AWS mutation.
type ECSOperationPreparer interface {
	Prepare(context.Context, ECSOperationRequest) (ECSOperationResult, error)
}

func (r ECSOperationResult) Clone() ECSOperationResult {
	if r.PreviousService != nil {
		snapshot := *r.PreviousService
		r.PreviousService = &snapshot
	}
	return r
}

// ECSCallError distinguishes confirmed rejection from an ambiguous AWS outcome.
// Unclassified errors are ambiguous and always go through read-only recovery.
type ECSCallError struct {
	Err       error
	Retryable bool
	Rejected  bool
}

func (e *ECSCallError) Error() string {
	if e == nil || e.Err == nil {
		return "ECS call failed without an underlying error"
	}
	return e.Err.Error()
}
func (e *ECSCallError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ECSExecution is an explicit, fail-closed live boundary. No production handlers
// are installed by default; supplying handlers alone does not opt into execution.
type ECSExecution struct {
	Enabled  bool
	Handlers map[string]ECSOperationHandler
	// Authorize must recheck a trusted authenticated subject's operator role and
	// deployment scope. Record.Actor is attribution, and must not authorize work.
	Authorize     func(context.Context, Record) error
	CurrentIntent func(context.Context, string) (*domain.ECSDeploymentIntent, error)
}

// ECSExecutionStore combines durable state changes and exclusive execution of
// a deployment/service. LockExecution must protect against other processes.
type ECSExecutionStore interface {
	Store
	MutationStore
	LockExecution(context.Context, string, string) (func(), error)
}

var ErrExecutionBusy = fmt.Errorf("%w: ECS execution is owned by another active apply", ErrAlreadyClaimed)

func (e Executor) advanceECS(ctx context.Context, snapshot Record) error {
	store, ok := e.Service.Store.(ECSExecutionStore)
	if !ok {
		return fmt.Errorf("ECS execution requires atomic durable storage and execution locks")
	}
	if len(snapshot.Operations) == 0 || snapshot.Operations[0].ECS == nil {
		return fmt.Errorf("reviewed native ECS operations are required")
	}
	unlock, err := store.LockExecution(ctx, snapshot.DeploymentName, snapshot.Operations[0].ECS.ServiceARN)
	if err != nil {
		if errors.Is(err, ErrExecutionBusy) {
			return nil
		}
		return err
	}
	defer unlock()
	record, err := store.Get(ctx, snapshot.ID)
	if err != nil {
		return err
	}
	if record.Status.IsTerminal() {
		return nil
	}
	if record.DeploymentName != snapshot.DeploymentName || len(record.Operations) == 0 || record.Operations[0].ECS == nil || record.Operations[0].ECS.ServiceARN != snapshot.Operations[0].ECS.ServiceARN {
		return fmt.Errorf("execution target changed while acquiring ownership")
	}
	records, err := store.List(ctx)
	if err != nil {
		return err
	}
	for _, other := range records {
		if ecsApplyConflicts(record, other) {
			return nil
		}
	}

	// Validate every operation/handler before claiming or mutating anything.
	role, err := e.validateECS(ctx, record)
	if err != nil {
		return err
	}
	for i, op := range record.Operations {
		if ecsAllComplete(record) {
			break
		}
		if err := e.ECS.Handlers[op.Kind].Preflight(ctx, ecsRequest(record, i, role)); err != nil {
			return err
		}
	}
	record, err = store.Mutate(ctx, record.ID, func(current Record) (Record, error) {
		if current.Claim != nil && current.Claim.WorkerID == e.workerID() {
			if expired, at := persistedClaimExpiresAtExpired(*current.Claim, e.Service.now()); expired {
				return recoverExpiredClaim(ctx, current, e.workerID(), current.Claim.WorkerID, at, e.Service.now()), nil
			}
		}
		return e.Service.claimRecord(ctx, ClaimRequest{WorkerID: e.workerID(), AllowExpiredClaim: true})(current)
	})
	if err != nil {
		if errors.Is(err, ErrAlreadyClaimed) || errors.Is(err, ErrInvalidState) {
			return nil
		}
		return err
	}
	record, err = e.ecsOwnedChange(ctx, record, e.ecsRefreshHeartbeat)
	if err != nil {
		return err
	}
	// Hold the execution lock throughout each call, including heartbeats. A
	// replacement worker cannot overlap a still-running call after lease expiry.
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		err := e.ecsHeartbeat(callCtx, record, claimFreshnessTTL/3)
		if err != nil {
			cancel()
		}
		done <- err
	}()
	runErr := e.runECSOperation(callCtx, record, role)
	cancel()
	heartbeatErr := <-done
	if runErr != nil {
		return runErr
	}
	return heartbeatErr
}

func ecsRequest(record Record, index int, role DeployerRoleReadiness) ECSOperationRequest {
	op := record.Operations[index]
	sum := sha256.Sum256([]byte(record.ID + "/" + op.ID))
	request := ECSOperationRequest{ApplyID: record.ID, OperationID: op.ID, RequestID: "tenet-" + hex.EncodeToString(sum[:16]), Intent: *op.ECS, Role: role}
	for _, previous := range record.Operations[:index] {
		if previous.ECSCheckpoint != nil && previous.ECSCheckpoint.State == "complete" {
			request.PriorResults = append(request.PriorResults, previous.ECSCheckpoint.Result.Clone())
		}
	}
	if op.ECSCheckpoint != nil {
		request.PreparedResult = op.ECSCheckpoint.Result.Clone()
	}
	return request
}

func (e Executor) validateECS(ctx context.Context, record Record) (DeployerRoleReadiness, error) {
	if e.ECS == nil || !e.ECS.Enabled || !e.Service.EnableAWSApply {
		return DeployerRoleReadiness{}, fmt.Errorf("live ECS execution is disabled")
	}
	if e.ECS.Authorize == nil || e.ECS.CurrentIntent == nil || e.Service.LoadPlan == nil || e.Service.CurrentFingerprint == nil || e.Service.DeployerRoleReadiness == nil {
		return DeployerRoleReadiness{}, fmt.Errorf("live ECS authorization, intent, plan, fingerprint and deployer-role providers are required")
	}
	plan, err := e.Service.LoadPlan(ctx, record.PlanID)
	if err != nil {
		return DeployerRoleReadiness{}, err
	}
	plan = planpreview.NormalizeOperationIDs(plan)
	if plan.Status != planpreview.StatusReady || plan.ID != record.PlanID || plan.DeploymentName != record.DeploymentName || record.Source.Fingerprint != plan.Source.Fingerprint || record.Source.Fingerprint == "" || len(plan.Operations) != len(record.Operations) || len(record.Operations) != 3 {
		return DeployerRoleReadiness{}, fmt.Errorf("apply does not match its reviewed native ECS plan")
	}
	kinds := []string{string(reconcile.StepRegisterECSTaskDefinition), string(reconcile.StepUpdateECSService), string(reconcile.StepVerifyServiceStability)}
	expectedID, idErr := stableRecordID(record)
	if idErr != nil || expectedID != record.ID {
		return DeployerRoleReadiness{}, fmt.Errorf("apply identity does not match reviewed source")
	}
	started := false
	incomplete := false
	for i, op := range record.Operations {
		reviewed := plan.Operations[i]
		if op.ECS == nil || op.ECS.DeploymentController != "ECS" || op.ECS.DeploymentStrategy != "ROLLING" || op.Kind != kinds[i] || e.ECS.Handlers[op.Kind] == nil || op.ID != reviewed.ID || op.Kind != reviewed.Kind || op.Target != reviewed.Target || !reflect.DeepEqual(op.ECS, reviewed.ECS) || !reflect.DeepEqual(op.ECS, record.Operations[0].ECS) {
			return DeployerRoleReadiness{}, fmt.Errorf("unsupported or altered reviewed ECS operation")
		}
		if cp := op.ECSCheckpoint; cp != nil {
			if incomplete || cp.RequestID != ecsRequest(record, i, DeployerRoleReadiness{}).RequestID || cp.Attempts < 1 || cp.Attempts > 3 || (cp.State != "in_flight" && cp.State != "complete" && cp.State != "retryable_rejection") {
				return DeployerRoleReadiness{}, fmt.Errorf("invalid ECS execution checkpoint")
			}
		}
		if op.ECSCheckpoint == nil || op.ECSCheckpoint.State != "complete" {
			incomplete = true
		}
		started = started || op.ECSCheckpoint != nil
	}
	if ecsAllComplete(record) {
		return DeployerRoleReadiness{}, nil
	}
	if err := e.ECS.Authorize(ctx, record); err != nil {
		return DeployerRoleReadiness{}, err
	}
	current, err := e.ECS.CurrentIntent(ctx, record.DeploymentName)
	if err != nil {
		return DeployerRoleReadiness{}, err
	}
	if !reflect.DeepEqual(current, record.Operations[0].ECS) {
		return DeployerRoleReadiness{}, ErrSourceFingerprintConflict
	}
	// The full preview fingerprint includes observed runtime state; once execution
	// starts, compare desired intent above so our own rollout does not invalidate it.
	if !started {
		if err := e.Service.validateCurrentFingerprint(ctx, plan); err != nil {
			return DeployerRoleReadiness{}, err
		}
	}
	role, err := e.Service.DeployerRoleReadiness(ctx, DeployerRoleTarget{DeploymentName: record.DeploymentName})
	if err != nil {
		return role, err
	}
	intent := record.Operations[0].ECS
	if !role.Configured || !role.CheckedWithAWS || !role.Ready || role.RoleARN == "" || role.AWSAccountID != intent.AWSAccountID || role.Region != intent.Region {
		return role, fmt.Errorf("deployer role is not ready for reviewed ECS placement")
	}
	parsed, parseErr := arn.Parse(role.RoleARN)
	serviceARN, serviceErr := arn.Parse(intent.ServiceARN)
	if parseErr != nil || serviceErr != nil || parsed.Partition != serviceARN.Partition || parsed.AccountID != intent.AWSAccountID || parsed.Service != "iam" || !strings.HasPrefix(parsed.Resource, "role/") {
		return role, fmt.Errorf("deployer role ARN does not match reviewed placement")
	}
	return role, nil
}

func (e Executor) ecsOwnedChange(ctx context.Context, owner Record, change func(Record) (Record, error)) (Record, error) {
	store, ok := e.Service.Store.(MutationStore)
	if !ok {
		return Record{}, fmt.Errorf("ECS execution requires atomic durable storage")
	}
	return store.Mutate(ctx, owner.ID, func(current Record) (Record, error) {
		if current.Status != StatusRunning || current.Claim == nil || owner.Claim == nil || current.Claim.WorkerID != owner.Claim.WorkerID || current.Claim.ClaimedAt != owner.Claim.ClaimedAt {
			return Record{}, ErrAlreadyClaimed
		}
		if expired, _, _ := claimExpired(*current.Claim, e.Service.now()); expired {
			return Record{}, ErrAlreadyClaimed
		}
		return change(current)
	})
}

func (e Executor) ecsHeartbeat(ctx context.Context, owner Record, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_, err := e.ecsOwnedChange(ctx, owner, e.ecsRefreshHeartbeat)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

func (e Executor) runECSOperation(ctx context.Context, record Record, role DeployerRoleReadiness) error {
	index := 0
	for index < len(record.Operations) && record.Operations[index].ECSCheckpoint != nil && record.Operations[index].ECSCheckpoint.State == "complete" {
		index++
	}
	if index == len(record.Operations) {
		return e.finishECS(ctx, record, StatusSucceeded, "")
	}
	op := record.Operations[index]
	request := ecsRequest(record, index, role)
	handler := e.ECS.Handlers[op.Kind]
	if op.ECSCheckpoint != nil && op.ECSCheckpoint.RetryAfter != "" {
		retry, err := time.Parse(time.RFC3339Nano, op.ECSCheckpoint.RetryAfter)
		if err != nil {
			return err
		}
		if e.Service.now().Before(retry) {
			return nil
		}
	}
	if op.ECSCheckpoint != nil && op.ECSCheckpoint.State == "retryable_rejection" && op.ECSCheckpoint.Attempts >= 3 {
		return e.finishECS(ctx, record, StatusFailed, "ECS execution retries exhausted after confirmed AWS rejections")
	}
	if op.ECSCheckpoint != nil && op.ECSCheckpoint.State == "in_flight" {
		result, err := handler.Reconcile(ctx, request)
		if err != nil {
			return e.recordECSError(ctx, record, index, err)
		}
		if (result.Complete && result.Absent) || (result.Failure != "" && (result.Complete || result.Absent)) {
			return fmt.Errorf("invalid ECS reconciliation result")
		}
		if result.Failure != "" {
			return e.finishECS(ctx, record, StatusFailed, result.Failure)
		}
		if result.Complete {
			return e.completeECSOperation(ctx, record, index, result.Result)
		}
		if !result.Absent {
			return e.recordECSError(ctx, record, index, fmt.Errorf("AWS outcome is unresolved; retaining deployment ownership"))
		}
		if op.ECSCheckpoint.Attempts >= 3 {
			return e.finishECS(ctx, record, StatusFailed, "ECS execution retries exhausted after confirmed absent outcome")
		}
	}
	// Refresh authorization/intent/role and concrete capabilities immediately before
	// a write, then persist in-flight intent before allowing the handler to call AWS.
	refreshedRole, err := e.validateECS(ctx, record)
	if err != nil {
		return err
	}
	request.Role = refreshedRole
	if err := handler.Preflight(ctx, request); err != nil {
		return err
	}
	if preparer, ok := handler.(ECSOperationPreparer); ok {
		prepared, prepareErr := preparer.Prepare(ctx, request)
		if prepareErr != nil {
			return prepareErr
		}
		request.PreparedResult = prepared.Clone()
	}
	record, err = e.ecsOwnedChange(ctx, record, func(current Record) (Record, error) {
		now := e.Service.now().UTC().Format(time.RFC3339Nano)
		attempts := 0
		if cp := current.Operations[index].ECSCheckpoint; cp != nil {
			attempts = cp.Attempts
		}
		current.Operations[index].ECSCheckpoint = &ECSCheckpoint{RequestID: request.RequestID, State: "in_flight", Attempts: attempts + 1, UpdatedAt: now, Result: request.PreparedResult.Clone()}
		current.Error = ""
		current.Details = []string{"Live ECS operation started with durable request identity " + request.RequestID + "; AWS outcome must be reconciled after interruption."}
		current.Operations = transitionOperations(current.Operations, StatusRunning, index+1)
		current.Operations = recordOperationAttempt(current.Operations, operationAttemptEvent{Status: StatusRunning, Step: index + 1, WorkerID: e.workerID(), Timestamp: now, Summary: "Live ECS executor started durable request " + request.RequestID})
		current.Lifecycle = transitionLifecycle(current.Lifecycle, len(current.Operations), TransitionRequest{Status: StatusRunning, Step: index + 1}, e.Service.now())
		return current, nil
	})
	if err != nil {
		return err
	}
	// A cancelled context or expired/stolen claim must never start a later write.
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err = e.ecsOwnedChange(ctx, record, func(current Record) (Record, error) { return current, nil }); err != nil {
		return err
	}
	result, err := handler.Execute(ctx, request)
	if err != nil {
		var callErr *ECSCallError
		if errors.As(err, &callErr) && callErr != nil && callErr.Rejected && !callErr.Retryable {
			return e.finishECS(ctx, record, StatusFailed, err.Error())
		}
		return e.recordECSError(ctx, record, index, err)
	}
	return e.completeECSOperation(ctx, record, index, result)
}

func (e Executor) completeECSOperation(ctx context.Context, record Record, index int, result ECSOperationResult) error {
	updated, err := e.ecsOwnedChange(ctx, record, func(current Record) (Record, error) {
		if index < 0 || index >= len(current.Operations) || current.Operations[index].ECSCheckpoint == nil {
			return Record{}, fmt.Errorf("ECS operation checkpoint is missing")
		}
		checkpoint := *current.Operations[index].ECSCheckpoint
		checkpoint.State = "complete"
		checkpoint.Result = result.Clone()
		checkpoint.Error = ""
		checkpoint.RetryAfter = ""
		checkpoint.UpdatedAt = e.Service.now().UTC().Format(time.RFC3339Nano)
		current.Operations[index].ECSCheckpoint = &checkpoint
		current.Operations[index].Status = OperationStatusSucceeded
		attempt := ensureOperationAttempt(current.Operations[index].ExecutionAttempt)
		attempt.Status = OperationStatusSucceeded
		attempt.WorkerID = e.workerID()
		attempt.LastHeartbeatAt = checkpoint.UpdatedAt
		attempt.FinishedAt = checkpoint.UpdatedAt
		attempt.Reason = ""
		attempt.Summary = "Live ECS operation completed with durable request " + checkpoint.RequestID
		current.Operations[index].ExecutionAttempt = attempt
		current.Error = ""
		current.Lifecycle.CompletedSteps = index + 1
		current.Lifecycle.UpdatedAt = checkpoint.UpdatedAt
		current.Transitions = appendTransition(current.Transitions, Transition{TransitionedAt: checkpoint.UpdatedAt, PreviousStatus: StatusRunning, Status: StatusRunning, Step: index + 1, Actor: current.Actor, Details: []string{"ECS operation " + current.Operations[index].ID + " completed; request " + checkpoint.RequestID}, Lifecycle: current.Lifecycle})
		return current, nil
	})
	if err == nil && index == len(updated.Operations)-1 {
		return e.finishECS(ctx, updated, StatusSucceeded, "")
	}
	return err
}

func (e Executor) recordECSError(ctx context.Context, record Record, index int, cause error) error {
	_, err := e.ecsOwnedChange(ctx, record, func(current Record) (Record, error) {
		if index < 0 || index >= len(current.Operations) || current.Operations[index].ECSCheckpoint == nil {
			return Record{}, fmt.Errorf("ECS operation checkpoint is missing")
		}
		checkpoint := *current.Operations[index].ECSCheckpoint
		var callErr *ECSCallError
		if errors.As(cause, &callErr) && callErr != nil && callErr.Rejected && callErr.Retryable {
			checkpoint.State = "retryable_rejection"
		}
		changed := checkpoint.Error != cause.Error()
		checkpoint.Error = cause.Error()
		checkpoint.UpdatedAt = e.Service.now().UTC().Format(time.RFC3339Nano)
		checkpoint.RetryAfter = e.Service.now().Add(30 * time.Second).UTC().Format(time.RFC3339Nano)
		current.Operations[index].ECSCheckpoint = &checkpoint
		detail := "ECS request " + checkpoint.RequestID + " requires reconciliation before another mutation."
		if checkpoint.State == "retryable_rejection" {
			detail = "ECS request " + checkpoint.RequestID + " was explicitly rejected by AWS; a bounded retry may run after backoff."
		}
		if changed {
			current.Transitions = appendTransition(current.Transitions, Transition{TransitionedAt: checkpoint.UpdatedAt, PreviousStatus: StatusRunning, Status: StatusRunning, Step: index + 1, Actor: current.Actor, Error: checkpoint.Error, Details: []string{detail}, Lifecycle: current.Lifecycle})
		}
		current.Error = checkpoint.Error
		current.Details = []string{detail}
		return current, nil
	})
	return err
}

func (e Executor) finishECS(ctx context.Context, record Record, status Status, message string) error {
	_, err := e.ecsOwnedChange(ctx, record, func(current Record) (Record, error) {
		step := current.Lifecycle.CurrentStep
		request := TransitionRequest{Status: status, Step: step, Error: message}
		if status == StatusSucceeded {
			request.Step = len(current.Operations)
		}
		current.Status = status
		if status == StatusFailed && request.Step > 0 && request.Step <= len(current.Operations) && current.Operations[request.Step-1].ECSCheckpoint != nil {
			checkpoint := *current.Operations[request.Step-1].ECSCheckpoint
			checkpoint.State = "failed"
			checkpoint.Error = message
			checkpoint.UpdatedAt = e.Service.now().UTC().Format(time.RFC3339Nano)
			current.Operations[request.Step-1].ECSCheckpoint = &checkpoint
		}
		current.Error = message
		current.Operations = transitionOperations(current.Operations, status, request.Step)
		if status != StatusSucceeded {
			current.Operations = recordOperationAttempt(current.Operations, operationAttemptEvent{Status: status, Step: request.Step, WorkerID: e.workerID(), Timestamp: e.Service.now().UTC().Format(time.RFC3339Nano), Reason: message, Summary: "Live ECS execution finished with status " + string(status)})
		}
		current.Lifecycle = transitionLifecycle(current.Lifecycle, len(current.Operations), request, e.Service.now())
		current.Transitions = appendTransition(current.Transitions, Transition{TransitionedAt: e.Service.now().UTC().Format(time.RFC3339Nano), PreviousStatus: StatusRunning, Status: status, Step: request.Step, Actor: current.Actor, Error: message, Lifecycle: current.Lifecycle})
		return current, nil
	})
	return err
}

func ecsApplyConflicts(candidate, other Record) bool {
	if candidate.ID == other.ID || other.Status != StatusRunning || len(candidate.Operations) == 0 || candidate.Operations[0].ECS == nil {
		return false
	}
	if candidate.DeploymentName == other.DeploymentName {
		return true
	}
	for _, op := range other.Operations {
		if op.ECS != nil && op.ECS.ServiceARN == candidate.Operations[0].ECS.ServiceARN {
			return true
		}
	}
	return false
}

func (e Executor) ecsRefreshHeartbeat(current Record) (Record, error) {
	now := e.Service.now().UTC()
	claim := *current.Claim
	claim.LastHeartbeatAt = now.Format(time.RFC3339Nano)
	claim.ExpiresAt = now.Add(claimFreshnessTTL).Format(time.RFC3339Nano)
	current.Claim = &claim
	step := currentOperationStep(current)
	if step > 0 && current.Operations[step-1].Status == OperationStatusRunning {
		current.Operations = recordOperationAttempt(current.Operations, operationAttemptEvent{Status: StatusRunning, Step: step, WorkerID: e.workerID(), Timestamp: claim.LastHeartbeatAt, HeartbeatOnly: true, Summary: "Live ECS executor refreshed operation ownership."})
	}
	return current, nil
}

func ecsAllComplete(record Record) bool {
	if len(record.Operations) != 3 {
		return false
	}
	for _, op := range record.Operations {
		if op.ECSCheckpoint == nil || op.ECSCheckpoint.State != "complete" {
			return false
		}
	}
	return true
}
