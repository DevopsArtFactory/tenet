package ecsexecution

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func nativeDriftFixture(t *testing.T, strategy string) (*nativeObservationFake, NativeManagedDrift, applyrequest.ECSOperationRequest, applyrequest.Record) {
	t.Helper()
	f, observer, r, _ := nativeObserverFixture(t, strategy)
	r.Intent.SelfHeal = true
	r.Intent.SyncMode = "automatic"
	outcome, err := observer.Reconcile(context.Background(), r)
	if err != nil || !outcome.Complete {
		t.Fatal(outcome, err)
	}
	for i := range f.history {
		f.history[i].Status = f.deployments[aws.ToString(f.history[i].ServiceDeploymentArn)].Status
	}
	verified := applyrequest.Record{ID: "verified-native", Status: applyrequest.StatusSucceeded, Principal: &applyrequest.ExecutionPrincipal{Identity: requestactor.AuthenticatedIdentity{Authenticated: true, ID: "operator"}}, ECSRuntime: outcome.Result.Observation, Operations: []applyrequest.Operation{
		{Kind: "RegisterECSTaskDefinition", ECS: r.Intent.Clone(), ECSCheckpoint: &applyrequest.ECSCheckpoint{State: "complete", Result: r.PriorResults[0]}},
		{Kind: "UpdateECSService", ECS: r.Intent.Clone(), ECSCheckpoint: &applyrequest.ECSCheckpoint{State: "complete", Result: r.PriorResults[1]}},
		{Kind: "VerifyServiceStability", ECS: r.Intent.Clone(), ECSCheckpoint: &applyrequest.ECSCheckpoint{State: "complete", Result: outcome.Result}},
	}}
	probe := NativeManagedDrift{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (NativeDriftClient, error) { return f, nil }, CheckCapabilities: func(_ context.Context, _ applyrequest.DeployerRoleReadiness, checks []applyrequest.OperationExecutionCapabilityCheck) error {
		for _, c := range checks {
			if c.Action == "ecs:UpdateService" || c.Action == "ecs:StopServiceDeployment" || c.Action == "iam:PassRole" {
				t.Fatal("read-only drift probe requested mutation capabilities")
			}
		}
		return nil
	}, ObserveRouting: func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error) {
		return f.routing, f.routingError
	}}
	return f, probe, r, verified
}
func TestNativeDriftDetectsManagedFieldsAndIgnoresHealthAndRollingBounds(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		for _, field := range []string{"healthy", "count", "bake", "alarms", "hooks", "traffic", "task", "health only", "rolling bounds"} {
			t.Run(strategy+"/"+field, func(t *testing.T) {
				f, p, r, v := nativeDriftFixture(t, strategy)
				switch field {
				case "count":
					f.service.DesiredCount++
				case "bake":
					f.service.DeploymentConfiguration.BakeTimeInMinutes = aws.Int32(29)
				case "alarms":
					f.service.DeploymentConfiguration.Alarms.AlarmNames = []string{"external"}
				case "hooks":
					f.service.DeploymentConfiguration.LifecycleHooks[0].HookTargetArn = aws.String("arn:aws:lambda:ap-northeast-2:123456789012:function:external:7")
				case "traffic":
					f.routing.Production = ALBWeightPair{Primary: 1, Alternate: 99}
				case "health only":
					f.service.RunningCount = 1
				case "rolling bounds":
					f.service.DeploymentConfiguration.MaximumPercent = aws.Int32(123)
				case "task":
					d := f.deployments[v.Operations[1].ECSCheckpoint.Result.ServiceDeploymentARN]
					d.ServiceDeploymentArn = aws.String(strings.Replace(aws.ToString(d.ServiceDeploymentArn), "new-1", "external", 1))
					d.TargetServiceRevision = &types.ServiceRevisionSummary{Arn: aws.String(strings.Replace(aws.ToString(d.TargetServiceRevision.Arn), "new-1", "external", 1))}
					later := d.CreatedAt.Add(time.Minute)
					d.CreatedAt = &later
					f.deployments[aws.ToString(d.ServiceDeploymentArn)] = d
					f.history = append(f.history, types.ServiceDeploymentBrief{ServiceDeploymentArn: d.ServiceDeploymentArn, TargetServiceRevisionArn: d.TargetServiceRevision.Arn, ServiceArn: d.ServiceArn, ClusterArn: d.ClusterArn, CreatedAt: d.CreatedAt, Status: types.ServiceDeploymentStatusSuccessful})
					oldRef := f.revisions[v.Operations[1].ECSCheckpoint.Result.ServiceRevisionARN]
					oldRef.ServiceRevisionArn = d.TargetServiceRevision.Arn
					oldRef.TaskDefinition = aws.String(strings.Replace(r.Intent.BaseTaskDefinitionARN, ":17", ":99", 1))
					f.revisions[aws.ToString(d.TargetServiceRevision.Arn)] = oldRef
					f.service.TaskDefinition = oldRef.TaskDefinition
				}
				found, sig, err := p.Observe(context.Background(), r.Intent, r.Role, v)
				expected := field != "healthy" && field != "health only" && field != "rolling bounds"
				if err != nil || found != expected || ((sig != "") != expected) || f.writes != 1 {
					t.Fatalf("drift %s: %v %s %v", field, found, sig, err)
				}
				if expected {
					again, stable, err := p.Observe(context.Background(), r.Intent, r.Role, v)
					if err != nil || !again || stable != sig || len(sig) != 64 {
						t.Fatal("drift fingerprint was unstable", stable, err)
					}
				}
			})
		}
	}
}
func TestNativeDriftRefusesActiveUnknownAndUnsupportedEvidence(t *testing.T) {
	for _, name := range []string{"unverified role", "failed verification", "recovery verification", "missing checkpoint", "wrong verification intent", "active rollout", "rollback active", "rollback failed", "stopped", "pending tasks", "source not drained", "foreign binding", "strategy migration", "unknown history", "unknown routing", "read denied", "uncorrelated workload", "missing resolution"} {
		t.Run(name, func(t *testing.T) {
			f, p, r, v := nativeDriftFixture(t, "CANARY")
			id := v.Operations[1].ECSCheckpoint.Result.ServiceDeploymentARN
			d := f.deployments[id]
			f.service.DesiredCount++
			switch name {
			case "unverified role":
				r.Role.CheckedWithAWS = false
			case "failed verification":
				v.Status = applyrequest.StatusFailed
			case "recovery verification":
				v.NativeRollback = &applyrequest.ECSNativeRollback{ParentApplyID: "forward"}
			case "missing checkpoint":
				v.Operations[0].ECSCheckpoint = nil
			case "wrong verification intent":
				v.Operations[0].ECS.Image = "example/unreviewed:tag"
			case "active rollout":
				f.history[len(f.history)-1].Status = types.ServiceDeploymentStatusInProgress
			case "rollback active":
				d.Status = types.ServiceDeploymentStatusRollbackInProgress
			case "rollback failed":
				d.Status = types.ServiceDeploymentStatusRollbackFailed
			case "stopped":
				d.Status = types.ServiceDeploymentStatusStopped
			case "pending tasks":
				f.service.PendingCount = 1
			case "source not drained":
				d.SourceServiceRevisions[0].RunningTaskCount = 1
			case "foreign binding":
				f.service.LoadBalancers = nil
			case "strategy migration":
				f.service.DeploymentConfiguration.Strategy = types.DeploymentStrategyRolling
			case "unknown history":
				f.history = nil
			case "unknown routing":
				f.routing.Signature = ""
			case "read denied":
				f.readError = errors.New("denied")
			case "uncorrelated workload":
				f.service.TaskDefinition = aws.String(strings.Replace(r.Intent.BaseTaskDefinitionARN, ":17", ":99", 1))
			case "missing resolution":
				ref := f.revisions[v.Operations[1].ECSCheckpoint.Result.ServiceRevisionARN]
				ref.ResolvedConfiguration = nil
				f.revisions[v.Operations[1].ECSCheckpoint.Result.ServiceRevisionARN] = ref
			}
			f.deployments[id] = d
			found, sig, err := p.Observe(context.Background(), r.Intent, r.Role, v)
			if err == nil || found || sig != "" || f.writes != 1 {
				t.Fatal("unavailable/active drift authorized repair", name, found, sig, err)
			}
		})
	}
}
func TestNativeManagedRepairForcesOnlyNecessaryDeploymentAndRestoresCount(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		for _, kind := range []string{"count", "traffic", "disappeared"} {
			t.Run(strategy+"/"+kind, func(t *testing.T) {
				f, _, r, v := nativeDriftFixture(t, strategy)
				r.PriorResults = r.PriorResults[:1]
				r.PreparedResult = applyrequest.ECSOperationResult{}
				r.ManagedRepair = true
				f.sourceOverride = v.Operations[1].ECSCheckpoint.Result.ServiceRevisionARN
				if kind == "count" {
					f.service.DesiredCount = 3
				} else if kind == "traffic" {
					f.routing.Production = ALBWeightPair{Primary: 20, Alternate: 80}
				}
				h := AdvancedService{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (NativeServiceClient, error) {
					return f.nativeFake, nil
				}, CheckCapabilities: func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
					return nil
				}, ObserveRouting: func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error) {
					return f.routing, nil
				}}
				var err error
				r.PreparedResult, err = h.Prepare(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				before := f.writes
				result, err := h.Execute(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "disappeared" {
					if f.writes != before || result.NativeService.NoOpDeploymentARN == "" {
						t.Fatal("vanished drift forced a deployment")
					}
					return
				}
				if f.writes != before+1 || !f.input.ForceNewDeployment || result.ServiceDeploymentARN == v.Operations[1].ECSCheckpoint.Result.ServiceDeploymentARN {
					t.Fatal("native repair adopted stale success instead of creating a traffic deployment")
				}
				if kind == "count" && (f.input.DesiredCount == nil || *f.input.DesiredCount != 2) {
					t.Fatal("managed capacity was not restored")
				}
				if kind == "traffic" && f.input.DesiredCount != nil {
					t.Fatal("traffic repair changed capacity")
				}
				if len(f.input.LoadBalancers) > 0 || f.input.NetworkConfiguration != nil || f.input.DeploymentConfiguration.MinimumHealthyPercent != nil {
					t.Fatal("native repair wrote unmanaged service fields")
				}
			})
		}
	}
}

