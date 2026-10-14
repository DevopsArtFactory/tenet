package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
)

func TestAdvancedInputSupportDoesNotEnableProductionExecution(t *testing.T) {
	t.Setenv("TENET_AWS_ENABLE_ECS_EXECUTION", "true")
	t.Setenv("TENET_AWS_ENABLE_APPLY", "true")
	source := newFilesystemCatalogSource(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plans.json")
	applyPath := filepath.Join(dir, "applies.json")
	options := newAPIOptionsWithCatalogSourceAndStores(buildinfo.Current("test", "test"), source, nil, planPath, applyPath)
	plans := planpreview.Service{Store: planpreview.NewFileStore(planPath), LoadDetail: options.OverviewDeployment}
	service := newApplyService(source, options, planPath, applyPath)
	for _, name := range []string{"web-blue-green", "web-canary", "web-linear"} {
		plan, err := plans.Create(context.Background(), name, planpreview.CreateRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(context.Background(), plan.Record.ID); !errors.Is(err, applyrequest.ErrPlanNotReady) {
			t.Fatalf("%s advanced inputs enabled live execution: %v", name, err)
		}
	}
	records, err := service.List(context.Background(), applyrequest.ListQuery{})
	if err != nil || len(records) != 0 {
		t.Fatal("unsupported production strategy queued live work")
	}
}
