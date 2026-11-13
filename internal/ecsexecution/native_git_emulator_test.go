//go:build awsemulator

package ecsexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/ecsreconcile"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/DevopsArtFactory/tenet/internal/spec"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

type emulatorRole = applyrequest.DeployerRoleReadiness

func emulatorProvider[T any](client T) func(context.Context, emulatorRole) (T, error) {
	return func(context.Context, emulatorRole) (T, error) { return client, nil }
}

func emulatorGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir, "-c", "user.name=Tenet test", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", args[0], out, err)
	}
	return strings.TrimSpace(string(out))
}
func emulatorManifest(t *testing.T, dir, kind string, doc any) {
	t.Helper()
	folder := filepath.Join(dir, kind)
	emulatorCheck(t, os.MkdirAll(folder, 0755))
	b, err := json.Marshal(doc)
	emulatorCheck(t, err)
	emulatorCheck(t, os.WriteFile(filepath.Join(folder, "test.json"), b, 0644))
}

type emulatorNetwork struct {
	Name, Rule, Security string
	Subnets, Groups      []string
}

func TestNativeEmulatorGitRecovery(t *testing.T) {
	raw, err := manifestfs.Load("../../examples/ecs-native-advanced-repo")
	emulatorCheck(t, err)
	for _, d := range raw.Deployments {
		t.Run(string(d.Strategy.Type), func(t *testing.T) {
			var net emulatorNetwork
			cmd := exec.CommandContext(t.Context(), "python3", "../../tests/ecs-native-integration/emulator_network.py")
			cmd.Stderr = os.Stderr
			out, err := cmd.Output()
			emulatorCheck(t, err)
			emulatorCheck(t, json.Unmarshal(out, &net))
			emulatorGitRecovery(t, net, raw.Accounts[0], raw.Environments[0], d)
		})
	}
}

