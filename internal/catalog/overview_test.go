package catalog

import (
	"encoding/json"
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
				{Name: "billing-api-prod", AppRef: "billing-api", EnvironmentRef: "prod-apne2", TargetType: "ec2", Strategy: "blueGreen", ExecutionMode: "asgWeightedTargetGroups", DesiredArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/billing-api/prod/billing-api-linux-amd64"}},
				{Name: "payments-api-staging", AppRef: "payments-api", EnvironmentRef: "staging-apne2", TargetType: "ecs", Strategy: "linear", ExecutionMode: "ecsCodeDeploy", DesiredArtifact: domain.DeploymentArtifact{Image: "ghcr.io/devopsartfactory/payments-api:staging-20260415"}},
			},
		},
		append(snapshots, statusstore.Snapshot{DeploymentName: "billing-api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 9, Sync: domain.Sync{State: domain.SyncStateOutOfSync, Reason: "PendingPromotion"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "warming up"}}}),
	)
	if err != nil {
		t.Fatalf("BuildAccountOverviews returned error: %v", err)
	}
	if len(overviews) != 2 {
		t.Fatalf("expected two accounts, got %+v", overviews)
	}
	if overviews[0].Name != "prod-core" || len(overviews[0].Deployments) != 2 {
		t.Fatalf("unexpected prod overview: %+v", overviews[0])
	}
	if overviews[0].DeploymentCount != 2 || overviews[0].SyncedCount != 1 || overviews[0].ProgressingCount != 1 || overviews[0].PausedCount != 0 || overviews[0].ErrorCount != 0 {
		t.Fatalf("expected additive deployment counts for prod overview, got %+v", overviews[0])
	}
	if overviews[0].Deployments[0].Name != "billing-api-prod" || overviews[0].Deployments[1].Name != "payments-api-prod" {
		t.Fatalf("expected deployment sort order to be preserved, got %+v", overviews[0].Deployments)
	}
	if overviews[0].Deployments[1].ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("expected rollout execution metadata, got %+v", overviews[0].Deployments[0])
	}
	if overviews[0].Deployments[1].Rollout == nil {
		t.Fatalf("expected rollout summary to be surfaced, got %+v", overviews[0].Deployments[1])
	}
	if overviews[0].Deployments[1].Rollout.Phase != domain.RolloutPhaseAccepted || overviews[0].Deployments[1].Rollout.CurrentStep != "PromoteCandidateCapacity" || len(overviews[0].Deployments[1].Rollout.Steps) != 3 {
		t.Fatalf("expected rollout summary data, got %+v", overviews[0].Deployments[1].Rollout)
	}
	if got := overviews[0].Deployments[1].Rollout.Steps; got[0].Kind != "CreateCandidateCapacity" || got[0].Current || got[1].Kind != "ShiftTrafficCanary" || got[1].Current || got[2].Kind != "PromoteCandidateCapacity" || !got[2].Current {
		t.Fatalf("expected strategy-aware EC2 rollout steps, got %+v", got)
	}
	if got := overviews[0].Deployments[1].Rollout.Execution; len(got) != 2 || got[0].Resource != "autoScalingGroup" || got[0].Name != "payments-api-prod-canary" || got[1].Description != "Primary target group currently serving traffic" {
		t.Fatalf("expected rollout execution details to be preserved, got %+v", got)
	}

	snapshots[0].Status.Rollout.Execution[0].Name = "mutated"
	if overviews[0].Deployments[1].Rollout.Execution[0].Name != "payments-api-prod-canary" {
		t.Fatalf("expected rollout execution details to be cloned defensively, got %+v", overviews[0].Deployments[1].Rollout.Execution)
	}
	if overviews[1].DeploymentCount != 1 || overviews[1].SyncedCount != 0 || overviews[1].ProgressingCount != 0 || overviews[1].PausedCount != 0 || overviews[1].ErrorCount != 0 {
		t.Fatalf("expected missing status snapshot to count only toward deploymentCount, got %+v", overviews[1])
	}
	if overviews[1].Deployments[0].Sync.State != "" {
		t.Fatalf("expected missing status to remain zero-valued, got %+v", overviews[1].Deployments[0])
	}
	if overviews[1].Deployments[0].Rollout != nil {
		t.Fatalf("expected missing rollout summary to remain nil, got %+v", overviews[1].Deployments[0].Rollout)
	}
}

