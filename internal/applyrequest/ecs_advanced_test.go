package applyrequest

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
)

func TestAdvancedTrafficApprovalProjectionPreservesIndependentTypedInputs(t *testing.T) {
	raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := catalog.BuildDeploymentOverviewDetail(compiled, nil, "web-canary")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	plans := planpreview.Service{Store: planpreview.NewFileStore(filepath.Join(dir, "plans.json")), LoadDetail: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) { return detail, nil }}
	plan, err := plans.Create(context.Background(), detail.Name, planpreview.CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	service := Service{LoadPlan: plans.Get, Store: NewFileStore(filepath.Join(dir, "applies.json")), CurrentFingerprint: func(context.Context, string) (string, error) { return plan.Record.Source.Fingerprint, nil }}
	created, err := service.Create(context.Background(), plan.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range created.Record.Operations {
		if !reflect.DeepEqual(operation.ECS, detail.ECS) {
			t.Fatal("Apply dropped reviewed advanced traffic policy")
		}
	}
	*created.Record.Operations[0].ECS.Advanced.Canary.Percent = 99
	if *created.Record.Operations[1].ECS.Advanced.Canary.Percent != 5.5 {
		t.Fatal("Apply operation payloads alias")
	}
	persisted, err := service.Get(context.Background(), created.Record.ID)
	if err != nil || *persisted.Operations[0].ECS.Advanced.Canary.Percent != 5.5 {
		t.Fatal("caller mutated durable advanced approval")
	}
}
