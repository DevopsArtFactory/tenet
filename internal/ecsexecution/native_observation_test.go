package ecsexecution

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

var _ applyrequest.ECSOperationHandler = NativeObservation{}
var _ applyrequest.ECSOperationPreparer = NativeObservation{}

type nativeObservationFake struct {
	*nativeFake
	sampler      *observationFake
	routing      ALBRoutingSnapshot
	routingError error
}

func (f *nativeObservationFake) ListTasks(ctx context.Context, input *ecs.ListTasksInput, options ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	return f.sampler.ListTasks(ctx, input, options...)
}
func (f *nativeObservationFake) DescribeTasks(ctx context.Context, input *ecs.DescribeTasksInput, options ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	return f.sampler.DescribeTasks(ctx, input, options...)
}
func nativeObserverFixture(t *testing.T, strategy string) (*nativeObservationFake, NativeObservation, applyrequest.ECSOperationRequest, *time.Time) {
	t.Helper()
	backend, update, r := nativeFixture(t, strategy)
	prepared, err := update.Prepare(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.PreparedResult = prepared
	accepted, err := update.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.PriorResults = append(r.PriorResults, accepted)
	r.PreparedResult = applyrequest.ECSOperationResult{}
	backend.service.DesiredCount = int32(r.Intent.DesiredCount)
	backend.service.RunningCount = int32(r.Intent.DesiredCount)
	backend.service.PendingCount = 0
	f := &nativeObservationFake{nativeFake: backend, sampler: &observationFake{serviceFake: backend.serviceFake, tasks: map[string]types.Task{}}, routing: ALBRoutingSnapshot{Signature: "AWS routing", Production: ALBWeightPair{Alternate: 100}}}
	for i := 0; i < r.Intent.DesiredCount; i++ {
		value := fmt.Sprintf("arn:aws:ecs:ap-northeast-2:123456789012:task/production/native-%d", i)
		f.sampler.running = append(f.sampler.running, value)
		f.sampler.tasks[value] = types.Task{TaskArn: aws.String(value), ClusterArn: aws.String(r.Intent.ClusterARN), TaskDefinitionArn: aws.String(accepted.TaskDefinitionARN), Group: aws.String("service:worker"), LastStatus: aws.String("RUNNING"), Containers: []types.Container{{Name: aws.String("worker"), LastStatus: aws.String("RUNNING")}}}
	}
	deployment := backend.deployments[accepted.ServiceDeploymentARN]
	deployment.Status = types.ServiceDeploymentStatusSuccessful
	deployment.LifecycleStage = types.ServiceDeploymentLifecycleStageCleanUp
	deployment.TargetServiceRevision.RequestedProductionTrafficWeight = aws.Float64(100)
	deployment.TargetServiceRevision.RunningTaskCount = int32(r.Intent.DesiredCount)
	deployment.SourceServiceRevisions[0].RequestedProductionTrafficWeight = aws.Float64(0)
	backend.deployments[accepted.ServiceDeploymentARN] = deployment
	revision := backend.revisions[accepted.ServiceRevisionARN]
	revision.ResolvedConfiguration = &types.ResolvedConfiguration{LoadBalancers: []types.ServiceRevisionLoadBalancer{{ProductionListenerRule: aws.String(r.Intent.Advanced.Routing.ProductionListenerRuleARN), TargetGroupArn: aws.String(r.Intent.Advanced.Routing.AlternateTargetGroupARN)}}}
	backend.revisions[accepted.ServiceRevisionARN] = revision
	now := time.Date(2026, 10, 14, 2, 0, 0, 0, time.UTC)
	observer := NativeObservation{ObserveTargetHealth: func(_ context.Context, _ domain.ECSDeploymentIntent, _ applyrequest.DeployerRoleReadiness, _ string, tasks []types.Task) (NativeTargetHealthEvidence, error) {
		evidence := NativeTargetHealthEvidence{}
		for _, task := range tasks {
			evidence.HealthyTaskARNs = append(evidence.HealthyTaskARNs, aws.ToString(task.TaskArn))
		}
		return evidence, nil
	},
		ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (NativeObservationClient, error) {
			return f, nil
		}, CheckCapabilities: func(_ context.Context, _ applyrequest.DeployerRoleReadiness, checks []applyrequest.OperationExecutionCapabilityCheck) error {
			for _, check := range checks {
				if check.Action == "ecs:UpdateService" || check.Action == "iam:PassRole" {
					t.Fatal("native observer requested mutation authority")
				}
			}
			return nil
		}, ObserveRouting: func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error) {
			return f.routing, f.routingError
		}, Timeout: time.Hour, Now: func() time.Time { return now }}
	r.PreparedResult, err = observer.Prepare(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return f, observer, r, &now
}
func TestNativeObservationRequiresAWSCompletionTrafficAndHealthyTasks(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		t.Run(strategy, func(t *testing.T) {
			f, h, r, _ := nativeObserverFixture(t, strategy)
			result, err := h.Reconcile(context.Background(), r)
			if err != nil || !result.Complete || result.Failure != "" || result.Result.Observation.Native.Status != "SUCCESSFUL" || result.Result.Observation.HealthyCount != 2 || aws.ToFloat64(result.Result.Observation.Native.TargetConfiguredPercent) != 100 || f.writes != 1 {
				t.Fatalf("native goal not verified without mutation: %+v %v", result, err)
			}
			// Optional requested-weight snapshots are not current routing proof. Actual
			// ALB weight, AWS success and sampled task health still provide full evidence.
			d := f.deployments[r.PriorResults[1].ServiceDeploymentARN]
			d.TargetServiceRevision.RequestedProductionTrafficWeight = nil
			d.SourceServiceRevisions[0].RequestedProductionTrafficWeight = nil
			f.deployments[r.PriorResults[1].ServiceDeploymentARN] = d
			result, err = h.Reconcile(context.Background(), r)
			if err != nil || !result.Complete || result.Result.Observation.Native.TargetRequestedPercent != nil {
				t.Fatal("missing optional requested-weight snapshot was invented or hid actual convergence")
			}
		})
	}
}
func TestNativeTrafficAndBakeProgressNeverInferCompletion(t *testing.T) {
	for _, stage := range []types.ServiceDeploymentLifecycleStage{types.ServiceDeploymentLifecycleStageScaleUp, types.ServiceDeploymentLifecycleStageProductionTrafficShift, types.ServiceDeploymentLifecycleStageBakeTime} {
		t.Run(string(stage), func(t *testing.T) {
			f, h, r, _ := nativeObserverFixture(t, "CANARY")
			value := r.PriorResults[1].ServiceDeploymentARN
			d := f.deployments[value]
			d.Status = types.ServiceDeploymentStatusInProgress
			d.LifecycleStage = stage
			d.TargetServiceRevision.RequestedProductionTrafficWeight = aws.Float64(5.5)
			d.SourceServiceRevisions[0].RequestedProductionTrafficWeight = aws.Float64(94.5)
			f.deployments[value] = d
			f.routing.Production = ALBWeightPair{Primary: 945, Alternate: 55}
			result, err := h.Reconcile(context.Background(), r)
			if err != nil || result.Complete || result.Failure != "" || result.RetryAfter != 2*time.Second || result.Result.Observation.Native.Stage != string(stage) || aws.ToFloat64(result.Result.Observation.Native.TargetConfiguredPercent) != 5.5 {
				t.Fatalf("native phase was inferred as complete: %+v %v", result, err)
			}
			// Even 100% routing and healthy target tasks cannot complete an AWS bake phase.
			f.routing.Production = ALBWeightPair{Alternate: 100}
			d = f.deployments[value]
			d.TargetServiceRevision.RequestedProductionTrafficWeight = aws.Float64(100)
			d.SourceServiceRevisions[0].RequestedProductionTrafficWeight = aws.Float64(0)
			f.deployments[value] = d
			result, err = h.Reconcile(context.Background(), r)
			if err != nil || result.Complete {
				t.Fatal("100% traffic skipped AWS bake/lifecycle completion")
			}
		})
	}
}
func TestNativeObservationMissingEvidenceCannotBecomeHealthy(t *testing.T) {
	for _, name := range []string{"missing task", "unhealthy essential", "old revision", "pending service", "wrong count", "wrong service target", "source tasks", "wrong traffic", "missing routing", "unknown revision", "wrong revision workload", "foreign target resolution", "native read denied", "task read denied", "unknown traffic", "NaN traffic", "permission revoked", "revision deactivated"} {
		t.Run(name, func(t *testing.T) {
			f, h, r, _ := nativeObserverFixture(t, "BLUE_GREEN")
			value := r.PriorResults[1].ServiceDeploymentARN
			switch name {
			case "missing task":
				delete(f.sampler.tasks, f.sampler.running[0])
			case "unhealthy essential":
				definition := f.definitions[r.PriorResults[0].TaskDefinitionARN].TaskDefinition
				definition.ContainerDefinitions[0].HealthCheck = &types.HealthCheck{Command: []string{"CMD-SHELL", "false"}}
			case "old revision":
				task := f.sampler.tasks[f.sampler.running[0]]
				task.TaskDefinitionArn = aws.String(r.Intent.BaseTaskDefinitionARN)
				f.sampler.tasks[f.sampler.running[0]] = task
			case "pending service":
				f.service.PendingCount = 1
			case "wrong count":
				f.service.DesiredCount = 3
			case "wrong service target":
				f.service.TaskDefinition = aws.String(r.Intent.BaseTaskDefinitionARN)
			case "source tasks":
				d := f.deployments[value]
				d.SourceServiceRevisions[0].RunningTaskCount = 1
				f.deployments[value] = d
			case "wrong traffic":
				f.routing.Production = ALBWeightPair{Primary: 1, Alternate: 1}
			case "missing routing":
				f.routingError = errors.New("ALB unavailable")
			case "unknown revision":
				delete(f.revisions, r.PriorResults[1].ServiceRevisionARN)
			case "wrong revision workload":
				revision := f.revisions[r.PriorResults[1].ServiceRevisionARN]
				revision.TaskDefinition = aws.String(r.Intent.BaseTaskDefinitionARN)
				f.revisions[r.PriorResults[1].ServiceRevisionARN] = revision
			case "foreign target resolution":
				revision := f.revisions[r.PriorResults[1].ServiceRevisionARN]
				revision.ResolvedConfiguration.LoadBalancers[0].TargetGroupArn = aws.String("foreign")
				f.revisions[r.PriorResults[1].ServiceRevisionARN] = revision
			case "native read denied":
				f.readError = errors.New("denied")
			case "task read denied":
				f.sampler.describeError = errors.New("denied")
			case "unknown traffic":
				f.routing = ALBRoutingSnapshot{}
			case "NaN traffic":
				d := f.deployments[value]
				d.TargetServiceRevision.RequestedProductionTrafficWeight = aws.Float64(math.NaN())
				f.deployments[value] = d
			case "permission revoked":
				h.CheckCapabilities = func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
					return errors.New("revoked")
				}
			case "revision deactivated":
				f.definitions[r.PriorResults[0].TaskDefinitionARN].TaskDefinition.Status = types.TaskDefinitionStatusInactive
			}
			result, err := h.Reconcile(context.Background(), r)
			if err != nil || result.Complete || f.writes != 1 {
				t.Fatalf("missing/unhealthy native evidence became success: %+v %v", result, err)
			}
		})
	}
}
func TestNativeObservationReportsAWSFailureRollbackAndDurableDeadline(t *testing.T) {
	for _, status := range []types.ServiceDeploymentStatus{types.ServiceDeploymentStatusStopped, types.ServiceDeploymentStatusRollbackSuccessful, types.ServiceDeploymentStatusRollbackFailed} {
		t.Run(string(status), func(t *testing.T) {
			f, h, r, _ := nativeObserverFixture(t, "LINEAR")
			value := r.PriorResults[1].ServiceDeploymentARN
			d := f.deployments[value]
			d.Status = status
			d.StatusReason = aws.String("AWS alarm or hook failure")
			d.Alarms = &types.ServiceDeploymentAlarms{TriggeredAlarmNames: []string{"health"}}
			f.deployments[value] = d
			result, err := h.Reconcile(context.Background(), r)
			if err != nil || result.Failure == "" || result.Complete || result.Result.Observation.Native.Status != string(status) || len(result.Result.Observation.Native.TriggeredAlarms) != 1 || result.Result.Observation.TasksObserved {
				t.Fatal("native AWS failure was treated as a healthy forward deployment")
			}
		})
	}
	f, h, r, now := nativeObserverFixture(t, "CANARY")
	start := r.PreparedResult.Observation.StartedAt
	deadline := r.PreparedResult.Observation.DeadlineAt
	*now = now.Add(30 * time.Minute)
	resumed, err := h.Prepare(context.Background(), r)
	if err != nil || resumed.Observation.StartedAt != start || resumed.Observation.DeadlineAt != deadline {
		t.Fatal("restart reset native observation deadline")
	}
	*now = now.Add(time.Hour)
	result, err := h.Reconcile(context.Background(), r)
	if err != nil || result.Failure == "" || result.Result.Observation.Reason != "observation_timeout" || f.writes != 1 {
		t.Fatal("native observation did not time out without mutation")
	}
}

