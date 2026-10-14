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
)

type fakeECSHandler struct {
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
func (f *fakeECSHandler) Execute(_ context.Context, r ECSOperationRequest) (ECSOperationResult, error) {
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
