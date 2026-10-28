package ecsexecution

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

type nativeCapacityFake struct {
	*nativeRollbackFake
	capacityCalls    int
	capacityError    error
	noCapacityEffect bool
}

func (f *nativeCapacityFake) UpdateService(_ context.Context, input *ecs.UpdateServiceInput, opts ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error) {
	expected := &ecs.UpdateServiceInput{Cluster: f.service.ClusterArn, Service: f.service.ServiceArn, DesiredCount: input.DesiredCount}
	if input.DesiredCount == nil || !reflect.DeepEqual(input, expected) {
		return nil, errors.New("only captured desired count may be written")
	}
	options := &ecs.Options{}
	for _, opt := range opts {
		opt(options)
	}
	if options.RetryMaxAttempts != 1 || options.Retryer == nil {
		return nil, errors.New("capacity mutation retries must be disabled")
	}
	f.capacityCalls++
	if !f.noCapacityEffect {
		f.service.DesiredCount = *input.DesiredCount
	}
	if f.capacityError != nil {
		return nil, f.capacityError
	}
	return &ecs.UpdateServiceOutput{Service: &f.service}, nil
}
func nativeCapacityFixture(t *testing.T, strategy string, forwardCount int) (*nativeCapacityFake, NativeCapacityRestore, applyrequest.ECSOperationRequest, *time.Time, func()) {
	t.Helper()
	backend, stop, r, target, _ := nativeRollbackFixture(t, strategy)
	f := &nativeCapacityFake{nativeRollbackFake: backend}
	r.Intent.DesiredCount = forwardCount
	f.service.DesiredCount = int32(forwardCount)
	r.NativeRollback = &target
	r.PriorResults = []applyrequest.ECSOperationResult{rollbackResult(target)}
	r.PreparedResult = applyrequest.ECSOperationResult{}
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	h := NativeCapacityRestore{NativeRollback: NativeRollback{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (NativeRollbackClient, error) { return f, nil }, CheckCapabilities: stop.CheckCapabilities}, Now: func() time.Time { return now }, ObserveRouting: func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error) {
		return ALBRoutingSnapshot{Signature: "current source", Production: ALBWeightPair{Primary: 100}}, nil
	}}
	var err error
	r.PreparedResult, err = h.Prepare(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	complete := func() {
		d := f.deployments[target.DeploymentARN]
		d.Status = types.ServiceDeploymentStatusRollbackSuccessful
		d.Rollback = &types.Rollback{ServiceRevisionArn: aws.String(target.SourceRevisionARN)}
		d.TargetServiceRevision.RunningTaskCount = 0
		d.TargetServiceRevision.PendingTaskCount = 0
		f.deployments[target.DeploymentARN] = d
		source := f.revisions[target.SourceRevisionARN]
		source.ResolvedConfiguration = &types.ResolvedConfiguration{LoadBalancers: []types.ServiceRevisionLoadBalancer{{TargetGroupArn: aws.String(r.Intent.Advanced.Routing.TargetGroupARN), ProductionListenerRule: aws.String(r.Intent.Advanced.Routing.ProductionListenerRuleARN)}}}
		f.revisions[target.SourceRevisionARN] = source
		f.service.TaskDefinition = aws.String(target.SourceTaskDefinitionARN)
	}
	return f, h, r, &now, complete
}
func TestNativeCapacityRecoveryWaitsAndRestoresBothDirectionsWithoutDeployment(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		for _, count := range []int{1, 4} {
			t.Run(strategy+string(rune('0'+count)), func(t *testing.T) {
				f, h, r, _, complete := nativeCapacityFixture(t, strategy, count)
				_, err := h.Execute(context.Background(), r)
				var pending *applyrequest.ECSObservationPending
				if !errors.As(err, &pending) || f.capacityCalls != 0 {
					t.Fatal("capacity changed before source rollback", err)
				}
				r.PreparedResult = pending.Result
				complete()
				result, err := h.Execute(context.Background(), r)
				if err != nil || f.capacityCalls != 1 || f.service.DesiredCount != 2 || result.Observation.TasksObserved {
					t.Fatal("captured capacity not restored or acceptance claimed health", result, err)
				}
				outcome, err := h.Reconcile(context.Background(), r)
				if err != nil || !outcome.Complete || f.capacityCalls != 1 {
					t.Fatal("count update was replayed", outcome, err)
				}
			})
		}
	}
}
func TestNativeCapacityLostResponseAndUncertainAbsenceNeverReplay(t *testing.T) {
	for _, effect := range []bool{false, true} {
		t.Run(map[bool]string{false: "no effect", true: "accepted"}[effect], func(t *testing.T) {
			f, h, r, now, complete := nativeCapacityFixture(t, "CANARY", 4)
			complete()
			f.capacityError = errors.New("response lost")
			f.noCapacityEffect = !effect
			if _, err := h.Execute(context.Background(), r); err == nil {
				t.Fatal("unknown count outcome hidden")
			}
			f.capacityError = nil
			for i := 0; i < 2; i++ {
				outcome, err := h.Reconcile(context.Background(), r)
				if err != nil || outcome.Complete != effect || f.capacityCalls != 1 {
					t.Fatal("count uncertainty replayed or falsely completed", outcome, err)
				}
			}
			if !effect {
				*now = now.Add(37 * time.Hour)
				outcome, err := h.Reconcile(context.Background(), r)
				if err != nil || outcome.Failure == "" || f.capacityCalls != 1 {
					t.Fatal("uncertain recovery deadline reset", outcome, err)
				}
			}
		})
	}
}
func TestNativeCapacityRecoveryFencesCurrentSourceTrafficAndAuthority(t *testing.T) {
	for _, name := range []string{"already restored", "split traffic", "new deployment", "foreign count", "forward tasks", "rollback failed", "source changed", "denied", "role changed", "revoked", "deadline at guard"} {
		t.Run(name, func(t *testing.T) {
			f, h, r, now, complete := nativeCapacityFixture(t, "LINEAR", 4)
			complete()
			switch name {
			case "already restored":
				f.service.DesiredCount = 2
			case "split traffic":
				h.ObserveRouting = func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error) {
					return ALBRoutingSnapshot{Signature: "split", Production: ALBWeightPair{Primary: 95, Alternate: 5}}, nil
				}
			case "new deployment":
				f.addDeployment("external", &ecs.UpdateServiceInput{TaskDefinition: f.service.TaskDefinition, DeploymentConfiguration: f.service.DeploymentConfiguration})
			case "foreign count":
				f.service.DesiredCount = 3
			case "forward tasks":
				d := f.deployments[r.NativeRollback.DeploymentARN]
				d.TargetServiceRevision.RunningTaskCount = 1
				f.deployments[r.NativeRollback.DeploymentARN] = d
			case "rollback failed":
				d := f.deployments[r.NativeRollback.DeploymentARN]
				d.Status = types.ServiceDeploymentStatusRollbackFailed
				f.deployments[r.NativeRollback.DeploymentARN] = d
			case "source changed":
				f.service.TaskDefinition = aws.String(r.NativeRollback.ForwardTaskDefinitionARN)
			case "denied":
				f.readError = errors.New("denied")
			case "role changed":
				r.Role.CheckedWithAWS = false
			case "revoked":
				r.BeforeMutation = func(context.Context) error { return errors.New("revoked") }
			case "deadline at guard":
				r.BeforeMutation = func(context.Context) error { *now = now.Add(37 * time.Hour); return nil }
			}
			result, err := h.Execute(context.Background(), r)
			if f.capacityCalls != 0 {
				t.Fatal("unsafe capacity mutation", name)
			}
			if name == "already restored" {
				if err != nil || result.Observation.DesiredCount != 2 {
					t.Fatal(result, err)
				}
			} else if err == nil {
				t.Fatal("unavailable or unsafe recovery completed", name)
			}
		})
	}
}

func TestNativeCapacityAcceptedWriteCanReconcileWithoutUpdatePermission(t *testing.T) {
	f, h, r, _, complete := nativeCapacityFixture(t, "CANARY", 4)
	complete()
	f.capacityError = errors.New("response lost")
	if _, err := h.Execute(context.Background(), r); err == nil {
		t.Fatal("response loss hidden")
	}
	h.CheckCapabilities = func(_ context.Context, _ applyrequest.DeployerRoleReadiness, checks []applyrequest.OperationExecutionCapabilityCheck) error {
		for _, check := range checks {
			if check.Action == "ecs:UpdateService" {
				return errors.New("write permission revoked")
			}
		}
		return nil
	}
	outcome, err := h.Reconcile(context.Background(), r)
	if err != nil || !outcome.Complete || f.capacityCalls != 1 {
		t.Fatal("read-only capacity verification required mutation permission", outcome, err)
	}
}