func TestNativeObservationRequiresCurrentALBHealthAndNoExtraTargets(t *testing.T) {
	for _, name := range []string{"unhealthy", "unavailable", "foreign member"} {
		t.Run(name, func(t *testing.T) {
			f, h, r, _ := nativeObserverFixture(t, "CANARY")
			h.ObserveTargetHealth = func(_ context.Context, _ domain.ECSDeploymentIntent, _ applyrequest.DeployerRoleReadiness, _ string, tasks []types.Task) (NativeTargetHealthEvidence, error) {
				if name == "unavailable" {
					return NativeTargetHealthEvidence{}, errors.New("ALB health unavailable")
				}
				proof := NativeTargetHealthEvidence{}
				for _, task := range tasks {
					proof.HealthyTaskARNs = append(proof.HealthyTaskARNs, aws.ToString(task.TaskArn))
				}
				if name == "unhealthy" {
					proof.HealthyTaskARNs = proof.HealthyTaskARNs[:1]
				} else {
					proof.UnexpectedTargets = 1
				}
				return proof, nil
			}
			result, err := h.Reconcile(context.Background(), r)
			if err != nil || result.Complete || f.writes != 1 {
				t.Fatal("historical AWS success hid unhealthy/current foreign ALB targets")
			}
			if name == "unavailable" && result.Result.Observation.TasksObserved {
				t.Fatal("unavailable ALB health rendered stale verified task counts")
			}
		})
	}
}

