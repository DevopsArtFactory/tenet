package applyrequest

import (
	"context"
	"fmt"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"time"
)

// ECSRolloutObservation is secret-free AWS runtime evidence for the Apply monitor.
// A timestamp is present only after an observation attempt; unavailable evidence
// must never be interpreted as a healthy deployment.
type ECSRolloutObservation struct {
	TasksObserved        bool   `json:"tasksObserved"`
	PreviousCount        int32  `json:"previousCount"`
	ServiceRunningCount  int32  `json:"serviceRunningCount"`
	Source               string `json:"source"`
	State                string `json:"state"`
	Reason               string `json:"reason"`
	Summary              string `json:"summary"`
	StartedAt            string `json:"startedAt"`
	DeadlineAt           string `json:"deadlineAt"`
	ObservedAt           string `json:"observedAt,omitempty"`
	ClusterARN           string `json:"clusterArn"`
	ServiceARN           string `json:"serviceArn"`
	TaskDefinitionARN    string `json:"taskDefinitionArn"`
	DeploymentID         string `json:"deploymentId,omitempty"`
	ServiceDeploymentARN string `json:"serviceDeploymentArn,omitempty"`
	DesiredCount         int32  `json:"desiredCount"`
	RunningCount         int32  `json:"runningCount"`
	PendingCount         int32  `json:"pendingCount"`
	HealthyCount         int32  `json:"healthyCount"`
	Polls                int    `json:"polls"`
}

func (o *ECSRolloutObservation) Clone() *ECSRolloutObservation {
	if o == nil {
		return nil
	}
	copy := *o
	return &copy
}

// ECSObservationPending is normal read-only convergence progress, not an
// ambiguous mutation error. Its result is persisted before the next poll.
type ECSObservationPending struct {
	Result     ECSOperationResult
	RetryAfter time.Duration
}

func (p *ECSObservationPending) Error() string {
	if p != nil && p.Result.Observation != nil {
		return p.Result.Observation.Summary
	}
	return "awaiting ECS rollout observation"
}

// Waiting for RetryAfter must not probe AWS/IAM every executor scan. Only renew
// our owned local lease when needed; the next due poll rechecks authorization.
func (e Executor) ecsRetryPending(ctx context.Context, record Record) (bool, error) {
	for _, op := range record.Operations {
		cp := op.ECSCheckpoint
		if cp != nil && cp.State == "complete" {
			continue
		}
		if cp == nil || cp.RetryAfter == "" {
			return false, nil
		}
		due, err := time.Parse(time.RFC3339Nano, cp.RetryAfter)
		if err != nil {
			return true, err
		}
		now := e.Service.now()
		if !now.Before(due) {
			return false, nil
		}
		if record.Claim != nil && record.Claim.WorkerID == e.workerID() {
			last, err := time.Parse(time.RFC3339Nano, record.Claim.LastHeartbeatAt)
			if err != nil {
				return true, err
			}
			if now.Sub(last) >= claimFreshnessTTL/3 {
				_, err := e.ecsOwnedChange(ctx, record, e.ecsRefreshHeartbeat)
				return true, err
			}
		}
		return true, nil
	}
	return false, nil
}

func (e Executor) recordECSObservation(ctx context.Context, record Record, index int, result ECSOperationResult, delay time.Duration) error {
	if result.Observation == nil {
		return fmt.Errorf("ECS observation evidence is required")
	}
	if delay <= 0 {
		delay = 2 * time.Second
	}
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	_, err := e.ecsOwnedChange(ctx, record, func(current Record) (Record, error) {
		if index < 0 || index >= len(current.Operations) || current.Operations[index].ECSCheckpoint == nil {
			return Record{}, fmt.Errorf("ECS operation checkpoint is missing")
		}
		checkpoint := *current.Operations[index].ECSCheckpoint
		previous := checkpoint.Result.Observation
		checkpoint.Result = result.Clone()
		checkpoint.Error = ""
		checkpoint.UpdatedAt = e.Service.now().UTC().Format(time.RFC3339Nano)
		checkpoint.RetryAfter = e.Service.now().Add(delay).UTC().Format(time.RFC3339Nano)
		current.Operations[index].ECSCheckpoint = &checkpoint
		current.ECSRuntime = result.Observation.Clone()
		current.Error = ""
		current.Details = []string{result.Observation.Summary}
		attempt := ensureOperationAttempt(current.Operations[index].ExecutionAttempt)
		attempt.Summary = result.Observation.Summary
		attempt.Reason = result.Observation.Reason
		attempt.LastHeartbeatAt = checkpoint.UpdatedAt
		current.Operations[index].ExecutionAttempt = attempt
		current.Lifecycle.UpdatedAt = checkpoint.UpdatedAt
		if observationChanged(previous, result.Observation) {
			current.Transitions = appendTransition(current.Transitions, Transition{TransitionedAt: checkpoint.UpdatedAt, PreviousStatus: StatusRunning, Status: StatusRunning, Step: index + 1, Actor: current.Actor, Details: []string{result.Observation.Summary}, Lifecycle: current.Lifecycle})
		}
		return current, nil
	})
	return err
}