func emulatorGitRecovery(t *testing.T, net emulatorNetwork, account spec.AccountSpec, environment spec.EnvironmentSpec, d spec.DeploymentSpec) {
	t.Helper()
	ctx := t.Context()
	cfg := emulatorConfig(t)
	client := ecs.NewFromConfig(cfg)
	lb := elb.NewFromConfig(cfg)
	cluster, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{ClusterName: &net.Name})
	emulatorCheck(t, err)
	d.Image = "docker.io/library/nginx@sha256:" + strings.Repeat("a", 64)
	td, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{Family: &net.Name, NetworkMode: types.NetworkModeAwsvpc, RequiresCompatibilities: []types.Compatibility{types.CompatibilityFargate}, Cpu: aws.String("256"), Memory: aws.String("512"), ContainerDefinitions: []types.ContainerDefinition{{Name: aws.String("web"), Image: &d.Image, PortMappings: []types.PortMapping{{ContainerPort: aws.Int32(8080)}}}}})
	emulatorCheck(t, err)
	account.AWSAccountID = "000000000000"
	account.RoleARN = "arn:aws:iam::000000000000:role/deployer"
	account.ObserverRoleARN = ""
	e := d.ECS
	e.ClusterARN = aws.ToString(cluster.Cluster.ClusterArn)
	e.ServiceARN = strings.Replace(e.ClusterARN, ":cluster/", ":service/", 1) + "/" + d.Name
	e.BaseTaskDefinitionARN = aws.ToString(td.TaskDefinition.TaskDefinitionArn)
	route := &e.Advanced.Routing
	route.TargetGroupARN = net.Groups[0]
	route.AlternateTargetGroupARN = net.Groups[1]
	route.ProductionListenerRuleARN = net.Rule
	route.TestListenerRuleARN = ""
	route.InfrastructureRoleARN = "arn:aws:iam::000000000000:role/routing"
	e.Advanced.LifecycleHooks = nil
	e.Advanced.Alarms.Rollback = false
	e.RollbackOnFailure = true
	dir := t.TempDir()
	emulatorGit(t, dir, "init", "-b", "main")
	emulatorManifest(t, dir, "accounts", account)
	emulatorManifest(t, dir, "environments", environment)
	commit := func(message string) string {
		emulatorManifest(t, dir, "deployments", d)
		emulatorGit(t, dir, "add", ".")
		emulatorGit(t, dir, "commit", "-m", message)
		return emulatorGit(t, dir, "rev-parse", "HEAD")
	}
	baselineSHA := commit("Record baseline")
	load := func(context.Context) (manifest.Catalog, error) {
		if emulatorGit(t, dir, "status", "--porcelain") != "" {
			t.Fatal("uncommitted catalog")
		}
		raw, err := manifestfs.Load(dir)
		if err != nil {
			return manifest.Catalog{}, err
		}
		raw.Git = &domain.GitSource{Repository: "fixture/desired", Ref: "main", Revision: emulatorGit(t, dir, "rev-parse", "HEAD")}
		return manifest.Compile(raw)
	}
	compiled, err := load(ctx)
	emulatorCheck(t, err)
	intent := compiled.Deployments[0].ECS
	config, err := advancedAWSConfig(*intent)
	emulatorCheck(t, err)
	baselineCount := int32(d.Capacity.Desired)
	_, err = client.CreateService(ctx, &ecs.CreateServiceInput{Cluster: &e.ClusterARN, ServiceName: &d.Name, TaskDefinition: &e.BaseTaskDefinitionARN, DesiredCount: &baselineCount, LaunchType: types.LaunchTypeFargate, DeploymentController: &types.DeploymentController{Type: types.DeploymentControllerTypeEcs}, SchedulingStrategy: types.SchedulingStrategyReplica, DeploymentConfiguration: config, NetworkConfiguration: &types.NetworkConfiguration{AwsvpcConfiguration: &types.AwsVpcConfiguration{Subnets: net.Subnets, SecurityGroups: []string{net.Security}, AssignPublicIp: types.AssignPublicIpEnabled}}, LoadBalancers: []types.LoadBalancer{{ContainerName: &e.ContainerName, ContainerPort: aws.Int32(8080), TargetGroupArn: &route.TargetGroupARN, AdvancedConfiguration: &types.AdvancedConfiguration{AlternateTargetGroupArn: &route.AlternateTargetGroupARN, ProductionListenerRule: &net.Rule, RoleArn: &route.InfrastructureRoleARN}}}})
	emulatorCheck(t, err)
	r := applyrequest.ECSOperationRequest{Intent: *intent}
	var baselineRevision string
	for deadline := time.Now().Add(30 * time.Second); ; {
		rows, err := listNativeDeployments(ctx, client, r)
		emulatorCheck(t, err)
		if len(rows) == 1 && rows[0].Status == types.ServiceDeploymentStatusSuccessful {
			baselineRevision = aws.ToString(rows[0].TargetServiceRevisionArn)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("baseline never stabilized: %+v", rows)
		}
		time.Sleep(200 * time.Millisecond)
	}
	role := emulatorRole{RoleARN: account.RoleARN, AWSAccountID: account.AWSAccountID, Region: cfg.Region, Ready: true, CheckedWithAWS: true, Configured: true}
	check := func(_ context.Context, actual emulatorRole, checks []applyrequest.OperationExecutionCapabilityCheck) error {
		if actual.RoleARN != role.RoleARN || len(checks) == 0 {
			return fmt.Errorf("unreviewed emulator role")
		}
		return nil
	}
	routing := ALBReadiness{ClientFor: emulatorProvider[ALBClient](lb), CheckCapabilities: check}
	health := ALBTargetHealth{ClientFor: emulatorProvider[TargetHealthClient](lb), CheckCapabilities: check}
	update := AdvancedService{ClientFor: emulatorProvider[NativeServiceClient](client), CheckCapabilities: check, ObserveRouting: routing.Observe}
	stop := NativeRollback{ClientFor: emulatorProvider[NativeRollbackClient](client), CheckCapabilities: check}
	now := time.Now().UTC()
	clock := func() time.Time { return now }
	alarm := &emulatorFailedImageAlarm{Client: client, name: e.Advanced.Alarms.Names[0]}
	observe := NativeObservation{ClientFor: emulatorProvider[NativeObservationClient](alarm), CheckCapabilities: check, ObserveRouting: routing.Observe, ObserveTargetHealth: health.Observe, Now: clock}
	stores := t.TempDir()
	planPath := filepath.Join(stores, "plans.json")
	plans := planpreview.NewFileStore(planPath)
	service := applyrequest.Service{Store: applyrequest.NewFileStore(filepath.Join(stores, "applies.json")), LoadPlan: plans.Get, EnableAWSApply: true, Now: clock,
		CurrentFingerprint: func(ctx context.Context, name string) (string, error) {
			c, err := load(ctx)
			if err != nil {
				return "", err
			}
			detail, err := catalog.BuildDeploymentOverviewDetail(c, nil, name)
			if err != nil {
				return "", err
			}
			return catalog.DeriveDeploymentSourceFingerprint(detail)
		},
		DeployerRoleReadiness: func(context.Context, applyrequest.DeployerRoleTarget) (emulatorRole, error) { return role, nil },
		AuthorizeCreate: func(ctx context.Context, r applyrequest.Record) (*applyrequest.ExecutionPrincipal, error) {
			id := requestactor.AuthenticatedIdentityFromContext(ctx)
			if !id.Authenticated || id.ID != "ecs-sync" || !applyrequest.AutomaticApprovalFromContext(ctx) || r.Source.Git == nil {
				return nil, applyrequest.ErrExecutionUnauthorized
			}
			return &applyrequest.ExecutionPrincipal{Identity: id, Automatic: true}, nil
		},
		PrepareNativeRollback: func(ctx context.Context, parent applyrequest.Record, target applyrequest.ECSNativeRollback) error {
			_, err := stop.Prepare(ctx, applyrequest.ECSOperationRequest{Intent: *parent.Operations[0].ECS, Role: role}, target)
			return err
		},
	}
	authorize := func(_ context.Context, r applyrequest.Record) error {
		if r.Principal == nil || !r.Principal.Automatic || r.Principal.Identity.ID != "ecs-sync" {
			return applyrequest.ErrExecutionUnauthorized
		}
		return nil
	}
	loop := ecsreconcile.Reconciler{Enabled: true, Automatic: true, LiveECS: true, AllowAdvanced: true, SubjectID: "ecs-sync", LoadCatalog: load, Plans: plans, Applies: service, ListPlans: func(context.Context) ([]planpreview.Record, error) {
		rows, err := planpreview.Load(planPath)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return rows, err
	}}
	executor := applyrequest.Executor{Service: service, WorkerID: "emulator", ECS: &applyrequest.ECSExecution{Enabled: true, AllowAdvanced: true, Authorize: authorize, AuthorizeMutation: authorize, CurrentIntent: func(ctx context.Context, _ string) (*domain.ECSDeploymentIntent, error) {
		c, err := load(ctx)
		if err != nil {
			return nil, err
		}
		return c.Deployments[0].ECS, nil
	}, Handlers: map[string]applyrequest.ECSOperationHandler{
		string(reconcile.StepRegisterECSTaskDefinition): Registration{AllowAdvanced: true, ClientFor: emulatorProvider[RegistrationClient](client), CheckCapabilities: check},
		string(reconcile.StepUpdateECSService):          StrategyHandler{Advanced: update}, string(reconcile.StepVerifyServiceStability): StrategyHandler{Advanced: observe},
		applyrequest.NativeRollbackOperationKind: NativeRollbackOperation{NativeRollback: stop}, applyrequest.NativeCapacityRestoreOperationKind: NativeCapacityRestore{NativeRollback: stop, ObserveRouting: routing.Observe, Now: clock},
	}}}
	records := func() []applyrequest.Record { rows, err := service.Store.List(ctx); emulatorCheck(t, err); return rows }
	emulatorCheck(t, loop.RunOnce(ctx))
	if len(records()) != 0 {
		t.Fatal("manual baseline created an Apply")
	}
	e.SyncPolicy.Mode = "automatic"
	d.Image = "000000000000.dkr.ecr.ap-northeast-2.amazonaws.com/missing@sha256:" + strings.Repeat("b", 64)
	d.Capacity.Desired = 4
	if d.Strategy.Type == spec.StrategyTypeCanary {
		d.Capacity.Desired = 1
	}
	candidateSHA := commit("Change image and desired count")
	emulatorCheck(t, loop.RunOnce(ctx))
	rows := records()
	if len(rows) != 1 || rows[0].Source.Git == nil || rows[0].Source.Git.Revision != candidateSHA || candidateSHA == baselineSHA {
		t.Fatalf("expected one Apply for the changed commit: %v", rows)
	}
	parentID := rows[0].ID
	tick := func() { now = now.Add(31 * time.Second); emulatorCheck(t, executor.RunOnce(ctx)) }
	tick()
	tick()
	parent, err := service.Get(ctx, parentID)
	emulatorCheck(t, err)
	if parent.Status != applyrequest.StatusRunning || parent.Operations[1].ECSCheckpoint == nil || parent.Operations[1].ECSCheckpoint.Result.ServiceDeploymentARN == "" {
		t.Fatalf("forward update not accepted: %s %v", parent.Status, parent.Details)
	}
	forward := parent.Operations[1].ECSCheckpoint.Result
	describe := func() types.Service {
		out, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: &e.ClusterARN, Services: []string{e.ServiceARN}})
		emulatorCheck(t, err)
		if len(out.Services) != 1 {
			t.Fatal("service missing")
		}
		return out.Services[0]
	}
	live := describe()
	if forward.PreviousServiceRevisionARN != baselineRevision || forward.PreviousTaskDefinitionARN != e.BaseTaskDefinitionARN || forward.ServiceRevisionARN == baselineRevision || aws.ToString(live.TaskDefinition) != forward.TaskDefinitionARN || live.DesiredCount != int32(d.Capacity.Desired) {
		t.Fatal("forward update differs from Git or captured baseline")
	}
	// The emulator records the task failure, but does not evaluate an application
	// CloudWatch metric from it. Publish the reviewed alarm on this exact rollout.
	for deadline := time.Now().Add(45 * time.Second); ; {
		stopped, err := client.ListTasks(ctx, &ecs.ListTasksInput{Cluster: &e.ClusterARN, ServiceName: &d.Name, DesiredStatus: types.DesiredStatusStopped})
		emulatorCheck(t, err)
		if len(stopped.TaskArns) > 0 {
			tasks, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: &e.ClusterARN, Tasks: stopped.TaskArns})
			emulatorCheck(t, err)
			for _, task := range tasks.Tasks {
				if aws.ToString(task.TaskDefinitionArn) == forward.TaskDefinitionARN && strings.Contains(aws.ToString(task.StoppedReason), "CannotPullContainerError") {
					alarm.deployment = forward.ServiceDeploymentARN
					break
				}
			}
		}
		if alarm.deployment != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("candidate did not fail to pull its image")
		}
		time.Sleep(150 * time.Millisecond)
	}
	for deadline := time.Now().Add(90 * time.Second); ; {
		tick()
		rows = records()
		var child applyrequest.Record
		for _, row := range rows {
			if row.NativeRollback != nil {
				child = row
			} else {
				parent = row
			}
		}
		if len(rows) == 2 && child.Status == applyrequest.StatusSucceeded {
			live = describe()
			deployment, err := describeNativeDeployment(ctx, client, forward.ServiceDeploymentARN, r)
			emulatorCheck(t, err)
			if parent.Status != applyrequest.StatusFailed || parent.NativeRollbackRequestID != child.ID || !child.NativeRollback.Automatic || child.NativeRollback.ParentApplyID != parentID || child.NativeRollback.SourceRevisionARN != baselineRevision || child.NativeRollback.DesiredCount != baselineCount || deployment.Status != types.ServiceDeploymentStatusRollbackSuccessful || deployment.Rollback == nil || aws.ToString(deployment.Rollback.ServiceRevisionArn) != baselineRevision || live.DesiredCount != baselineCount || aws.ToString(live.TaskDefinition) != e.BaseTaskDefinitionARN {
				t.Fatal("automatic recovery did not restore the captured source and count")
			}
			emulatorCheck(t, loop.RunOnce(ctx))
			tick()
			if len(records()) != 2 {
				t.Fatal("reconciliation replayed the failed commit")
			}
			break
		}
		if time.Now().After(deadline) || child.Status == applyrequest.StatusFailed {
			for _, row := range rows {
				t.Logf("Apply %s: %s, %+v, %v", row.ID, row.Status, row.ECSRuntime, row.Details)
			}
			t.Fatal("automatic recovery did not converge")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Inject only the reviewed alarm signal. All rollback status and revision
// evidence, including observations after this signal, comes from ECS.
type emulatorFailedImageAlarm struct {
	*ecs.Client
	name, deployment string
}

func (a *emulatorFailedImageAlarm) DescribeServiceDeployments(ctx context.Context, in *ecs.DescribeServiceDeploymentsInput, opts ...func(*ecs.Options)) (*ecs.DescribeServiceDeploymentsOutput, error) {
	out, err := a.Client.DescribeServiceDeployments(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	for i := range out.ServiceDeployments {
		d := &out.ServiceDeployments[i]
		if aws.ToString(d.ServiceDeploymentArn) == a.deployment && (d.Status == types.ServiceDeploymentStatusInProgress || d.Status == types.ServiceDeploymentStatusPending) {
			d.Alarms = &types.ServiceDeploymentAlarms{Status: types.ServiceDeploymentRollbackMonitorsStatusTriggered, TriggeredAlarmNames: []string{a.name}}
		}
	}
	return out, nil
}
