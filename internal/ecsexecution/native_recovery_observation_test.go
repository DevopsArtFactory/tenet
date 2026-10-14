package ecsexecution

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func nativeRecoveryObserverFixture(t *testing.T, strategy string) (*nativeObservationFake, NativeObservation, applyrequest.ECSOperationRequest, NativeRollbackTarget, *time.Time) {
	t.Helper()
	f, h, r, now := nativeObserverFixture(t, strategy)
	accepted := r.PriorResults[1]
	target := NativeRollbackTarget{ParentApplyID: "owned-parent", DeploymentARN: accepted.ServiceDeploymentARN, ForwardRevisionARN: accepted.ServiceRevisionARN, SourceRevisionARN: accepted.PreviousServiceRevisionARN, ForwardTaskDefinitionARN: accepted.TaskDefinitionARN, SourceTaskDefinitionARN: accepted.PreviousTaskDefinitionARN, DesiredCount: int32(r.Intent.DesiredCount)}
	deployment := f.deployments[target.DeploymentARN]
	deployment.Status = types.ServiceDeploymentStatusRollbackSuccessful
	deployment.Rollback = &types.Rollback{ServiceRevisionArn: aws.String(target.SourceRevisionARN)}
	deployment.TargetServiceRevision.RequestedProductionTrafficWeight = aws.Float64(0)
	deployment.TargetServiceRevision.RunningTaskCount = 0
	deployment.SourceServiceRevisions[0].RequestedProductionTrafficWeight = aws.Float64(100)
	deployment.SourceServiceRevisions[0].RunningTaskCount = target.DesiredCount
	f.deployments[target.DeploymentARN] = deployment
	revision := f.revisions[target.SourceRevisionARN]
	revision.ResolvedConfiguration = &types.ResolvedConfiguration{LoadBalancers: []types.ServiceRevisionLoadBalancer{{ProductionListenerRule: aws.String(r.Intent.Advanced.Routing.ProductionListenerRuleARN), TargetGroupArn: aws.String(r.Intent.Advanced.Routing.TargetGroupARN)}}}
	f.revisions[target.SourceRevisionARN] = revision
	f.service.TaskDefinition = aws.String(target.SourceTaskDefinitionARN)
	// ECS may restore previous bake/alarm policy; forward policy isn't the recovery goal.
	f.service.DeploymentConfiguration.BakeTimeInMinutes = aws.Int32(29)
	f.routing.Production = ALBWeightPair{Primary: 100}
	for key, task := range f.sampler.tasks {
		task.TaskDefinitionArn = aws.String(target.SourceTaskDefinitionARN)
		f.sampler.tasks[key] = task
	}
	r.PriorResults = []applyrequest.ECSOperationResult{rollbackResult(target)}
	r.PreparedResult = applyrequest.ECSOperationResult{}
	r.PreparedResult, _ = h.PrepareRollback(context.Background(), r, target)
	return f, h, r, target, now
}
func TestNativeRecoveryRequiresActualSourceTrafficAndHealth(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		t.Run(strategy, func(t *testing.T) {
			f, h, r, target, _ := nativeRecoveryObserverFixture(t, strategy)
			outcome, err := h.ReconcileRollback(context.Background(), r, target)
			if err != nil || !outcome.Complete || outcome.Failure != "" {
				t.Fatalf("source recovery not verified: %+v %v", outcome, err)
			}
			o := outcome.Result.Observation
			if o.Source != "aws-ecs-native-rollback" || o.Reason != "native_source_recovered" || o.Native.Status != "ROLLBACK_SUCCESSFUL" || o.HealthyCount != 2 || o.TaskDefinitionARN != target.SourceTaskDefinitionARN || o.Native.TargetRevisionARN != target.SourceRevisionARN || aws.ToFloat64(o.Native.TargetConfiguredPercent) != 100 || aws.ToFloat64(o.Native.TargetRequestedPercent) != 100 || aws.ToFloat64(o.Native.SourceRequestedPercent) != 0 || f.writes != 1 {
				t.Fatalf("wrong source health/traffic attribution: %+v", o)
			}
			if r.PreparedResult.Observation.TasksObserved || r.PreparedResult.Observation.Native != nil {
				t.Fatal("poll mutated persisted input")
			}
		})
	}
}
func TestNativeRecoveryControlStatusAloneCannotProveRecovery(t *testing.T) {
	for _, name := range []string{"rollback active", "forward succeeded", "forward tasks remain", "forward pending", "wrong traffic", "missing task", "unhealthy ALB", "ALB read denied", "extra targets", "wrong current workload", "wrong capacity", "missing rollback source", "different rollback source", "different source workload", "newer deployment", "history unavailable", "source inactive"} {
		t.Run(name, func(t *testing.T) {
			f, h, r, target, _ := nativeRecoveryObserverFixture(t, "CANARY")
			deployment := f.deployments[target.DeploymentARN]
			switch name {
			case "rollback active":
				deployment.Status = types.ServiceDeploymentStatusRollbackInProgress
			case "forward succeeded":
				deployment.Status = types.ServiceDeploymentStatusSuccessful
				deployment.Rollback = nil
			case "forward tasks remain":
				deployment.TargetServiceRevision.RunningTaskCount = 1
			case "forward pending":
				deployment.TargetServiceRevision.PendingTaskCount = 1
			case "wrong traffic":
				f.routing.Production = ALBWeightPair{Primary: 99, Alternate: 1}
			case "missing task":
				delete(f.sampler.tasks, f.sampler.running[0])
			case "unhealthy ALB":
				h.ObserveTargetHealth = func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness, string, []types.Task) (NativeTargetHealthEvidence, error) {
					return NativeTargetHealthEvidence{}, nil
				}
			case "ALB read denied":
				h.ObserveTargetHealth = func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness, string, []types.Task) (NativeTargetHealthEvidence, error) {
					return NativeTargetHealthEvidence{}, errors.New("denied")
				}
			case "extra targets":
				h.ObserveTargetHealth = func(_ context.Context, _ domain.ECSDeploymentIntent, _ applyrequest.DeployerRoleReadiness, _ string, tasks []types.Task) (NativeTargetHealthEvidence, error) {
					e := NativeTargetHealthEvidence{UnexpectedTargets: 1}
					for _, t := range tasks {
						e.HealthyTaskARNs = append(e.HealthyTaskARNs, aws.ToString(t.TaskArn))
					}
					return e, nil
				}
			case "wrong current workload":
				f.service.TaskDefinition = aws.String(target.ForwardTaskDefinitionARN)
			case "wrong capacity":
				f.service.DesiredCount++
			case "missing rollback source":
				deployment.Rollback = nil
			case "different rollback source":
				deployment.Rollback.ServiceRevisionArn = aws.String(target.ForwardRevisionARN)
			case "different source workload":
				revision := f.revisions[target.SourceRevisionARN]
				revision.TaskDefinition = aws.String(target.ForwardTaskDefinitionARN)
				f.revisions[target.SourceRevisionARN] = revision
			case "newer deployment":
				f.addDeployment("newer", f.input)
				brief := &f.history[len(f.history)-1]
				newer := brief.CreatedAt.Add(time.Minute)
				brief.CreatedAt = &newer
			case "history unavailable":
				f.history = nil
			case "source inactive":
				f.definitions[target.SourceTaskDefinitionARN].TaskDefinition.Status = types.TaskDefinitionStatusInactive
			}
			f.deployments[target.DeploymentARN] = deployment
			outcome, err := h.ReconcileRollback(context.Background(), r, target)
			// Existing source tasks can be healthy even after task-definition deregistration.
			if name == "source inactive" {
				if err != nil || !outcome.Complete {
					t.Fatal("inactive existing source revision cannot be observed", outcome, err)
				}
				return
			}
			if err != nil || outcome.Complete || outcome.Absent || f.writes != 1 {
				t.Fatalf("false recovery evidence %s: %+v %v", name, outcome, err)
			}
		})
	}
}
func TestNativeRecoveryFailureAndRestartDeadlineRemainDistinct(t *testing.T) {
	for _, status := range []types.ServiceDeploymentStatus{types.ServiceDeploymentStatusStopped, types.ServiceDeploymentStatusRollbackFailed} {
		t.Run(string(status), func(t *testing.T) {
			f, h, r, target, _ := nativeRecoveryObserverFixture(t, "LINEAR")
			deployment := f.deployments[target.DeploymentARN]
			deployment.Status = status
			f.deployments[target.DeploymentARN] = deployment
			outcome, err := h.ReconcileRollback(context.Background(), r, target)
			if err != nil || outcome.Complete || outcome.Failure == "" || outcome.Result.Observation.State != "failed" {
				t.Fatal("terminal recovery failure hidden", outcome, err)
			}
		})
	}
	_, h, r, target, now := nativeRecoveryObserverFixture(t, "BLUE_GREEN")
	data, err := json.Marshal(r.PreparedResult)
	if err != nil {
		t.Fatal(err)
	}
	var restored applyrequest.ECSOperationResult
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	r.PreparedResult = restored
	start, deadline := restored.Observation.StartedAt, restored.Observation.DeadlineAt
	*now = now.Add(2 * time.Hour)
	outcome, err := h.ReconcileRollback(context.Background(), r, target)
	if err != nil || outcome.Failure == "" || outcome.Result.Observation.Reason != "observation_timeout" || outcome.Result.Observation.StartedAt != start || outcome.Result.Observation.DeadlineAt != deadline {
		t.Fatal("recovery restart extended its deadline", outcome, err)
	}
	r.PreparedResult.ServiceRevisionARN = target.ForwardRevisionARN
	if _, err := h.PrepareRollback(context.Background(), r, target); err == nil {
		t.Fatal("altered recovery target accepted")
	}
}
