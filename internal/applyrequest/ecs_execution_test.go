package applyrequest

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

type fakeECSHandler struct {
	before     func(ECSOperationRequest) error
	calls      atomic.Int32
	reconciles atomic.Int32
	preflight  func(ECSOperationRequest) error
	execute    func(ECSOperationRequest) (ECSOperationResult, error)
	recover    func(ECSOperationRequest) (ECSReconciliation, error)
}

func (f *fakeECSHandler) Preflight(_ context.Context, r ECSOperationRequest) error {
	if f.preflight != nil {
		return f.preflight(r)
	}
	return nil
}
func (f *fakeECSHandler) Execute(ctx context.Context, r ECSOperationRequest) (ECSOperationResult, error) {
	if f.before != nil {
		if err := f.before(r); err != nil {
			return ECSOperationResult{}, err
		}
	}
	if len(r.PriorResults) < 2 {
		if err := r.BeforeMutation(ctx); err != nil {
			return ECSOperationResult{}, err
		}
	}
	f.calls.Add(1)
	if f.execute != nil {
		return f.execute(r)
	}
	return ECSOperationResult{TaskDefinitionARN: r.Intent.BaseTaskDefinitionARN, AWSRequestID: "aws-request"}, nil
}
func (f *fakeECSHandler) Reconcile(_ context.Context, r ECSOperationRequest) (ECSReconciliation, error) {
	f.reconciles.Add(1)
	if f.recover != nil {
		return f.recover(r)
	}
	return ECSReconciliation{Complete: true, Result: ECSOperationResult{TaskDefinitionARN: r.Intent.BaseTaskDefinitionARN, AWSRequestID: "recovered-request"}}, nil
}

type ecsFixture struct {
	executor    Executor
	record      Record
	handler     *fakeECSHandler
	now         time.Time
	intent      *domain.ECSDeploymentIntent
	fingerprint string
}

