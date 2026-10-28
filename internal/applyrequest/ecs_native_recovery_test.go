package applyrequest

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

func nativeRecoveryFixture(t *testing.T) (*ecsFixture, context.Context) {
	t.Helper()
	f := newECSFixture(t)
	raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := catalog.BuildDeploymentOverviewDetail(compiled, nil, "web-canary")
	if err != nil {
		t.Fatal(err)
	}
	plans := planpreview.Service{Store: planpreview.NewFileStore(filepath.Join(t.TempDir(), "native-plans.json")), LoadDetail: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) { return detail, nil }}
	plan, err := plans.Create(context.Background(), detail.Name, planpreview.CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	f.intent = detail.ECS.Clone()
	f.fingerprint = plan.Record.Source.Fingerprint
	f.executor.Service.Store = NewFileStore(filepath.Join(t.TempDir(), "native-applies.json"))
	f.executor.Service.LoadPlan = plans.Get
	f.executor.Service.AuthorizeCreate = func(ctx context.Context, _ Record) (*ExecutionPrincipal, error) {
		id := requestactor.AuthenticatedIdentityFromContext(ctx)
		if !id.Authenticated {
			return nil, ErrExecutionUnauthorized
		}
		return &ExecutionPrincipal{Identity: id}, nil
	}
	f.executor.Service.PrepareNativeRollback = func(context.Context, Record, ECSNativeRollback) error { return nil }
	ctx := requestactor.WithAuthenticatedIdentityContext(context.Background(), requestactor.AuthenticatedIdentity{ID: "operator", Authenticated: true})
	created, err := f.executor.Service.Create(ctx, plan.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.record = created.Record
	f.record.Status = StatusRunning
	prefix := "arn:aws:ecs:ap-northeast-2:123456789012:"
	// Use the manifest's exact service path rather than a caller-selected target.
	service := f.intent.ServiceARN
	servicePath := service[len(prefix+"service/"):]
	base := f.intent.BaseTaskDefinitionARN
	target := base[:len(base)-2] + "18"
	result := ECSOperationResult{TaskDefinitionARN: target, TaskDefinitionIntent: "content", PreviousTaskDefinitionARN: base, ServiceDeploymentARN: prefix + "service-deployment/" + servicePath + "/owned", ServiceRevisionARN: prefix + "service-revision/" + servicePath + "/new", PreviousServiceRevisionARN: prefix + "service-revision/" + servicePath + "/old", PreviousService: &ECSServiceSnapshot{TaskDefinitionARN: base, DesiredCount: int32(f.intent.DesiredCount)}, NativeService: &ECSNativeServiceSnapshot{SourceRevisions: []ECSNativeSourceRevision{{ARN: prefix + "service-revision/" + servicePath + "/old", TaskDefinitionARN: base}}}}
	for i := 0; i < 2; i++ {
		f.record.Operations[i].ECSCheckpoint = &ECSCheckpoint{State: "complete", Attempts: 1, MutationGuarded: true, MutationAuthorizedAt: "2026-10-28T00:00:00Z", Result: result, RequestID: ecsRequest(f.record, i, DeployerRoleReadiness{}).RequestID}
		f.record.Operations[i].Status = OperationStatusSucceeded
	}
	if _, err := f.executor.Service.Store.(MutationStore).Mutate(ctx, f.record.ID, func(Record) (Record, error) { return f.record, nil }); err != nil {
		t.Fatal(err)
	}
	f.executor.ECS.AllowAdvanced = true
	f.executor.ECS.Handlers[NativeRollbackOperationKind] = f.handler
	for _, op := range f.record.Operations {
		f.executor.ECS.Handlers[op.Kind] = f.handler
	}
	return f, ctx
}
func TestNativeRecoveryHandoffIsDurableIdempotentAndKeepsForwardEvidence(t *testing.T) {
	f, ctx := nativeRecoveryFixture(t)
	before := f.stored(t)
	child, err := f.executor.Service.RequestRollback(ctx, f.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	parent := f.stored(t)
	if !child.Created || len(child.Record.Operations) != 2 || child.Record.NativeRollback == nil || parent.Status != StatusCancelled || parent.NativeRollbackRequestID != child.Record.ID || !reflect.DeepEqual(parent.Operations, before.Operations) {
		t.Fatalf("handoff lost evidence or ownership: %+v %+v", parent, child)
	}
	again, err := f.executor.Service.RequestRollback(ctx, f.record.ID)
	if err != nil || again.Created || again.Record.ID != child.Record.ID {
		t.Fatal("duplicate stop request", again, err)
	}
	for i := 0; i < 3; i++ {
		if err := f.executor.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	done, err := f.executor.Service.Get(ctx, child.Record.ID)
	if err != nil || done.Status != StatusSucceeded || f.handler.calls.Load() != 2 {
		t.Fatal("native child did not finish two operations", done, err)
	}
	if f.stored(t).Status != StatusCancelled {
		t.Fatal("source recovery rewrote forward outcome")
	}
	// Repeated worker scans must leave the terminal child alone.
	if err := f.executor.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
}

type interruptedNativeStore struct {
	ECSExecutionStore
	afterSave bool
}

func (s *interruptedNativeStore) Save(ctx context.Context, r Record) error {
	if err := s.ECSExecutionStore.Save(ctx, r); err != nil {
		return err
	}
	if r.NativeRollback != nil && s.afterSave {
		s.afterSave = false
		return errors.New("process interrupted after child persistence")
	}
	return nil
}
func TestNativeRecoveryRepairsInterruptionBeforeParentLink(t *testing.T) {
	f, ctx := nativeRecoveryFixture(t)
	base := f.executor.Service.Store.(ECSExecutionStore)
	f.executor.Service.Store = &interruptedNativeStore{ECSExecutionStore: base, afterSave: true}
	if _, err := f.executor.Service.RequestRollback(ctx, f.record.ID); err == nil {
		t.Fatal("fixture did not interrupt")
	}
	if f.stored(t).Status != StatusRunning {
		t.Fatal("parent unexpectedly linked")
	}
	records, err := base.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var child Record
	for _, r := range records {
		if r.NativeRollback != nil {
			child = r
		}
	}
	if child.ID == "" {
		t.Fatal("recovery intent was not durable")
	}
	// The old owner's next scan must not resume the forward observer.
	if err := f.executor.advanceECS(ctx, f.record); err != nil {
		t.Fatal(err)
	}
	if f.handler.calls.Load() != 0 {
		t.Fatal("forward work resumed across saved recovery intent")
	}
	f.executor.Service.Store = base
	if err := f.executor.advanceECS(ctx, child); err != nil {
		t.Fatal(err)
	}
	if f.stored(t).NativeRollbackRequestID != child.ID {
		t.Fatal("restarted executor did not repair handoff")
	}
}
func TestNativeRecoveryChecksApprovalAndPayloadAtEveryBoundary(t *testing.T) {
	for _, name := range []string{"anonymous", "prepare denied", "advanced off", "RBAC revoked", "payload changed", "intent moved", "duplicate principal"} {
		t.Run(name, func(t *testing.T) {
			f, ctx := nativeRecoveryFixture(t)
			if name == "anonymous" {
				if _, err := f.executor.Service.RequestRollback(context.Background(), f.record.ID); !errors.Is(err, ErrExecutionUnauthorized) {
					t.Fatal(err)
				}
				return
			}
			if name == "prepare denied" {
				f.executor.Service.PrepareNativeRollback = func(context.Context, Record, ECSNativeRollback) error { return errors.New("AWS readiness denied") }
				if _, err := f.executor.Service.RequestRollback(ctx, f.record.ID); err == nil {
					t.Fatal("unready stop queued")
				}
				return
			}
			child, err := f.executor.Service.RequestRollback(ctx, f.record.ID)
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "advanced off":
				f.executor.ECS.AllowAdvanced = false
			case "RBAC revoked":
				f.executor.ECS.Authorize = func(context.Context, Record) error { return ErrExecutionUnauthorized }
			case "payload changed":
				child.Record.NativeRollback.SourceTaskDefinitionARN = child.Record.NativeRollback.ForwardTaskDefinitionARN
				if _, err := f.executor.Service.Store.(MutationStore).Mutate(ctx, child.Record.ID, func(Record) (Record, error) { return child.Record, nil }); err != nil {
					t.Fatal(err)
				}
			case "intent moved":
				f.intent.Image = "registry.example.com/new:sha"
			case "duplicate principal":
				f.executor.Service.AuthorizeCreate = func(context.Context, Record) (*ExecutionPrincipal, error) { return nil, ErrExecutionUnauthorized }
				if _, err := f.executor.Service.RequestRollback(ctx, f.record.ID); !errors.Is(err, ErrExecutionUnauthorized) {
					t.Fatal(err)
				}
				return
			}
			err = f.executor.advanceECS(ctx, child.Record)
			if name == "intent moved" {
				if err != nil {
					t.Fatal("fixed-ARN rollback was incorrectly tied to new Git intent", err)
				}
				return
			}
			if err == nil || f.handler.calls.Load() != 0 {
				t.Fatal("invalid approval wrote AWS", name, err)
			}
		})
	}
}
func TestNativeRecoveryQueuedReservationBlocksOtherServiceOwners(t *testing.T) {
	f, ctx := nativeRecoveryFixture(t)
	child, err := f.executor.Service.RequestRollback(ctx, f.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := f.record
	candidate.ID = "another-approved-apply"
	candidate.Status = StatusQueued
	if !ecsApplyConflicts(candidate, child.Record) || ecsApplyConflicts(child.Record, child.Record) {
		t.Fatal("queued recovery does not reserve its physical service")
	}
	clone := NormalizeOperationIDs(child.Record)
	clone.NativeRollback.SourceRevisionARN = "changed"
	if child.Record.NativeRollback.SourceRevisionARN == "changed" {
		t.Fatal("read projection shares native approval memory")
	}
}

func TestNativeRecoveryPreservesFailedParentAndRejectsManualTransitions(t *testing.T) {
	f, ctx := nativeRecoveryFixture(t)
	if _, err := f.executor.Service.Store.(MutationStore).Mutate(ctx, f.record.ID, func(r Record) (Record, error) {
		r.Status = StatusFailed
		r.Error = "forward alarm triggered"
		return r, nil
	}); err != nil {
		t.Fatal(err)
	}
	before := f.stored(t)
	child, err := f.executor.Service.RequestRollback(ctx, f.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	after := f.stored(t)
	if after.Status != StatusFailed || after.Error != before.Error || !reflect.DeepEqual(after.Operations, before.Operations) {
		t.Fatal("recovery changed original failure evidence")
	}
	if _, err := f.executor.Service.Transition(ctx, child.Record.ID, TransitionRequest{Status: StatusCancelled}); !errors.Is(err, ErrInvalidState) {
		t.Fatal("manual transition bypassed native recovery owner", err)
	}
}

func TestNativeRecoveryConcurrentRequestsCreateOnePhysicalServiceOwner(t *testing.T) {
	f, ctx := nativeRecoveryFixture(t)
	results := make(chan error, 8)
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() { <-start; _, err := f.executor.Service.RequestRollback(ctx, f.record.ID); results <- err }()
	}
	close(start)
	for i := 0; i < 8; i++ {
		if err := <-results; err != nil && !errors.Is(err, ErrExecutionBusy) {
			t.Fatal(err)
		}
	}
	records, err := f.executor.Service.Store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	children := 0
	var childID string
	for _, r := range records {
		if r.NativeRollback != nil {
			children++
			childID = r.ID
		}
	}
	if children != 1 || f.stored(t).NativeRollbackRequestID != childID {
		t.Fatal("concurrent recovery approvals created multiple owners")
	}
}

func TestNativeRecoveryHoldsHandedOffIntentUntilExplicitRetry(t *testing.T) {
	f, ctx := nativeRecoveryFixture(t)
	child, err := f.executor.Service.RequestRollback(ctx, f.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.executor.Service.Create(ctx, f.record.PlanID); !errors.Is(err, ErrFailedIntentHeld) {
		t.Fatal("handed-off Git intent redeployed without explicit retry", err)
	}
	f.executor.Service.CreateRetryPlan = func(ctx context.Context, _ string) (planpreview.Record, error) {
		return f.executor.Service.LoadPlan(ctx, f.record.PlanID)
	}
	if _, err := f.executor.Service.Retry(ctx, f.record.ID); !errors.Is(err, ErrRecoveryUnavailable) {
		t.Fatal("forward retry bypassed active recovery", err)
	}
	for i := 0; i < 3; i++ {
		if err := f.executor.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	done, err := f.executor.Service.Get(ctx, child.Record.ID)
	if err != nil || done.Status != StatusSucceeded {
		t.Fatal(done, err)
	}
	if _, err := f.executor.Service.Create(ctx, f.record.PlanID); !errors.Is(err, ErrFailedIntentHeld) {
		t.Fatal("source recovery implicitly reapproved forward Git intent", err)
	}
	retried, err := f.executor.Service.Retry(ctx, f.record.ID)
	if err != nil || !retried.Created || retried.Record.RetryOf != f.record.ID || retried.Record.RetryDeploymentID != child.Record.NativeRollback.DeploymentARN {
		t.Fatal("explicit retry did not bind original native deployment", retried, err)
	}
}

func TestNativeCapacityRecoveryBindsCapturedCountAndExtraOperation(t *testing.T) {
	for _, name := range []string{"valid", "count changed", "restore omitted", "restore ID changed", "restore payload changed"} {
		t.Run(name, func(t *testing.T) {
			f, ctx := nativeRecoveryFixture(t)
			if _, err := f.executor.Service.Store.(MutationStore).Mutate(ctx, f.record.ID, func(r Record) (Record, error) {
				r.Operations[1].ECSCheckpoint.Result.PreviousService.DesiredCount = 1
				return r, nil
			}); err != nil {
				t.Fatal(err)
			}
			f.executor.ECS.Handlers[NativeCapacityRestoreOperationKind] = f.handler
			child, err := f.executor.Service.RequestRollback(ctx, f.record.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(child.Record.Operations) != 3 || child.Record.NativeRollback.DesiredCount != 1 || child.Record.Operations[1].Kind != NativeCapacityRestoreOperationKind || child.Record.Operations[0].ID == child.Record.Operations[1].ID {
				t.Fatal("capacity recovery omitted its immutable extra operation", child)
			}
			switch name {
			case "count changed":
				child.Record.NativeRollback.DesiredCount = 3
			case "restore omitted":
				child.Record.Operations = append(child.Record.Operations[:1], child.Record.Operations[2:]...)
			case "restore ID changed":
				child.Record.Operations[1].ID = child.Record.Operations[0].ID
			case "restore payload changed":
				child.Record.Operations[1].ECS.DesiredCount = 99
			}
			if name != "valid" {
				if _, err := f.executor.Service.Store.(MutationStore).Mutate(ctx, child.Record.ID, func(Record) (Record, error) { return child.Record, nil }); err != nil {
					t.Fatal(err)
				}
			}
			err = f.executor.advanceECS(ctx, child.Record)
			if name == "valid" {
				if err != nil || f.handler.calls.Load() != 1 {
					t.Fatal("valid three-stage recovery rejected", err)
				}
			} else if err == nil || f.handler.calls.Load() != 0 {
				t.Fatal("altered capacity recovery executed", name, err)
			}
		})
	}
}
