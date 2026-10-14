package ecsreconcile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

type fixture struct {
	r          Reconciler
	catalog    manifest.Catalog
	executor   applyrequest.Executor
	store      applyrequest.FileStore
	plans      string
	writes     int
	authorized bool
	now        time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-repo"))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	compiled.Git = &domain.GitSource{Repository: "org/desired", Ref: "main", Revision: strings.Repeat("a", 40)}
	compiled.Deployments[0].ECS.SyncMode = "automatic"
	dir := t.TempDir()
	f := &fixture{catalog: compiled, plans: filepath.Join(dir, "plans.json"), store: applyrequest.NewFileStore(filepath.Join(dir, "applies.json")), authorized: true, now: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	plans := planpreview.NewFileStore(f.plans)
	service := applyrequest.Service{Store: f.store, EnableAWSApply: true, LoadPlan: plans.Get, Now: func() time.Time { return f.now }, CurrentFingerprint: func(ctx context.Context, name string) (string, error) {
		detail, err := catalog.BuildDeploymentOverviewDetail(f.catalog, nil, name)
		if err != nil {
			return "", err
		}
		return catalog.DeriveDeploymentSourceFingerprint(detail)
	}, AuthorizeCreate: func(ctx context.Context, r applyrequest.Record) (*applyrequest.ExecutionPrincipal, error) {
		identity := requestactor.AuthenticatedIdentityFromContext(ctx)
		if !f.authorized || !identity.Authenticated || identity.ID != "ecs-sync" {
			return nil, applyrequest.ErrExecutionUnauthorized
		}
		return &applyrequest.ExecutionPrincipal{Identity: identity, Automatic: applyrequest.AutomaticApprovalFromContext(ctx)}, nil
	}, DeployerRoleReadiness: func(context.Context, applyrequest.DeployerRoleTarget) (applyrequest.DeployerRoleReadiness, error) {
		e := f.catalog.Deployments[0].ECS
		return applyrequest.DeployerRoleReadiness{Configured: true, Ready: true, CheckedWithAWS: true, AWSAccountID: e.AWSAccountID, Region: e.Region, RoleARN: "arn:aws:iam::" + e.AWSAccountID + ":role/deployer"}, nil
	}}
	f.r = Reconciler{Enabled: true, Automatic: true, LiveECS: true, SubjectID: "ecs-sync", LoadCatalog: func(context.Context) (manifest.Catalog, error) { return f.catalog, nil }, Plans: plans, ListPlans: func(context.Context) ([]planpreview.Record, error) {
		records, err := planpreview.Load(f.plans)
		if os.IsNotExist(errors.Unwrap(err)) {
			return nil, nil
		}
		return records, err
	}, Applies: service}
	handler := &testHandler{f: f}
	f.executor = applyrequest.Executor{Service: service, WorkerID: "reconcile-test", ECS: &applyrequest.ECSExecution{Enabled: true, Authorize: func(context.Context, applyrequest.Record) error {
		if !f.authorized {
			return applyrequest.ErrExecutionUnauthorized
		}
		return nil
	}, CurrentIntent: func(context.Context, string) (*domain.ECSDeploymentIntent, error) {
		return f.catalog.Deployments[0].ECS.Clone(), nil
	}, Handlers: map[string]applyrequest.ECSOperationHandler{}}}
	// The planner's operation names are server-authored; use a real reviewed plan.
	detail, err := catalog.BuildDeploymentOverviewDetail(f.catalog, nil, "worker")
	if err != nil {
		t.Fatal(err)
	}
	preview := planpreview.Service{Store: plans, LoadDetail: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) { return detail, nil }}
	plan, err := preview.Create(context.Background(), "worker", planpreview.CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range plan.Record.Operations {
		f.executor.ECS.Handlers[op.Kind] = handler
	}
	if err := os.Remove(f.plans); err != nil {
		t.Fatal(err)
	}
	return f
}

type testHandler struct{ f *fixture }

func (h *testHandler) Preflight(context.Context, applyrequest.ECSOperationRequest) error { return nil }
func (h *testHandler) Execute(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	if len(r.PriorResults) < 2 {
		if err := r.BeforeMutation(ctx); err != nil {
			return applyrequest.ECSOperationResult{}, err
		}
		h.f.writes++
	}
	result := applyrequest.ECSOperationResult{TaskDefinitionARN: r.Intent.BaseTaskDefinitionARN, TaskDefinitionIntent: "reviewed", ServiceDeploymentID: "ecs-svc/verified"}
	if len(r.PriorResults) == 2 {
		result.Observation = &applyrequest.ECSRolloutObservation{State: "converged", ObservedAt: h.f.now.Format(time.RFC3339Nano), TaskDefinitionARN: r.Intent.BaseTaskDefinitionARN}
	}
	return result, nil
}
func (h *testHandler) Reconcile(context.Context, applyrequest.ECSOperationRequest) (applyrequest.ECSReconciliation, error) {
	return applyrequest.ECSReconciliation{}, errors.New("unexpected ambiguous replay")
}
func (f *fixture) tick(t *testing.T) {
	t.Helper()
	if err := f.r.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func (f *fixture) records(t *testing.T) []applyrequest.Record {
	t.Helper()
	records, err := f.store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return records
}
func (f *fixture) finish(t *testing.T) {
	t.Helper()
	for i := 0; i < 4; i++ {
		f.now = f.now.Add(time.Second)
		if err := f.executor.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}
func (f *fixture) change(revision, image string) {
	f.catalog.Git.Revision = strings.Repeat(revision, 40)
	f.catalog.Deployments[0].ECS.Image = image
	f.catalog.Deployments[0].DesiredArtifact.Image = image
}

func TestAutomaticGitSyncDeduplicatesRestartsAndHandlesBranchReversion(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.tick(t)
	f.tick(t)
	records := f.records(t)
	if len(records) != 1 || records[0].Principal == nil || !records[0].Principal.Automatic || records[0].Source.Git.Revision != strings.Repeat("a", 40) {
		t.Fatal("missing immutable automated approval")
	}
	original := records[0].ID
	originalImage := f.catalog.Deployments[0].ECS.Image
	f.finish(t)
	if f.records(t)[0].Status != applyrequest.StatusSucceeded || f.writes != 2 {
		t.Fatal("apply did not verify convergence")
	}
	// A new process using the same durable state does not create another Apply.
	restarted := f.r
	if err := restarted.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.records(t)) != 1 || f.writes != 2 {
		t.Fatal("restart reapplied unchanged Git")
	}
	f.change("b", "example/worker:next")
	f.tick(t)
	f.finish(t)
	if len(f.records(t)) != 2 || f.writes != 4 {
		t.Fatal("new Git image did not deploy exactly once")
	}
	// An out-of-order webhook is irrelevant: only the loader's current HEAD wins.
	f.tick(t)
	if len(f.records(t)) != 2 {
		t.Fatal("unchanged HEAD duplicated apply")
	}
	f.change("a", originalImage)
	f.tick(t)
	f.finish(t)
	records = f.records(t)
	if len(records) != 3 || f.writes != 6 {
		t.Fatal("returning to an older Git commit reused stale success")
	}
	found := false
	for _, r := range records {
		if r.ID != original && r.Source.Git.Revision == strings.Repeat("a", 40) && r.Status == applyrequest.StatusSucceeded {
			found = true
		}
	}
	if !found {
		t.Fatal("branch reversion omitted new execution evidence")
	}
	f.tick(t)
	if len(f.records(t)) != 3 {
		t.Fatal("branch reversion looped")
	}
}
func TestAutomaticSyncAllGatesAndSourceFailuresPreventApply(t *testing.T) {
	for _, name := range []string{"disabled", "manual", "global off", "live off", "apply off", "subject missing", "scope revoked", "unpinned", "source unavailable"} {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			switch name {
			case "disabled":
				f.r.Enabled = false
			case "manual":
				f.catalog.Deployments[0].ECS.SyncMode = ""
			case "global off":
				f.r.Automatic = false
			case "live off":
				f.r.LiveECS = false
			case "apply off":
				f.r.Applies.EnableAWSApply = false
			case "subject missing":
				f.r.SubjectID = ""
			case "scope revoked":
				f.authorized = false
			case "unpinned":
				f.catalog.Git = nil
			case "source unavailable":
				f.r.LoadCatalog = func(context.Context) (manifest.Catalog, error) {
					return manifest.Catalog{}, errors.New("GitHub unavailable")
				}
			}
			err := f.r.RunOnce(context.Background())
			if (name == "scope revoked" || name == "unpinned" || name == "source unavailable") && err == nil {
				t.Fatal("expected blocked evidence")
			}
			if len(f.records(t)) != 0 || f.writes != 0 {
				t.Fatal("disabled/untrusted sync created execution")
			}
		})
	}
}
func TestSelfHealIsExplicitAndDeduplicatesEachDriftEpoch(t *testing.T) {
	for _, heal := range []bool{false, true} {
		t.Run(map[bool]string{false: "report only", true: "self heal"}[heal], func(t *testing.T) {
			f := setup(t)
			f.catalog.Deployments[0].ECS.SelfHeal = heal
			f.tick(t)
			f.finish(t)
			probes := 0
			f.r.ObserveDrift = func(context.Context, domain.Deployment, applyrequest.Record) (Drift, error) {
				probes++
				return Drift{Detected: true, Signature: "stable-external-drift"}, nil
			}
			f.tick(t)
			f.tick(t)
			if !heal {
				if probes != 0 || len(f.records(t)) != 1 || f.writes != 2 {
					t.Fatal("report-only policy started self healing")
				}
				return
			}
			if len(f.records(t)) != 2 || probes != 1 {
				t.Fatal("same drift created duplicate queued applies")
			}
			f.finish(t)
			f.tick(t)
			if len(f.records(t)) != 3 {
				t.Fatal("recurring identical external drift reused previous success")
			}
			f.finish(t)
			f.r.ObserveDrift = func(context.Context, domain.Deployment, applyrequest.Record) (Drift, error) {
				return Drift{}, errors.New("throttled")
			}
			if err := f.r.RunOnce(context.Background()); err == nil {
				t.Fatal("unknown observation treated as drift")
			}
			if len(f.records(t)) != 3 || f.writes != 6 {
				t.Fatal("unavailable drift observation wrote")
			}
			f.r.ObserveDrift = func(context.Context, domain.Deployment, applyrequest.Record) (Drift, error) { return Drift{}, nil }
			f.tick(t)
			if len(f.records(t)) != 3 {
				t.Fatal("healthy service reapplied")
			}
		})
	}
}
func TestNewGitSupersedesUnsentButWaitsForPossiblySentServiceUpdate(t *testing.T) {
	for _, sent := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsent", true: "possibly sent"}[sent], func(t *testing.T) {
			f := setup(t)
			f.tick(t)
			old := f.records(t)[0]
			if _, err := f.store.Mutate(context.Background(), old.ID, func(r applyrequest.Record) (applyrequest.Record, error) {
				r.Operations[1].ECSCheckpoint = &applyrequest.ECSCheckpoint{State: "in_flight", MutationGuarded: true}
				if sent {
					r.Operations[1].ECSCheckpoint.MutationAuthorizedAt = "2026-10-14T00:00:00Z"
				}
				return r, nil
			}); err != nil {
				t.Fatal(err)
			}
			f.change("b", "example/worker:next")
			f.tick(t)
			records := f.records(t)
			if sent {
				if len(records) != 1 || records[0].Status != applyrequest.StatusQueued {
					t.Fatal("possibly sent write was abandoned")
				}
				return
			}
			if len(records) != 2 {
				t.Fatal("unsent old Git blocked replacement")
			}
			previous, err := f.store.Get(context.Background(), old.ID)
			if err != nil || previous.Status != applyrequest.StatusCancelled {
				t.Fatal("old request not safely cancelled")
			}
			f.finish(t)
			if f.writes != 2 {
				t.Fatal("superseded request executed AWS mutations")
			}
		})
	}
}
func TestFailedIntentRemainsHeldAcrossCommitsUntilExplicitRetryOrNewIntent(t *testing.T) {
	f := setup(t)
	f.tick(t)
	first := f.records(t)[0]
	if _, err := f.store.Mutate(context.Background(), first.ID, func(r applyrequest.Record) (applyrequest.Record, error) {
		r.Status = applyrequest.StatusFailed
		r.ECSRuntime = &applyrequest.ECSRolloutObservation{State: "failed", Reason: "rollout_failed"}
		return r, nil
	}); err != nil {
		t.Fatal(err)
	}
	f.catalog.Git.Revision = strings.Repeat("b", 40)
	f.tick(t)
	if len(f.records(t)) != 1 {
		t.Fatal("new commit bypassed failed-intent hold")
	}
	f.change("c", "example/worker:fixed")
	f.tick(t)
	f.finish(t)
	if len(f.records(t)) != 2 || f.writes != 2 {
		t.Fatal("fixed desired intent stayed held")
	}
}

