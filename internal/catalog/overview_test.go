package catalog

import (
	"errors"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

func TestBuildAccountOverviewsGroupsDeploymentsByAccount(t *testing.T) {
	snapshots := []statusstore.Snapshot{
		{DeploymentName: "payments-api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "stable"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "3/3 healthy"}, DeployedArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64"}, Rollout: &domain.RolloutSummary{DesiredGeneration: 4, ObservedGeneration: 3, Phase: domain.RolloutPhaseAccepted, CurrentStep: "PromoteCandidateCapacity", Summary: "Observing current state", Steps: []domain.RolloutStepSummary{{Kind: "CreateCandidateCapacity"}, {Kind: "ShiftTrafficCanary"}, {Kind: "PromoteCandidateCapacity", Current: true}}, Execution: []domain.RolloutExecutionDetail{{Resource: "autoScalingGroup", Name: "payments-api-prod-canary", Description: "Candidate EC2 capacity for weighted target groups"}, {Resource: "targetGroup", Name: "payments-api-prod-active", Description: "Primary target group currently serving traffic"}}}}},
	}

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
		snapshots,
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
	if overviews[0].Deployments[0].Rollout.Phase != domain.RolloutPhaseAccepted || overviews[0].Deployments[0].Rollout.CurrentStep != "PromoteCandidateCapacity" || len(overviews[0].Deployments[0].Rollout.Steps) != 3 {
		t.Fatalf("expected rollout summary data, got %+v", overviews[0].Deployments[0].Rollout)
	}
	if got := overviews[0].Deployments[0].Rollout.Steps; got[0].Kind != "CreateCandidateCapacity" || got[0].Current || got[1].Kind != "ShiftTrafficCanary" || got[1].Current || got[2].Kind != "PromoteCandidateCapacity" || !got[2].Current {
		t.Fatalf("expected strategy-aware EC2 rollout steps, got %+v", got)
	}
	if got := overviews[0].Deployments[0].Rollout.Execution; len(got) != 2 || got[0].Resource != "autoScalingGroup" || got[0].Name != "payments-api-prod-canary" || got[1].Description != "Primary target group currently serving traffic" {
		t.Fatalf("expected rollout execution details to be preserved, got %+v", got)
	}

	snapshots[0].Status.Rollout.Execution[0].Name = "mutated"
	if overviews[0].Deployments[0].Rollout.Execution[0].Name != "payments-api-prod-canary" {
		t.Fatalf("expected rollout execution details to be cloned defensively, got %+v", overviews[0].Deployments[0].Rollout.Execution)
	}
	if overviews[1].Deployments[0].Sync.State != "" {
		t.Fatalf("expected missing status to remain zero-valued, got %+v", overviews[1].Deployments[0])
	}
	if overviews[1].Deployments[0].Rollout != nil {
		t.Fatalf("expected missing rollout summary to remain nil, got %+v", overviews[1].Deployments[0].Rollout)
	}
}

