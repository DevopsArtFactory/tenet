package applyrequest

import (
	"context"
	"fmt"
	"reflect"
	"slices"

	"github.com/DevopsArtFactory/tenet/internal/planpreview"
)

// Only persisted, correlated AWS failure evidence can inherit forward approval.
func nativeAutomaticFailure(parent Record) bool {
	if parent.Status != StatusFailed || parent.Recovery != nil || parent.NativeRollback != nil || parent.Principal == nil || !parent.Principal.Identity.Authenticated || !nativeForwardOperations(parent) || parent.Operations[0].ECS.DeploymentStrategy == "ROLLING" || !parent.Operations[0].ECS.RollbackOnFailure || parent.ECSRuntime == nil || parent.ECSRuntime.Source != "aws-ecs-native" || parent.ECSRuntime.State != "failed" || parent.ECSRuntime.Native == nil {
		return false
	}
	if _, err := NativeRollbackEvidence(parent); err != nil {
		return false
	}
	final := parent.Operations[2].ECSCheckpoint
	runtime := parent.ECSRuntime
	result := parent.Operations[1].ECSCheckpoint.Result
	if final == nil || final.State != "failed" || !reflect.DeepEqual(final.Result.Observation, runtime) || runtime.ServiceDeploymentARN != result.ServiceDeploymentARN || runtime.TaskDefinitionARN != result.TaskDefinitionARN || runtime.Native.TargetRevisionARN != result.ServiceRevisionARN || (runtime.Native.RollbackRevisionARN != "" && runtime.Native.RollbackRevisionARN != result.PreviousServiceRevisionARN) {
		return false
	}
	switch runtime.Reason {
	case "native_alarm_triggered":
		if runtime.Native.Status != "PENDING" && runtime.Native.Status != "IN_PROGRESS" {
			return false
		}
		alarms := parent.Operations[0].ECS.Advanced.Alarms
		if alarms == nil || !alarms.Enable {
			return false
		}
		for _, triggered := range runtime.Native.TriggeredAlarms {
			if slices.Contains(alarms.Names, triggered) {
				return true
			}
		}
	case "native_rollback_reported":
		return runtime.Native.Status == "ROLLBACK_REQUESTED" || runtime.Native.Status == "ROLLBACK_IN_PROGRESS" || runtime.Native.Status == "ROLLBACK_SUCCESSFUL"
	}
	return false
}

// advanceECS holds the service lock, shared with explicit recovery/retry requests.
func (e Executor) automaticNativeRollback(ctx context.Context, parent Record) error {
	if !nativeAutomaticFailure(parent) || e.Service.PrepareNativeRollback == nil {
		return nil
	}
	if e.ECS == nil || !e.ECS.AllowAdvanced || e.ECS.Authorize == nil || e.ECS.CurrentIntent == nil || e.Service.LoadPlan == nil {
		return ErrExecutionUnauthorized
	}
	records, err := e.Service.Store.List(ctx)
	if err != nil {
		return err
	}
	for _, r := range records {
		if r.RetryOf == parent.ID || (r.NativeRollback != nil && r.NativeRollback.ParentApplyID == parent.ID) {
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
	if e.ECS.AuthorizeMutation != nil {
		if err := e.ECS.AuthorizeMutation(ctx, parent); err != nil {
			return err
		}
	}
	plan, err := e.Service.LoadPlan(ctx, parent.PlanID)
	if err != nil {
		return err
	}
	plan = planpreview.NormalizeOperationIDs(plan)
	id, idErr := stableRecordID(parent)
	if idErr != nil || id != parent.ID || plan.ID != parent.PlanID || plan.Status != planpreview.StatusReady || plan.DeploymentName != parent.DeploymentName || parent.Source.Fingerprint == "" || plan.Source.Fingerprint != parent.Source.Fingerprint || !reflect.DeepEqual(plan.Source.Git, parent.Source.Git) || !reviewedDriftLink(parent, plan) || len(plan.Operations) != 3 {
		return fmt.Errorf("automatic native recovery requires its immutable reviewed plan")
	}
	for i, op := range parent.Operations {
		reviewed := plan.Operations[i]
		if op.ID != reviewed.ID || op.Kind != reviewed.Kind || op.Target != reviewed.Target || !reflect.DeepEqual(op.ECS, reviewed.ECS) {
			return fmt.Errorf("automatic native recovery differs from its reviewed plan")
		}
	}
	_, err = e.Service.createNativeRollback(ctx, parent, parent.Principal, true)
	return err
}
