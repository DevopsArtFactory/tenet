package ecsexecution

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

var _ applyrequest.ECSOperationHandler = RollingService{}
var _ applyrequest.ECSOperationPreparer = RollingService{}

type serviceFake struct {
	*registrationFake
	service       types.Service
	writes        int
	input         *ecs.UpdateServiceInput
	updateError   error
	missing       bool
	noEffect      bool
	singleAttempt bool
	lagPrimary    bool
}

func (f *serviceFake) DescribeServices(context.Context, *ecs.DescribeServicesInput, ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
	if f.missing {
		return &ecs.DescribeServicesOutput{Failures: []types.Failure{{Reason: aws.String("MISSING")}}}, nil
	}
	content, _ := json.Marshal(f.service)
	var service types.Service
	_ = json.Unmarshal(content, &service)
	return &ecs.DescribeServicesOutput{Services: []types.Service{service}}, nil
}
func (f *serviceFake) UpdateService(_ context.Context, input *ecs.UpdateServiceInput, options ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error) {
	f.writes++
	var opts ecs.Options
	for _, option := range options {
		option(&opts)
	}
	f.singleAttempt = opts.RetryMaxAttempts == 1 && opts.Retryer.MaxAttempts() == 1
	f.input = input
	if !f.noEffect {
		f.service.TaskDefinition = input.TaskDefinition
		f.service.DesiredCount = aws.ToInt32(input.DesiredCount)
		f.service.DeploymentConfiguration = input.DeploymentConfiguration
		f.service.Deployments = []types.Deployment{{Id: aws.String("ecs-svc/new"), Status: aws.String("PRIMARY"), TaskDefinition: input.TaskDefinition, RolloutState: types.DeploymentRolloutStateInProgress}}
	}
	if f.updateError != nil {
		return nil, f.updateError
	}
	if f.lagPrimary {
		response := f.service
		response.Deployments = []types.Deployment{{Id: aws.String("ecs-svc/old"), Status: aws.String("PRIMARY"), TaskDefinition: aws.String("arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:17")}}
		return &ecs.UpdateServiceOutput{Service: &response}, nil
	}
	return &ecs.UpdateServiceOutput{Service: &f.service}, nil
}

func serviceFixture(t *testing.T) (*serviceFake, RollingService, applyrequest.ECSOperationRequest) {
	t.Helper()
	registration, register, request := registrationFixture()
	request.Intent.ClusterARN = "arn:aws:ecs:ap-northeast-2:123456789012:cluster/production"
	request.Intent.ServiceARN = "arn:aws:ecs:ap-northeast-2:123456789012:service/production/worker"
	request.Intent.DesiredCount = 2
	request.Intent.MinimumHealthyPercent = 100
	request.Intent.MaximumPercent = 200
	base := registration.definitions[request.Intent.BaseTaskDefinitionARN].TaskDefinition
	base.ContainerDefinitions[0].PortMappings = []types.PortMapping{{ContainerPort: aws.Int32(8080)}}
	result, err := register.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PriorResults = []applyrequest.ECSOperationResult{result}
	request.OperationID = "update"
	request.RequestID = "tenet-update-request"
	fake := &serviceFake{registrationFake: registration, service: types.Service{ServiceArn: aws.String(request.Intent.ServiceARN), ClusterArn: aws.String(request.Intent.ClusterARN), Status: aws.String("ACTIVE"), DeploymentController: &types.DeploymentController{Type: types.DeploymentControllerTypeEcs}, SchedulingStrategy: types.SchedulingStrategyReplica, TaskDefinition: aws.String(request.Intent.BaseTaskDefinitionARN), DesiredCount: 1, RunningCount: 1, LaunchType: types.LaunchTypeFargate, PlatformFamily: aws.String("Linux"), PlatformVersion: aws.String("1.4.0"), NetworkConfiguration: &types.NetworkConfiguration{AwsvpcConfiguration: &types.AwsVpcConfiguration{Subnets: []string{"subnet-a"}, SecurityGroups: []string{"sg-a"}}}, LoadBalancers: []types.LoadBalancer{{ContainerName: aws.String("worker"), ContainerPort: aws.Int32(8080), TargetGroupArn: aws.String("arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/worker/abc")}}, DeploymentConfiguration: &types.DeploymentConfiguration{Strategy: types.DeploymentStrategyRolling, MinimumHealthyPercent: aws.Int32(50), MaximumPercent: aws.Int32(150), DeploymentCircuitBreaker: &types.DeploymentCircuitBreaker{Enable: true, Rollback: true}, Alarms: &types.DeploymentAlarms{Enable: true, Rollback: true, AlarmNames: []string{"existing-alarm"}}}, Deployments: []types.Deployment{{Id: aws.String("ecs-svc/old"), Status: aws.String("PRIMARY"), TaskDefinition: aws.String(request.Intent.BaseTaskDefinitionARN), RolloutState: types.DeploymentRolloutStateCompleted}}}}
	handler := RollingService{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (ServiceClient, error) { return fake, nil }, CheckCapabilities: func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
		return nil
	}}
	return fake, handler, request
}