func TestBuildAccountOverviewsCountsStatusBuckets(t *testing.T) {
	catalog := manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}}},
		Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments: []domain.Deployment{
			{Name: "payments-api", AppRef: "payments", EnvironmentRef: "prod-apne2"},
			{Name: "billing-api", AppRef: "billing", EnvironmentRef: "prod-apne2"},
			{Name: "orders-api", AppRef: "orders", EnvironmentRef: "prod-apne2"},
			{Name: "search-api", AppRef: "search", EnvironmentRef: "prod-apne2"},
			{Name: "profiles-api", AppRef: "profiles", EnvironmentRef: "prod-apne2"},
		},
	}
	snapshots := []statusstore.Snapshot{
		{DeploymentName: "payments-api", Status: domain.DeploymentStatus{Sync: domain.Sync{State: domain.SyncStateSynced}}},
		{DeploymentName: "billing-api", Status: domain.DeploymentStatus{Sync: domain.Sync{State: domain.SyncStatePaused}}},
		{DeploymentName: "orders-api", Status: domain.DeploymentStatus{Sync: domain.Sync{State: domain.SyncStateError}}},
		{DeploymentName: "search-api", Status: domain.DeploymentStatus{Sync: domain.Sync{State: domain.SyncStateDrifted}}},
		{DeploymentName: "profiles-api", Status: domain.DeploymentStatus{Sync: domain.Sync{State: domain.SyncStateReconciling}}},
	}

	overviews, err := BuildAccountOverviews(catalog, snapshots)
	if err != nil {
		t.Fatalf("BuildAccountOverviews returned error: %v", err)
	}
	if len(overviews) != 1 {
		t.Fatalf("expected one account overview, got %+v", overviews)
	}
	if overviews[0].DeploymentCount != 5 || overviews[0].SyncedCount != 1 || overviews[0].ProgressingCount != 2 || overviews[0].PausedCount != 1 || overviews[0].ErrorCount != 1 {
		t.Fatalf("expected per-status counts to classify synced/progressing/paused/error states, got %+v", overviews[0])
	}
}

func TestBuildAccountOverviewsRuntimeSummaryUsesLatestHistory(t *testing.T) {
	catalog := manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}}},
		Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments:  []domain.Deployment{{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
	}
	snapshots := []statusstore.Snapshot{{
		DeploymentName: "payments-api-prod",
		Status: domain.DeploymentStatus{
			Sync:   domain.Sync{State: domain.SyncStateOutOfSync, Reason: "stale-current"},
			Health: domain.Health{Overall: domain.HealthUnhealthy, Summary: "stale health"},
			Rollout: &domain.RolloutSummary{
				Phase:       domain.RolloutPhaseFailed,
				CurrentStep: "stale-step",
				Summary:     "stale rollout",
			},
			History: []domain.StatusHistoryEntry{
				{Timestamp: "2026-04-16T06:15:00Z", Summary: "Accepted rollout for r7", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "candidate warming"}, RolloutPhase: domain.RolloutPhaseAccepted, CurrentStep: "CreateCandidateCapacity"},
				{Timestamp: "2026-04-16T06:20:00Z", Summary: "Shifting canary traffic", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "progressing"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"}, RolloutPhase: domain.RolloutPhasePromoting, CurrentStep: "ShiftTrafficCanary"},
			},
		},
	}}

	overviews, err := BuildAccountOverviews(catalog, snapshots)
	if err != nil {
		t.Fatalf("BuildAccountOverviews returned error: %v", err)
	}
	if len(overviews) != 1 || len(overviews[0].Deployments) != 1 {
		t.Fatalf("expected one deployment overview, got %+v", overviews)
	}

	summary := overviews[0].Deployments[0].RuntimeSummary
	if summary == nil {
		t.Fatalf("expected runtime summary to be surfaced, got %+v", overviews[0].Deployments[0])
	}
	if summary.Timestamp != "2026-04-16T06:20:00Z" || summary.Summary != "Shifting canary traffic" || summary.Sync.Reason != "progressing" || summary.Health.Summary != "2/3 targets healthy" || summary.RolloutPhase != domain.RolloutPhasePromoting || summary.CurrentStep != "ShiftTrafficCanary" || summary.HistoryCount != 2 {
		t.Fatalf("expected latest history entry to drive runtime summary, got %+v", summary)
	}

	snapshots[0].Status.History[1].Summary = "mutated"
	if overviews[0].Deployments[0].RuntimeSummary.Summary != "Shifting canary traffic" {
		t.Fatalf("expected runtime summary to be cloned defensively, got %+v", overviews[0].Deployments[0].RuntimeSummary)
	}
}

