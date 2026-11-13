//go:build awsemulator

package ecsexecution

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func emulatorCheck(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Use only the credentials and CA minted by this local emulator process.
func emulatorClient(t *testing.T) *ecs.Client {
	t.Helper()
	dir := os.Getenv("TENET_EMULATOR_STATE")
	if dir == "" {
		t.Fatal("set TENET_EMULATOR_STATE to the emulator's 19297 directory")
	}
	profile, err := config.LoadSharedConfigProfile(t.Context(), "default", func(o *config.LoadSharedConfigOptions) {
		o.ConfigFiles = []string{}
		o.CredentialsFiles = []string{filepath.Join(dir, "emulator_credentials")}
	})
	emulatorCheck(t, err)
	if !profile.Credentials.HasKeys() {
		t.Fatal("missing emulator credentials")
	}
	ca, err := os.ReadFile(filepath.Join(dir, "ca.pem"))
	emulatorCheck(t, err)
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		t.Fatal("invalid emulator CA")
	}
	client := &http.Client{
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       10 * time.Second,
	}
	return ecs.New(ecs.Options{
		BaseEndpoint: aws.String("https://127.0.0.1:19297"), Region: "ap-northeast-2",
		Credentials: credentials.StaticCredentialsProvider{Value: profile.Credentials},
		HTTPClient:  client, Retryer: aws.NopRetryer{},
	})
}