func TestBuildDeploymentOverviewDetail(t *testing.T) {
	snapshots := []statusstore.Snapshot{
		{
			DeploymentName: "payments-api-prod",
			Status: domain.DeploymentStatus{
				ObservedGeneration: 7,
				Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"},
				Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"},
				Conditions: []domain.Condition{
					{Type: "Progressing", Status: true, Reason: "RolloutInFlight", Message: "candidate receiving traffic"},
				},
				DeployedArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6"},
				Rollout: &domain.RolloutSummary{
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					Phase:              domain.RolloutPhasePromoting,
					CurrentStep:        "ShiftTrafficCanary",
					Summary:            "Canary is serving 10% of traffic",
					Steps: []domain.RolloutStepSummary{
						{Kind: "CreateCandidateCapacity"},
						{Kind: "ShiftTrafficCanary", Current: true},
					},
					Execution: []domain.RolloutExecutionDetail{
						{Resource: "autoScalingGroup", Name: "payments-api-prod-canary", Description: "Candidate EC2 capacity for weighted target groups"},
						{Resource: "targetGroup", Name: "payments-api-prod-green", Description: "Candidate target group receiving canary traffic"},
					},
				},
			},
		},
	}
	catalog := manifest.Catalog{
		Accounts: []domain.Account{
			{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2", "us-west-2"}},
		},
		Environments: []domain.Environment{
			{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"},
		},
		Deployments: []domain.Deployment{
			{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", TargetType: "ec2", Strategy: "canary", ExecutionMode: "asgWeightedTargetGroups", DesiredArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7"}},
		},
	}

	detail, err := BuildDeploymentOverviewDetail(catalog, snapshots, "payments-api-prod")
	if err != nil {
		t.Fatalf("BuildDeploymentOverviewDetail returned error: %v", err)
	}
	if detail.Name != "payments-api-prod" || detail.AppRef != "payments-api" || detail.EnvironmentRef != "prod-apne2" {
		t.Fatalf("expected deployment identity fields, got %+v", detail)
	}
	if detail.AccountName != "prod-core" || detail.AWSAccountID != "123456789012" || detail.Region != "ap-northeast-2" {
		t.Fatalf("expected placement metadata, got %+v", detail)
	}
	if detail.TargetType != "ec2" || detail.Strategy != "canary" || detail.ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("expected execution metadata, got %+v", detail)
	}
	if detail.DesiredArtifact.Binary != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7" || detail.DeployedArtifact.Binary != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6" {
		t.Fatalf("expected desired/deployed artifacts, got %+v", detail)
	}
	if detail.ObservedGeneration != 7 || detail.Sync.Reason != "new artifact" || detail.Health.Summary != "2/3 targets healthy" {
		t.Fatalf("expected status summary fields, got %+v", detail)
	}
	if detail.Rollout == nil || detail.Rollout.Phase != domain.RolloutPhasePromoting || detail.Rollout.CurrentStep != "ShiftTrafficCanary" || len(detail.Rollout.Steps) != 2 {
		t.Fatalf("expected rollout summary, got %+v", detail.Rollout)
	}
	if got := detail.Rollout.Execution; len(got) != 2 || got[0].Resource != "autoScalingGroup" || got[0].Name != "payments-api-prod-canary" || got[1].Description != "Candidate target group receiving canary traffic" {
		t.Fatalf("expected rollout execution details, got %+v", got)
	}
	if len(detail.Conditions) != 1 || detail.Conditions[0].Reason != "RolloutInFlight" {
		t.Fatalf("expected conditions to be surfaced, got %+v", detail.Conditions)
	}

	snapshots[0].Status.Rollout.Steps[0].Kind = "mutated"
	snapshots[0].Status.Rollout.Execution[0].Name = "mutated"
	snapshots[0].Status.Conditions[0].Reason = "Mutated"
	if detail.Rollout.Steps[0].Kind != "CreateCandidateCapacity" {
		t.Fatalf("expected rollout summary to be cloned defensively, got %+v", detail.Rollout)
	}
	if detail.Rollout.Execution[0].Name != "payments-api-prod-canary" {
		t.Fatalf("expected rollout execution details to be cloned defensively, got %+v", detail.Rollout.Execution)
	}
	if detail.Conditions[0].Reason != "RolloutInFlight" {
		t.Fatalf("expected conditions to be cloned defensively, got %+v", detail.Conditions)
	}
}

func TestBuildDeploymentOverviewDetailReturnsNotFound(t *testing.T) {
	_, err := BuildDeploymentOverviewDetail(manifest.Catalog{}, nil, "missing")
	if !errors.Is(err, statusstore.ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestBuildDeploymentOverviewDetailPropagatesPlacementErrors(t *testing.T) {
	t.Run("missing environment", func(t *testing.T) {
		catalog := manifest.Catalog{
			Accounts:    []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}}},
			Deployments: []domain.Deployment{{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
		}

		_, err := BuildDeploymentOverviewDetail(catalog, nil, "payments-api-prod")
		if err == nil {
			t.Fatal("expected placement resolution error, got nil")
		}
		if errors.Is(err, statusstore.ErrNotFound) {
			t.Fatalf("expected placement error to be preserved, got %v", err)
		}
		if err.Error() != "environment \"prod-apne2\" not found" {
			t.Fatalf("expected missing environment error, got %v", err)
		}
	})

	t.Run("disallowed region", func(t *testing.T) {
		catalog := manifest.Catalog{
			Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"us-west-2"}}},
			Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
			Deployments:  []domain.Deployment{{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
		}

		_, err := BuildDeploymentOverviewDetail(catalog, nil, "payments-api-prod")
		if err == nil {
			t.Fatal("expected placement resolution error, got nil")
		}
		if errors.Is(err, statusstore.ErrNotFound) {
			t.Fatalf("expected placement error to be preserved, got %v", err)
		}
		if err.Error() != "region \"ap-northeast-2\" is not allowed for account \"prod-core\"" {
			t.Fatalf("expected disallowed region error, got %v", err)
		}
	})
}
