package ecsexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

type nativeSDKRequest struct {
	action string
	body   map[string]any
}

func nativeSDKFixture(t *testing.T, response func(string) string) (*ecs.Client, <-chan nativeSDKRequest) {
	t.Helper()
	requests := make(chan nativeSDKRequest, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := strings.TrimPrefix(r.Header.Get("X-Amz-Target"), "AmazonEC2ContainerServiceV20141113.")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "invalid SDK JSON", http.StatusBadRequest)
			return
		}
		requests <- nativeSDKRequest{action: action, body: body}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		fmt.Fprint(w, response(action))
	}))
	t.Cleanup(server.Close)
	client := ecs.New(ecs.Options{Region: "ap-northeast-2", BaseEndpoint: aws.String(server.URL), Credentials: credentials.NewStaticCredentialsProvider("fixture-access", "fixture-secret", ""), Retryer: aws.NopRetryer{}})
	return client, requests
}
func TestNativeSDKSerializesAllAdvancedStrategiesAndRouting(t *testing.T) {
	for _, strategy := range []types.DeploymentStrategy{types.DeploymentStrategyBlueGreen, types.DeploymentStrategyCanary, types.DeploymentStrategyLinear} {
		t.Run(string(strategy), func(t *testing.T) {
			client, requests := nativeSDKFixture(t, func(string) string { return `{"service":{"deploymentController":{"type":"ECS"}}}` })
			config := &types.DeploymentConfiguration{Strategy: strategy, BakeTimeInMinutes: aws.Int32(0), Alarms: &types.DeploymentAlarms{Enable: true, Rollback: true, AlarmNames: []string{"fixture-health"}}, LifecycleHooks: []types.DeploymentLifecycleHook{{HookTargetArn: aws.String("arn:aws:lambda:ap-northeast-2:123456789012:function:fixture-health"), RoleArn: aws.String("arn:aws:iam::123456789012:role/fixture-hook"), LifecycleStages: []types.DeploymentLifecycleHookStage{types.DeploymentLifecycleHookStagePostScaleUp}}}}
			if strategy == types.DeploymentStrategyCanary {
				config.CanaryConfiguration = &types.CanaryConfiguration{CanaryPercent: aws.Float64(5.5), CanaryBakeTimeInMinutes: aws.Int32(0)}
			}
			if strategy == types.DeploymentStrategyLinear {
				config.LinearConfiguration = &types.LinearConfiguration{StepPercent: aws.Float64(12.5), StepBakeTimeInMinutes: aws.Int32(0)}
			}
			revision := "arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:18"
			_, err := client.UpdateService(context.Background(), &ecs.UpdateServiceInput{Cluster: aws.String("production"), Service: aws.String("worker"), TaskDefinition: aws.String(revision), DeploymentController: &types.DeploymentController{Type: types.DeploymentControllerTypeEcs}, DeploymentConfiguration: config, LoadBalancers: []types.LoadBalancer{{ContainerName: aws.String("worker"), ContainerPort: aws.Int32(8080), TargetGroupArn: aws.String("arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/blue/123"), AdvancedConfiguration: &types.AdvancedConfiguration{AlternateTargetGroupArn: aws.String("arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/green/456"), ProductionListenerRule: aws.String("production-rule"), TestListenerRule: aws.String("test-rule"), RoleArn: aws.String("arn:aws:iam::123456789012:role/fixture-routing")}}}})
			if err != nil {
				t.Fatal(err)
			}
			wire := <-requests
			deployment := wire.body["deploymentConfiguration"].(map[string]any)
			if wire.action != "UpdateService" || wire.body["taskDefinition"] != revision || deployment["strategy"] != string(strategy) || deployment["bakeTimeInMinutes"] != float64(0) || wire.body["deploymentController"].(map[string]any)["type"] != "ECS" {
				t.Fatal("native strategy/revision/explicit zero lost in SDK serialization")
			}
			if strategy == types.DeploymentStrategyCanary {
				value := deployment["canaryConfiguration"].(map[string]any)
				if value["canaryPercent"] != 5.5 || value["canaryBakeTimeInMinutes"] != float64(0) {
					t.Fatal("canary configuration lost")
				}
			}
			if strategy == types.DeploymentStrategyLinear {
				value := deployment["linearConfiguration"].(map[string]any)
				if value["stepPercent"] != 12.5 || value["stepBakeTimeInMinutes"] != float64(0) {
					t.Fatal("linear configuration lost")
				}
			}
			routing := wire.body["loadBalancers"].([]any)[0].(map[string]any)["advancedConfiguration"].(map[string]any)
			if routing["productionListenerRule"] != "production-rule" || routing["testListenerRule"] != "test-rule" || routing["roleArn"] != "arn:aws:iam::123456789012:role/fixture-routing" || routing["alternateTargetGroupArn"] == nil || deployment["alarms"] == nil || deployment["lifecycleHooks"] == nil {
				t.Fatal("routing/role/alarm/hook review inputs dropped")
			}
		})
	}
}
func TestNativeSDKPreservesDeploymentRevisionTrafficAndRollbackEvidence(t *testing.T) {
	deploymentARN := "arn:aws:ecs:ap-northeast-2:123456789012:service-deployment/production/worker/deploy-1"
	revisionARN := "arn:aws:ecs:ap-northeast-2:123456789012:service-revision/production/worker/revision-1"
	client, requests := nativeSDKFixture(t, func(action string) string {
		switch action {
		case "ListServiceDeployments":
			return fmt.Sprintf(`{"serviceDeployments":[{"serviceDeploymentArn":%q,"status":"IN_PROGRESS"}],"nextToken":"page-2"}`, deploymentARN)
		case "DescribeServiceDeployments":
			return fmt.Sprintf(`{"serviceDeployments":[{"serviceDeploymentArn":%q,"status":"IN_PROGRESS","lifecycleStage":"PRODUCTION_TRAFFIC_SHIFT","targetServiceRevision":{"arn":%q,"requestedProductionTrafficWeight":5.5,"requestedTestTrafficWeight":100,"runningTaskCount":2},"sourceServiceRevisions":[{"arn":"old-revision","requestedProductionTrafficWeight":94.5}],"deploymentConfiguration":{"strategy":"CANARY","canaryConfiguration":{"canaryPercent":5.5,"canaryBakeTimeInMinutes":0}},"alarms":{"status":"TRIGGERED","triggeredAlarmNames":["fixture-health"]}}]}`, deploymentARN, revisionARN)
		case "DescribeServiceRevisions":
			return fmt.Sprintf(`{"serviceRevisions":[{"serviceRevisionArn":%q,"taskDefinition":"arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:18"}]}`, revisionARN)
		case "StopServiceDeployment":
			return fmt.Sprintf(`{"serviceDeploymentArn":%q}`, deploymentARN)
		default:
			t.Errorf("unexpected native SDK action: %s", action)
			return `{}`
		}
	})
	ctx := context.Background()
	listed, err := client.ListServiceDeployments(ctx, &ecs.ListServiceDeploymentsInput{Cluster: aws.String("production"), Service: aws.String("worker")})
	if err != nil {
		t.Fatal(err)
	}
	if wire := <-requests; wire.action != "ListServiceDeployments" || len(listed.ServiceDeployments) != 1 || aws.ToString(listed.ServiceDeployments[0].ServiceDeploymentArn) != deploymentARN || aws.ToString(listed.NextToken) != "page-2" {
		t.Fatal("deployment correlation/pagination missing")
	}
	described, err := client.DescribeServiceDeployments(ctx, &ecs.DescribeServiceDeploymentsInput{ServiceDeploymentArns: []string{deploymentARN}})
	if err != nil {
		t.Fatal(err)
	}
	wire := <-requests
	if len(described.ServiceDeployments) != 1 {
		t.Fatalf("expected one described service deployment, got %d", len(described.ServiceDeployments))
	}
	d := described.ServiceDeployments[0]
	if wire.action != "DescribeServiceDeployments" || wire.body["serviceDeploymentArns"].([]any)[0] != deploymentARN || d.Status != types.ServiceDeploymentStatusInProgress || d.LifecycleStage != types.ServiceDeploymentLifecycleStageProductionTrafficShift || aws.ToString(d.TargetServiceRevision.Arn) != revisionARN || aws.ToFloat64(d.TargetServiceRevision.RequestedProductionTrafficWeight) != 5.5 || aws.ToFloat64(d.SourceServiceRevisions[0].RequestedProductionTrafficWeight) != 94.5 || len(d.Alarms.TriggeredAlarmNames) != 1 || aws.ToInt32(d.DeploymentConfiguration.CanaryConfiguration.CanaryBakeTimeInMinutes) != 0 {
		t.Fatal("native phase/traffic/failure evidence lost")
	}
	revisions, err := client.DescribeServiceRevisions(ctx, &ecs.DescribeServiceRevisionsInput{ServiceRevisionArns: []string{revisionARN}})
	if err != nil {
		t.Fatal(err)
	}
	if wire := <-requests; wire.action != "DescribeServiceRevisions" || len(revisions.ServiceRevisions) != 1 || aws.ToString(revisions.ServiceRevisions[0].ServiceRevisionArn) != revisionARN || aws.ToString(revisions.ServiceRevisions[0].TaskDefinition) == "" {
		t.Fatal("service revision did not resolve workload identity")
	}
	stopped, err := client.StopServiceDeployment(ctx, &ecs.StopServiceDeploymentInput{ServiceDeploymentArn: aws.String(deploymentARN), StopType: types.StopServiceDeploymentStopTypeRollback})
	if err != nil {
		t.Fatal(err)
	}
	wire = <-requests
	if wire.action != "StopServiceDeployment" || wire.body["stopType"] != "ROLLBACK" || wire.body["serviceDeploymentArn"] != deploymentARN || aws.ToString(stopped.ServiceDeploymentArn) != deploymentARN {
		t.Fatal("rollback request lost native deployment identity")
	}
}
