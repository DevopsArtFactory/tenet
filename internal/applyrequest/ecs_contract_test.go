package applyrequest

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/DevopsArtFactory/tenet/internal/spec"
)

func TestNativeECSContractSurvivesPlanApplyPersistenceAndRejectsStaleInputs(t *testing.T) {
	ctx := context.Background()
	raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-repo"))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	generation := compiled.Deployments[0].Generation
	if compiled.Deployments[0].ExecutionMode != "ecsNative" {
		t.Fatal("native inputs routed to legacy execution")
	}
	detail, err := catalog.BuildDeploymentOverviewDetail(compiled, nil, "worker")
	if err != nil {
		t.Fatal(err)
	}
	application, err := catalog.BuildApplicationOverviewDetail(compiled, nil, "worker")
	if err != nil || application.ECS == nil {
		t.Fatalf("application projection lost ECS: %v", err)
	}
	application.ECS.Image = "changed"
	if detail.ECS.Image == application.ECS.Image || compiled.Deployments[0].ECS.Image == application.ECS.Image {
		t.Fatal("catalog projections alias mutable intent")
	}
	// Old runtime evidence must not turn a native plan into CodeDeploy operations.
	detail.Rollout = &domain.RolloutSummary{Steps: []domain.RolloutStepSummary{{Kind: "CreateCodeDeployDeployment"}}}
	detail.DeployedArtifact.Image = "registry.example.com/worker:previous"
	dir := t.TempDir()
	plans := planpreview.Service{Store: planpreview.NewFileStore(filepath.Join(dir, "plans.json")), LoadDetail: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) { return detail, nil }}
	first, err := plans.Create(ctx, "worker", planpreview.CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	again, err := plans.Create(ctx, "worker", planpreview.CreateRequest{})
	if err != nil || again.Record.ID != first.Record.ID || again.Created {
		t.Fatalf("equivalent intent is not stable: %v", err)
	}
	if len(first.Record.Operations) != 3 {
		t.Fatalf("expected registration/update/verify, got %+v", first.Record.Operations)
	}
	for _, op := range first.Record.Operations {
		if op.ECS == nil || op.ECS.ServiceARN != detail.ECS.ServiceARN || strings.Contains(op.Kind, "CodeDeploy") {
			t.Fatalf("lost or misrouted intent: %+v", op)
		}
	}
	if !strings.Contains(first.Record.Operations[0].Summary, "previous") || !strings.Contains(first.Record.Operations[0].Summary, detail.ECS.Image) {
		t.Fatal("missing before/after image summary")
	}
	currentFingerprint := first.Record.Source.Fingerprint
	store := NewFileStore(filepath.Join(dir, "applies.json"))
	service := Service{LoadPlan: plans.Get, Store: store, EnableAWSApply: true, CurrentFingerprint: func(context.Context, string) (string, error) { return currentFingerprint, nil }}
	apply, err := service.Create(ctx, first.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.Get(ctx, apply.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, op := range stored.Operations {
		if op.ECS == nil || *op.ECS != *first.Record.Operations[i].ECS || op.Kind != first.Record.Operations[i].Kind {
			t.Fatalf("apply dropped reviewed input: %+v", op)
		}
	}
	if err := (Executor{Service: service}).RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if err := (Executor{Service: service}).RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	stopped, err := store.Get(ctx, apply.Record.ID)
	if err != nil || stopped.Status != StatusFailed || stopped.Error != executorFailedError {
		t.Fatalf("contract accidentally enabled AWS mutation: %+v, %v", stopped, err)
	}
	for _, op := range stopped.Operations {
		if op.ECS == nil || op.ECS.Image != detail.ECS.Image {
			t.Fatal("lifecycle transition discarded reviewed intent")
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*domain.ECSDeploymentIntent)
	}{
		{"base revision", func(e *domain.ECSDeploymentIntent) {
			e.BaseTaskDefinitionARN = strings.TrimSuffix(e.BaseTaskDefinitionARN, ":17") + ":18"
		}},
		{"target", func(e *domain.ECSDeploymentIntent) { e.ServiceARN += "-other" }},
		{"count", func(e *domain.ECSDeploymentIntent) { e.DesiredCount++ }},
		{"bounds", func(e *domain.ECSDeploymentIntent) { e.MaximumPercent = 150 }},
		{"image", func(e *domain.ECSDeploymentIntent) { e.Image = "registry.example.com/worker:v3" }},
		{"container", func(e *domain.ECSDeploymentIntent) { e.ContainerName = "sidecar" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := detail
			changed.ECS = detail.ECS.Clone()
			tc.change(changed.ECS)
			fp, err := catalog.DeriveDeploymentSourceFingerprint(changed)
			if err != nil || fp == first.Record.Source.Fingerprint {
				t.Fatalf("input change did not invalidate fingerprint: %v", err)
			}
			currentFingerprint = fp
			if _, err := service.Create(ctx, first.Record.ID); !errors.Is(err, ErrSourceFingerprintConflict) {
				t.Fatalf("stale apply accepted: %v", err)
			}
		})
	}
	tagDetail := detail
	tagDetail.ECS = detail.ECS.Clone()
	tagDetail.ECS.Image = "registry.example.com/worker:v3"
	tagPlans := planpreview.Service{Store: plans.Store, LoadDetail: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) { return tagDetail, nil }}
	tagPlan, err := tagPlans.Create(ctx, "worker", planpreview.CreateRequest{})
	if err != nil || tagPlan.Record.ID == first.Record.ID || tagPlan.Record.Risk.Level == "low" || !strings.Contains(strings.Join(tagPlan.Record.Risk.Reasons, " "), "mutable") {
		t.Fatalf("mutable image review warning missing: %+v, %v", tagPlan.Record.Risk, err)
	}
	bad := raw
	bad.Environments = append(raw.Environments[:0:0], raw.Environments...)
	bad.Environments[0].Region = "us-east-1"
	bad.Accounts = append(raw.Accounts[:0:0], raw.Accounts...)
	bad.Accounts[0].AllowedRegions = append([]string(nil), "us-east-1")
	if _, err := manifest.Compile(bad); err == nil {
		t.Fatal("compiler allowed ECS target to escape its environment")
	}
	same := raw
	same.Deployments = append(raw.Deployments[:0:0], raw.Deployments...)
	sameECS := *same.Deployments[0].ECS
	sameECS.MinimumHealthyPercent = nil
	sameECS.MaximumPercent = nil
	same.Deployments[0].ECS = &sameECS
	defaulted, err := manifest.Compile(same)
	if err != nil || defaulted.Deployments[0].Generation != generation {
		t.Fatalf("explicit and default rolling settings are not equivalent: %v", err)
	}
	// Every native strategy uses native planner steps; legacy manifests keep their old behavior.
	for _, strategy := range []string{"rolling", "blueGreen", "canary", "linear"} {
		input := raw
		input.Deployments = append(raw.Deployments[:0:0], raw.Deployments...)
		input.Deployments[0].Strategy.Type = spec.StrategyType(strategy)
		c, err := manifest.Compile(input)
		if err != nil {
			t.Fatal(err)
		}
		plan := reconcile.Planner{}.Build(c.Deployments[0], domain.DeploymentStatus{})
		if !plan.ContainsStep(reconcile.StepRegisterECSTaskDefinition) || plan.ContainsStep(reconcile.StepCreateCodeDeployDeployment) {
			t.Fatalf("native %s misrouted: %+v", strategy, plan)
		}
	}
	raw.Deployments[0].ECS.BaseTaskDefinitionARN = strings.TrimSuffix(raw.Deployments[0].ECS.BaseTaskDefinitionARN, ":17") + ":18"
	updated, err := manifest.Compile(raw)
	if err != nil || updated.Deployments[0].Generation == generation {
		t.Fatalf("base revision did not change desired generation: %v", err)
	}
	raw.Deployments[0].ECS = nil
	legacy, err := manifest.Compile(raw)
	if err != nil || legacy.Deployments[0].ExecutionMode != "ecsRolling" {
		t.Fatalf("legacy manifest changed execution mode: %v", err)
	}
}

func TestApplyOperationProjectionPreservesKindsForEveryTarget(t *testing.T) {
	for _, kind := range []string{"RegisterECSTaskDefinition", "UpdateECSService", "StartInstanceRefresh", "CreateCodeDeployDeployment"} {
		t.Run(kind, func(t *testing.T) {
			plan := planpreview.Record{Operations: []planpreview.Operation{{ID: "reviewed-operation", Kind: kind, Target: "reviewed target", Summary: "reviewed intent"}}}
			operations := deriveOperations(plan, OperationStatusQueued)
			if len(operations) != 1 || operations[0].Kind != kind || operations[0].ID != "reviewed-operation" {
				t.Fatalf("plan operation kind/identity was discarded: %+v", operations)
			}
		})
	}
}
