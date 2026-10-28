package applyrequest

import (
	"context"
	"errors"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

func TestCanSupersedeECSRequiresPositiveNoServiceSendEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		cp   *ECSCheckpoint
		want bool
	}{
		{"not prepared", nil, true}, {"guard not reached", &ECSCheckpoint{State: "in_flight", MutationGuarded: true}, true}, {"confirmed rejection", &ECSCheckpoint{State: "retryable_rejection", MutationAuthorizedAt: "sent"}, true},
		{"old ambiguous checkpoint", &ECSCheckpoint{State: "in_flight"}, false}, {"authorized send", &ECSCheckpoint{State: "in_flight", MutationGuarded: true, MutationAuthorizedAt: "sent"}, false}, {"accepted update", &ECSCheckpoint{State: "complete"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newECSFixture(t)
			r := f.record
			r.Operations[1].ECSCheckpoint = tc.cp
			if CanSupersedeECS(r) != tc.want {
				t.Fatal("unsafe supersede decision")
			}
			r.Status = StatusSucceeded
			if CanSupersedeECS(r) {
				t.Fatal("terminal history cancellable")
			}
		})
	}
}
func TestSupersedeRequiresReviewedReplacementAndCurrentAuthorization(t *testing.T) {
	f := newECSFixture(t)
	service := f.executor.Service
	service.AuthorizeCreate = func(context.Context, Record) (*ExecutionPrincipal, error) { return nil, ErrExecutionUnauthorized }
	if _, err := service.SupersedeECS(context.Background(), f.record.ID, f.record.ID); !errors.Is(err, ErrInvalidState) {
		t.Fatal("untrusted replacement accepted")
	}
	// Mutating an ID-bound source/approval cannot authorize cancellation.
	if _, err := service.Store.(MutationStore).Mutate(context.Background(), f.record.ID, func(r Record) (Record, error) {
		r.Principal = &ExecutionPrincipal{Identity: requestactor.AuthenticatedIdentity{Authenticated: true, ID: "operator"}}
		return r, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SupersedeECS(context.Background(), f.record.ID, f.record.ID); !errors.Is(err, ErrInvalidState) {
		t.Fatal("altered replacement identity accepted")
	}
	if r := f.stored(t); r.Status != StatusQueued {
		t.Fatal("invalid replacement cancelled existing request")
	}
}

func TestAutomationPauseStopsMutationsButAllowsReadOnlyVerification(t *testing.T) {
	f := newECSFixture(t)
	for i := 0; i < 2; i++ {
		if err := f.executor.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	before := f.handler.calls.Load()
	f.executor.ECS.AuthorizeMutation = func(context.Context, Record) error { return ErrExecutionUnauthorized }
	// The already accepted update's observation is read-only and can finish.
	if err := f.executor.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.stored(t).Status != StatusSucceeded || f.handler.calls.Load() != before+1 {
		t.Fatal("pausing writes prevented read-only completion")
	}
	blocked := newECSFixture(t)
	blocked.executor.ECS.AuthorizeMutation = f.executor.ECS.AuthorizeMutation
	if err := blocked.executor.RunOnce(context.Background()); !errors.Is(err, ErrExecutionUnauthorized) {
		t.Fatal("paused automation accepted new mutation")
	}
	if blocked.handler.calls.Load() != 0 || hasECSCheckpoints(blocked.stored(t)) {
		t.Fatal("paused automation wrote a checkpoint or AWS mutation")
	}
}

func TestAdvancedSupersessionPreservesRecoveryAndAmbiguousWrites(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		t.Run(strategy, func(t *testing.T) {
			f := newECSFixture(t)
			r := f.record
			for i := range r.Operations {
				r.Operations[i].ECS.DeploymentStrategy = strategy
				r.Operations[i].ECS.Advanced = &domain.ECSAdvancedConfiguration{}
			}
			if !CanSupersedeECS(r) {
				t.Fatal("unstarted native update cannot be replaced")
			}
			r.Operations[1].ECSCheckpoint = &ECSCheckpoint{State: "in_flight", MutationGuarded: true, MutationAuthorizedAt: "sent"}
			if CanSupersedeECS(r) {
				t.Fatal("native accepted/uncertain write was replaceable")
			}
			r.Operations[1].ECSCheckpoint = nil
			r.NativeRollback = &ECSNativeRollback{ParentApplyID: "parent"}
			if CanSupersedeECS(r) {
				t.Fatal("native recovery was replaceable")
			}
			r.NativeRollback = nil
			r.NativeRollbackRequestID = "recovery-child"
			if CanSupersedeECS(r) {
				t.Fatal("handed-off forward owner was replaceable")
			}
			r.NativeRollbackRequestID = ""
			r.Recovery = &ECSRecovery{ApplyID: "parent"}
			if CanSupersedeECS(r) {
				t.Fatal("recovery was replaceable")
			}
		})
	}
}

func TestSupersessionRestoresStoredAutomationApprovalContext(t *testing.T) {
	principal := &ExecutionPrincipal{Automatic: true, Identity: requestactor.AuthenticatedIdentity{ID: "ecs-sync", Authenticated: true}}
	ctx := principalContext(context.Background(), principal)
	if !AutomaticApprovalFromContext(ctx) || requestactor.AuthenticatedIdentityFromContext(ctx).ID != "ecs-sync" {
		t.Fatal("stored automation identity lost its approval context")
	}
	principal.Automatic = false
	if AutomaticApprovalFromContext(principalContext(ctx, principal)) {
		t.Fatal("human approval inherited an automation context")
	}
}