func TestNativeRepairDoesNotFightResolvedDrift(t *testing.T) {
	f, _, r, v := nativeDriftFixture(t, "CANARY")
	r.PriorResults = r.PriorResults[:1]
	r.PreparedResult = applyrequest.ECSOperationResult{}
	f.service.DesiredCount = 3
	h := AdvancedService{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (NativeServiceClient, error) {
		return f.nativeFake, nil
	}, CheckCapabilities: func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
		return nil
	}, ObserveRouting: func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error) {
		return f.routing, nil
	}}
	if err := h.Preflight(context.Background(), r); err != nil {
		t.Fatal("reviewed capacity change was rejected", err)
	}
	r.ManagedRepair = true
	var err error
	r.PreparedResult, err = h.Prepare(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	f.service.DesiredCount = 2
	result, err := h.Execute(context.Background(), r)
	if err != nil || f.writes != 1 || result.NativeService.NoOpDeploymentARN != v.Operations[1].ECSCheckpoint.Result.ServiceDeploymentARN {
		t.Fatal("resolved drift after preparation was overwritten", result, err)
	}
	r.Intent.SelfHeal = false
	if err := h.Preflight(context.Background(), r); err == nil {
		t.Fatal("repair flag bypassed explicit self-heal policy")
	}
}

func TestNativeDriftUsesCompletedRollbackSourceAndCurrentTargetColor(t *testing.T) {
	f, p, r, v := nativeDriftFixture(t, "CANARY")
	id := v.Operations[1].ECSCheckpoint.Result.ServiceDeploymentARN
	d := f.deployments[id]
	d.Status = types.ServiceDeploymentStatusRollbackSuccessful
	d.Rollback = &types.Rollback{ServiceRevisionArn: d.SourceServiceRevisions[0].Arn}
	d.TargetServiceRevision.RunningTaskCount = 0
	f.deployments[id] = d
	for i := range f.history {
		if aws.ToString(f.history[i].ServiceDeploymentArn) == id {
			f.history[i].Status = d.Status
		}
	}
	sourceID := aws.ToString(d.SourceServiceRevisions[0].Arn)
	source := f.revisions[sourceID]
	source.ResolvedConfiguration = &types.ResolvedConfiguration{LoadBalancers: []types.ServiceRevisionLoadBalancer{{TargetGroupArn: aws.String(r.Intent.Advanced.Routing.TargetGroupARN), ProductionListenerRule: aws.String(r.Intent.Advanced.Routing.ProductionListenerRuleARN)}}}
	f.revisions[sourceID] = source
	f.service.TaskDefinition = source.TaskDefinition
	f.routing.Production = ALBWeightPair{Primary: 100}
	found, sig, err := p.Observe(context.Background(), r.Intent, r.Role, v)
	if err != nil || !found || sig == "" {
		t.Fatal("completed source rollback was not correlated", found, sig, err)
	}
	// The same current source can be the last verified workload; its rotated color
	// must not itself create drift relative to an older target-group assignment.
	for _, i := range []int{0, 1} {
		v.Operations[i].ECSCheckpoint.Result.TaskDefinitionARN = aws.ToString(source.TaskDefinition)
	}
	v.ECSRuntime.TaskDefinitionARN = aws.ToString(source.TaskDefinition)
	found, sig, err = p.Observe(context.Background(), r.Intent, r.Role, v)
	if err != nil || found || sig != "" {
		t.Fatal("old target color forced repair", found, sig, err)
	}
}