func TestNativePipelineChecksRealTaskToALBHealthBinding(t *testing.T) {
	f, h, r, _ := nativeObserverFixture(t, "CANARY")
	health := &targetHealthFake{t: t, group: r.Intent.Advanced.Routing.AlternateTargetGroupARN}
	for i, value := range f.sampler.running {
		task := f.sampler.tasks[value]
		ip := fmt.Sprintf("10.0.1.%d", i+1)
		task.Containers[0].NetworkInterfaces = []types.NetworkInterface{{PrivateIpv4Address: aws.String(ip)}}
		f.sampler.tasks[value] = task
		health.targets = append(health.targets, elbtypes.TargetHealthDescription{Target: &elbtypes.TargetDescription{Id: aws.String(ip), Port: aws.Int32(r.Intent.Advanced.Routing.ContainerPort)}, TargetHealth: &elbtypes.TargetHealth{State: elbtypes.TargetHealthStateEnumHealthy}})
	}
	probe := ALBTargetHealth{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (TargetHealthClient, error) {
		return health, nil
	}, CheckCapabilities: func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
		return nil
	}}
	h.ObserveTargetHealth = probe.Observe
	result, err := h.Reconcile(context.Background(), r)
	if err != nil || !result.Complete || health.calls != 1 || f.writes != 1 {
		t.Fatal("native update/observation/ALB health pipeline did not verify exact task targets")
	}
	health.targets[0].TargetHealth.State = elbtypes.TargetHealthStateEnumUnhealthy
	result, err = h.Reconcile(context.Background(), r)
	if err != nil || result.Complete || result.Result.Observation.HealthyCount != 1 || f.writes != 1 {
		t.Fatal("current ALB failure was hidden by native completion history")
	}
}
