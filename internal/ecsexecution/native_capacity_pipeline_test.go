package ecsexecution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func (p *nativeHealPipeline) commitCapacity(t *testing.T, count int, sha string) {
	t.Helper()
	p.raw.Git = &domain.GitSource{Repository: "org/desired", Ref: "main", Revision: strings.Repeat(sha, 40)}
	p.raw.Deployments[0].Capacity.Desired = count
	var err error
	p.catalog, err = manifest.Compile(p.raw)
	if err != nil {
		t.Fatal(err)
	}
}

func (p *nativeHealPipeline) queued(t *testing.T) applyrequest.Record {
	t.Helper()
	var queued applyrequest.Record
	for _, r := range p.rows(t) {
		if r.Status == applyrequest.StatusQueued {
			if queued.ID != "" {
				t.Fatal("multiple queued Applies")
			}
			queued = r
		}
	}
	if queued.ID == "" {
		t.Fatal("no queued Apply")
	}
	return queued
}

func TestNativeGitCapacityChangesScaleBothWaysWithoutSelfHealAndStartNewEpoch(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		for _, heal := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/selfHeal=%v", strategy, heal), func(t *testing.T) {
				p := setupNativeHealPipeline(t, strategy, heal)
				for index, count := range []int{4, 1, 2, 3} {
					p.commitCapacity(t, count, string(rune('b'+index)))
					p.tick(t)
					p.tick(t)
					queued := p.queued(t)
					if queued.Source.Git.Revision != p.catalog.Git.Revision || !queued.Principal.Automatic || queued.Operations[0].ECS.DesiredCount != count {
						t.Fatal("capacity approval was not bound to the new Git intent", queued)
					}
					before := p.f.writes
					p.run(t, 2)
					if p.f.writes != before+1 || p.f.input.DesiredCount == nil || int(*p.f.input.DesiredCount) != count || !p.f.input.ForceNewDeployment || string(p.f.input.DeploymentConfiguration.Strategy) != strategy {
						t.Fatal("count-only Git change did not start one reviewed native deployment")
					}
					p.executor.WorkerID = "capacity-restarted"
					p.now = p.now.Add(6 * time.Minute)
					p.healthy(t, queued.ID)
					p.run(t, 1)
					done, err := p.service.Get(context.Background(), queued.ID)
					if err != nil || done.Status != applyrequest.StatusSucceeded || done.ECSRuntime.DesiredCount != int32(count) || done.ECSRuntime.HealthyCount != int32(count) {
						t.Fatal("new capacity was not verified", done, err)
					}
					p.parent = done.ID
					p.tick(t)
					if len(p.rows(t)) != index+2 || p.f.writes != before+1 {
						t.Fatal("capacity change was duplicated")
					}
				}
				if heal {
					// Future drift repairs the newly verified Git count, not the old scale-in goal.
					p.f.service.DesiredCount = 4
					p.tick(t)
					repair := p.queued(t)
					if repair.Source.LastVerifiedApplyID != p.parent {
						t.Fatal("self-heal used stale capacity evidence")
					}
					p.run(t, 2)
					p.healthy(t, repair.ID)
					p.run(t, 1)
					done, err := p.service.Get(context.Background(), repair.ID)
					if err != nil || done.Status != applyrequest.StatusSucceeded || p.f.service.DesiredCount != 3 {
						t.Fatal(done, err)
					}
				}
			})
		}
	}
}

