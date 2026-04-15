package catalog

import (
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

func TestBuildAccountOverviewsGroupsDeploymentsByAccount(t *testing.T) {
	overviews, err := BuildAccountOverviews(
		manifest.Catalog{
			Accounts: []domain.Account{
				{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}},
				{Name: "staging-core", AWSAccountID: "123456789013", AllowedRegions: []string{"ap-northeast-2"}},
			},
			Environments: []domain.Environment{
				{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"},
				{Name: "staging-apne2", AccountRef: "staging-core", Region: "ap-northeast-2"},
			},
			Deployments: []domain.Deployment{
				{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", TargetType: "ec2", Strategy: "canary", ExecutionMode: "asgWeightedTargetGroups", DesiredArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64"}},
				{Name: "payments-api-staging", AppRef: "payments-api", EnvironmentRef: "staging-apne2", TargetType: "ecs", Strategy: "linear", ExecutionMode: "ecsCodeDeploy", DesiredArtifact: domain.DeploymentArtifact{Image: "ghcr.io/devopsartfactory/payments-api:staging-20260415"}},
			},
		},
		[]statusstore.Snapshot{
			{DeploymentName: "payments-api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "stable"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "3/3 healthy"}, DeployedArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64"}, Rollout: &domain.RolloutSummary{DesiredGeneration: 4, ObservedGeneration: 3, Phase: domain.RolloutPhaseHealthChecking, CurrentStep: "ShiftTrafficCanary", Summary: "waiting for candidate bake window", Steps: []domain.RolloutStepSummary{{Kind: "CreateCandidateCapacity"}, {Kind: "ShiftTrafficCanary", Current: true}, {Kind: "PromoteCandidateCapacity"}}}}},
		},
	)
	if err != nil {
		t.Fatalf("BuildAccountOverviews returned error: %v", err)
	}
	if len(overviews) != 2 {
		t.Fatalf("expected two accounts, got %+v", overviews)
	}
	if overviews[0].Name != "prod-core" || len(overviews[0].Deployments) != 1 {
		t.Fatalf("unexpected prod overview: %+v", overviews[0])
	}
	if overviews[0].Deployments[0].ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("expected rollout execution metadata, got %+v", overviews[0].Deployments[0])
	}
	if overviews[0].Deployments[0].Rollout == nil {
		t.Fatalf("expected rollout summary to be surfaced, got %+v", overviews[0].Deployments[0])
	}
	if overviews[0].Deployments[0].Rollout.Phase != domain.RolloutPhaseHealthChecking || overviews[0].Deployments[0].Rollout.CurrentStep != "ShiftTrafficCanary" || len(overviews[0].Deployments[0].Rollout.Steps) != 3 {
		t.Fatalf("expected rollout summary data, got %+v", overviews[0].Deployments[0].Rollout)
	}
	if got := overviews[0].Deployments[0].Rollout.Steps; got[0].Kind != "CreateCandidateCapacity" || got[1].Kind != "ShiftTrafficCanary" || !got[1].Current || got[2].Kind != "PromoteCandidateCapacity" {
		t.Fatalf("expected strategy-aware EC2 rollout steps, got %+v", got)
	}
	if overviews[1].Deployments[0].Sync.State != "" {
		t.Fatalf("expected missing status to remain zero-valued, got %+v", overviews[1].Deployments[0])
	}
	if overviews[1].Deployments[0].Rollout != nil {
		t.Fatalf("expected missing rollout summary to remain nil, got %+v", overviews[1].Deployments[0].Rollout)
	}
}