func newECSFixture(t *testing.T) *ecsFixture {
	t.Helper()
	raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-repo"))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := catalog.BuildDeploymentOverviewDetail(compiled, nil, "worker")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	plans := planpreview.Service{Store: planpreview.NewFileStore(filepath.Join(dir, "plans.json")), LoadDetail: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) { return detail, nil }}
	plan, err := plans.Create(context.Background(), "worker", planpreview.CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	f := &ecsFixture{now: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), intent: detail.ECS.Clone(), fingerprint: plan.Record.Source.Fingerprint, handler: &fakeECSHandler{}}
	service := Service{Store: NewFileStore(filepath.Join(dir, "applies.json")), EnableAWSApply: true, LoadPlan: plans.Get, Now: func() time.Time { return f.now }, CurrentFingerprint: func(context.Context, string) (string, error) { return f.fingerprint, nil }, DeployerRoleReadiness: func(context.Context, DeployerRoleTarget) (DeployerRoleReadiness, error) {
		return DeployerRoleReadiness{Configured: true, Ready: true, CheckedWithAWS: true, AWSAccountID: f.intent.AWSAccountID, Region: f.intent.Region, RoleARN: "arn:aws:iam::" + f.intent.AWSAccountID + ":role/tenet-deployer"}, nil
	}}
	apply, err := service.Create(context.Background(), plan.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.record = apply.Record
	f.executor = Executor{Service: service, WorkerID: "test-worker", ECS: &ECSExecution{Enabled: true, Authorize: func(context.Context, Record) error { return nil }, CurrentIntent: func(context.Context, string) (*domain.ECSDeploymentIntent, error) { return f.intent.Clone(), nil }, Handlers: map[string]ECSOperationHandler{}}}
	for _, op := range apply.Record.Operations {
		f.executor.ECS.Handlers[op.Kind] = f.handler
	}
	return f
}
func (f *ecsFixture) stored(t *testing.T) Record {
	t.Helper()
	r, err := f.executor.Service.Store.Get(context.Background(), f.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestECSExecutionRunsReviewedOperationsAndPersistsAudit(t *testing.T) {
	f := newECSFixture(t)
	var requests []ECSOperationRequest
	f.handler.execute = func(request ECSOperationRequest) (ECSOperationResult, error) {
		r := f.stored(t)
		index := len(request.PriorResults)
		cp := r.Operations[index].ECSCheckpoint
		if cp == nil || cp.State != "in_flight" || cp.RequestID != request.RequestID {
			t.Fatal("AWS call preceded durable intent")
		}
		if request.Role.RoleARN == "" || request.Intent != *f.intent {
			t.Fatal("lost reviewed context")
		}
		requests = append(requests, request)
		return ECSOperationResult{TaskDefinitionARN: request.Intent.BaseTaskDefinitionARN, AWSRequestID: "aws-request"}, nil
	}
	for i := 0; i < 4; i++ {
		if err := f.executor.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		// Observation refreshes from our own operations must not stale desired intent.
		f.fingerprint = "runtime-observation-changed"
	}
	r := f.stored(t)
	if r.Status != StatusSucceeded || r.Lifecycle.CompletedSteps != 3 || len(requests) != 3 || len(r.Transitions) != 5 {
		t.Fatalf("incomplete execution/audit: %+v", r)
	}
	for i, op := range r.Operations {
		if op.ECSCheckpoint == nil || op.ECSCheckpoint.State != "complete" || op.ECSCheckpoint.Attempts != 1 || op.Status != OperationStatusSucceeded || len(requests[i].PriorResults) != i {
			t.Fatalf("bad operation: %+v", op)
		}
	}
}

func TestECSExecutionRejectsUnsafeInputsBeforeMutation(t *testing.T) {
	denied := errors.New("denied")
	for _, tc := range []struct {
		name   string
		change func(*ecsFixture)
	}{
		{"missing authorizer", func(f *ecsFixture) { f.executor.ECS.Authorize = nil }},
		{"operator or scope denied", func(f *ecsFixture) { f.executor.ECS.Authorize = func(context.Context, Record) error { return denied } }},
		{"missing current intent", func(f *ecsFixture) { f.executor.ECS.CurrentIntent = nil }},
		{"stale fingerprint", func(f *ecsFixture) { f.fingerprint = "stale" }},
		{"stale desired intent", func(f *ecsFixture) { f.intent.Image = "registry.example.com/worker:changed" }},
		{"missing plan provider", func(f *ecsFixture) { f.executor.Service.LoadPlan = nil }},
		{"missing handler", func(f *ecsFixture) { delete(f.executor.ECS.Handlers, f.record.Operations[2].Kind) }},
		{"denied concrete action or PassRole", func(f *ecsFixture) { f.handler.preflight = func(ECSOperationRequest) error { return denied } }},
		{"role not ready", func(f *ecsFixture) {
			f.executor.Service.DeployerRoleReadiness = func(context.Context, DeployerRoleTarget) (DeployerRoleReadiness, error) {
				return DeployerRoleReadiness{}, nil
			}
		}},
		{"wrong role account", func(f *ecsFixture) {
			provider := f.executor.Service.DeployerRoleReadiness
			f.executor.Service.DeployerRoleReadiness = func(ctx context.Context, target DeployerRoleTarget) (DeployerRoleReadiness, error) {
				r, e := provider(ctx, target)
				r.RoleARN = "arn:aws:iam::999999999999:role/wrong"
				return r, e
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newECSFixture(t)
			tc.change(f)
			if err := f.executor.RunOnce(context.Background()); err == nil {
				t.Fatal("expected fail-closed rejection")
			}
			if f.handler.calls.Load() != 0 || f.stored(t).Claim != nil {
				t.Fatal("unsafe request executed or claimed")
			}
		})
	}
	for _, enabled := range []bool{false, true} {
		f := newECSFixture(t)
		f.executor.ECS.Enabled = enabled
		f.executor.Service.EnableAWSApply = false
		_ = f.executor.RunOnce(context.Background())
		if f.handler.calls.Load() != 0 {
			t.Fatal("disabled execution wrote to AWS")
		}
	}
}

type interruptedCheckpointStore struct {
	FileStore
	fail bool
}

func (s *interruptedCheckpointStore) Mutate(ctx context.Context, id string, change func(Record) (Record, error)) (Record, error) {
	return s.FileStore.Mutate(ctx, id, func(r Record) (Record, error) {
		updated, err := change(r)
		if err != nil {
			return Record{}, err
		}
		if s.fail {
			for _, op := range updated.Operations {
				if op.ECSCheckpoint != nil && op.ECSCheckpoint.State == "complete" {
					return Record{}, errors.New("simulated crash before checkpoint commit")
				}
			}
		}
		return updated, nil
	})
}

func TestECSExecutionRecoversCallBeforeCheckpointWithoutReplay(t *testing.T) {
	f := newECSFixture(t)
	store := &interruptedCheckpointStore{FileStore: f.executor.Service.Store.(FileStore), fail: true}
	f.executor.Service.Store = store
	if err := f.executor.RunOnce(context.Background()); err == nil {
		t.Fatal("expected simulated interrupted commit")
	}
	before := f.stored(t)
	if before.Operations[0].ECSCheckpoint.State != "in_flight" || f.handler.calls.Load() != 1 {
		t.Fatal("missing ambiguous call evidence")
	}
	requestID := before.Operations[0].ECSCheckpoint.RequestID
	store.fail = false
	f.now = f.now.Add(6 * time.Minute)
	// Reusing a worker name after restart must still recover an expired lease.
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := f.stored(t)
	if f.handler.calls.Load() != 1 || f.handler.reconciles.Load() != 1 || after.Operations[0].ECSCheckpoint.State != "complete" || after.Operations[0].ECSCheckpoint.RequestID != requestID {
		t.Fatalf("recovery replayed or lost identity: %+v", after)
	}
}

func TestECSExecutionRetriesOnlyAfterConfirmedAbsence(t *testing.T) {
	f := newECSFixture(t)
	f.handler.execute = func(ECSOperationRequest) (ECSOperationResult, error) {
		return ECSOperationResult{}, errors.New("connection lost after send")
	}
	f.handler.recover = func(ECSOperationRequest) (ECSReconciliation, error) { return ECSReconciliation{}, nil }
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		f.now = f.now.Add(31 * time.Second)
		if err := f.executor.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if f.handler.calls.Load() != 1 || f.handler.reconciles.Load() != 3 || f.stored(t).Status != StatusRunning {
		t.Fatal("uncertain outcome was replayed or ownership released")
	}
	f.handler.recover = func(ECSOperationRequest) (ECSReconciliation, error) { return ECSReconciliation{Absent: true}, nil }
	for i := 0; i < 3; i++ {
		f.now = f.now.Add(31 * time.Second)
		if err := f.executor.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if f.handler.calls.Load() != 3 || f.stored(t).Status != StatusFailed {
		t.Fatal("confirmed absent retries were not bounded")
	}
}

func TestECSExecutionStopsOnPermanentRejectionAndLeaseLoss(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(map[bool]string{false: "permanent rejection", true: "lease loss"}[lost], func(t *testing.T) {
			f := newECSFixture(t)
			f.handler.execute = func(ECSOperationRequest) (ECSOperationResult, error) {
				if lost {
					_, err := f.executor.Service.Store.(MutationStore).Mutate(context.Background(), f.record.ID, func(r Record) (Record, error) { r.Claim.WorkerID = "replacement-worker"; return r, nil })
					if err != nil {
						t.Fatal(err)
					}
					return ECSOperationResult{}, nil
				}
				return ECSOperationResult{}, &ECSCallError{Err: errors.New("AccessDenied"), Rejected: true}
			}
			err := f.executor.RunOnce(context.Background())
			if lost && !errors.Is(err, ErrAlreadyClaimed) {
				t.Fatalf("expected ownership failure: %v", err)
			}
			if !lost && (err != nil || f.stored(t).Status != StatusFailed) {
				t.Fatalf("permanent error was retried: %v", err)
			}
			if lost && f.stored(t).Operations[0].ECSCheckpoint.State != "in_flight" {
				t.Fatal("lost owner persisted a completion")
			}
			if f.handler.calls.Load() != 1 {
				t.Fatal("later operation executed")
			}
		})
	}
}

func TestECSExecutionLocksServiceAliasesAndProtectsCheckpointsFromManualTransitions(t *testing.T) {
	f := newECSFixture(t)
	store := f.executor.Service.Store.(FileStore)
	unlock, err := store.LockExecution(context.Background(), "other-deployment", f.intent.ServiceARN)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.handler.calls.Load() != 0 {
		t.Fatal("overlapped alias service execution")
	}
	unlock()
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.executor.Service.Transition(context.Background(), f.record.ID, TransitionRequest{Status: StatusSucceeded}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("manual transition overwrote executor checkpoint: %v", err)
	}
	local := f.executor
	local.ECS = nil
	if err := local.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.stored(t).Status != StatusRunning {
		t.Fatal("disabled executor discarded active AWS execution")
	}
}

func TestECSExecutionHeartbeatPreservesCheckpointAndRejectsStolenLease(t *testing.T) {
	f := newECSFixture(t)
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	owner := f.stored(t)
	f.now = f.now.Add(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.executor.ecsHeartbeat(ctx, owner, time.Millisecond) }()
	deadline := time.After(time.Second)
	for {
		current := f.stored(t)
		if current.Claim.LastHeartbeatAt != owner.Claim.LastHeartbeatAt {
			break
		}
		select {
		case <-deadline:
			cancel()
			<-done
			t.Fatal("heartbeat did not refresh lease")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	current := f.stored(t)
	if current.Operations[0].ECSCheckpoint.State != "complete" {
		t.Fatal("heartbeat discarded durable execution result")
	}
	_, err := f.executor.Service.Store.(MutationStore).Mutate(context.Background(), owner.ID, func(r Record) (Record, error) { r.Claim.WorkerID = "stolen"; return r, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := f.executor.ecsHeartbeat(context.Background(), owner, time.Millisecond); !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("heartbeat extended stolen lease: %v", err)
	}
}

func TestECSExecutionRejectsUnsupportedOperationsAndSerializesDifferentApplies(t *testing.T) {
	for _, change := range []func(*Record){
		func(r *Record) { r.Operations[0].ECS.DeploymentStrategy = "CANARY" },
		func(r *Record) { r.Operations[0].Kind = "CreateCodeDeployDeployment" },
		func(r *Record) { r.Operations[0].Target = "another-service" },
	} {
		f := newECSFixture(t)
		changed := f.stored(t)
		change(&changed)
		if err := f.executor.Service.Store.Update(context.Background(), changed); err != nil {
			t.Fatal(err)
		}
		if err := f.executor.RunOnce(context.Background()); err == nil || f.handler.calls.Load() != 0 {
			t.Fatal("unsupported/altered apply reached AWS")
		}
	}
	f := newECSFixture(t)
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	other := f.record
	other.ID = "competing-apply"
	other.DeploymentName = "alias-deployment"
	if err := f.executor.Service.Store.Save(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	// An expired claim does not prove an ambiguous previous AWS write is absent.
	f.now = f.now.Add(6 * time.Minute)
	if _, err := f.executor.Service.Claim(context.Background(), other.ID, ClaimRequest{WorkerID: "competitor"}); !errors.Is(err, ErrExecutionBusy) {
		t.Fatalf("conflicting apply acquired ownership: %v", err)
	}
	before := f.handler.calls.Load()
	if err := f.executor.advanceECS(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if f.handler.calls.Load() != before {
		t.Fatal("conflicting apply issued another mutation")
	}
}

func TestECSExecutionRetriesConfirmedTransientRejectionWithoutGuessingAbsence(t *testing.T) {
	f := newECSFixture(t)
	f.handler.execute = func(ECSOperationRequest) (ECSOperationResult, error) {
		return ECSOperationResult{}, &ECSCallError{Err: errors.New("ThrottlingException"), Rejected: true, Retryable: true}
	}
	for i := 0; i < 4; i++ {
		if err := f.executor.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		f.now = f.now.Add(31 * time.Second)
	}
	if f.handler.calls.Load() != 3 || f.handler.reconciles.Load() != 0 || f.stored(t).Status != StatusFailed {
		t.Fatal("confirmed rejected calls should retry at most three times")
	}
}

func TestECSExecutionRejectsMissingCheckpointAndEmptyHandlerErrors(t *testing.T) {
	if (&ECSCallError{Rejected: true}).Error() == "" {
		t.Fatal("empty handler error lost diagnostic")
	}
	f := newECSFixture(t)
	owner, err := f.executor.Service.Claim(context.Background(), f.record.ID, ClaimRequest{WorkerID: f.executor.WorkerID})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.executor.completeECSOperation(context.Background(), owner, 0, ECSOperationResult{}); err == nil {
		t.Fatal("completion accepted a missing checkpoint")
	}
	if err := f.executor.recordECSError(context.Background(), owner, 0, errors.New("failed")); err == nil {
		t.Fatal("error update accepted a missing checkpoint")
	}
	f.executor.Service.Store = &memoryStore{}
	if _, err := f.executor.ecsOwnedChange(context.Background(), owner, func(r Record) (Record, error) { return r, nil }); err == nil {
		t.Fatal("non-atomic store accepted execution update")
	}
}

type preparingECSHandler struct {
	*fakeECSHandler
	snapshot *ECSServiceSnapshot
}

func (h preparingECSHandler) Prepare(context.Context, ECSOperationRequest) (ECSOperationResult, error) {
	return ECSOperationResult{PreviousService: h.snapshot, PreviousTaskDefinitionARN: h.snapshot.TaskDefinitionARN}, nil
}

func TestECSExecutionPersistsPreparedRollbackStateBeforeServiceCall(t *testing.T) {
	f := newECSFixture(t)
	snapshot := &ECSServiceSnapshot{TaskDefinitionARN: f.intent.BaseTaskDefinitionARN, DesiredCount: 1, MinimumHealthyPercent: 50, MaximumPercent: 150, PrimaryDeploymentID: "ecs-svc/before"}
	update := &fakeECSHandler{}
	update.execute = func(request ECSOperationRequest) (ECSOperationResult, error) {
		record := f.stored(t)
		persisted := record.Operations[1].ECSCheckpoint.Result.PreviousService
		if persisted == nil || *persisted != *snapshot || request.PreparedResult.PreviousService == nil || *request.PreparedResult.PreviousService != *snapshot {
			t.Fatal("AWS update preceded durable rollback evidence")
		}
		result := request.PreparedResult.Clone()
		result.ServiceDeploymentID = "ecs-svc/after"
		return result, nil
	}
	f.executor.ECS.Handlers[f.record.Operations[1].Kind] = preparingECSHandler{fakeECSHandler: update, snapshot: snapshot}
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	record := f.stored(t)
	if record.Status != StatusRunning || record.Operations[2].Status != OperationStatusQueued || record.Operations[1].ECSCheckpoint.Result.ServiceDeploymentID != "ecs-svc/after" {
		t.Fatal("update acceptance incorrectly finished deployment")
	}
	copied := NormalizeOperationIDs(record)
	copied.Operations[1].ECSCheckpoint.Result.PreviousService.DesiredCount = 99
	if record.Operations[1].ECSCheckpoint.Result.PreviousService.DesiredCount != 1 {
		t.Fatal("rollback snapshot aliases read-model copies")
	}
}

func TestECSExecutionFinishesKnownReconciledFailureWithoutMutation(t *testing.T) {
	f := newECSFixture(t)
	f.handler.execute = func(ECSOperationRequest) (ECSOperationResult, error) {
		return ECSOperationResult{}, errors.New("unknown response")
	}
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.handler.recover = func(ECSOperationRequest) (ECSReconciliation, error) {
		return ECSReconciliation{Failure: "deployment failed after acceptance"}, nil
	}
	f.now = f.now.Add(31 * time.Second)
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	record := f.stored(t)
	if record.Status != StatusFailed || record.Operations[0].ECSCheckpoint.State != "failed" || f.handler.calls.Load() != 1 {
		t.Fatal("known terminal failure remained running or repeated the write")
	}
}

func TestECSExecutionPersistsObservationBackoffAndResumesWithoutUpdates(t *testing.T) {
	f := newECSFixture(t)
	observer := &fakeECSHandler{}
	progress := &ECSRolloutObservation{Source: "aws-ecs", State: "progressing", Reason: "missing_tasks", Summary: "1/2 intended tasks running", StartedAt: f.now.Format(time.RFC3339Nano), DeadlineAt: f.now.Add(15 * time.Minute).Format(time.RFC3339Nano), ObservedAt: f.now.Format(time.RFC3339Nano), TaskDefinitionARN: f.intent.BaseTaskDefinitionARN, DesiredCount: 2, RunningCount: 1, HealthyCount: 1, TasksObserved: true}
	observer.execute = func(ECSOperationRequest) (ECSOperationResult, error) {
		return ECSOperationResult{}, &ECSObservationPending{Result: ECSOperationResult{Observation: progress}, RetryAfter: 5 * time.Second}
	}
	f.executor.ECS.Handlers[f.record.Operations[2].Kind] = observer
	for i := 0; i < 3; i++ {
		if err := f.executor.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	pending := f.stored(t)
	if pending.Status != StatusRunning || pending.Error != "" || pending.ECSRuntime == nil || pending.ECSRuntime.RunningCount != 1 || pending.Operations[2].ECSCheckpoint.State != "in_flight" {
		t.Fatalf("normal progress became an error or was lost: %+v", pending)
	}
	f.executor.ECS.Authorize = func(context.Context, Record) error {
		t.Error("backoff still contacted authorization/AWS providers")
		return errors.New("should not be called")
	}
	f.now = f.now.Add(time.Second)
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.executor.ECS.Authorize = func(context.Context, Record) error { return nil }
	observer.recover = func(request ECSOperationRequest) (ECSReconciliation, error) {
		result := request.PreparedResult.Clone()
		result.Observation.State = "converged"
		result.Observation.Reason = "intended_revision_healthy"
		result.Observation.RunningCount = 2
		result.Observation.HealthyCount = 2
		result.Observation.Summary = "2/2 intended tasks verified"
		return ECSReconciliation{Complete: true, Result: result}, nil
	}
	f.now = f.now.Add(6 * time.Minute)
	f.executor.WorkerID = "restarted-observer"
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	completed := f.stored(t)
	if completed.Status != StatusSucceeded || completed.ECSRuntime.State != "converged" || observer.calls.Load() != 1 || observer.reconciles.Load() != 1 || f.handler.calls.Load() != 2 {
		t.Fatalf("observer restart replayed mutation or lost verified runtime: %+v", completed)
	}
	if completed.ExecutionReadiness == nil {
		completed = NormalizeOperationIDs(completed)
	}
	if NormalizeOperationIDs(completed).ExecutionReadiness.State != "completed" {
		t.Fatal("verified live execution still reported local-safe readiness")
	}
	copy := NormalizeOperationIDs(completed)
	copy.ECSRuntime.HealthyCount = 99
	copy.Operations[2].ECSCheckpoint.Result.Observation.HealthyCount = 98
	if completed.ECSRuntime.HealthyCount != 2 || completed.Operations[2].ECSCheckpoint.Result.Observation.HealthyCount != 2 {
		t.Fatal("runtime evidence aliases read-model copies")
	}
}

func TestECSExecutionPersistsTerminalObservationEvidenceAtomically(t *testing.T) {
	f := newECSFixture(t)
	observer := &fakeECSHandler{}
	observer.execute = func(ECSOperationRequest) (ECSOperationResult, error) {
		return ECSOperationResult{}, &ECSObservationFailure{Result: ECSOperationResult{Observation: &ECSRolloutObservation{Source: "aws-ecs", State: "failed", Reason: "observation_timeout", Summary: "Rollout observation timed out", ObservedAt: f.now.Format(time.RFC3339Nano)}}}
	}
	f.executor.ECS.Handlers[f.record.Operations[2].Kind] = observer
	for i := 0; i < 3; i++ {
		if err := f.executor.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	record := f.stored(t)
	if record.Status != StatusFailed || record.ECSRuntime == nil || record.ECSRuntime.Reason != "observation_timeout" || record.Operations[2].ECSCheckpoint.State != "failed" || record.Error != "Rollout observation timed out" {
		t.Fatal("terminal outcome lost its actual observation evidence")
	}
	last := record.Transitions[len(record.Transitions)-1]
	if last.Status != StatusFailed || len(last.Details) == 0 || last.Details[0] != "Runtime reason: observation_timeout" {
		t.Fatal("terminal runtime reason missing from audit evidence")
	}
}

func TestECSMutationGuardStopsLeaseLossAndChangedIntentDuringDiscovery(t *testing.T) {
	for _, lost := range []bool{true, false} {
		t.Run(map[bool]string{true: "lease lost", false: "source changed"}[lost], func(t *testing.T) {
			f := newECSFixture(t)
			f.handler.before = func(ECSOperationRequest) error {
				if lost {
					_, err := f.executor.Service.Store.(MutationStore).Mutate(context.Background(), f.record.ID, func(r Record) (Record, error) { r.Claim.WorkerID = "replacement"; return r, nil })
					return err
				}
				f.intent.Image = "registry.example.com/worker:changed"
				return nil
			}
			_ = f.executor.RunOnce(context.Background())
			if f.handler.calls.Load() != 0 || f.stored(t).Operations[0].ECSCheckpoint.MutationAuthorizedAt != "" {
				t.Fatal("write escaped the final mutation guard")
			}
		})
	}
}

func TestECSGuardedUnsentCheckpointCanResumeWithoutAssumingAWSAbsence(t *testing.T) {
	f := newECSFixture(t)
	stopped := errors.New("process stopped during discovery")
	f.handler.before = func(ECSOperationRequest) error { return stopped }
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	cp := f.stored(t).Operations[0].ECSCheckpoint
	if !cp.MutationGuarded || cp.MutationAuthorizedAt != "" || f.handler.calls.Load() != 0 {
		t.Fatal("invalid no-send checkpoint")
	}
	f.now = f.now.Add(31 * time.Second)
	f.handler.before = nil
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	cp = f.stored(t).Operations[0].ECSCheckpoint
	if cp.State != "complete" || cp.MutationAuthorizedAt == "" || cp.Attempts != 1 || f.handler.calls.Load() != 1 || f.handler.reconciles.Load() != 0 {
		t.Fatalf("no-send recovery lost write evidence: %+v", cp)
	}
}

func TestECSObservationCanFinishApprovedIntentAfterGitChanges(t *testing.T) {
	f := newECSFixture(t)
	for i := 0; i < 2; i++ {
		if err := f.executor.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	f.intent.Image = "registry.example.com/worker:next"
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.stored(t).Status != StatusSucceeded {
		t.Fatal("new Git intent blocked read-only convergence of the approved rollout")
	}
}

func TestECSApprovalPrincipalCannotBeChangedWithoutChangingApplyIdentity(t *testing.T) {
	f := newECSFixture(t)
	_, err := f.executor.Service.Store.(MutationStore).Mutate(context.Background(), f.record.ID, func(record Record) (Record, error) {
		record.Principal = &ExecutionPrincipal{Identity: requestactor.AuthenticatedIdentity{Authenticated: true, ID: "another-user"}}
		return record, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.executor.RunOnce(context.Background()); err == nil || f.handler.calls.Load() != 0 {
		t.Fatalf("altered approval executed: %v", err)
	}
}

func TestDisablingNativeExecutionPreservesApprovedQueuedRequests(t *testing.T) {
	f := newECSFixture(t)
	_, err := f.executor.Service.Store.(MutationStore).Mutate(context.Background(), f.record.ID, func(record Record) (Record, error) {
		record.Principal = &ExecutionPrincipal{Identity: requestactor.AuthenticatedIdentity{Authenticated: true, ID: "operator"}}
		return record, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.executor.ECS = nil
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.stored(t).Status != StatusQueued || f.stored(t).Claim != nil {
		t.Fatal("local-safe fallback changed a queued live approval")
	}
}
