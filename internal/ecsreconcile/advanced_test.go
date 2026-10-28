package ecsreconcile

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/DevopsArtFactory/tenet/internal/spec"
)

func setupAdvanced(t *testing.T, strategy string) *fixture {
	t.Helper()
	f := setup(t)
	raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw.Deployments {
		raw.Deployments[i].ECS.SyncPolicy = &spec.ECSSyncPolicy{Mode: "automatic"}
	}
	raw.Git = f.catalog.Git.Clone()
	compiled, err := manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range compiled.Deployments {
		if d.ECS.DeploymentStrategy == strategy {
			compiled.Deployments = []domain.Deployment{d}
			break
		}
	}
	f.catalog = compiled
	f.r.AllowAdvanced = true
	f.executor.ECS.AllowAdvanced = true
	return f
}
func TestAdvancedAutomaticGitCommitsDeduplicateAndRevertWithFreshEvidence(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		t.Run(strategy, func(t *testing.T) {
			f := setupAdvanced(t, strategy)
			f.tick(t)
			f.tick(t)
			rows := f.records(t)
			if len(rows) != 1 || !rows[0].Principal.Automatic || rows[0].Source.Git.Revision != strings.Repeat("a", 40) {
				t.Fatal("missing pinned automatic approval", rows)
			}
			first := rows[0].ID
			original := f.catalog.Deployments[0].ECS.Image
			f.finish(t)
			f.tick(t)
			if len(f.records(t)) != 1 || f.writes != 2 {
				t.Fatal("same commit redeployed")
			}
			f.change("b", "example/worker@sha256:"+strings.Repeat("b", 64))
			f.tick(t)
			f.finish(t)
			f.change("a", original)
			f.tick(t)
			f.finish(t)
			f.tick(t)
			rows = f.records(t)
			if len(rows) != 3 || f.writes != 6 {
				t.Fatal("native branch reversion reused stale success", rows)
			}
			for _, r := range rows {
				if r.ID != first && r.Source.Git.Revision == strings.Repeat("a", 40) && r.Source.LastVerifiedApplyID != "" {
					return
				}
			}
			t.Fatal("reversion lost verification epoch")
		})
	}
}
func TestAdvancedAutomaticGatesAndDuplicateOwnersPreventApproval(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		for _, name := range []string{"advanced off", "automatic off", "manual", "live off", "apply off", "subject missing", "RBAC revoked", "duplicate owner", "missing policy"} {
			t.Run(strategy+"/"+name, func(t *testing.T) {
				f := setupAdvanced(t, strategy)
				switch name {
				case "advanced off":
					f.r.AllowAdvanced = false
				case "automatic off":
					f.r.Automatic = false
				case "manual":
					f.catalog.Deployments[0].ECS.SyncMode = "manual"
				case "live off":
					f.r.LiveECS = false
				case "apply off":
					f.r.Applies.EnableAWSApply = false
				case "subject missing":
					f.r.SubjectID = ""
				case "RBAC revoked":
					f.authorized = false
				case "duplicate owner":
					alias := f.catalog.Deployments[0]
					alias.Name += "-alias"
					f.catalog.Deployments = append(f.catalog.Deployments, alias)
				case "missing policy":
					f.catalog.Deployments[0].ECS.Advanced = nil
				}
				err := f.r.RunOnce(context.Background())
				if (name == "RBAC revoked" || name == "duplicate owner" || name == "missing policy") && err == nil {
					t.Fatal("missing blocked evidence")
				}
				if len(f.records(t)) != 0 || f.writes != 0 {
					t.Fatal("disabled/untrusted advanced sync approved execution")
				}
			})
		}
	}
}
func TestAdvancedNewGitSupersedesOnlyPositivelyUnsentUpdates(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		for _, state := range []string{"queued", "guarded unsent", "possibly sent", "accepted"} {
			t.Run(strategy+"/"+state, func(t *testing.T) {
				f := setupAdvanced(t, strategy)
				f.tick(t)
				old := f.records(t)[0]
				if state != "queued" {
					if _, err := f.store.Mutate(context.Background(), old.ID, func(r applyrequest.Record) (applyrequest.Record, error) {
						cp := &applyrequest.ECSCheckpoint{State: "in_flight", MutationGuarded: true}
						if state == "possibly sent" {
							cp.MutationAuthorizedAt = "sent"
						}
						if state == "accepted" {
							cp.State = "complete"
						}
						r.Operations[1].ECSCheckpoint = cp
						return r, nil
					}); err != nil {
						t.Fatal(err)
					}
				}
				f.change("b", "example/worker:next")
				f.tick(t)
				rows := f.records(t)
				if state == "accepted" || state == "possibly sent" {
					if len(rows) != 1 {
						t.Fatal("accepted/uncertain native update was replaced")
					}
					return
				}
				original, err := f.store.Get(context.Background(), old.ID)
				if err != nil || original.Status != applyrequest.StatusCancelled || len(rows) != 2 {
					t.Fatal("unsent native intent was not replaced", err)
				}
				f.finish(t)
				if f.writes != 2 {
					t.Fatal("superseded native intent wrote to AWS")
				}
			})
		}
	}
}
func TestAdvancedFailureAndActiveRecoveryBlockAutomaticRedeployment(t *testing.T) {
	f := setupAdvanced(t, "CANARY")
	f.tick(t)
	parent := f.records(t)[0]
	if _, err := f.store.Mutate(context.Background(), parent.ID, func(r applyrequest.Record) (applyrequest.Record, error) {
		r.Status = applyrequest.StatusFailed
		return r, nil
	}); err != nil {
		t.Fatal(err)
	}
	f.catalog.Git.Revision = strings.Repeat("b", 40)
	f.tick(t)
	if len(f.records(t)) != 1 {
		t.Fatal("new commit bypassed failed-intent hold")
	}
	// Recovery owns the same physical service until terminal, even for a newer image.
	recovery := applyrequest.Record{ID: "native-recovery", DeploymentName: parent.DeploymentName, Status: applyrequest.StatusQueued, NativeRollback: &applyrequest.ECSNativeRollback{ParentApplyID: parent.ID}, Operations: parent.Operations[1:]}
	if err := f.store.Save(context.Background(), recovery); err != nil {
		t.Fatal(err)
	}
	f.change("c", "example/worker:fixed")
	f.tick(t)
	if len(f.records(t)) != 2 {
		t.Fatal("new Git bypassed recovery service reservation")
	}
	if _, err := f.store.Mutate(context.Background(), recovery.ID, func(r applyrequest.Record) (applyrequest.Record, error) {
		r.Status = applyrequest.StatusSucceeded
		r.ECSRuntime = &applyrequest.ECSRolloutObservation{State: "converged"}
		return r, nil
	}); err != nil {
		t.Fatal(err)
	}
	f.tick(t)
	if len(f.records(t)) != 3 || f.writes != 0 {
		t.Fatal("recovered source was mistaken for desired Git success")
	}
}