func TestNativeGitCapacityWaitsForAcceptedUpdateAndReconcilesLostResponse(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		t.Run(strategy, func(t *testing.T) {
			p := setupNativeHealPipeline(t, strategy, false)
			p.commitCapacity(t, 4, "b")
			p.tick(t)
			first := p.queued(t)
			p.run(t, 1)
			p.f.updateError = errors.New("accepted update response lost")
			p.run(t, 1)
			p.f.updateError = nil
			before := p.f.writes
			p.commitCapacity(t, 1, "c")
			p.tick(t)
			if len(p.rows(t)) != 2 {
				t.Fatal("new count preempted an accepted deployment")
			}
			p.executor.WorkerID = "capacity-response-restarted"
			p.now = p.now.Add(6 * time.Minute)
			p.enabled = false
			p.run(t, 1)
			p.healthy(t, first.ID)
			p.run(t, 1)
			if p.f.writes != before {
				t.Fatal("lost capacity response replayed UpdateService")
			}
			done, err := p.service.Get(context.Background(), first.ID)
			if err != nil || done.Status != applyrequest.StatusSucceeded || done.ECSRuntime.HealthyCount != 4 {
				t.Fatal("old approval did not verify its own count", done, err)
			}
			p.enabled = true
			p.tick(t)
			second := p.queued(t)
			p.run(t, 2)
			p.healthy(t, second.ID)
			p.run(t, 1)
			done, err = p.service.Get(context.Background(), second.ID)
			if err != nil || done.Status != applyrequest.StatusSucceeded || p.f.service.DesiredCount != 1 || p.f.writes != before+1 {
				t.Fatal(done, err)
			}
		})
	}
}

func TestNativeGitCapacityPauseRevocationStalenessAndUncertainOwnership(t *testing.T) {
	for _, name := range []string{"paused", "revoked", "stale", "uncertain", "rejected"} {
		t.Run(name, func(t *testing.T) {
			p := setupNativeHealPipeline(t, "CANARY", false)
			p.commitCapacity(t, 4, "b")
			p.tick(t)
			queued := p.queued(t)
			before := p.f.writes
			switch name {
			case "paused":
				p.enabled = false
			case "revoked":
				p.authorized = false
			case "stale":
				p.commitCapacity(t, 1, "c")
			case "uncertain", "rejected":
				p.f.noEffect = true
				p.f.updateError = errors.New("connection outcome unknown")
				if name == "rejected" {
					p.f.updateError = &types.ClientException{Message: aws.String("confirmed rejection")}
				}
			}
			if name != "uncertain" && name != "rejected" {
				if err := p.executor.RunOnce(context.Background()); err == nil {
					t.Fatal("invalid capacity approval executed")
				}
				if p.f.writes != before {
					t.Fatal("invalid approval changed capacity")
				}
				return
			}
			p.run(t, 2)
			p.f.updateError = nil
			if name == "rejected" {
				failed, err := p.service.Get(context.Background(), queued.ID)
				if err != nil || failed.Status != applyrequest.StatusFailed {
					t.Fatal(failed, err)
				}
				p.commitCapacity(t, 4, "d")
				p.tick(t)
				if len(p.rows(t)) != 2 || p.f.writes != before+1 {
					t.Fatal("documentation-only commit retried rejected capacity change")
				}
				return
			}
			p.run(t, 1)
			p.tick(t)
			if len(p.rows(t)) != 2 || p.f.writes != before+1 {
				t.Fatal("uncertain capacity request duplicated")
			}
			r, err := p.service.Get(context.Background(), queued.ID)
			if err != nil || r.Status != applyrequest.StatusRunning {
				t.Fatal("uncertain write lost durable ownership", r, err)
			}
		})
	}
}

func TestNativeGitCapacityAndImageChangeAreSubmittedTogether(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		t.Run(strategy, func(t *testing.T) {
			p := setupNativeHealPipeline(t, strategy, false)
			p.raw.Deployments[0].Image = "example/worker@sha256:" + strings.Repeat("b", 64)
			p.commitCapacity(t, 4, "b")
			p.tick(t)
			queued := p.queued(t)
			p.run(t, 2)
			if p.f.input.DesiredCount == nil || *p.f.input.DesiredCount != 4 || p.f.input.ForceNewDeployment {
				t.Fatal("combined count/image update payload is incorrect")
			}
			running, err := p.service.Get(context.Background(), queued.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := applyrequest.NativeRollbackEvidence(running); err == nil {
				t.Fatal("capacity-changing Apply bypassed the existing fixed-capacity rollback contract")
			}
			p.healthy(t, queued.ID)
			p.run(t, 1)
			done, err := p.service.Get(context.Background(), queued.ID)
			if err != nil || done.Status != applyrequest.StatusSucceeded || done.ECSRuntime.HealthyCount != 4 {
				t.Fatal(done, err)
			}
		})
	}
}
