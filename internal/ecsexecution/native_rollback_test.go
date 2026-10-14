package ecsexecution

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

type nativeRollbackFake struct {
	*nativeFake
	stops        int
	stopError    error
	noStopEffect bool
	badOutput    bool
}

func (f *nativeRollbackFake) StopServiceDeployment(_ context.Context, input *ecs.StopServiceDeploymentInput, opts ...func(*ecs.Options)) (*ecs.StopServiceDeploymentOutput, error) {
	options := &ecs.Options{}
	for _, option := range opts {
		option(options)
	}
	if input.StopType != types.StopServiceDeploymentStopTypeRollback || options.RetryMaxAttempts != 1 || options.Retryer == nil {
		return nil, errors.New("exact ROLLBACK and no SDK retries required")
	}
	key := aws.ToString(input.ServiceDeploymentArn)
	d, exists := f.deployments[key]
	if !exists {
		return nil, errors.New("unknown deployment")
	}
	f.stops++
	if !f.noStopEffect {
		d.Status = types.ServiceDeploymentStatusRollbackRequested
		d.Rollback = &types.Rollback{ServiceRevisionArn: d.SourceServiceRevisions[0].Arn}
		f.deployments[key] = d
	}
	if f.stopError != nil {
		return nil, f.stopError
	}
	if f.badOutput {
		return nil, nil
	}
	return &ecs.StopServiceDeploymentOutput{ServiceDeploymentArn: aws.String(key)}, nil
}
func nativeRollbackFixture(t *testing.T, strategy string) (*nativeRollbackFake, NativeRollback, applyrequest.ECSOperationRequest, NativeRollbackTarget, applyrequest.Record) {
	t.Helper()
	f, forward, r := nativeFixture(t, strategy)
	prepared, err := forward.Prepare(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.PreparedResult = prepared
	result, err := forward.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	parent := applyrequest.Record{ID: "parent-native-forward", Status: applyrequest.StatusRunning, Principal: &applyrequest.ExecutionPrincipal{Identity: requestactor.AuthenticatedIdentity{Authenticated: true}}, Operations: []applyrequest.Operation{
		{Kind: string(reconcile.StepRegisterECSTaskDefinition), ECS: r.Intent.Clone(), ECSCheckpoint: &applyrequest.ECSCheckpoint{State: "complete", Result: r.PriorResults[0]}},
		{Kind: string(reconcile.StepUpdateECSService), ECS: r.Intent.Clone(), ECSCheckpoint: &applyrequest.ECSCheckpoint{State: "complete", MutationAuthorizedAt: "2026-10-14T01:00:00Z", Result: result}},
		{Kind: string(reconcile.StepVerifyServiceStability), ECS: r.Intent.Clone()},
	}}
	target, err := NativeRollbackEvidence(parent)
	if err != nil {
		t.Fatal(err)
	}
	stop := &nativeRollbackFake{nativeFake: f}
	handler := NativeRollback{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (NativeRollbackClient, error) {
		return stop, nil
	}, CheckCapabilities: forward.CheckCapabilities}
	r.PriorResults = nil
	r.PreparedResult = applyrequest.ECSOperationResult{}
	r.ApplyID = "child-native-rollback"
	return stop, handler, r, target, parent
}
func TestNativeRollbackRequestsExactDeploymentOnceForEachStrategy(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		t.Run(strategy, func(t *testing.T) {
			f, h, r, target, _ := nativeRollbackFixture(t, strategy)
			guards := f.guards
			var stopChecks []applyrequest.OperationExecutionCapabilityCheck
			h.CheckCapabilities = func(_ context.Context, _ applyrequest.DeployerRoleReadiness, checks []applyrequest.OperationExecutionCapabilityCheck) error {
				for _, c := range checks {
					if c.Action == "ecs:UpdateService" || c.Action == "iam:PassRole" {
						t.Fatal("native rollback inherited Rolling mutation permissions")
					}
					if c.Action == "ecs:StopServiceDeployment" {
						stopChecks = append(stopChecks, c)
					}
				}
				return nil
			}
			var err error
			r.PreparedResult, err = h.Prepare(context.Background(), r, target)
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(struct {
				Target NativeRollbackTarget
				Result applyrequest.ECSOperationResult
			}{target, r.PreparedResult})
			if err != nil {
				t.Fatal(err)
			}
			var restored struct {
				Target NativeRollbackTarget
				Result applyrequest.ECSOperationResult
			}
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}
			r.PreparedResult = restored.Result
			result, err := h.Execute(context.Background(), r, restored.Target)
			if err != nil {
				t.Fatal(err)
			}
			if f.stops != 1 || f.writes != 1 || f.guards != guards+1 || result.TaskDefinitionARN != target.SourceTaskDefinitionARN || result.ServiceRevisionARN != target.SourceRevisionARN || result.Observation != nil {
				t.Fatalf("incorrect stop or falsely healthy recovery: %+v", result)
			}
			if len(stopChecks) != 2 || stopChecks[0].ResourceName != r.Intent.ServiceARN || stopChecks[1].ResourceName != target.DeploymentARN || stopChecks[1].Context["ecs:service"] != r.Intent.ServiceARN {
				t.Fatalf("wrong scoped stop capabilities: %+v", stopChecks)
			}
			outcome, err := h.Reconcile(context.Background(), r, target)
			if err != nil || !outcome.Complete || outcome.Absent || outcome.Result.Observation != nil || f.stops != 1 {
				t.Fatalf("acceptance reconciliation: %+v %v", outcome, err)
			}
			_, err = h.Execute(context.Background(), r, target)
			if err != nil || f.stops != 1 {
				t.Fatalf("already rolling back must be read-only: %v", err)
			}
		})
	}
}
func TestNativeRollbackLostResponseNeverAuthorizesReplay(t *testing.T) {
	for _, applied := range []bool{false, true} {
		t.Run(map[bool]string{true: "accepted", false: "unknown"}[applied], func(t *testing.T) {
			f, h, r, target, _ := nativeRollbackFixture(t, "CANARY")
			var err error
			r.PreparedResult, err = h.Prepare(context.Background(), r, target)
			if err != nil {
				t.Fatal(err)
			}
			f.stopError = errors.New("connection lost")
			f.noStopEffect = !applied
			_, err = h.Execute(context.Background(), r, target)
			if err == nil {
				t.Fatal("lost response must be ambiguous")
			}
			for i := 0; i < 3; i++ {
				outcome, err := h.Reconcile(context.Background(), r, target)
				if err != nil || outcome.Absent || outcome.Complete != applied {
					t.Fatalf("unsafe reconciliation %+v %v", outcome, err)
				}
			}
			if f.stops != 1 {
				t.Fatal("reconciliation resent StopServiceDeployment")
			}
		})
	}
}
func TestNativeRollbackBlocksChangedIdentityOwnershipAndInterveningWork(t *testing.T) {
	cases := map[string]func(*nativeRollbackFake, *NativeRollback, *applyrequest.ECSOperationRequest, *NativeRollbackTarget){
		"role": func(_ *nativeRollbackFake, _ *NativeRollback, r *applyrequest.ECSOperationRequest, _ *NativeRollbackTarget) {
			r.Role.CheckedWithAWS = false
		},
		"guard": func(_ *nativeRollbackFake, _ *NativeRollback, r *applyrequest.ECSOperationRequest, _ *NativeRollbackTarget) {
			r.BeforeMutation = func(context.Context) error { return errors.New("ownership revoked") }
		},
		"foreignDesiredTask": func(f *nativeRollbackFake, _ *NativeRollback, _ *applyrequest.ECSOperationRequest, _ *NativeRollbackTarget) {
			f.service.TaskDefinition = aws.String("arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:99")
		},
		"capacity": func(f *nativeRollbackFake, _ *NativeRollback, _ *applyrequest.ECSOperationRequest, _ *NativeRollbackTarget) {
			f.service.DesiredCount++
		},
		"binding": func(f *nativeRollbackFake, _ *NativeRollback, _ *applyrequest.ECSOperationRequest, _ *NativeRollbackTarget) {
			f.service.LoadBalancers = nil
		},
		"source": func(f *nativeRollbackFake, _ *NativeRollback, _ *applyrequest.ECSOperationRequest, target *NativeRollbackTarget) {
			d := f.deployments[target.DeploymentARN]
			d.SourceServiceRevisions = nil
			f.deployments[target.DeploymentARN] = d
		},
		"revisionWorkload": func(f *nativeRollbackFake, _ *NativeRollback, _ *applyrequest.ECSOperationRequest, target *NativeRollbackTarget) {
			d := f.revisions[target.SourceRevisionARN]
			d.TaskDefinition = aws.String(target.ForwardTaskDefinitionARN)
			f.revisions[target.SourceRevisionARN] = d
		},
		"foreign": func(_ *nativeRollbackFake, _ *NativeRollback, _ *applyrequest.ECSOperationRequest, target *NativeRollbackTarget) {
			target.DeploymentARN = "arn:aws:ecs:ap-northeast-2:999999999999:service-deployment/production/worker/other"
		},
		"newer": func(f *nativeRollbackFake, _ *NativeRollback, _ *applyrequest.ECSOperationRequest, _ *NativeRollbackTarget) {
			f.addDeployment("external", f.input)
			d := &f.history[len(f.history)-1]
			newer := d.CreatedAt.Add(time.Minute)
			d.CreatedAt = &newer
		},
		"permission": func(_ *nativeRollbackFake, h *NativeRollback, _ *applyrequest.ECSOperationRequest, _ *NativeRollbackTarget) {
			h.CheckCapabilities = func(_ context.Context, _ applyrequest.DeployerRoleReadiness, c []applyrequest.OperationExecutionCapabilityCheck) error {
				for _, v := range c {
					if v.Action == "ecs:StopServiceDeployment" {
						return errors.New("denied")
					}
				}
				return nil
			}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f, h, r, target, _ := nativeRollbackFixture(t, "LINEAR")
			var err error
			r.PreparedResult, err = h.Prepare(context.Background(), r, target)
			if err != nil {
				t.Fatal(err)
			}
			change(f, &h, &r, &target)
			if _, err := h.Execute(context.Background(), r, target); err == nil {
				t.Fatal("changed evidence allowed stop")
			}
			if f.stops != 0 {
				t.Fatal("unsafe rollback mutated AWS")
			}
		})
	}
}
func TestNativeRollbackTerminalStatesAndMissingResponse(t *testing.T) {
	for _, status := range []types.ServiceDeploymentStatus{types.ServiceDeploymentStatusSuccessful, types.ServiceDeploymentStatusStopped, types.ServiceDeploymentStatusStopRequested, types.ServiceDeploymentStatusRollbackFailed, types.ServiceDeploymentStatusRollbackSuccessful} {
		t.Run(string(status), func(t *testing.T) {
			f, h, r, target, _ := nativeRollbackFixture(t, "BLUE_GREEN")
			var err error
			r.PreparedResult, err = h.Prepare(context.Background(), r, target)
			if err != nil {
				t.Fatal(err)
			}
			d := f.deployments[target.DeploymentARN]
			d.Status = status
			accepted := status == types.ServiceDeploymentStatusRollbackSuccessful || status == types.ServiceDeploymentStatusRollbackFailed
			if accepted {
				d.Rollback = &types.Rollback{ServiceRevisionArn: aws.String(target.SourceRevisionARN)}
			}
			f.deployments[target.DeploymentARN] = d
			_, err = h.Execute(context.Background(), r, target)
			if (err == nil) != accepted || f.stops != 0 {
				t.Fatalf("wrong terminal handling %v stops=%d", err, f.stops)
			}
		})
	}
	f, h, r, target, _ := nativeRollbackFixture(t, "CANARY")
	r.PreparedResult, _ = h.Prepare(context.Background(), r, target)
	f.badOutput = true
	if _, err := h.Execute(context.Background(), r, target); err == nil {
		t.Fatal("missing response treated as accepted")
	}
	outcome, err := h.Reconcile(context.Background(), r, target)
	if err != nil || !outcome.Complete || f.stops != 1 {
		t.Fatal(outcome, err)
	}
}
func TestNativeRollbackEvidenceRejectsUnownedAndAdoptedUpdates(t *testing.T) {
	for _, name := range []string{"unowned", "adopted", "unknownSource", "success", "rolling", "registration"} {
		t.Run(name, func(t *testing.T) {
			_, _, _, _, parent := nativeRollbackFixture(t, "CANARY")
			cp := parent.Operations[1].ECSCheckpoint
			switch name {
			case "unowned":
				cp.MutationAuthorizedAt = ""
			case "adopted":
				cp.Result.NativeService.NoOpDeploymentARN = cp.Result.ServiceDeploymentARN
			case "unknownSource":
				cp.Result.NativeService.SourceRevisions = nil
			case "success":
				parent.Status = applyrequest.StatusSucceeded
			case "rolling":
				parent.Operations[0].ECS.DeploymentStrategy = "ROLLING"
			case "registration":
				parent.Operations[0].ECSCheckpoint.Result.TaskDefinitionARN = ""
			}
			if _, err := NativeRollbackEvidence(parent); err == nil {
				t.Fatal("unsafe rollback evidence accepted")
			}
		})
	}
}

func TestNativeRollbackUsesActualSourceInsteadOfFailedDesiredMetadata(t *testing.T) {
	_, _, _, expected, parent := nativeRollbackFixture(t, "CANARY")
	parent.Status = applyrequest.StatusFailed
	parent.Operations[1].ECSCheckpoint.Result.PreviousService.TaskDefinitionARN = expected.ForwardTaskDefinitionARN
	target, err := NativeRollbackEvidence(parent)
	if err != nil || target.SourceTaskDefinitionARN != expected.SourceTaskDefinitionARN {
		t.Fatalf("failed desired metadata replaced actual source: %+v %v", target, err)
	}
}
func TestNativeRollbackCancelledContextCannotSend(t *testing.T) {
	f, h, r, target, _ := nativeRollbackFixture(t, "CANARY")
	r.PreparedResult, _ = h.Prepare(context.Background(), r, target)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Execute(ctx, r, target); err == nil || f.stops != 0 {
		t.Fatal("cancelled rollback mutated AWS", err)
	}
}
