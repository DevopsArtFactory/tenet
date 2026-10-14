package planpreview

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
)

func TestAdvancedTrafficSurvivesCompilationReviewAndPersistence(t *testing.T) {
	raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Deployments) != 3 {
		t.Fatal("missing native strategy examples")
	}
	for _, deployment := range compiled.Deployments {
		t.Run(deployment.Name, func(t *testing.T) {
			detail, err := catalog.BuildDeploymentOverviewDetail(compiled, nil, deployment.Name)
			if err != nil {
				t.Fatal(err)
			}
			if detail.ExecutionMode != "ecsNative" || detail.ECS.Advanced == nil {
				t.Fatal("native intent became CodeDeploy")
			}
			store := NewFileStore(filepath.Join(t.TempDir(), "plans.json"))
			service := Service{Store: store, LoadDetail: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) { return detail, nil }}
			result, err := service.Create(context.Background(), deployment.Name, CreateRequest{})
			if err != nil {
				t.Fatal(err)
			}
			persisted, err := service.Get(context.Background(), result.Record.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(persisted.Operations) != 3 || persisted.Risk.Level != "high" || strings.Contains(persisted.Operations[1].Summary, "rolling bounds") || !strings.Contains(persisted.Operations[1].Summary, "Post-traffic bake 5 minutes") {
				t.Fatal("review omitted native traffic semantics")
			}
			for _, operation := range persisted.Operations {
				if !reflect.DeepEqual(operation.ECS, detail.ECS) || operation.Kind == "CreateCodeDeployDeployment" {
					t.Fatal("native traffic input was lost or converted")
				}
			}
			// Each operation and persisted record must own its nested approved values.
			*persisted.Operations[0].ECS.Advanced.BakeTimeInMinutes = 999
			if *persisted.Operations[1].ECS.Advanced.BakeTimeInMinutes != 5 || *detail.ECS.Advanced.BakeTimeInMinutes != 5 {
				t.Fatal("operation payloads alias approved input")
			}
			reloaded, err := service.Get(context.Background(), result.Record.ID)
			if err != nil || *reloaded.Operations[0].ECS.Advanced.BakeTimeInMinutes != 5 {
				t.Fatal("caller changed durable review")
			}
		})
	}
}
func TestEachAdvancedPolicyChangeInvalidatesReviewAndDesiredGeneration(t *testing.T) {
	source, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
	if err != nil {
		t.Fatal(err)
	}
	index := -1
	for i := range source.Deployments {
		if source.Deployments[i].Name == "web-canary" {
			index = i
		}
	}
	if index < 0 {
		t.Fatal("missing canary fixture")
	}
	compiled, err := manifest.Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	original, err := catalog.BuildDeploymentOverviewDetail(compiled, nil, "web-canary")
	if err != nil {
		t.Fatal(err)
	}
	originalFingerprint, err := catalog.DeriveDeploymentSourceFingerprint(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"percent", "phase bake", "post bake", "routing", "role", "alarm", "hook", "port"} {
		t.Run(field, func(t *testing.T) {
			// Reload independent spec storage; no fixture mutation can leak across cases.
			raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
			if err != nil {
				t.Fatal(err)
			}
			c := raw.Deployments[index].ECS.Advanced
			switch field {
			case "percent":
				*c.Canary.Percent = 10
			case "phase bake":
				*c.Canary.BakeTimeInMinutes = 11
			case "post bake":
				*c.BakeTimeInMinutes = 6
			case "routing":
				c.Routing.TargetGroupARN = strings.ReplaceAll(c.Routing.TargetGroupARN, "-blue/", "-other/")
			case "role":
				c.Routing.InfrastructureRoleARN += "-other"
			case "alarm":
				c.Alarms.Names[0] += "-other"
			case "hook":
				c.LifecycleHooks[0].FunctionARN = strings.TrimSuffix(c.LifecycleHooks[0].FunctionARN, ":3") + ":4"
			case "port":
				c.Routing.ContainerPort = 8081
			}
			next, err := manifest.Compile(raw)
			if err != nil {
				t.Fatal(err)
			}
			detail, err := catalog.BuildDeploymentOverviewDetail(next, nil, "web-canary")
			if err != nil {
				t.Fatal(err)
			}
			fingerprint, err := catalog.DeriveDeploymentSourceFingerprint(detail)
			if err != nil || fingerprint == originalFingerprint {
				t.Fatal("changed traffic policy retained approval fingerprint")
			}
			if next.Deployments[index].Generation == compiled.Deployments[index].Generation {
				t.Fatal("changed traffic policy retained desired generation")
			}
		})
	}
}