func observationChanged(before, after *ECSRolloutObservation) bool {
	if before == nil || after == nil {
		return before != after
	}
	return before.State != after.State || before.Reason != after.Reason || (before.State != "unavailable" && before.Summary != after.Summary) || before.DeploymentID != after.DeploymentID || before.TaskDefinitionARN != after.TaskDefinitionARN || before.DesiredCount != after.DesiredCount || before.RunningCount != after.RunningCount || before.PendingCount != after.PendingCount || before.HealthyCount != after.HealthyCount || before.TasksObserved != after.TasksObserved || before.PreviousCount != after.PreviousCount
}

// ECSObservationFailure carries observed terminal evidence into the same atomic
// write as the failed Apply lifecycle, preserving the reason across restarts.
type ECSObservationFailure struct{ Result ECSOperationResult }

func (p *ECSObservationFailure) Error() string {
	if p != nil && p.Result.Observation != nil {
		return p.Result.Observation.Summary
	}
	return "ECS rollout observation failed"
}

func hasECSCheckpoints(record Record) bool {
	for _, operation := range record.Operations {
		if operation.ECSCheckpoint != nil {
			return true
		}
	}
	return false
}

func ecsReadiness(record Record, checkedAt string) ExecutionReadiness {
	state := "executing"
	summary := "Native ECS executor owns this Apply; inspect operation checkpoints for execution evidence."
	if record.Status == StatusQueued {
		state = "queued"
		summary = "Approved native ECS Apply awaits executor validation of current authorization, intent and AWS capabilities."
	}
	if record.Status == StatusQueued && record.Error != "" {
		state = "blocked"
		summary = record.Error
	}
	if record.Status == StatusSucceeded {
		state = "completed"
		summary = "Native ECS execution completed with persisted operation results."
	} else if record.Status == StatusFailed {
		state = "failed"
		summary = record.Error
	}
	evidence := []string{"Execution evidence belongs to immutable Apply " + record.ID + "; checkpoints do not authorize manual AWS calls."}
	if record.ECSRuntime != nil {
		summary = record.ECSRuntime.Summary
		checkedAt = record.ECSRuntime.ObservedAt
		evidence = append(evidence, "Runtime reason: "+record.ECSRuntime.Reason)
	}
	if record.Recovery != nil {
		summary = "Rolling recovery of failed Apply " + record.Recovery.ApplyID + ": " + summary
	}
	return ExecutionReadiness{State: state, Summary: summary, CheckedAt: checkedAt, Evidence: evidence, Operations: deriveOperationExecutionPreflight(record, nil)}
}

func nativeECSPreflight(record Record, operation Operation, role *DeployerRoleReadiness) OperationExecutionPreflight {
	resourceType := "ecsService"
	resourceName := operation.ECS.ServiceARN
	capability := "ecs:DescribeServices"
	switch operation.Kind {
	case string(reconcile.StepRegisterECSTaskDefinition):
		resourceType = "taskDefinition"
		resourceName = operation.ECS.BaseTaskDefinitionARN
		capability = "ecs:RegisterTaskDefinition"
	case string(reconcile.StepUpdateECSService):
		capability = "ecs:UpdateService"
	}
	requiredRole := "target-account deployer role"
	if role != nil && role.RoleARN != "" {
		requiredRole = role.RoleARN
	}
	reason := "live_executor_not_configured"
	summary := "Reviewed native ECS intent awaits an explicitly configured live executor."
	if record.Principal != nil {
		reason = "executor_validation_pending"
		summary = "Trusted approval recorded; executor must validate current authorization, intent and AWS capabilities before execution."
	}
	if record.Status == StatusFailed && record.Error == disabledError {
		reason = "execution_disabled"
		summary = "Live execution is disabled; reviewed ECS intent is preserved."
	}
	if checkpoint := operation.ECSCheckpoint; checkpoint != nil {
		if checkpoint.RoleARN != "" {
			requiredRole = checkpoint.RoleARN
		}
		reason = "executor_owned"
		summary = "The live executor owns this operation; use its checkpoint and observation evidence."
		if operation.ExecutionAttempt != nil && operation.ExecutionAttempt.Summary != "" {
			summary = operation.ExecutionAttempt.Summary
		}
		if checkpoint.Error != "" {
			summary = checkpoint.Error
		}
		if checkpoint.State == "complete" {
			reason = "operation_completed"
		} else if checkpoint.State == "failed" {
			reason = "operation_failed"
		}
	}
	return OperationExecutionPreflight{OperationID: operation.ID, Phase: operation.Phase, Target: operation.Target, ResourceType: resourceType, ResourceName: resourceName, RequiredCapability: capability, RequiredRole: requiredRole, InputsSufficient: true, Ready: false, UnavailableReason: reason, Summary: summary, CapabilityChecks: []OperationExecutionCapabilityCheck{{Action: capability, ResourceType: resourceType, ResourceName: resourceName, RequiredRole: requiredRole, State: "required", Reason: reason, Summary: "AWS capability checks are enforced by the explicitly configured executor; this read-only preview never authorizes a call."}}, Handoffs: operationExecutionHandoffs(record, "ecsService", operation.ECS.ServiceARN)}
}
