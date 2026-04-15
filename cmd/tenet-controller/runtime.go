package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

func emitRuntimePlans(logger *log.Logger, config startupConfig) error {
	if config.RepoRoot == "" {
		logger.Printf("controller runtime disabled reason=no_repo_root")
		return nil
	}

	compiled, err := catalog.LoadFromRepository(config.RepoRoot)
	if err != nil {
		return fmt.Errorf("load runtime catalog: %w", err)
	}

	statuses := map[string]domain.DeploymentStatus{}
	if config.StatusPath != "" {
		snapshots, err := statusstore.Load(config.StatusPath)
		if err != nil {
			return fmt.Errorf("load runtime statuses: %w", err)
		}
		for _, snapshot := range snapshots {
			statuses[snapshot.DeploymentName] = snapshot.Status
		}
	}

	if len(compiled.Deployments) == 0 {
		logger.Printf("controller runtime found no deployments")
		return nil
	}

	planner := reconcile.Planner{}
	for _, deployment := range compiled.Deployments {
		status := statuses[deployment.Name]
		plan := planner.Build(deployment, status)
		logger.Printf(
			"controller reconcile deployment=%s desired_generation=%d observed_generation=%d sync_state=%s steps=%s",
			deployment.Name,
			deployment.Generation,
			status.ObservedGeneration,
			syncStateLabel(status.Sync.State),
			stepSummary(plan),
		)
	}

	return nil
}

func syncStateLabel(state domain.SyncState) string {
	if state == "" {
		return "Unknown"
	}
	return string(state)
}

func stepSummary(plan reconcile.Plan) string {
	if len(plan.Steps) == 0 {
		return "none"
	}
	steps := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		steps = append(steps, string(step.Kind))
	}
	return strings.Join(steps, ",")
}
