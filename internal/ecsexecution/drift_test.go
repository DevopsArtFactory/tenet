package ecsexecution

import (
	"context"
	"errors"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func TestManagedDriftComparesOnlyReviewedFieldsAndNeverMutates(t *testing.T) {
	fake, _, request := serviceFixture(t)
	target := request.PriorResults[0].TaskDefinitionARN
	fake.service.TaskDefinition = aws.String(target)
	fake.service.DesiredCount = int32(request.Intent.DesiredCount)
	fake.service.DeploymentConfiguration.MinimumHealthyPercent = aws.Int32(int32(request.Intent.MinimumHealthyPercent))
	fake.service.DeploymentConfiguration.MaximumPercent = aws.Int32(int32(request.Intent.MaximumPercent))
	fake.service.Deployments[0].TaskDefinition = aws.String(target)
	verified := applyrequest.Record{Status: applyrequest.StatusSucceeded, ECSRuntime: &applyrequest.ECSRolloutObservation{State: "converged"}, Operations: make([]applyrequest.Operation, 3)}
	verified.Operations[0].ECS = request.Intent.Clone()
	verified.Operations[0].ECSCheckpoint = &applyrequest.ECSCheckpoint{State: "complete", Result: applyrequest.ECSOperationResult{TaskDefinitionARN: target}}
	checks := 0
	probe := ManagedDrift{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (DriftClient, error) { return fake, nil }, CheckCapabilities: func(_ context.Context, _ applyrequest.DeployerRoleReadiness, c []applyrequest.OperationExecutionCapabilityCheck) error {
		checks++
		if len(c) != 1 || c[0].Action != "ecs:DescribeServices" || c[0].ResourceName != request.Intent.ServiceARN {
			t.Fatal("probe requested mutation permission")
		}
		return nil
	}}
	detected, signature, err := probe.Observe(context.Background(), request.Intent, request.Role, verified)
	if err != nil || detected || signature != "" || checks != 1 {
		t.Fatalf("matching service not clean: %v %v", detected, err)
	}
	// Health alone is handled by observation; it cannot trigger another deployment.
	fake.service.RunningCount = 0
	detected, _, err = probe.Observe(context.Background(), request.Intent, request.Role, verified)
	if err != nil || detected {
		t.Fatal("health-only change triggered self heal")
	}
	fake.service.DesiredCount++
	detected, signature, err = probe.Observe(context.Background(), request.Intent, request.Role, verified)
	if err != nil || !detected || signature == "" {
		t.Fatal("managed count drift missed")
	}
	_, same, err := probe.Observe(context.Background(), request.Intent, request.Role, verified)
	if err != nil || same != signature {
		t.Fatal("drift signature unstable")
	}
	fake.service.TaskDefinition = aws.String(request.Intent.BaseTaskDefinitionARN)
	detected, changed, err := probe.Observe(context.Background(), request.Intent, request.Role, verified)
	if err != nil || !detected || changed == signature {
		t.Fatal("revision drift omitted")
	}
	if fake.writes != 0 {
		t.Fatal("read-only observation updated AWS")
	}
}
func TestManagedDriftRejectsUnknownDeniedAndConflictingRollouts(t *testing.T) {
	for _, name := range []string{"missing", "wrong placement", "cross role", "unverified role", "missing config", "in progress", "active old revision", "missing primary", "failed apply", "denied", "unavailable"} {
		t.Run(name, func(t *testing.T) {
			fake, _, request := serviceFixture(t)
			verified := applyrequest.Record{Status: applyrequest.StatusSucceeded, ECSRuntime: &applyrequest.ECSRolloutObservation{State: "converged"}, Operations: make([]applyrequest.Operation, 3)}
			verified.Operations[0].ECS = request.Intent.Clone()
			verified.Operations[0].ECSCheckpoint = &applyrequest.ECSCheckpoint{State: "complete", Result: request.PriorResults[0]}
			probe := ManagedDrift{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (DriftClient, error) { return fake, nil }, CheckCapabilities: func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
				return nil
			}}
			switch name {
			case "missing":
				fake.missing = true
			case "wrong placement":
				fake.service.ClusterArn = aws.String("wrong")
			case "cross role":
				request.Role.RoleARN = "arn:aws:iam::999999999999:role/other"
			case "unverified role":
				request.Role.CheckedWithAWS = false
			case "missing config":
				fake.service.DeploymentConfiguration.MaximumPercent = nil
			case "in progress":
				fake.service.Deployments[0].RolloutState = types.DeploymentRolloutStateInProgress
			case "active old revision":
				fake.service.Deployments = append(fake.service.Deployments, types.Deployment{Status: aws.String("ACTIVE"), RunningCount: 1})
			case "missing primary":
				fake.service.Deployments = nil
			case "failed apply":
				verified.Status = applyrequest.StatusFailed
			case "denied":
				probe.CheckCapabilities = func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
					return errors.New("denied")
				}
			case "unavailable":
				probe.ClientFor = func(context.Context, applyrequest.DeployerRoleReadiness) (DriftClient, error) {
					return nil, errors.New("unavailable")
				}
			}
			detected, signature, err := probe.Observe(context.Background(), request.Intent, request.Role, verified)
			if err == nil || detected || signature != "" || fake.writes != 0 {
				t.Fatalf("untrusted observation authorized self heal: %v %v", detected, err)
			}
		})
	}
}