func TestRollingServiceUsesExactReviewedFieldsAndPreservesOtherSettings(t *testing.T) {
	fake, handler, request := serviceFixture(t)
	initialConfig := fake.service.DeploymentConfiguration
	if err := handler.Preflight(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	prepared, err := handler.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if fake.writes != 0 || prepared.PreviousService == nil || prepared.PreviousService.TaskDefinitionARN != request.Intent.BaseTaskDefinitionARN || prepared.PreviousService.DesiredCount != 1 || prepared.PreviousService.MinimumHealthyPercent != 50 || prepared.PreviousService.PrimaryDeploymentID != "ecs-svc/old" {
		t.Fatalf("missing pre-update state: %+v", prepared)
	}
	request.PreparedResult = prepared
	result, err := handler.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	input := fake.input
	if fake.writes != 1 || !fake.singleAttempt || aws.ToString(input.Cluster) != request.Intent.ClusterARN || aws.ToString(input.Service) != request.Intent.ServiceARN || aws.ToString(input.TaskDefinition) != request.PriorResults[0].TaskDefinitionARN || aws.ToInt32(input.DesiredCount) != 2 || aws.ToInt32(input.DeploymentConfiguration.MinimumHealthyPercent) != 100 || aws.ToInt32(input.DeploymentConfiguration.MaximumPercent) != 200 {
		t.Fatalf("wrong update request: %+v", input)
	}
	if input.ForceNewDeployment || input.NetworkConfiguration != nil || input.LoadBalancers != nil || input.DeploymentController != nil || input.CapacityProviderStrategy != nil || input.PlatformVersion != nil || input.ServiceRegistries != nil {
		t.Fatal("update changed unreviewed service fields")
	}
	if !reflect.DeepEqual(input.DeploymentConfiguration.Alarms, initialConfig.Alarms) || !reflect.DeepEqual(input.DeploymentConfiguration.DeploymentCircuitBreaker, initialConfig.DeploymentCircuitBreaker) || aws.ToInt32(initialConfig.MinimumHealthyPercent) != 50 {
		t.Fatal("update lost/mutated existing deployment settings")
	}
	if result.ServiceDeploymentID != "ecs-svc/new" || result.PreviousTaskDefinitionARN != request.Intent.BaseTaskDefinitionARN || result.PreviousService.DesiredCount != 1 || fake.service.Deployments[0].RolloutState != types.DeploymentRolloutStateInProgress {
		t.Fatal("update acceptance lost correlation/rollback state or claimed convergence")
	}
	// Acceptance is not convergence: the returned primary rollout remains in progress.
	if _, err := handler.Execute(context.Background(), request); err != nil || fake.writes != 1 {
		t.Fatalf("already matching service started another rollout: %v", err)
	}
}

func TestRollingServiceReconcilesAmbiguousAcceptanceWithoutRepeatingUpdate(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		fake, handler, request := serviceFixture(t)
		prepared, err := handler.Prepare(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		request.PreparedResult = prepared
		fake.updateError = errors.New("connection lost after send")
		fake.noEffect = !accepted
		if _, err := handler.Execute(context.Background(), request); err == nil {
			t.Fatal("expected uncertain response")
		}
		result, err := handler.Reconcile(context.Background(), request)
		if err != nil || result.Complete != accepted || result.Absent || fake.writes != 1 {
			t.Fatalf("ambiguous update was replayed or falsely absent: %+v %v", result, err)
		}
	}
}

func TestRollingServiceRejectsConflictsMissingServicesAndUnsupportedControllers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*serviceFake, *RollingService, *applyrequest.ECSOperationRequest)
	}{
		{"service missing", func(f *serviceFake, _ *RollingService, _ *applyrequest.ECSOperationRequest) { f.missing = true }},
		{"CodeDeploy controller", func(f *serviceFake, _ *RollingService, _ *applyrequest.ECSOperationRequest) {
			f.service.DeploymentController.Type = types.DeploymentControllerTypeCodeDeploy
		}},
		{"native BlueGreen", func(f *serviceFake, _ *RollingService, _ *applyrequest.ECSOperationRequest) {
			f.service.DeploymentConfiguration.Strategy = types.DeploymentStrategyBlueGreen
		}},
		{"DAEMON service", func(f *serviceFake, _ *RollingService, _ *applyrequest.ECSOperationRequest) {
			f.service.SchedulingStrategy = types.SchedulingStrategyDaemon
		}},
		{"conflicting active rollout", func(f *serviceFake, _ *RollingService, _ *applyrequest.ECSOperationRequest) {
			f.service.Deployments[0].RolloutState = types.DeploymentRolloutStateInProgress
		}},
		{"no service network", func(f *serviceFake, _ *RollingService, _ *applyrequest.ECSOperationRequest) {
			f.service.NetworkConfiguration = nil
		}},
		{"runtime OS conflict", func(f *serviceFake, _ *RollingService, _ *applyrequest.ECSOperationRequest) {
			f.service.PlatformFamily = aws.String("Windows Server 2022 Full")
		}},
		{"missing LB port", func(f *serviceFake, _ *RollingService, r *applyrequest.ECSOperationRequest) {
			f.definitions[r.PriorResults[0].TaskDefinitionARN].TaskDefinition.ContainerDefinitions[0].PortMappings = nil
		}},
		{"denied update", func(_ *serviceFake, h *RollingService, _ *applyrequest.ECSOperationRequest) {
			h.CheckCapabilities = func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
				return errors.New("UpdateService denied")
			}
		}},
		{"wrong registered placement", func(_ *serviceFake, _ *RollingService, r *applyrequest.ECSOperationRequest) {
			r.PriorResults[0].TaskDefinitionARN = "arn:aws:ecs:ap-northeast-2:999999999999:task-definition/worker:18"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake, handler, request := serviceFixture(t)
			tc.change(fake, &handler, &request)
			if _, err := handler.Prepare(context.Background(), request); err == nil || fake.writes != 0 {
				t.Fatal("unsafe service update was prepared")
			}
		})
	}
	fake, handler, request := serviceFixture(t)
	if _, err := handler.Execute(context.Background(), request); err == nil || fake.writes != 0 {
		t.Fatal("update proceeded without durable pre-update state")
	}
	prepared, err := handler.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PreparedResult = prepared
	fake.service.DesiredCount = 4
	if _, err := handler.Execute(context.Background(), request); err == nil || fake.writes != 0 {
		t.Fatal("update overwrote out-of-band changes after snapshot")
	}
}

