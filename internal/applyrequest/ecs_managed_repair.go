package applyrequest

import (
	"context"
	"fmt"
	"reflect"

	"github.com/DevopsArtFactory/tenet/internal/domain"
)

// VerifiedNativeService returns the workload/deployment identities of a verified
// forward result. Recovery outcomes cannot authorize a new managed repair.
func VerifiedNativeService(record Record, intent domain.ECSDeploymentIntent) (ECSOperationResult, error) {
	var empty ECSOperationResult
	if record.Status != StatusSucceeded || record.Recovery != nil || record.NativeRollback != nil || record.NativeRollbackRequestID != "" || record.Principal == nil || !record.Principal.Identity.Authenticated || !nativeForwardOperations(record) || !reflect.DeepEqual(record.Operations[0].ECS, &intent) || record.ECSRuntime == nil || record.ECSRuntime.State != "converged" || record.ECSRuntime.Native == nil || record.ECSRuntime.Native.Status != "SUCCESSFUL" || !record.ECSRuntime.TasksObserved || record.ECSRuntime.DesiredCount != int32(intent.DesiredCount) || record.ECSRuntime.HealthyCount != int32(intent.DesiredCount) || record.ECSRuntime.Native.TargetConfiguredPercent == nil || *record.ECSRuntime.Native.TargetConfiguredPercent != 100 {
		return empty, fmt.Errorf("managed repair requires a verified native forward Apply of the same intent")
	}
	for _, op := range record.Operations {
		if op.ECSCheckpoint == nil || op.ECSCheckpoint.State != "complete" {
			return empty, fmt.Errorf("verified native checkpoints are incomplete")
		}
	}
	registered := record.Operations[0].ECSCheckpoint.Result
	result := record.Operations[1].ECSCheckpoint.Result
	final := record.Operations[2].ECSCheckpoint.Result
	if registered.TaskDefinitionARN == "" || registered.TaskDefinitionIntent == "" || result.TaskDefinitionARN != registered.TaskDefinitionARN || result.TaskDefinitionIntent != registered.TaskDefinitionIntent || result.ServiceDeploymentARN == "" || result.ServiceRevisionARN == "" || final.ServiceDeploymentARN != result.ServiceDeploymentARN || final.ServiceRevisionARN != result.ServiceRevisionARN || record.ECSRuntime.ServiceDeploymentARN != result.ServiceDeploymentARN || record.ECSRuntime.TaskDefinitionARN != registered.TaskDefinitionARN || record.ECSRuntime.Native.TargetRevisionARN != result.ServiceRevisionARN {
		return empty, fmt.Errorf("verified native deployment/workload evidence is inconsistent")
	}
	return result.Clone(), nil
}

// Repair authority is derived from the reviewed server-created drift plan and
// its verified predecessor, never from an HTTP field or an AWS health change.
func (s Service) nativeManagedRepair(ctx context.Context, record Record) (bool, error) {
	if record.NativeRollback != nil || record.Recovery != nil || len(record.Operations) == 0 || record.Operations[0].ECS == nil {
		return false, nil
	}
	intent := record.Operations[0].ECS
	if intent.DeploymentStrategy == "ROLLING" || !intent.SelfHeal || intent.SyncMode != "automatic" {
		return false, nil
	}
	seen := map[string]bool{}
	for {
		if seen[record.ID] {
			return false, fmt.Errorf("managed repair retry ancestry contains a cycle")
		}
		seen[record.ID] = true
		if record.Source.LastVerifiedApplyID != "" {
			plan, err := s.LoadPlan(ctx, record.PlanID)
			if err != nil {
				return false, err
			}
			if plan.Source.ManagedDrift == nil || plan.Source.ManagedDrift.Signature == "" || !reviewedDriftLink(record, plan) {
				return false, fmt.Errorf("managed repair omitted its reviewed drift link")
			}
			previous, err := s.Get(ctx, record.Source.LastVerifiedApplyID)
			if err != nil {
				return false, err
			}
			// A new image/policy may carry the same repair scope. Changing service,
			// strategy or Git capacity is not permission to resize an unverified target.
			if len(previous.Operations) == 0 || previous.Operations[0].ECS == nil || !sameNativeRepairScope(*previous.Operations[0].ECS, *intent) {
				return false, nil
			}
			if _, err := VerifiedNativeService(previous, *previous.Operations[0].ECS); err != nil {
				return false, err
			}
			return true, nil
		}
		if record.RetryOf == "" {
			return false, nil
		}
		previous, err := s.Get(ctx, record.RetryOf)
		if err != nil {
			return false, err
		}
		if !ecsRetryableParent(previous) || len(previous.Operations) == 0 || !reflect.DeepEqual(previous.Operations[0].ECS, intent) {
			return false, ErrRecoveryUnavailable
		}
		record = previous
	}
}

func (e Executor) reviewedECSRequest(ctx context.Context, record Record, index int, role DeployerRoleReadiness) (ECSOperationRequest, error) {
	request := ecsRequest(record, index, role)
	repair, err := e.Service.nativeManagedRepair(ctx, record)
	if err != nil {
		return request, err
	}
	request.ManagedRepair = repair
	return request, nil
}

func sameNativeRepairScope(a, b domain.ECSDeploymentIntent) bool {
	return a.DeploymentController == b.DeploymentController && a.DeploymentStrategy == b.DeploymentStrategy && a.ClusterARN == b.ClusterARN && a.ServiceARN == b.ServiceARN && a.AWSAccountID == b.AWSAccountID && a.Region == b.Region && a.ContainerName == b.ContainerName && a.DesiredCount == b.DesiredCount
}