func TestBuildAccountOverviewsRuntimeSummaryFallsBackToCurrentStatus(t *testing.T) {
	catalog := manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}}},
		Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments:  []domain.Deployment{{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
	}
	snapshots := []statusstore.Snapshot{{
		DeploymentName: "payments-api-prod",
		Status: domain.DeploymentStatus{
			Sync:   domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
			Health: domain.Health{Overall: domain.HealthHealthy, Summary: "3/3 healthy"},
			Rollout: &domain.RolloutSummary{
				Phase:       domain.RolloutPhaseCompleted,
				CurrentStep: "Complete",
				Summary:     "Serving stable capacity",
			},
		},
	}}

	overviews, err := BuildAccountOverviews(catalog, snapshots)
	if err != nil {
		t.Fatalf("BuildAccountOverviews returned error: %v", err)
	}
	summary := overviews[0].Deployments[0].RuntimeSummary
	if summary == nil {
		t.Fatalf("expected runtime summary fallback, got %+v", overviews[0].Deployments[0])
	}
	if summary.Timestamp != "" || summary.Summary != "Serving stable capacity" || summary.Sync.Reason != "steady" || summary.Health.Summary != "3/3 healthy" || summary.RolloutPhase != domain.RolloutPhaseCompleted || summary.CurrentStep != "Complete" || summary.HistoryCount != 0 {
		t.Fatalf("expected current status fallback runtime summary, got %+v", summary)
	}
}

