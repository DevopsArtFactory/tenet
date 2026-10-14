package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/ecsexecution"
	"github.com/DevopsArtFactory/tenet/internal/ecsreconcile"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

func ecsReconcileEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TENET_ECS_RECONCILE")), "true")
}
func ecsAutoSyncEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TENET_ECS_AUTO_SYNC")), "true")
}
func ecsSourcePinningEnabled() bool { return ecsExecutionEnabled() || ecsReconcileEnabled() }
func ecsReconcileInterval() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv("TENET_ECS_RECONCILE_INTERVAL"))
	if raw == "" {
		return 30 * time.Second, nil
	}
	interval, err := time.ParseDuration(raw)
	if err != nil || interval < time.Second {
		return 0, fmt.Errorf("TENET_ECS_RECONCILE_INTERVAL must be a duration of at least 1s")
	}
	return interval, nil
}
func validateECSReconcileConfiguration(source apiCatalogSource) error {
	if ecsAutoSyncEnabled() && (!ecsReconcileEnabled() || !ecsExecutionEnabled() || !awsApplyEnabled() || strings.TrimSpace(os.Getenv("TENET_ECS_AUTO_SYNC_SUBJECT")) == "") {
		return fmt.Errorf("automatic ECS sync requires TENET_ECS_RECONCILE, TENET_AWS_ENABLE_ECS_EXECUTION, TENET_AWS_ENABLE_APPLY=true and TENET_ECS_AUTO_SYNC_SUBJECT")
	}
	if !ecsReconcileEnabled() {
		return nil
	}
	if source.kind != apiCatalogSourceGitHub || strings.TrimSpace(source.githubRequest.Ref) == "" {
		return fmt.Errorf("ECS Git reconciliation requires a GitHub catalog with an explicit TENET_CATALOG_GITHUB_REF")
	}
	_, err := ecsReconcileInterval()
	return err
}
func newECSGitReconciler(source apiCatalogSource, options api.Options, statuses statusstore.Store, planPath, applyPath string, logger *slog.Logger) ecsreconcile.Reconciler {
	interval, _ := ecsReconcileInterval() // Startup rejects an invalid interval.
	service := newApplyService(source, options, planPath, applyPath)
	subject := strings.TrimSpace(os.Getenv("TENET_ECS_AUTO_SYNC_SUBJECT"))
	return ecsreconcile.Reconciler{
		Enabled: ecsReconcileEnabled(), Automatic: ecsAutoSyncEnabled(), LiveECS: ecsExecutionEnabled(), SubjectID: subject, Interval: interval, Logger: logger,
		LoadCatalog: source.loadCatalog, Plans: planpreview.NewFileStore(planPath),
		ListPlans: func(ctx context.Context) ([]planpreview.Record, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return loadPlanPreviewRecords(planPath)
		},
		Applies: service, Statuses: statuses,
		ObserveDrift: func(ctx context.Context, deployment domain.Deployment, verified applyrequest.Record) (ecsreconcile.Drift, error) {
			principal := &applyrequest.ExecutionPrincipal{Identity: requestactor.AuthenticatedIdentity{Authenticated: true, ID: subject}}
			if err := authorizeECSPrincipal(ctx, options, principal, deployment.Name); err != nil {
				return ecsreconcile.Drift{}, err
			}
			role, err := service.DeployerRoleReadiness(ctx, applyrequest.DeployerRoleTarget{DeploymentName: deployment.Name})
			if err != nil {
				return ecsreconcile.Drift{}, err
			}
			probe := ecsexecution.ManagedDrift{ClientFor: func(ctx context.Context, role applyrequest.DeployerRoleReadiness) (ecsexecution.DriftClient, error) {
				return ecsexecution.DeployerClient(ctx, role)
			}, CheckCapabilities: checkECSCapabilities}
			detected, signature, err := probe.Observe(ctx, *deployment.ECS, role, verified)
			return ecsreconcile.Drift{Detected: detected, Signature: signature}, err
		},
	}
}
