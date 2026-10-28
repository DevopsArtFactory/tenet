package ecsexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

var _ applyrequest.ECSOperationHandler = AdvancedService{}
var _ applyrequest.ECSOperationPreparer = AdvancedService{}

type nativeFake struct {
	*serviceFake
	history          []types.ServiceDeploymentBrief
	deployments      map[string]types.ServiceDeployment
	revisions        map[string]types.ServiceRevision
	request          applyrequest.ECSOperationRequest
	readError        error
	pages            bool
	routingSignature string
	guards           int
	sourceOverride   string
}

func (f *nativeFake) ListServiceDeployments(_ context.Context, input *ecs.ListServiceDeploymentsInput, _ ...func(*ecs.Options)) (*ecs.ListServiceDeploymentsOutput, error) {
	if f.readError != nil {
		return nil, f.readError
	}
	if aws.ToString(input.Service) != f.request.Intent.ServiceARN || aws.ToString(input.Cluster) != f.request.Intent.ClusterARN {
		return nil, errors.New("unscoped native list")
	}
	if f.pages && len(f.history) > 1 && input.NextToken == nil {
		return &ecs.ListServiceDeploymentsOutput{ServiceDeployments: f.history[:1], NextToken: aws.String("next")}, nil
	}
	if f.pages && len(f.history) > 1 && aws.ToString(input.NextToken) == "next" {
		return &ecs.ListServiceDeploymentsOutput{ServiceDeployments: f.history[1:]}, nil
	}
	return &ecs.ListServiceDeploymentsOutput{ServiceDeployments: f.history}, nil
}
func (f *nativeFake) DescribeServiceDeployments(_ context.Context, input *ecs.DescribeServiceDeploymentsInput, _ ...func(*ecs.Options)) (*ecs.DescribeServiceDeploymentsOutput, error) {
	if f.readError != nil {
		return nil, f.readError
	}
	if len(input.ServiceDeploymentArns) != 1 {
		return nil, errors.New("exact deployment ARN required")
	}
	value, ok := f.deployments[input.ServiceDeploymentArns[0]]
	if !ok {
		return &ecs.DescribeServiceDeploymentsOutput{Failures: []types.Failure{{Reason: aws.String("MISSING")}}}, nil
	}
	return &ecs.DescribeServiceDeploymentsOutput{ServiceDeployments: []types.ServiceDeployment{value}}, nil
}
func (f *nativeFake) DescribeServiceRevisions(_ context.Context, input *ecs.DescribeServiceRevisionsInput, _ ...func(*ecs.Options)) (*ecs.DescribeServiceRevisionsOutput, error) {
	if f.readError != nil {
		return nil, f.readError
	}
	if len(input.ServiceRevisionArns) != 1 {
		return nil, errors.New("exact revision ARN required")
	}
	value, ok := f.revisions[input.ServiceRevisionArns[0]]
	if !ok {
		return &ecs.DescribeServiceRevisionsOutput{Failures: []types.Failure{{Reason: aws.String("MISSING")}}}, nil
	}
	return &ecs.DescribeServiceRevisionsOutput{ServiceRevisions: []types.ServiceRevision{value}}, nil
}
func (f *nativeFake) UpdateService(ctx context.Context, input *ecs.UpdateServiceInput, options ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error) {
	previous := f.service.DeploymentConfiguration
	previousCount := f.service.DesiredCount
	output, err := f.serviceFake.UpdateService(ctx, input, options...)
	if !f.noEffect {
		// Model ECS partial update semantics rather than replacing omitted fields.
		if input.DesiredCount == nil {
			f.service.DesiredCount = previousCount
		}
		content, _ := json.Marshal(input.DeploymentConfiguration)
		var actual types.DeploymentConfiguration
		_ = json.Unmarshal(content, &actual)
		if actual.MinimumHealthyPercent == nil {
			actual.MinimumHealthyPercent = previous.MinimumHealthyPercent
		}
		if actual.MaximumPercent == nil {
			actual.MaximumPercent = previous.MaximumPercent
		}
		f.service.DeploymentConfiguration = &actual
		if output != nil {
			output.Service = &f.service
		}
		f.addDeployment(fmt.Sprintf("new-%d", f.writes), input)
	}
	return output, err
}
func (f *nativeFake) addDeployment(id string, input *ecs.UpdateServiceInput) {
	prefix := "arn:aws:ecs:ap-northeast-2:123456789012:"
	deploymentARN := prefix + "service-deployment/production/worker/" + id
	revisionARN := prefix + "service-revision/production/worker/" + id
	created := time.Date(2026, 10, 14, 1, 0, f.writes, 0, time.UTC)
	oldRevision := prefix + "service-revision/production/worker/old"
	if f.sourceOverride != "" {
		oldRevision = f.sourceOverride
	}
	f.history = append(f.history, types.ServiceDeploymentBrief{ServiceDeploymentArn: aws.String(deploymentARN), TargetServiceRevisionArn: aws.String(revisionARN), ServiceArn: aws.String(f.request.Intent.ServiceARN), ClusterArn: aws.String(f.request.Intent.ClusterARN), Status: types.ServiceDeploymentStatusInProgress, CreatedAt: &created})
	f.deployments[deploymentARN] = types.ServiceDeployment{ServiceDeploymentArn: aws.String(deploymentARN), ServiceArn: aws.String(f.request.Intent.ServiceARN), ClusterArn: aws.String(f.request.Intent.ClusterARN), Status: types.ServiceDeploymentStatusInProgress, CreatedAt: &created, TargetServiceRevision: &types.ServiceRevisionSummary{Arn: aws.String(revisionARN), RequestedTaskCount: f.service.DesiredCount}, SourceServiceRevisions: []types.ServiceRevisionSummary{{Arn: aws.String(oldRevision), RequestedTaskCount: 2}}, DeploymentConfiguration: input.DeploymentConfiguration}
	f.revisions[revisionARN] = types.ServiceRevision{ServiceRevisionArn: aws.String(revisionARN), ServiceArn: aws.String(f.request.Intent.ServiceARN), ClusterArn: aws.String(f.request.Intent.ClusterARN), TaskDefinition: input.TaskDefinition}
}
func nativeFixture(t *testing.T, strategy string) (*nativeFake, AdvancedService, applyrequest.ECSOperationRequest) {
	t.Helper()
	backend, _, r := serviceFixture(t)
	raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range compiled.Deployments {
		if d.ECS.DeploymentStrategy == strategy {
			r.Intent.Advanced = d.ECS.Advanced.Clone()
		}
	}
	r.Intent.DeploymentStrategy = strategy
	config, err := advancedAWSConfig(r.Intent)
	if err != nil {
		t.Fatal(err)
	}
	config.MinimumHealthyPercent = aws.Int32(50)
	config.MaximumPercent = aws.Int32(150)
	backend.service.DeploymentConfiguration = config
	backend.service.DesiredCount = 2
	backend.service.RunningCount = 2
	routing := r.Intent.Advanced.Routing
	backend.service.LoadBalancers = []types.LoadBalancer{{ContainerName: aws.String(r.Intent.ContainerName), ContainerPort: aws.Int32(routing.ContainerPort), TargetGroupArn: aws.String(routing.TargetGroupARN), AdvancedConfiguration: &types.AdvancedConfiguration{AlternateTargetGroupArn: aws.String(routing.AlternateTargetGroupARN), ProductionListenerRule: aws.String(routing.ProductionListenerRuleARN), TestListenerRule: aws.String(routing.TestListenerRuleARN), RoleArn: aws.String(routing.InfrastructureRoleARN)}}}
	f := &nativeFake{serviceFake: backend, request: r, deployments: map[string]types.ServiceDeployment{}, revisions: map[string]types.ServiceRevision{}, routingSignature: "verified-routing"}
	oldDeployment := "arn:aws:ecs:ap-northeast-2:123456789012:service-deployment/production/worker/old"
	oldRevision := "arn:aws:ecs:ap-northeast-2:123456789012:service-revision/production/worker/old"
	created := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	f.history = []types.ServiceDeploymentBrief{{ServiceDeploymentArn: aws.String(oldDeployment), TargetServiceRevisionArn: aws.String(oldRevision), ServiceArn: aws.String(r.Intent.ServiceARN), ClusterArn: aws.String(r.Intent.ClusterARN), Status: types.ServiceDeploymentStatusSuccessful, CreatedAt: &created}}
	f.deployments[oldDeployment] = types.ServiceDeployment{ServiceDeploymentArn: aws.String(oldDeployment), ServiceArn: aws.String(r.Intent.ServiceARN), ClusterArn: aws.String(r.Intent.ClusterARN), Status: types.ServiceDeploymentStatusSuccessful, CreatedAt: &created, TargetServiceRevision: &types.ServiceRevisionSummary{Arn: aws.String(oldRevision)}, DeploymentConfiguration: config}
	f.revisions[oldRevision] = types.ServiceRevision{ServiceRevisionArn: aws.String(oldRevision), ServiceArn: aws.String(r.Intent.ServiceARN), ClusterArn: aws.String(r.Intent.ClusterARN), TaskDefinition: backend.service.TaskDefinition}
	r.BeforeMutation = func(context.Context) error { f.guards++; return nil }
	h := AdvancedService{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (NativeServiceClient, error) { return f, nil }, CheckCapabilities: func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
		return nil
	}, ObserveRouting: func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error) {
		return ALBRoutingSnapshot{Signature: f.routingSignature}, nil
	}}
	return f, h, r
}
func TestNativeServiceRequestsAllStrategiesWithDurableCorrelationAndSingleMutation(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		t.Run(strategy, func(t *testing.T) {
			f, h, r := nativeFixture(t, strategy)
			if err := h.Preflight(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			prepared, err := h.Prepare(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			if f.writes != 0 || prepared.NativeService == nil || len(prepared.NativeService.KnownDeploymentARNs) != 1 || prepared.PreviousService.TaskDefinitionARN != r.Intent.BaseTaskDefinitionARN {
				t.Fatal("native snapshot did not precede mutation")
			}
			r.PreparedResult = prepared
			result, err := h.Execute(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			if f.writes != 1 || f.guards != 1 || !f.singleAttempt || result.ServiceDeploymentARN == "" || result.ServiceRevisionARN == "" || result.Observation != nil || string(f.input.DeploymentConfiguration.Strategy) != strategy {
				t.Fatalf("native acceptance lost identity or claimed convergence: %+v %v", result, err)
			}
			if f.input.DesiredCount != nil || f.input.LoadBalancers != nil || f.input.DeploymentController != nil || f.input.NetworkConfiguration != nil || f.input.ForceNewDeployment {
				t.Fatal("native update changed unreviewed capacity/routing/controller/network or forced image rollout")
			}
			if f.input.DeploymentConfiguration.MinimumHealthyPercent != nil || f.input.DeploymentConfiguration.MaximumPercent != nil || aws.ToInt32(f.service.DeploymentConfiguration.MinimumHealthyPercent) != 50 || aws.ToInt32(f.service.DeploymentConfiguration.MaximumPercent) != 150 {
				t.Fatal("native update sent Rolling settings or lost inherited bounds")
			}
			// Restart/reconcile only reads the newly correlated deployment and revision.
			content, err := json.Marshal(r.PreparedResult)
			if err != nil {
				t.Fatal(err)
			}
			var restarted applyrequest.ECSOperationResult
			if err := json.Unmarshal(content, &restarted); err != nil {
				t.Fatal(err)
			}
			r.PreparedResult = restarted
			f.pages = true
			recovered, err := h.Reconcile(context.Background(), r)
			if err != nil || !recovered.Complete || recovered.Absent || recovered.Result.ServiceDeploymentARN != result.ServiceDeploymentARN || f.writes != 1 {
				t.Fatal("native recovery replayed traffic update or lost correlation")
			}
		})
	}
}
func TestNativeServiceDoesNotReplayUnknownOutcomesOrGuessBetweenMatches(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(fmt.Sprint(accepted), func(t *testing.T) {
			f, h, r := nativeFixture(t, "CANARY")
			prepared, err := h.Prepare(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			r.PreparedResult = prepared
			f.updateError = errors.New("response lost after send")
			f.noEffect = !accepted
			if _, err := h.Execute(context.Background(), r); err == nil {
				t.Fatal("ambiguous update treated as success")
			}
			h.CheckCapabilities = func(_ context.Context, _ applyrequest.DeployerRoleReadiness, checks []applyrequest.OperationExecutionCapabilityCheck) error {
				for _, check := range checks {
					if check.Action == "ecs:UpdateService" || check.Action == "iam:PassRole" {
						t.Fatal("read-only reconciliation required mutation authority")
					}
				}
				return nil
			}
			result, err := h.Reconcile(context.Background(), r)
			if err != nil || result.Complete != accepted || result.Absent || f.writes != 1 {
				t.Fatal("ambiguous update was guessed absent or sent twice")
			}
			if accepted {
				f.addDeployment("another-match", f.input)
				result, err = h.Reconcile(context.Background(), r)
				if err == nil || result.Complete || result.Absent || f.writes != 1 {
					t.Fatal("multiple native matches were guessed or replayed")
				}
			}
		})
	}
}
func TestNativeServiceNoOpAndConfigOnlyDeployment(t *testing.T) {
	for _, matching := range []bool{false, true} {
		t.Run(fmt.Sprint(matching), func(t *testing.T) {
			f, h, r := nativeFixture(t, "LINEAR")
			f.service.TaskDefinition = aws.String(r.PriorResults[0].TaskDefinitionARN)
			for arn, revision := range f.revisions {
				revision.TaskDefinition = f.service.TaskDefinition
				f.revisions[arn] = revision
			}
			if !matching {
				value := int32(6)
				f.service.DeploymentConfiguration.BakeTimeInMinutes = &value
			}
			r.PreparedResult, _ = h.Prepare(context.Background(), r)
			result, err := h.Execute(context.Background(), r)
			if err != nil || result.ServiceDeploymentARN == "" {
				t.Fatal(err)
			}
			if matching {
				if f.writes != 0 || f.guards != 0 {
					t.Fatal("matching native deployment started another update")
				}
			} else if f.writes != 1 || !f.input.ForceNewDeployment {
				t.Fatal("config-only change did not create a native lifecycle")
			}
		})
	}
}
func TestNativeServiceFencesInvalidTargetsAndInterveningChanges(t *testing.T) {
	for _, name := range []string{"resize", "Rolling", "CodeDeploy", "binding role", "foreign target", "Service Connect", "VPC Lattice", "missing routing proof", "read denied", "role unverified", "config changed", "routing changed", "new deployment", "mutation revoked", "active conflict"} {
		t.Run(name, func(t *testing.T) {
			f, h, r := nativeFixture(t, "BLUE_GREEN")
			prepared, err := h.Prepare(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			r.PreparedResult = prepared
			switch name {
			case "resize":
				f.service.DesiredCount = 3
			case "Rolling":
				f.service.DeploymentConfiguration.Strategy = types.DeploymentStrategyRolling
			case "CodeDeploy":
				f.service.DeploymentController.Type = types.DeploymentControllerTypeCodeDeploy
			case "binding role":
				f.service.LoadBalancers[0].AdvancedConfiguration.RoleArn = aws.String("foreign")
			case "foreign target":
				f.service.LoadBalancers[0].TargetGroupArn = aws.String("foreign")
			case "Service Connect":
				f.service.Deployments[0].ServiceConnectConfiguration = &types.ServiceConnectConfiguration{Enabled: true}
			case "VPC Lattice":
				f.service.Deployments[0].VpcLatticeConfigurations = []types.VpcLatticeConfiguration{{}}
			case "missing routing proof":
				f.routingSignature = ""
			case "read denied":
				f.readError = errors.New("denied")
			case "role unverified":
				r.Role.CheckedWithAWS = false
			case "config changed":
				value := int32(9)
				f.service.DeploymentConfiguration.BakeTimeInMinutes = &value
			case "routing changed":
				f.routingSignature = "changed"
			case "new deployment":
				f.addDeployment("intervening", &ecs.UpdateServiceInput{TaskDefinition: f.service.TaskDefinition, DeploymentConfiguration: f.service.DeploymentConfiguration})
			case "mutation revoked":
				r.BeforeMutation = func(context.Context) error { return errors.New("revoked") }
			case "active conflict":
				f.history[0].Status = types.ServiceDeploymentStatusInProgress
			}
			if _, err := h.Execute(context.Background(), r); err == nil {
				t.Fatal("invalid native preflight authorized update")
			}
			if f.writes != 0 {
				t.Fatal("native fence ran after AWS mutation")
			}
		})
	}
}
func TestNativeServiceCapabilitiesIncludeECSInfrastructurePassRole(t *testing.T) {
	_, h, r := nativeFixture(t, "CANARY")
	seen := map[string]bool{}
	h.CheckCapabilities = func(_ context.Context, _ applyrequest.DeployerRoleReadiness, checks []applyrequest.OperationExecutionCapabilityCheck) error {
		for _, check := range checks {
			if check.Action == "iam:PassRole" && check.Context["iam:PassedToService"] == "ecs.amazonaws.com" {
				seen[check.ResourceName] = true
			}
		}
		return nil
	}
	if err := h.Preflight(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if !seen[r.Intent.Advanced.Routing.InfrastructureRoleARN] || !seen[r.Intent.Advanced.LifecycleHooks[0].RoleARN] {
		t.Fatal("native infrastructure/hook roles omitted concrete ECS PassRole context")
	}
}
func TestNativeSnapshotCloneOwnsHistory(t *testing.T) {
	original := applyrequest.ECSOperationResult{NativeService: &applyrequest.ECSNativeServiceSnapshot{KnownDeploymentARNs: []string{"old"}}}
	copy := original.Clone()
	copy.NativeService.KnownDeploymentARNs[0] = "changed"
	if reflect.DeepEqual(original, copy) || original.NativeService.KnownDeploymentARNs[0] != "old" {
		t.Fatal("durable native baseline aliases caller storage")
	}
}

func TestNativePreflightAdoptsActiveMatchingBaseWithoutRegistrationResult(t *testing.T) {
	f, h, r := nativeFixture(t, "CANARY")
	r.PriorResults = nil
	base := f.definitions[r.Intent.BaseTaskDefinitionARN].TaskDefinition
	base.ContainerDefinitions[0].Image = aws.String(r.Intent.Image)
	f.history[0].Status = types.ServiceDeploymentStatusInProgress
	for value, deployment := range f.deployments {
		deployment.Status = types.ServiceDeploymentStatusInProgress
		f.deployments[value] = deployment
	}
	if err := h.Preflight(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if f.writes != 0 || f.guards != 0 {
		t.Fatal("matching active native preflight mutated")
	}
}

func TestNativeCorrelationUsesKnownSourceRevisionAfterFailedDesiredTask(t *testing.T) {
	f, h, r := nativeFixture(t, "CANARY")
	const prefix = "arn:aws:ecs:ap-northeast-2:123456789012:"
	sourceARN := prefix + "service-revision/production/worker/healthy-source"
	failedTask := prefix + "task-definition/worker:20"
	f.sourceOverride = sourceARN
	f.revisions[sourceARN] = types.ServiceRevision{ServiceRevisionArn: aws.String(sourceARN), ServiceArn: aws.String(r.Intent.ServiceARN), ClusterArn: aws.String(r.Intent.ClusterARN), TaskDefinition: aws.String(r.Intent.BaseTaskDefinitionARN)}
	for value, revision := range f.revisions {
		if value != sourceARN {
			revision.TaskDefinition = aws.String(failedTask)
			f.revisions[value] = revision
		}
	}
	f.service.TaskDefinition = aws.String(failedTask)
	f.service.Deployments[0].TaskDefinition = f.service.TaskDefinition
	f.history[0].Status = types.ServiceDeploymentStatusStopped
	for value, deployment := range f.deployments {
		deployment.Status = types.ServiceDeploymentStatusStopped
		deployment.SourceServiceRevisions = []types.ServiceRevisionSummary{{Arn: aws.String(sourceARN)}}
		f.deployments[value] = deployment
	}
	prepared, err := h.Prepare(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.PreparedResult = prepared
	result, err := h.Execute(context.Background(), r)
	if err != nil || result.PreviousServiceRevisionARN != sourceARN || result.PreviousTaskDefinitionARN != r.Intent.BaseTaskDefinitionARN || result.PreviousService.TaskDefinitionARN != failedTask || f.writes != 1 {
		t.Fatalf("native source was confused with failed desired task metadata: %+v %v", result, err)
	}
}
func TestNativeNoOpCanObserveECSOwnedTrafficAdvancingAfterPreparation(t *testing.T) {
	f, h, r := nativeFixture(t, "CANARY")
	f.service.TaskDefinition = aws.String(r.PriorResults[0].TaskDefinitionARN)
	for value, revision := range f.revisions {
		revision.TaskDefinition = f.service.TaskDefinition
		f.revisions[value] = revision
	}
	prepared, err := h.Prepare(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.PreparedResult = prepared
	f.routingSignature = "ECS-progressed-traffic"
	result, err := h.Execute(context.Background(), r)
	if err != nil || result.ServiceDeploymentARN == "" || f.writes != 0 || f.guards != 0 {
		t.Fatal("read-only adoption treated ordinary traffic progress as authority to mutate")
	}
}
func TestNativeCorrelationDoesNotAdoptAnUnknownSourceRevision(t *testing.T) {
	f, h, r := nativeFixture(t, "CANARY")
	prepared, err := h.Prepare(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.PreparedResult = prepared
	f.noEffect = true
	f.updateError = errors.New("response lost")
	if _, err := h.Execute(context.Background(), r); err == nil {
		t.Fatal("missing native acceptance not uncertain")
	}
	unknown := "arn:aws:ecs:ap-northeast-2:123456789012:service-revision/production/worker/unreviewed-source"
	f.revisions[unknown] = types.ServiceRevision{ServiceRevisionArn: aws.String(unknown), ServiceArn: aws.String(r.Intent.ServiceARN), ClusterArn: aws.String(r.Intent.ClusterARN), TaskDefinition: aws.String(r.Intent.BaseTaskDefinitionARN)}
	f.sourceOverride = unknown
	f.addDeployment("external", f.input)
	result, err := h.Reconcile(context.Background(), r)
	if err != nil || result.Complete || result.Absent || f.writes != 1 {
		t.Fatal("unknown native source was accepted or replayed")
	}
}

func TestNativeServiceCapacityCorrelationRequiresReviewedRequestedCount(t *testing.T) {
	f, h, r := nativeFixture(t, "CANARY")
	r.Intent.DesiredCount = 4
	var err error
	r.PreparedResult, err = h.Prepare(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := h.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	deployment := f.deployments[accepted.ServiceDeploymentARN]
	deployment.TargetServiceRevision.RequestedTaskCount = 3
	f.deployments[accepted.ServiceDeploymentARN] = deployment
	outcome, err := h.Reconcile(context.Background(), r)
	if err != nil || outcome.Complete || f.writes != 1 {
		t.Fatal("wrong capacity was correlated or mutation replayed", outcome, err)
	}
	deployment.TargetServiceRevision.RequestedTaskCount = 4
	f.deployments[accepted.ServiceDeploymentARN] = deployment
	outcome, err = h.Reconcile(context.Background(), r)
	if err != nil || !outcome.Complete || f.writes != 1 {
		t.Fatal("matching capacity did not reconcile read-only", outcome, err)
	}
}