func TestBuildAccountOverviewsRuntimeSummaryRemainsNilWhenStatusIsEmpty(t *testing.T) {
	catalog := manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}}},
		Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments:  []domain.Deployment{{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
	}
	snapshots := []statusstore.Snapshot{{
		DeploymentName: "payments-api-prod",
		Status:         domain.DeploymentStatus{},
	}}

	overviews, err := BuildAccountOverviews(catalog, snapshots)
	if err != nil {
		t.Fatalf("BuildAccountOverviews returned error: %v", err)
	}
	if len(overviews) != 1 || len(overviews[0].Deployments) != 1 {
		t.Fatalf("expected one deployment overview, got %+v", overviews)
	}

	deployment := overviews[0].Deployments[0]
	if deployment.RuntimeSummary != nil {
		t.Fatalf("expected empty status runtime summary to remain nil, got %+v", deployment.RuntimeSummary)
	}
	payload, err := json.Marshal(deployment)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if jsonContainsKey(payload, "runtimeSummary") {
		t.Fatalf("expected runtimeSummary to be omitted from JSON, got %s", payload)
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
				History: []domain.StatusHistoryEntry{
					{
						Timestamp:    "2026-04-16T06:15:00Z",
						Source:       "controller",
						Summary:      "Accepted rollout for r7",
						Sync:         domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"},
						Health:       domain.Health{Overall: domain.HealthProgressing, Summary: "candidate is warming"},
						RolloutPhase: domain.RolloutPhaseAccepted,
						CurrentStep:  "CreateCandidateCapacity",
					},
					{
						Timestamp:    "2026-04-16T06:20:00Z",
						Source:       "controller",
						Summary:      "Shifting canary traffic",
						Sync:         domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"},
						Health:       domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"},
						RolloutPhase: domain.RolloutPhasePromoting,
						CurrentStep:  "ShiftTrafficCanary",
					},
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
	if len(detail.History) != 2 {
		t.Fatalf("expected runtime history to be surfaced, got %+v", detail.History)
	}
	if got := detail.History; got[0].Timestamp != "2026-04-16T06:15:00Z" || got[0].Source != "controller" || got[0].Summary != "Accepted rollout for r7" || got[0].Sync.Reason != "new artifact" || got[0].Health.Summary != "candidate is warming" || got[0].RolloutPhase != domain.RolloutPhaseAccepted || got[0].CurrentStep != "CreateCandidateCapacity" || got[1].RolloutPhase != domain.RolloutPhasePromoting || got[1].CurrentStep != "ShiftTrafficCanary" {
		t.Fatalf("expected runtime history details, got %+v", got)
	}

	snapshots[0].Status.Rollout.Steps[0].Kind = "mutated"
	snapshots[0].Status.Rollout.Execution[0].Name = "mutated"
	snapshots[0].Status.Conditions[0].Reason = "Mutated"
	snapshots[0].Status.History[0].Summary = "mutated"
	detail.History[1].CurrentStep = "mutated-by-caller"
	if detail.Rollout.Steps[0].Kind != "CreateCandidateCapacity" {
		t.Fatalf("expected rollout summary to be cloned defensively, got %+v", detail.Rollout)
	}
	if detail.Rollout.Execution[0].Name != "payments-api-prod-canary" {
		t.Fatalf("expected rollout execution details to be cloned defensively, got %+v", detail.Rollout.Execution)
	}
	if detail.Conditions[0].Reason != "RolloutInFlight" {
		t.Fatalf("expected conditions to be cloned defensively, got %+v", detail.Conditions)
	}
	if detail.History[0].Summary != "Accepted rollout for r7" {
		t.Fatalf("expected runtime history to be cloned from stored status, got %+v", detail.History)
	}
	if snapshots[0].Status.History[1].CurrentStep != "ShiftTrafficCanary" {
		t.Fatalf("expected caller mutations to avoid mutating stored history, got %+v", snapshots[0].Status.History)
	}
}

func TestBuildDeploymentOverviewDetailRuntimeSummaryUsesLatestHistory(t *testing.T) {
	catalog := manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}}},
		Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments:  []domain.Deployment{{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
	}
	snapshots := []statusstore.Snapshot{{
		DeploymentName: "payments-api-prod",
		Status: domain.DeploymentStatus{
			Sync:   domain.Sync{State: domain.SyncStateOutOfSync, Reason: "stale-current"},
			Health: domain.Health{Overall: domain.HealthUnhealthy, Summary: "stale health"},
			Rollout: &domain.RolloutSummary{
				Phase:       domain.RolloutPhaseFailed,
				CurrentStep: "stale-step",
				Summary:     "stale rollout",
			},
			History: []domain.StatusHistoryEntry{
				{Timestamp: "2026-04-16T06:15:00Z", Summary: "Accepted rollout for r7", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "candidate warming"}, RolloutPhase: domain.RolloutPhaseAccepted, CurrentStep: "CreateCandidateCapacity"},
				{Timestamp: "2026-04-16T06:20:00Z", Summary: "Shifting canary traffic", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "progressing"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"}, RolloutPhase: domain.RolloutPhasePromoting, CurrentStep: "ShiftTrafficCanary"},
			},
		},
	}}

	detail, err := BuildDeploymentOverviewDetail(catalog, snapshots, "payments-api-prod")
	if err != nil {
		t.Fatalf("BuildDeploymentOverviewDetail returned error: %v", err)
	}
	if detail.RuntimeSummary == nil {
		t.Fatalf("expected runtime summary to be surfaced, got %+v", detail)
	}
	if detail.RuntimeSummary.Timestamp != "2026-04-16T06:20:00Z" || detail.RuntimeSummary.Summary != "Shifting canary traffic" || detail.RuntimeSummary.Sync.Reason != "progressing" || detail.RuntimeSummary.Health.Summary != "2/3 targets healthy" || detail.RuntimeSummary.RolloutPhase != domain.RolloutPhasePromoting || detail.RuntimeSummary.CurrentStep != "ShiftTrafficCanary" || detail.RuntimeSummary.HistoryCount != 2 {
		t.Fatalf("expected latest history entry runtime summary, got %+v", detail.RuntimeSummary)
	}

	snapshots[0].Status.History[1].CurrentStep = "mutated"
	if detail.RuntimeSummary.CurrentStep != "ShiftTrafficCanary" {
		t.Fatalf("expected runtime summary to be cloned defensively, got %+v", detail.RuntimeSummary)
	}
}

