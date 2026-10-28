package applyrequest

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func nativeAutomaticFixture(t *testing.T, policy bool) (*ecsFixture, context.Context) {
	t.Helper()
	f, ctx := nativeRecoveryFixture(t, policy)
	r := f.record
	result := r.Operations[1].ECSCheckpoint.Result
	r.Status = StatusFailed
	r.ECSRuntime = &ECSRolloutObservation{Source: "aws-ecs-native", State: "failed", Reason: "native_rollback_reported", ServiceDeploymentARN: result.ServiceDeploymentARN, TaskDefinitionARN: result.TaskDefinitionARN, Native: &ECSNativeRolloutObservation{Status: "ROLLBACK_SUCCESSFUL", TargetRevisionARN: result.ServiceRevisionARN, RollbackRevisionARN: result.PreviousServiceRevisionARN}}
	r.Operations[2].Status = OperationStatusFailed
	r.Operations[2].ECSCheckpoint = &ECSCheckpoint{State: "failed", Attempts: 1, RequestID: ecsRequest(r, 2, DeployerRoleReadiness{}).RequestID, Result: ECSOperationResult{Observation: r.ECSRuntime.Clone()}}
	f.record = r
	if _, err := f.executor.Service.Store.(MutationStore).Mutate(ctx, r.ID, func(Record) (Record, error) { return r, nil }); err != nil {
		t.Fatal(err)
	}
	return f, ctx
}
func TestNativeAutomaticRecoveryRejectsUnsafeFailuresAndWithdrawnApproval(t *testing.T) {
	for _, name := range []string{"off", "unavailable", "timeout", "stopped", "rollback failed", "wrong deployment", "wrong revision", "unknown alarm", "unowned update", "adopted update", "missing verification", "mismatched verification", "new intent", "RBAC revoked", "paused", "readiness denied", "unreviewed policy", "explicit retry", "advanced disabled", "missing authorizer", "missing intent", "missing plan"} {
		t.Run(name, func(t *testing.T) {
			f, ctx := nativeAutomaticFixture(t, name != "off" && name != "unreviewed policy")
			switch name {
			case "unavailable":
				f.record.ECSRuntime.Reason = "aws_unavailable"
			case "timeout":
				f.record.ECSRuntime.Reason = "observation_timeout"
			case "stopped":
				f.record.ECSRuntime.Reason = "native_deployment_stopped"
				f.record.ECSRuntime.Native.Status = "STOPPED"
			case "rollback failed":
				f.record.ECSRuntime.Reason = "native_rollback_failed"
				f.record.ECSRuntime.Native.Status = "ROLLBACK_FAILED"
			case "wrong deployment":
				f.record.ECSRuntime.ServiceDeploymentARN += "-foreign"
			case "wrong revision":
				f.record.ECSRuntime.Native.TargetRevisionARN += "-foreign"
			case "unknown alarm":
				f.record.ECSRuntime.Reason = "native_alarm_triggered"
				f.record.ECSRuntime.Native.Status = "IN_PROGRESS"
				f.record.ECSRuntime.Native.TriggeredAlarms = []string{"foreign"}
			case "unowned update":
				f.record.Operations[1].ECSCheckpoint.MutationAuthorizedAt = ""
			case "adopted update":
				f.record.Operations[1].ECSCheckpoint.Result.NativeService.NoOpDeploymentARN = f.record.ECSRuntime.ServiceDeploymentARN
			case "missing verification":
				f.record.Operations[2].ECSCheckpoint = nil
			case "new intent":
				f.intent.Image = "example/changed:new"
			case "advanced disabled":
				f.executor.ECS.AllowAdvanced = false
			case "missing authorizer":
				f.executor.ECS.Authorize = nil
			case "missing intent":
				f.executor.ECS.CurrentIntent = nil
			case "missing plan":
				f.executor.Service.LoadPlan = nil
			case "RBAC revoked":
				f.executor.ECS.Authorize = func(context.Context, Record) error { return ErrExecutionUnauthorized }
			case "paused":
				f.executor.ECS.AuthorizeMutation = func(context.Context, Record) error { return ErrExecutionUnauthorized }
			case "readiness denied":
				f.executor.Service.PrepareNativeRollback = func(context.Context, Record, ECSNativeRollback) error { return errors.New("AWS reads unavailable") }
			case "unreviewed policy":
				for i := range f.record.Operations {
					f.record.Operations[i].ECS.RollbackOnFailure = true
				}
				f.intent.RollbackOnFailure = true
			case "explicit retry":
				if err := f.executor.Service.Store.Save(ctx, Record{ID: "explicit-retry", RetryOf: f.record.ID, Status: StatusFailed}); err != nil {
					t.Fatal(err)
				}
			}
			if f.record.Operations[2].ECSCheckpoint != nil {
				f.record.Operations[2].ECSCheckpoint.Result.Observation = f.record.ECSRuntime.Clone()
			}
			if name == "mismatched verification" {
				f.record.Operations[2].ECSCheckpoint.Result.Observation.Reason = "aws_unavailable"
			}
			if _, err := f.executor.Service.Store.(MutationStore).Mutate(ctx, f.record.ID, func(Record) (Record, error) { return f.record, nil }); err != nil {
				t.Fatal(err)
			}
			_ = f.executor.advanceECS(ctx, f.record)
			rows, err := f.executor.Service.Store.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rows {
				if r.NativeRollback != nil {
					t.Fatal("unsafe automatic recovery was approved", name, r)
				}
			}
			if f.handler.calls.Load() != 0 {
				t.Fatal("unsafe automatic recovery wrote AWS", name)
			}
		})
	}
}
func TestNativeAutomaticRecoveryPersistsInheritanceAndRejectsTampering(t *testing.T) {
	for _, name := range []string{"valid", "principal changed", "automatic marker changed", "policy withdrawn", "explicit manual recovery"} {
		t.Run(name, func(t *testing.T) {
			f, ctx := nativeAutomaticFixture(t, true)
			if name == "explicit manual recovery" {
				if _, err := f.executor.Service.RequestRollback(ctx, f.record.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.executor.advanceECS(ctx, f.record); err != nil {
				t.Fatal(err)
			}
			var child Record
			rows, _ := f.executor.Service.Store.List(ctx)
			for _, r := range rows {
				if r.NativeRollback != nil {
					child = r
				}
			}
			if child.ID == "" || child.NativeRollback.Automatic != (name != "explicit manual recovery") || !reflect.DeepEqual(child.Principal, f.record.Principal) {
				t.Fatal("automatic native approval provenance lost", child)
			}
			if name == "explicit manual recovery" {
				if len(rows) != 2 {
					t.Fatal("automatic recovery competed with manual child")
				}
				return
			}
			switch name {
			case "principal changed":
				child.Principal.Identity.ID = "different-user"
			case "automatic marker changed":
				child.NativeRollback.Automatic = false
			case "policy withdrawn":
				f.intent.RollbackOnFailure = false
			}
			if name == "principal changed" || name == "automatic marker changed" {
				if _, err := f.executor.Service.Store.(MutationStore).Mutate(ctx, child.ID, func(Record) (Record, error) { return child, nil }); err != nil {
					t.Fatal(err)
				}
			}
			err := f.executor.advanceECS(ctx, child)
			if name == "valid" {
				if err != nil || f.handler.calls.Load() != 1 {
					t.Fatal("inherited native approval did not execute", err)
				}
			} else if err == nil || f.handler.calls.Load() != 0 {
				t.Fatal("changed automatic authority executed", name, err)
			}
		})
	}
}