func TestRollingServiceReconcilesObservedFailure(t *testing.T) {
	fake, handler, request := serviceFixture(t)
	prepared, err := handler.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PreparedResult = prepared
	fake.service.Deployments = append(fake.service.Deployments, types.Deployment{Id: aws.String("ecs-svc/failed"), TaskDefinition: aws.String(request.PriorResults[0].TaskDefinitionARN), RolloutState: types.DeploymentRolloutStateFailed, RolloutStateReason: aws.String("health checks failed")})
	result, err := handler.Reconcile(context.Background(), request)
	if err != nil || result.Failure == "" || result.Complete || fake.writes != 0 {
		t.Fatalf("known failed deployment remained ambiguous: %+v %v", result, err)
	}
}

func TestRollingServicePreflightsInheritedTemplateAndScopedPassRole(t *testing.T) {
	fake, handler, request := serviceFixture(t)
	var capabilities []applyrequest.OperationExecutionCapabilityCheck
	handler.CheckCapabilities = func(_ context.Context, _ applyrequest.DeployerRoleReadiness, checks []applyrequest.OperationExecutionCapabilityCheck) error {
		capabilities = checks
		return nil
	}
	if err := handler.Preflight(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	roles := map[string]bool{}
	for _, check := range capabilities {
		if check.Action == "iam:PassRole" {
			roles[check.ResourceName] = true
		}
	}
	if !roles["arn:aws:iam::123456789012:role/task"] || !roles["arn:aws:iam::123456789012:role/execution"] || len(roles) != 2 {
		t.Fatal("update omitted scoped inherited role checks")
	}
	request.PriorResults = nil
	fake.definitions[request.Intent.BaseTaskDefinitionARN].TaskDefinition.Status = types.TaskDefinitionStatusInactive
	if err := handler.Preflight(context.Background(), request); err != nil || fake.writes != 0 {
		t.Fatalf("inactive pinned template could not be inherited into a new active revision: %v", err)
	}
}

func TestRollingServiceDoesNotCorrelateAnOldPrimaryFromLaggingResponse(t *testing.T) {
	fake, handler, request := serviceFixture(t)
	prepared, err := handler.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PreparedResult = prepared
	fake.lagPrimary = true
	result, err := handler.Execute(context.Background(), request)
	if err != nil || result.ServiceDeploymentID != "" {
		t.Fatalf("correlated old primary to the requested revision: %+v %v", result, err)
	}
	resolved, err := handler.Reconcile(context.Background(), request)
	if err != nil || !resolved.Complete || resolved.Result.ServiceDeploymentID != "ecs-svc/new" || fake.writes != 1 {
		t.Fatalf("could not resolve intended primary without another update: %+v %v", resolved, err)
	}
}