func TestAutomaticSyncRejectsTwoDesiredOwnersForOnePhysicalService(t *testing.T) {
	f := setup(t)
	alias := f.catalog.Deployments[0]
	alias.Name = "worker-alias"
	f.catalog.Deployments = append(f.catalog.Deployments, alias)
	if err := f.r.RunOnce(context.Background()); err == nil {
		t.Fatal("conflicting service ownership accepted")
	}
	if len(f.records(t)) != 0 || f.writes != 0 {
		t.Fatal("two owners started competing deployments")
	}
}

func TestVerifiedCurrentApplyCanReleaseOlderUnsentQueuedReservation(t *testing.T) {
	f := setup(t)
	f.tick(t)
	old := f.records(t)[0]
	f.change("b", "example/worker:current")
	f.tick(t)
	f.finish(t)
	// Model an older queued request left by a manual-approval/executor interleaving.
	if _, err := f.store.Mutate(context.Background(), old.ID, func(r applyrequest.Record) (applyrequest.Record, error) {
		r.Status = applyrequest.StatusQueued
		return r, nil
	}); err != nil {
		t.Fatal(err)
	}
	f.tick(t)
	previous, err := f.store.Get(context.Background(), old.ID)
	if err != nil || previous.Status != applyrequest.StatusCancelled {
		t.Fatal("verified replacement left stale reservation active")
	}
	if len(f.records(t)) != 2 || f.writes != 2 {
		t.Fatal("releasing stale reservation issued another apply")
	}
}

func TestVerifiedOrderingUsesTimeRatherThanLexicalFractionalSeconds(t *testing.T) {
	previous := applyrequest.Record{ECSRuntime: &applyrequest.ECSRolloutObservation{ObservedAt: "2026-10-14T00:00:00Z"}}
	candidate := applyrequest.Record{ECSRuntime: &applyrequest.ECSRolloutObservation{ObservedAt: "2026-10-14T00:00:00.001Z"}}
	if !newerVerification(candidate, previous) || newerVerification(previous, candidate) {
		t.Fatal("same-second verification ordering reversed")
	}
}

func TestMalformedHistoricalSuccessCannotCrashOrSuppressGitReconciliation(t *testing.T) {
	f := setup(t)
	malformed := applyrequest.Record{ID: "legacy-incomplete", DeploymentName: "worker", Status: applyrequest.StatusSucceeded, ECSRuntime: &applyrequest.ECSRolloutObservation{State: "converged"}}
	if err := f.store.Save(context.Background(), malformed); err != nil {
		t.Fatal(err)
	}
	f.tick(t)
	if len(f.records(t)) != 2 {
		t.Fatal("incomplete historical projection suppressed current Git")
	}
}