func TestNativeEmulatorRollback(t *testing.T) {
	client := emulatorClient(t)
	ctx := t.Context()
	name := fmt.Sprintf("tenet-%d", time.Now().UnixNano())
	cluster, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{ClusterName: &name})
	emulatorCheck(t, err)
	register := func(image string) string {
		out, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
			Family: &name, ContainerDefinitions: []types.ContainerDefinition{{Name: aws.String("web"), Image: &image, Memory: aws.Int32(64)}},
		})
		emulatorCheck(t, err)
		return aws.ToString(out.TaskDefinition.TaskDefinitionArn)
	}
	baseline, candidate := register("web:baseline"), register("web:candidate")
	if baseline == candidate {
		t.Fatal("expected distinct task definitions")
	}

	raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
	emulatorCheck(t, err)
	catalog, err := manifest.Compile(raw)
	emulatorCheck(t, err)
	cases := map[string][2]int32{"BLUE_GREEN": {2, 4}, "CANARY": {3, 1}, "LINEAR": {1, 3}}
	for _, deployment := range catalog.Deployments {
		intent := deployment.ECS
		counts, ok := cases[intent.DeploymentStrategy]
		if !ok {
			t.Fatalf("unexpected or duplicate strategy %q", intent.DeploymentStrategy)
		}
		delete(cases, intent.DeploymentStrategy)
		t.Run(intent.DeploymentStrategy, func(t *testing.T) {
			// This API test uses native strategy settings from the example catalog.
			intent.Advanced.Alarms, intent.Advanced.LifecycleHooks = nil, nil
			config, err := advancedAWSConfig(*intent)
			emulatorCheck(t, err)
			created, err := client.CreateService(ctx, &ecs.CreateServiceInput{
				Cluster: cluster.Cluster.ClusterArn, ServiceName: &deployment.Name,
				TaskDefinition: &baseline, DesiredCount: &counts[0], DeploymentConfiguration: config,
			})
			emulatorCheck(t, err)
			intent.ClusterARN = aws.ToString(cluster.Cluster.ClusterArn)
			intent.ServiceARN = aws.ToString(created.Service.ServiceArn)
			intent.AWSAccountID = "000000000000"
			r := applyrequest.ECSOperationRequest{Intent: *intent}
			describe := func() *types.Service {
				out, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: &intent.ClusterARN, Services: []string{intent.ServiceARN}})
				emulatorCheck(t, err)
				if len(out.Services) != 1 || len(out.Failures) != 0 {
					t.Fatalf("missing service: %v", out.Failures)
				}
				return &out.Services[0]
			}
			source := serviceSnapshot(describe())
			if source == nil || source.TaskDefinitionARN != baseline || source.DesiredCount != counts[0] {
				t.Fatalf("unexpected initial service: %+v", source)
			}
			before, err := listNativeDeployments(ctx, client, r)
			emulatorCheck(t, err)
			if len(before) != 1 {
				t.Fatalf("expected one initial deployment, got %d", len(before))
			}
			sourceRevision := aws.ToString(before[0].TargetServiceRevisionArn)
			_, err = client.UpdateService(ctx, &ecs.UpdateServiceInput{Cluster: &intent.ClusterARN, Service: &intent.ServiceARN,
				TaskDefinition: &candidate, DesiredCount: &counts[1], DeploymentConfiguration: config})
			emulatorCheck(t, err)
			history, err := listNativeDeployments(ctx, client, r)
			emulatorCheck(t, err)
			if len(history) != 2 {
				t.Fatalf("expected a new deployment, got %d", len(history))
			}
			var forward string
			for _, item := range history {
				if aws.ToString(item.ServiceDeploymentArn) != aws.ToString(before[0].ServiceDeploymentArn) {
					forward = aws.ToString(item.ServiceDeploymentArn)
				}
			}
			d, err := describeNativeDeployment(ctx, client, forward, r)
			emulatorCheck(t, err)
			observed, err := normalizedAdvancedConfig(d.DeploymentConfiguration)
			emulatorCheck(t, err)
			expected, err := normalizedAdvancedConfig(config)
			emulatorCheck(t, err)
			if !reflect.DeepEqual(observed, expected) ||
				d.TargetServiceRevision.RequestedTaskCount != counts[1] ||
				len(d.SourceServiceRevisions) != 1 || aws.ToString(d.SourceServiceRevisions[0].Arn) != sourceRevision {
				t.Fatal("forward deployment did not capture the requested strategy, count and source revision")
			}
			target, err := describeNativeRevision(ctx, client, aws.ToString(d.TargetServiceRevision.Arn), r)
			emulatorCheck(t, err)
			if aws.ToString(target.TaskDefinition) != candidate || describe().DesiredCount != counts[1] {
				t.Fatal("candidate task definition or service count mismatch")
			}

			stop, err := client.StopServiceDeployment(ctx, &ecs.StopServiceDeploymentInput{ServiceDeploymentArn: &forward, StopType: types.StopServiceDeploymentStopTypeRollback})
			emulatorCheck(t, err)
			if aws.ToString(stop.ServiceDeploymentArn) != forward {
				t.Fatal("stop acknowledged a different deployment")
			}
			waitCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			defer cancel()
			for {
				d, err = describeNativeDeployment(waitCtx, client, forward, r)
				emulatorCheck(t, err)
				if d.Status == types.ServiceDeploymentStatusRollbackSuccessful {
					break
				}
				if nativeTerminal(d.Status) {
					t.Fatalf("rollback ended with %s", d.Status)
				}
				time.Sleep(250 * time.Millisecond)
			}
			if !nativeRollbackAccepted(d, NativeRollbackTarget{SourceRevisionARN: sourceRevision}) ||
				aws.ToString(describe().TaskDefinition) != source.TaskDefinitionARN {
				t.Fatal("rollback did not restore the captured source revision and workload")
			}
			if got := describe().DesiredCount; got != counts[1] {
				t.Fatalf("expected changed count %d before explicit restoration, got %d", counts[1], got)
			}
			_, err = client.UpdateService(ctx, &ecs.UpdateServiceInput{Cluster: &intent.ClusterARN, Service: &intent.ServiceARN, DesiredCount: &source.DesiredCount})
			emulatorCheck(t, err)
			final := describe()
			if final.DesiredCount != source.DesiredCount || aws.ToString(final.TaskDefinition) != source.TaskDefinitionARN {
				t.Fatalf("capacity restore returned count %d and task definition %s", final.DesiredCount, aws.ToString(final.TaskDefinition))
			}
		})
	}
	if len(cases) != 0 {
		t.Fatalf("missing catalog strategies: %v", cases)
	}
}
