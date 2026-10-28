package ecsreconcile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

type Drift struct {
	Detected  bool
	Signature string
}

// Reconciler only creates reviewed control-plane records. The existing ECS
// executor remains the sole owner of AWS mutation and convergence checkpoints.
type Reconciler struct {
	Enabled       bool
	Automatic     bool
	LiveECS       bool
	AllowAdvanced bool
	SubjectID     string
	Interval      time.Duration
	Logger        *slog.Logger
	LoadCatalog   func(context.Context) (manifest.Catalog, error)
	Plans         planpreview.Store
	ListPlans     func(context.Context) ([]planpreview.Record, error)
	Applies       applyrequest.Service
	Statuses      statusstore.Store
	ObserveDrift  func(context.Context, domain.Deployment, applyrequest.Record) (Drift, error)
}

func (r Reconciler) Run(ctx context.Context) error {
	interval := r.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	for {
		if err := r.RunOnce(ctx); err != nil && ctx.Err() == nil && r.Logger != nil {
			r.Logger.Warn("ECS Git reconciliation blocked", "error", err)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (r Reconciler) RunOnce(ctx context.Context) error {
	if !r.Enabled {
		return nil
	}
	if r.LoadCatalog == nil || r.Plans == nil || r.ListPlans == nil || r.Applies.Store == nil {
		return fmt.Errorf("Git reconciler requires catalog, durable plan and Apply stores")
	}
	compiled, err := r.LoadCatalog(ctx)
	if err != nil {
		return err
	}
	if compiled.Git == nil || compiled.Git.Revision == "" {
		return fmt.Errorf("Git reconciler requires an immutable source commit")
	}
	plans, err := r.ListPlans(ctx)
	if err != nil {
		return err
	}

	owners := map[string]int{}
	for _, deployment := range compiled.Deployments {
		if deployment.ECS != nil {
			owners[deployment.ECS.ServiceARN]++
		}
	}
	var failures error
	for _, deployment := range compiled.Deployments {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if deployment.ECS == nil || deployment.ECS.DeploymentController != "ECS" {
			continue
		}

		switch deployment.ECS.DeploymentStrategy {
		case "ROLLING", "BLUE_GREEN", "CANARY", "LINEAR":
		default:
			continue
		}

		if deployment.ECS.SyncMode == "automatic" && owners[deployment.ECS.ServiceARN] > 1 {
			failures = errors.Join(failures, fmt.Errorf("deployment %s: automatic sync requires one declared owner per physical ECS service", deployment.Name))
			continue
		}
		// Bound source/auth/drift I/O per deployment so one unavailable target
		// cannot indefinitely starve the remaining catalog.
		deploymentCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := r.reconcileDeployment(deploymentCtx, compiled, deployment, plans)
		cancel()
		if err != nil {
			failures = errors.Join(failures, fmt.Errorf("deployment %s: %w", deployment.Name, err))
		}
	}
	return failures
}

func (r Reconciler) reconcileDeployment(ctx context.Context, compiled manifest.Catalog, deployment domain.Deployment, plans []planpreview.Record) error {
	intent := deployment.ECS
	var plan planpreview.Record
	for _, candidate := range plans {
		if candidate.Status == planpreview.StatusReady && candidate.DeploymentName == deployment.Name && candidate.Source.ManagedDrift == nil && reflect.DeepEqual(candidate.Source.Git, compiled.Git) && len(candidate.Operations) == 3 && reflect.DeepEqual(candidate.Operations[0].ECS, intent) {
			plan = candidate
			break
		}
	}
	if plan.ID == "" {
		var err error
		plan, err = r.createPlan(ctx, compiled, deployment.Name, nil)
		if err != nil {
			return err
		}
	}
	if !r.Automatic || !r.LiveECS || !r.Applies.EnableAWSApply || r.SubjectID == "" || intent.SyncMode != "automatic" || (intent.DeploymentStrategy != "ROLLING" && !r.AllowAdvanced) {
		return nil
	}
	if intent.DeploymentStrategy != "ROLLING" && (intent.Advanced == nil || intent.SelfHeal) {
		return fmt.Errorf("advanced automatic sync requires reviewed traffic configuration and selfHeal disabled")
	}
	if r.Applies.AuthorizeCreate == nil {
		return fmt.Errorf("automatic sync requires a trusted authorization provider")
	}
	store, ok := r.Applies.Store.(applyrequest.ECSExecutionStore)
	if !ok {
		return fmt.Errorf("automatic sync requires process-shared execution storage")
	}
	unlock, err := store.LockExecution(ctx, deployment.Name, intent.ServiceARN)
	if err != nil {
		if errors.Is(err, applyrequest.ErrExecutionBusy) {
			return nil
		}
		return err
	}
	defer unlock()
	// Refresh under the execution lock before deciding whether another process has
	// already queued/finished this intent. Poll/webhook ordering never picks a SHA.
	records, err := r.Applies.List(ctx, applyrequest.ListQuery{})
	if err != nil {
		return err
	}
	var superseded []applyrequest.Record
	var verified *applyrequest.Record
	var successes []applyrequest.Record
	parentVerified := map[string]bool{}
	for i := range records {
		record := records[i]
		if !sameService(record, deployment) {
			continue
		}
		if record.Status == applyrequest.StatusQueued || record.Status == applyrequest.StatusRunning {
			wanted := recordIntent(record)
			same := reflect.DeepEqual(wanted, intent)
			unstarted := !hasCheckpoints(record)
			if same && (!unstarted || reflect.DeepEqual(record.Source.Git, compiled.Git)) {
				return nil
			}
			if !applyrequest.CanSupersedeECS(record) {
				return nil
			}
			superseded = append(superseded, record)
		}
		if record.Recovery == nil && record.NativeRollback == nil && record.NativeRollbackRequestID == "" && len(record.Operations) == 3 && record.Operations[0].ECS != nil && record.Status == applyrequest.StatusSucceeded && record.ECSRuntime != nil && record.ECSRuntime.State == "converged" {
			successes = append(successes, record)
			if record.Source.LastVerifiedApplyID != "" {
				parentVerified[record.Source.LastVerifiedApplyID] = true
			}
		}
	}
	for i := range successes {
		record := successes[i]
		if parentVerified[record.ID] {
			continue
		}
		if verified == nil || newerVerification(record, *verified) {
			copy := record
			verified = &copy
		}
	}

	var trigger *planpreview.ManagedDriftTrigger
	if verified != nil && reflect.DeepEqual(verified.Operations[0].ECS, intent) && len(superseded) == 0 {
		if !intent.SelfHeal {
			return nil
		}
		if r.ObserveDrift == nil {
			return fmt.Errorf("self-heal requires current managed-field observation")
		}
		observation, err := r.ObserveDrift(ctx, deployment, *verified)
		if err != nil {
			return err
		}
		if !observation.Detected {
			return nil
		}
		if observation.Signature == "" {
			return fmt.Errorf("self-heal observation omitted drift identity")
		}
		trigger = &planpreview.ManagedDriftTrigger{Signature: observation.Signature, LastVerifiedApplyID: verified.ID}
		plan, err = r.createPlan(ctx, compiled, deployment.Name, trigger)
		if err != nil {
			return err
		}
	}

	if verified != nil && !reflect.DeepEqual(verified.Operations[0].ECS, intent) {
		plan, err = r.createPlan(ctx, compiled, deployment.Name, &planpreview.ManagedDriftTrigger{Signature: "intervening-apply:" + verified.ID, LastVerifiedApplyID: verified.ID})
		if err != nil {
			return err
		}
	}
	actorCtx := r.automationContext(ctx)
	created, err := r.Applies.Create(actorCtx, plan.ID)
	if err != nil {
		if errors.Is(err, applyrequest.ErrFailedIntentHeld) {
			return nil
		}
		return err
	}
	// Once a service write may have happened, the old Apply must reconcile and
	// finish observing. Only positively unsent updates can be replaced by new Git.
	for _, previous := range superseded {
		if !applyrequest.ECSReplacementNeeded(created.Record, previous) {
			continue
		}
		if _, err := r.Applies.SupersedeECS(actorCtx, previous.ID, created.Record.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r Reconciler) createPlan(ctx context.Context, compiled manifest.Catalog, name string, trigger *planpreview.ManagedDriftTrigger) (planpreview.Record, error) {
	var snapshots []statusstore.Snapshot
	var err error
	if r.Statuses != nil {
		snapshots, err = r.Statuses.List(ctx)
		if err != nil {
			return planpreview.Record{}, err
		}
	}
	detail, err := catalog.BuildDeploymentOverviewDetail(compiled, snapshots, name)
	if err != nil {
		return planpreview.Record{}, err
	}
	service := planpreview.Service{Store: r.Plans, LoadDetail: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) { return detail, nil }}
	result, err := service.Create(r.automationContext(ctx), name, planpreview.CreateRequest{ManagedDrift: trigger})
	return result.Record, err
}
func (r Reconciler) automationContext(ctx context.Context) context.Context {
	id := r.SubjectID
	if id == "" {
		id = "system:ecs-git-planner"
	}
	ctx = requestactor.WithContext(ctx, requestactor.Actor{Available: true, ID: id, Name: "ECS Git reconciler"})
	// Only the explicitly configured automation identity can approve an Apply.
	if r.Automatic && r.LiveECS && r.SubjectID != "" {
		ctx = requestactor.WithAuthenticatedIdentityContext(ctx, requestactor.AuthenticatedIdentity{Authenticated: true, ID: r.SubjectID})
		ctx = applyrequest.WithAutomaticApproval(ctx)
	}
	return ctx
}
func sameService(record applyrequest.Record, deployment domain.Deployment) bool {
	return record.DeploymentName == deployment.Name || (len(record.Operations) > 0 && record.Operations[0].ECS != nil && record.Operations[0].ECS.ServiceARN == deployment.ECS.ServiceARN)
}
func recordIntent(record applyrequest.Record) *domain.ECSDeploymentIntent {
	if record.Recovery != nil {
		return record.Recovery.ForwardIntent
	}
	if len(record.Operations) > 0 {
		return record.Operations[0].ECS
	}
	return nil
}
func hasCheckpoints(record applyrequest.Record) bool {
	for _, operation := range record.Operations {
		if operation.ECSCheckpoint != nil {
			return true
		}
	}
	return false
}

func newerVerification(candidate, previous applyrequest.Record) bool {
	candidateAt, candidateErr := time.Parse(time.RFC3339Nano, candidate.ECSRuntime.ObservedAt)
	previousAt, previousErr := time.Parse(time.RFC3339Nano, previous.ECSRuntime.ObservedAt)
	if candidateErr == nil && previousErr == nil && !candidateAt.Equal(previousAt) {
		return candidateAt.After(previousAt)
	}
	candidateAt, candidateErr = time.Parse(time.RFC3339Nano, candidate.CreatedAt)
	previousAt, previousErr = time.Parse(time.RFC3339Nano, previous.CreatedAt)
	return candidateErr == nil && previousErr == nil && candidateAt.After(previousAt)
}