func TestBuildDeploymentOverviewDetailRuntimeSummaryFallsBackToCurrentStatus(t *testing.T) {
	catalog := manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}}},
		Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments:  []domain.Deployment{{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
	}
	snapshots := []statusstore.Snapshot{{
		DeploymentName: "payments-api-prod",
		Status: domain.DeploymentStatus{
			Sync:   domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
			Health: domain.Health{Overall: domain.HealthHealthy, Summary: "3/3 healthy"},
			Rollout: &domain.RolloutSummary{
				Phase:       domain.RolloutPhaseCompleted,
				CurrentStep: "Complete",
				Summary:     "Serving stable capacity",
			},
		},
	}}

	detail, err := BuildDeploymentOverviewDetail(catalog, snapshots, "payments-api-prod")
	if err != nil {
		t.Fatalf("BuildDeploymentOverviewDetail returned error: %v", err)
	}
	if detail.RuntimeSummary == nil {
		t.Fatalf("expected runtime summary fallback, got %+v", detail)
	}
	if detail.RuntimeSummary.Timestamp != "" || detail.RuntimeSummary.Summary != "Serving stable capacity" || detail.RuntimeSummary.Sync.Reason != "steady" || detail.RuntimeSummary.Health.Summary != "3/3 healthy" || detail.RuntimeSummary.RolloutPhase != domain.RolloutPhaseCompleted || detail.RuntimeSummary.CurrentStep != "Complete" || detail.RuntimeSummary.HistoryCount != 0 {
		t.Fatalf("expected current status fallback runtime summary, got %+v", detail.RuntimeSummary)
	}
}

func TestBuildDeploymentOverviewDetailRuntimeSummaryRemainsNilWhenStatusIsEmpty(t *testing.T) {
	catalog := manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}}},
		Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments:  []domain.Deployment{{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
	}
	snapshots := []statusstore.Snapshot{{
		DeploymentName: "payments-api-prod",
		Status:         domain.DeploymentStatus{},
	}}

	detail, err := BuildDeploymentOverviewDetail(catalog, snapshots, "payments-api-prod")
	if err != nil {
		t.Fatalf("BuildDeploymentOverviewDetail returned error: %v", err)
	}
	if detail.RuntimeSummary != nil {
		t.Fatalf("expected empty status runtime summary to remain nil, got %+v", detail.RuntimeSummary)
	}
	payload, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if jsonContainsKey(payload, "runtimeSummary") {
		t.Fatalf("expected runtimeSummary to be omitted from JSON, got %s", payload)
	}
}

func jsonContainsKey(payload []byte, key string) bool {
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return false
	}
	_, ok := decoded[key]
	return ok
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
