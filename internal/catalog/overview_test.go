package catalog

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

func TestBuildAccountOverviewsGroupsDeploymentsByAccount(t *testing.T) {
	snapshots := []statusstore.Snapshot{
		{DeploymentName: "payments-api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "stable"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "3/3 healthy"}, DeployedArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64"}, Rollout: &domain.RolloutSummary{DesiredGeneration: 4, ObservedGeneration: 3, Phase: domain.RolloutPhaseAccepted, CurrentStep: "PromoteCandidateCapacity", Summary: "Observing current state", Steps: []domain.RolloutStepSummary{{Kind: "CreateCandidateCapacity"}, {Kind: "ShiftTrafficCanary"}, {Kind: "PromoteCandidateCapacity", Current: true}}, Execution: []domain.RolloutExecutionDetail{{Resource: "candidateAutoScalingGroup", Name: "payments-api-prod-candidate", Description: "Candidate Auto Scaling Group Tenet will create for weighted target group rollout"}, {Resource: "activeTargetGroup", Name: "payments-api-prod-active", Description: "Currently active target group receiving production traffic"}, {Resource: "candidateTargetGroup", Name: "payments-api-prod-candidate", Description: "Candidate target group Tenet will register and warm before traffic shifting"}, {Resource: "productionListenerRule", Name: "payments-api-prod-prod-listener", Description: "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout"}, {Resource: "trafficShiftPlan", Name: "payments-api-prod-weighted-routing", Description: "Weighted routing plan Tenet follows to move production traffic from the active target group to the candidate target group"}, {Resource: "userDataBootstrapSource", Name: "file:userdata/payments-api-prod.sh", Description: "Normalized userdata bootstrap source for candidate EC2 capacity"}}}}},
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
	if overviews[0].DeploymentCount != 2 || overviews[0].ProjectCount != 2 || overviews[0].EnvironmentCount != 1 || overviews[0].SyncedCount != 1 || overviews[0].ProgressingCount != 1 || overviews[0].PausedCount != 0 || overviews[0].ErrorCount != 0 {
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
	if got := overviews[0].Deployments[1].Rollout.Execution; len(got) != 6 || got[0].Resource != "candidateAutoScalingGroup" || got[0].Name != "payments-api-prod-candidate" || got[3].Resource != "productionListenerRule" || got[4].Name != "payments-api-prod-weighted-routing" || got[5].Description != "Normalized userdata bootstrap source for candidate EC2 capacity" {
		t.Fatalf("expected rollout execution details to be preserved, got %+v", got)
	}

	snapshots[0].Status.Rollout.Execution[0].Name = "mutated"
	if overviews[0].Deployments[1].Rollout.Execution[0].Name != "payments-api-prod-candidate" {
		t.Fatalf("expected rollout execution details to be cloned defensively, got %+v", overviews[0].Deployments[1].Rollout.Execution)
	}
	if overviews[1].DeploymentCount != 1 || overviews[1].ProjectCount != 1 || overviews[1].EnvironmentCount != 1 || overviews[1].SyncedCount != 0 || overviews[1].ProgressingCount != 0 || overviews[1].PausedCount != 0 || overviews[1].ErrorCount != 0 {
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
	if overviews[0].DeploymentCount != 5 || overviews[0].ProjectCount != 5 || overviews[0].EnvironmentCount != 1 || overviews[0].SyncedCount != 1 || overviews[0].ProgressingCount != 2 || overviews[0].PausedCount != 1 || overviews[0].ErrorCount != 1 {
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
				{Timestamp: "2026-04-16T06:15:00Z", Source: "controller", Summary: "Accepted rollout for r7", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "candidate warming"}, DesiredGeneration: 7, ObservedGeneration: 6, RolloutPhase: domain.RolloutPhaseAccepted, CurrentStep: "CreateCandidateCapacity", TargetType: "ec2", Strategy: "blueGreen", ExecutionMode: "asgInstanceRefresh", Changes: []string{"initial"}},
				{Timestamp: "2026-04-16T06:20:00Z", Source: "controller", Summary: "Shifting canary traffic", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "progressing"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"}, DesiredGeneration: 8, ObservedGeneration: 7, RolloutPhase: domain.RolloutPhasePromoting, CurrentStep: "ShiftTrafficCanary", TargetType: "ec2", Strategy: "canary", ExecutionMode: "asgWeightedTargetGroups", Changes: []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep", "strategy", "executionMode"}, ChangeDetails: []string{"Sync changed to Reconciling: progressing", "Health changed to Progressing: 2/3 targets healthy", "Desired generation changed from 7 to 8", "Observed generation changed from 6 to 7", "Rollout phase changed from Accepted to Promoting", "Current step changed from CreateCandidateCapacity to ShiftTrafficCanary", "Strategy changed from blueGreen to canary", "Execution mode changed from asgInstanceRefresh to asgWeightedTargetGroups"}},
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
	if summary.Timestamp != "2026-04-16T06:20:00Z" || summary.Source != "controller" || summary.Summary != "Shifting canary traffic" || summary.Sync.Reason != "progressing" || summary.Health.Summary != "2/3 targets healthy" || summary.DesiredGeneration != 8 || summary.ObservedGeneration != 7 || summary.RolloutPhase != domain.RolloutPhasePromoting || summary.CurrentStep != "ShiftTrafficCanary" || summary.TargetType != "ec2" || summary.Strategy != "canary" || summary.ExecutionMode != "asgWeightedTargetGroups" || summary.HistoryCount != 2 || !reflect.DeepEqual(summary.Changes, []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep", "strategy", "executionMode"}) {
		t.Fatalf("expected latest history entry to drive runtime summary, got %+v", summary)
	}
	assertStringSliceContainsAll(t, summary.ChangeDetails,
		"Sync changed to Reconciling: progressing",
		"Strategy changed from blueGreen to canary",
		"Execution mode changed from asgInstanceRefresh to asgWeightedTargetGroups",
	)

	snapshots[0].Status.History[1].Summary = "mutated"
	snapshots[0].Status.History[1].ChangeDetails[0] = "mutated"
	if overviews[0].Deployments[0].RuntimeSummary.Summary != "Shifting canary traffic" {
		t.Fatalf("expected runtime summary to be cloned defensively, got %+v", overviews[0].Deployments[0].RuntimeSummary)
	}
	if overviews[0].Deployments[0].RuntimeSummary.ChangeDetails[0] != "Sync changed to Reconciling: progressing" {
		t.Fatalf("expected runtime summary changeDetails to be cloned defensively, got %+v", overviews[0].Deployments[0].RuntimeSummary.ChangeDetails)
	}
	overviews[0].Deployments[0].RuntimeSummary.ChangeDetails[0] = "mutated-by-caller"
	if snapshots[0].Status.History[1].ChangeDetails[0] != "mutated" {
		t.Fatalf("expected caller mutations to avoid mutating stored history changeDetails, got %+v", snapshots[0].Status.History[1].ChangeDetails)
	}
}

func TestBuildAccountOverviewsRuntimeSummaryBackfillsLegacyHistoryContextFromCurrentRollout(t *testing.T) {
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
				DesiredGeneration:  8,
				ObservedGeneration: 7,
				Phase:              domain.RolloutPhasePromoting,
				CurrentStep:        "ShiftTrafficCanary",
				Summary:            "stale rollout",
				TargetType:         "ec2",
				Strategy:           "canary",
				ExecutionMode:      "asgWeightedTargetGroups",
			},
			History: []domain.StatusHistoryEntry{
				{Timestamp: "2026-04-16T06:20:00Z", Source: "controller", Summary: "Shifting canary traffic", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "progressing"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"}, DesiredGeneration: 8, ObservedGeneration: 7, RolloutPhase: domain.RolloutPhasePromoting, CurrentStep: "ShiftTrafficCanary", Changes: []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"}},
			},
		},
	}}

	overviews, err := BuildAccountOverviews(catalog, snapshots)
	if err != nil {
		t.Fatalf("BuildAccountOverviews returned error: %v", err)
	}
	summary := overviews[0].Deployments[0].RuntimeSummary
	if summary == nil {
		t.Fatalf("expected runtime summary to be surfaced, got %+v", overviews[0].Deployments[0])
	}
	if summary.TargetType != "ec2" || summary.Strategy != "canary" || summary.ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("expected legacy latest history entry to backfill rollout context from current rollout, got %+v", summary)
	}
	if snapshots[0].Status.History[0].TargetType != "" || snapshots[0].Status.History[0].Strategy != "" || snapshots[0].Status.History[0].ExecutionMode != "" {
		t.Fatalf("expected backfill to avoid mutating stored legacy history, got %+v", snapshots[0].Status.History[0])
	}
	if !reflect.DeepEqual(summary.Changes, []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"}) {
		t.Fatalf("expected legacy history change labels to stay intact, got %+v", summary.Changes)
	}
	if summary.HistoryCount != 1 || summary.Timestamp != "2026-04-16T06:20:00Z" || summary.Source != "controller" || summary.Summary != "Shifting canary traffic" || summary.Sync.Reason != "progressing" || summary.Health.Summary != "2/3 targets healthy" || summary.DesiredGeneration != 8 || summary.ObservedGeneration != 7 || summary.RolloutPhase != domain.RolloutPhasePromoting || summary.CurrentStep != "ShiftTrafficCanary" {
		t.Fatalf("expected latest history fields to remain authoritative after backfill, got %+v", summary)
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
			ObservedGeneration: 6,
			Sync:               domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
			Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "3/3 healthy"},
			Rollout: &domain.RolloutSummary{
				DesiredGeneration:  7,
				ObservedGeneration: 6,
				Phase:              domain.RolloutPhaseCompleted,
				CurrentStep:        "Complete",
				Summary:            "Serving stable capacity",
				TargetType:         "ecs",
				Strategy:           "linear",
				ExecutionMode:      "ecsCodeDeploy",
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
	if summary.Timestamp != "" || summary.Source != "" || summary.Summary != "Serving stable capacity" || summary.Sync.Reason != "steady" || summary.Health.Summary != "3/3 healthy" || summary.DesiredGeneration != 7 || summary.ObservedGeneration != 6 || summary.RolloutPhase != domain.RolloutPhaseCompleted || summary.CurrentStep != "Complete" || summary.TargetType != "ecs" || summary.Strategy != "linear" || summary.ExecutionMode != "ecsCodeDeploy" || summary.HistoryCount != 0 {
		t.Fatalf("expected current status fallback runtime summary, got %+v", summary)
	}
}

func TestDeriveRuntimeSummaryPrefersLatestHistoryArtifactAndConditions(t *testing.T) {
	summary := deriveRuntimeSummary(domain.DeploymentStatus{
		ObservedGeneration: 7,
		Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "stale-current"},
		Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "stale health"},
		Conditions:         []domain.Condition{{Type: "Ready", Status: false, Reason: "CurrentSnapshot", Message: "should not win over latest history"}},
		DeployedArtifact:   domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6"},
		Rollout: &domain.RolloutSummary{
			DesiredGeneration:  8,
			ObservedGeneration: 7,
			Phase:              domain.RolloutPhasePromoting,
			CurrentStep:        "ShiftTrafficCanary",
			Summary:            "Canary is serving 10% of traffic",
			TargetType:         "ec2",
			Strategy:           "canary",
			ExecutionMode:      "asgWeightedTargetGroups",
		},
		History: []domain.StatusHistoryEntry{
			{Timestamp: "2026-04-16T06:15:00Z", Source: "controller", Summary: "Accepted rollout for r7", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "candidate warming"}, DesiredGeneration: 7, ObservedGeneration: 6, RolloutPhase: domain.RolloutPhaseAccepted, CurrentStep: "CreateCandidateCapacity", TargetType: "ec2", Strategy: "blueGreen", ExecutionMode: "asgInstanceRefresh", DeployedArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6"}, Conditions: []domain.Condition{{Type: "Ready", Status: false, Reason: "CandidateWarming", Message: "candidate target group is still warming"}}, Changes: []string{"initial"}},
			{Timestamp: "2026-04-16T06:20:00Z", Source: "controller", Summary: "Shifting canary traffic", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "progressing"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"}, DesiredGeneration: 8, ObservedGeneration: 7, RolloutPhase: domain.RolloutPhasePromoting, CurrentStep: "ShiftTrafficCanary", TargetType: "ec2", Strategy: "canary", ExecutionMode: "asgWeightedTargetGroups", DeployedArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7"}, Conditions: []domain.Condition{{Type: "Ready", Status: true, Reason: "TrafficShiftComplete", Message: "candidate target group is now serving canary traffic"}}, Changes: []string{"deployedArtifact", "conditions"}},
		},
	})
	if summary == nil {
		t.Fatalf("expected runtime summary")
	}
	payload, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("failed to decode runtime summary JSON: %v", err)
	}
	artifact, ok := decoded["deployedArtifact"].(map[string]any)
	if !ok || artifact["binary"] != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7" {
		t.Fatalf("expected latest history-backed deployedArtifact in runtime summary, got %#v", decoded["deployedArtifact"])
	}
	conditions, ok := decoded["conditions"].([]any)
	if !ok || len(conditions) != 1 {
		t.Fatalf("expected latest history-backed conditions in runtime summary, got %#v", decoded["conditions"])
	}
	condition, ok := conditions[0].(map[string]any)
	if !ok || condition["type"] != "Ready" || condition["status"] != true || condition["reason"] != "TrafficShiftComplete" {
		t.Fatalf("expected latest history-backed condition payload in runtime summary, got %#v", conditions)
	}
}

func TestDeriveRuntimeSummaryBackfillsCurrentArtifactAndConditionsFromLegacyHistoryWithoutRollout(t *testing.T) {
	summary := deriveRuntimeSummary(domain.DeploymentStatus{
		ObservedGeneration: 7,
		Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "stale-current"},
		Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "stale health"},
		Conditions:         []domain.Condition{{Type: "Ready", Status: true, Reason: "Stable", Message: "deployment is healthy"}},
		DeployedArtifact:   domain.DeploymentArtifact{Image: "ghcr.io/devopsartfactory/payments-api:2026-04-16.1"},
		History: []domain.StatusHistoryEntry{{
			Timestamp:          "2026-04-16T06:20:00Z",
			Source:             "controller",
			Summary:            "Observed stable capacity",
			Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "progressing"},
			Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"},
			DesiredGeneration:  8,
			ObservedGeneration: 7,
			RolloutPhase:       domain.RolloutPhasePromoting,
			CurrentStep:        "ShiftTrafficCanary",
			Changes:            []string{"summary", "sync", "health"},
		}},
	})
	if summary == nil {
		t.Fatalf("expected runtime summary")
	}
	payload, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("failed to decode runtime summary JSON: %v", err)
	}
	artifact, ok := decoded["deployedArtifact"].(map[string]any)
	if !ok || artifact["image"] != "ghcr.io/devopsartfactory/payments-api:2026-04-16.1" {
		t.Fatalf("expected current snapshot deployedArtifact backfill in runtime summary without rollout, got %#v", decoded["deployedArtifact"])
	}
	conditions, ok := decoded["conditions"].([]any)
	if !ok || len(conditions) != 1 {
		t.Fatalf("expected current snapshot conditions backfill in runtime summary without rollout, got %#v", decoded["conditions"])
	}
	condition, ok := conditions[0].(map[string]any)
	if !ok || condition["type"] != "Ready" || condition["status"] != true || condition["reason"] != "Stable" {
		t.Fatalf("expected current snapshot condition backfill payload in runtime summary without rollout, got %#v", conditions)
	}
	if summary.TargetType != "" || summary.Strategy != "" || summary.ExecutionMode != "" {
		t.Fatalf("expected rollout-specific backfill to remain absent without rollout, got %+v", summary)
	}
}

func TestDeriveRuntimeSummaryFallsBackToCurrentArtifactAndConditionsWithoutHistory(t *testing.T) {
	summary := deriveRuntimeSummary(domain.DeploymentStatus{
		ObservedGeneration: 6,
		Sync:               domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
		Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "3/3 healthy"},
		Conditions:         []domain.Condition{{Type: "Ready", Status: true, Reason: "Stable", Message: "deployment is healthy"}},
		DeployedArtifact:   domain.DeploymentArtifact{Image: "ghcr.io/devopsartfactory/payments-api:2026-04-16.1"},
		Rollout: &domain.RolloutSummary{
			DesiredGeneration:  7,
			ObservedGeneration: 6,
			Phase:              domain.RolloutPhaseCompleted,
			CurrentStep:        "Complete",
			Summary:            "Serving stable capacity",
			TargetType:         "ecs",
			Strategy:           "linear",
			ExecutionMode:      "ecsCodeDeploy",
		},
	})
	if summary == nil {
		t.Fatalf("expected runtime summary fallback")
	}
	payload, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("failed to decode runtime summary JSON: %v", err)
	}
	artifact, ok := decoded["deployedArtifact"].(map[string]any)
	if !ok || artifact["image"] != "ghcr.io/devopsartfactory/payments-api:2026-04-16.1" {
		t.Fatalf("expected current snapshot deployedArtifact fallback in runtime summary, got %#v", decoded["deployedArtifact"])
	}
	conditions, ok := decoded["conditions"].([]any)
	if !ok || len(conditions) != 1 {
		t.Fatalf("expected current snapshot conditions fallback in runtime summary, got %#v", decoded["conditions"])
	}
	condition, ok := conditions[0].(map[string]any)
	if !ok || condition["type"] != "Ready" || condition["status"] != true || condition["reason"] != "Stable" {
		t.Fatalf("expected current snapshot condition fallback payload in runtime summary, got %#v", conditions)
	}
}

func TestDeriveRuntimeSummaryReturnsArtifactAndConditionsWithoutRolloutOrSyncHealth(t *testing.T) {
	summary := deriveRuntimeSummary(domain.DeploymentStatus{
		DeployedArtifact: domain.DeploymentArtifact{Binary: "s3://artifacts.example.com/payments/runtime-only.zip"},
		Conditions:       []domain.Condition{{Type: "Ready", Status: true, Reason: "Stable", Message: "runtime-only status still carries condition context"}},
	})
	if summary == nil {
		t.Fatalf("expected runtime summary when artifact or conditions are present")
	}
	if summary.DeployedArtifact.Binary != "s3://artifacts.example.com/payments/runtime-only.zip" {
		t.Fatalf("expected deployed artifact to survive runtime-summary fallback, got %+v", summary.DeployedArtifact)
	}
	if len(summary.Conditions) != 1 || summary.Conditions[0].Reason != "Stable" {
		t.Fatalf("expected conditions to survive runtime-summary fallback, got %+v", summary.Conditions)
	}
	if summary.Sync != (domain.Sync{}) || summary.Health != (domain.Health{}) || summary.Summary != "" || summary.HistoryCount != 0 {
		t.Fatalf("expected runtime-only fallback to avoid inventing extra status data, got %+v", summary)
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

func TestBuildApplicationOverviews(t *testing.T) {
	catalog := manifest.Catalog{
		Accounts: []domain.Account{
			{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}},
			{Name: "staging-core", AWSAccountID: "210987654321", AllowedRegions: []string{"ap-northeast-2"}},
		},
		Environments: []domain.Environment{
			{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"},
			{Name: "staging-apne2", AccountRef: "staging-core", Region: "ap-northeast-2"},
		},
		Deployments: []domain.Deployment{
			{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", TargetType: "ec2", Strategy: "canary", ExecutionMode: "asgWeightedTargetGroups", DesiredArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7"}},
			{Name: "billing-api-prod", AppRef: "billing-api", EnvironmentRef: "prod-apne2", TargetType: "ec2", Strategy: "blueGreen", ExecutionMode: "asgWeightedTargetGroups", DesiredArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/billing-api/prod/billing-api-linux-amd64-r9"}},
			{Name: "payments-api-staging", AppRef: "payments-api", EnvironmentRef: "staging-apne2", TargetType: "ecs", Strategy: "linear", ExecutionMode: "ecsCodeDeploy", DesiredArtifact: domain.DeploymentArtifact{Image: "ghcr.io/devopsartfactory/payments-api:staging-20260416"}},
		},
	}
	snapshots := []statusstore.Snapshot{
		{
			DeploymentName: "payments-api-prod",
			Status: domain.DeploymentStatus{
				ObservedGeneration: 7,
				Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"},
				Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"},
				Conditions:         []domain.Condition{{Type: "Progressing", Status: true, Reason: "RolloutInFlight", Message: "candidate receiving traffic"}},
				History: []domain.StatusHistoryEntry{{
					Timestamp:          "2026-04-16T06:20:00Z",
					Source:             "controller",
					Summary:            "Shifting canary traffic",
					Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "progressing"},
					Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"},
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					RolloutPhase:       domain.RolloutPhasePromoting,
					CurrentStep:        "ShiftTrafficCanary",
					TargetType:         "ec2",
					Strategy:           "canary",
					ExecutionMode:      "asgWeightedTargetGroups",
					Changes:            []string{"summary", "sync", "health", "strategy", "executionMode"},
				}},
				DeployedArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6"},
				Rollout: &domain.RolloutSummary{
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					Phase:              domain.RolloutPhasePromoting,
					CurrentStep:        "ShiftTrafficCanary",
					Summary:            "Canary is serving 10% of traffic",
					TargetType:         "ec2",
					Strategy:           "canary",
					ExecutionMode:      "asgWeightedTargetGroups",
					Steps:              []domain.RolloutStepSummary{{Kind: "CreateCandidateCapacity"}, {Kind: "ShiftTrafficCanary", Current: true}},
					Execution:          []domain.RolloutExecutionDetail{{Resource: "candidateAutoScalingGroup", Name: "payments-api-prod-candidate", Description: "Candidate Auto Scaling Group Tenet will create for weighted target group rollout"}},
				},
			},
		},
		{
			DeploymentName: "billing-api-prod",
			Status: domain.DeploymentStatus{
				ObservedGeneration: 9,
				Sync:               domain.Sync{State: domain.SyncStateSynced, Reason: "stable"},
				Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "3/3 healthy"},
				DeployedArtifact:   domain.DeploymentArtifact{Binary: "s3://releases.example.com/billing-api/prod/billing-api-linux-amd64-r9"},
			},
		},
	}

	overviews, err := BuildApplicationOverviews(catalog, snapshots)
	if err != nil {
		t.Fatalf("BuildApplicationOverviews returned error: %v", err)
	}
	if len(overviews) != 3 {
		t.Fatalf("expected one application row per deployment, got %+v", overviews)
	}
	if overviews[0].Name != "billing-api-prod" || overviews[1].Name != "payments-api-prod" || overviews[2].Name != "payments-api-staging" {
		t.Fatalf("expected deterministic application sort order, got %+v", overviews)
	}

	payments := overviews[1]
	if payments.AppRef != "payments-api" || payments.EnvironmentRef != "prod-apne2" || payments.AccountName != "prod-core" || payments.AWSAccountID != "123456789012" || payments.Region != "ap-northeast-2" {
		t.Fatalf("expected joined placement metadata to be preserved, got %+v", payments)
	}
	if payments.TargetType != "ec2" || payments.Strategy != "canary" || payments.ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("expected deployment intent fields to be preserved, got %+v", payments)
	}
	if payments.DesiredArtifact.Binary != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7" || payments.DeployedArtifact.Binary != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6" {
		t.Fatalf("expected desired and deployed artifacts to be preserved, got %+v", payments)
	}
	if payments.ObservedGeneration != 7 || payments.Sync.Reason != "new artifact" || payments.Health.Summary != "2/3 targets healthy" {
		t.Fatalf("expected runtime status fields to be preserved, got %+v", payments)
	}
	if payments.RuntimeSummary == nil || payments.RuntimeSummary.Summary != "Shifting canary traffic" || payments.RuntimeSummary.CurrentStep != "ShiftTrafficCanary" || payments.RuntimeSummary.TargetType != "ec2" || payments.RuntimeSummary.Strategy != "canary" || payments.RuntimeSummary.ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("expected runtime summary to be preserved, got %+v", payments)
	}
	if payments.Rollout == nil || payments.Rollout.Phase != domain.RolloutPhasePromoting || payments.Rollout.Execution[0].Name != "payments-api-prod-candidate" {
		t.Fatalf("expected rollout summary to be preserved, got %+v", payments)
	}
	if len(payments.Conditions) != 1 || payments.Conditions[0].Reason != "RolloutInFlight" {
		t.Fatalf("expected conditions to be preserved, got %+v", payments)
	}

	snapshots[0].Status.History[0].Summary = "mutated"
	snapshots[0].Status.Rollout.Execution[0].Name = "mutated"
	if payments.RuntimeSummary.Summary != "Shifting canary traffic" {
		t.Fatalf("expected runtime summary to be cloned defensively, got %+v", payments.RuntimeSummary)
	}
	if payments.Rollout.Execution[0].Name != "payments-api-prod-candidate" {
		t.Fatalf("expected rollout summary to be cloned defensively, got %+v", payments.Rollout)
	}

	staging := overviews[2]
	if staging.AccountName != "staging-core" || staging.AWSAccountID != "210987654321" || staging.Sync.State != "" || staging.RuntimeSummary != nil || staging.Rollout != nil {
		t.Fatalf("expected missing snapshot fields to stay zero-valued, got %+v", staging)
	}
}

func TestBuildAccountOverviewDetail(t *testing.T) {
	snapshots := []statusstore.Snapshot{
		{
			DeploymentName: "payments-api-prod",
			Status: domain.DeploymentStatus{
				ObservedGeneration: 7,
				Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"},
				Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"},
				Conditions:         []domain.Condition{{Type: "Progressing", Status: true, Reason: "RolloutInFlight", Message: "candidate receiving traffic"}},
				History: []domain.StatusHistoryEntry{{
					Timestamp:          "2026-04-16T06:20:00Z",
					Source:             "controller",
					Summary:            "Shifting canary traffic",
					Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "progressing"},
					Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"},
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					RolloutPhase:       domain.RolloutPhasePromoting,
					CurrentStep:        "ShiftTrafficCanary",
					Changes:            []string{"summary"},
				}},
				DeployedArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6"},
				Rollout: &domain.RolloutSummary{
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					Phase:              domain.RolloutPhasePromoting,
					CurrentStep:        "ShiftTrafficCanary",
					Summary:            "Canary is serving 10% of traffic",
					Steps:              []domain.RolloutStepSummary{{Kind: "CreateCandidateCapacity"}, {Kind: "ShiftTrafficCanary", Current: true}},
					Execution:          []domain.RolloutExecutionDetail{{Resource: "candidateAutoScalingGroup", Name: "payments-api-prod-candidate", Description: "Candidate Auto Scaling Group Tenet will create for weighted target group rollout"}},
				},
			},
		},
		{DeploymentName: "billing-api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 9, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "stable"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "3/3 healthy"}}},
	}
	catalog := manifest.Catalog{
		Accounts: []domain.Account{
			{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2", "us-west-2"}},
			{Name: "prod-shadow", AWSAccountID: "prod-core", AllowedRegions: []string{"eu-west-1"}},
		},
		Environments: []domain.Environment{
			{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"},
			{Name: "prod-apne2-shadow", AccountRef: "prod-shadow", Region: "eu-west-1"},
		},
		Deployments: []domain.Deployment{
			{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", TargetType: "ec2", Strategy: "canary", ExecutionMode: "asgWeightedTargetGroups", DesiredArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7"}},
			{Name: "billing-api-prod", AppRef: "billing-api", EnvironmentRef: "prod-apne2", TargetType: "ec2", Strategy: "blueGreen", ExecutionMode: "asgWeightedTargetGroups", DesiredArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/billing-api/prod/billing-api-linux-amd64-r9"}},
			{Name: "shadow-api-prod", AppRef: "shadow-api", EnvironmentRef: "prod-apne2-shadow", TargetType: "ecs", Strategy: "rolling", ExecutionMode: "ecsCodeDeploy"},
		},
	}

	detail, err := BuildAccountOverviewDetail(catalog, snapshots, "123456789012")
	if err != nil {
		t.Fatalf("BuildAccountOverviewDetail returned error: %v", err)
	}
	accounts, err := BuildAccountOverviews(catalog, snapshots)
	if err != nil {
		t.Fatalf("BuildAccountOverviews returned error: %v", err)
	}
	if detail.Name != "prod-core" || detail.AWSAccountID != "123456789012" {
		t.Fatalf("expected account identity fields, got %+v", detail)
	}
	if !reflect.DeepEqual(detail.AllowedRegions, []string{"ap-northeast-2", "us-west-2"}) {
		t.Fatalf("expected allowed regions, got %+v", detail.AllowedRegions)
	}
	if detail.DeploymentCount != 2 || detail.ProjectCount != 2 || detail.EnvironmentCount != 1 || detail.SyncedCount != 1 || detail.ProgressingCount != 1 || detail.PausedCount != 0 || detail.ErrorCount != 0 {
		t.Fatalf("expected additive deployment counts for account detail, got %+v", detail)
	}
	if len(detail.Deployments) != 2 || detail.Deployments[0].Name != "billing-api-prod" || detail.Deployments[1].Name != "payments-api-prod" {
		t.Fatalf("expected sorted deployments, got %+v", detail.Deployments)
	}
	if detail.Deployments[1].RuntimeSummary == nil || detail.Deployments[1].RuntimeSummary.CurrentStep != "ShiftTrafficCanary" {
		t.Fatalf("expected runtime summary to be surfaced, got %+v", detail.Deployments[1])
	}

	snapshots[0].Status.Rollout.Steps[0].Kind = "mutated"
	snapshots[0].Status.Rollout.Execution[0].Name = "mutated"
	snapshots[0].Status.Conditions[0].Reason = "Mutated"
	snapshots[0].Status.History[0].Summary = "mutated"
	if detail.Deployments[1].Rollout.Steps[0].Kind != "CreateCandidateCapacity" {
		t.Fatalf("expected rollout summary to be cloned defensively, got %+v", detail.Deployments[1].Rollout)
	}
	if detail.Deployments[1].Rollout.Execution[0].Name != "payments-api-prod-candidate" {
		t.Fatalf("expected rollout execution details to be cloned defensively, got %+v", detail.Deployments[1].Rollout.Execution)
	}
	if detail.Deployments[1].Conditions[0].Reason != "RolloutInFlight" {
		t.Fatalf("expected conditions to be cloned defensively, got %+v", detail.Deployments[1].Conditions)
	}
	if detail.Deployments[1].RuntimeSummary.Summary != "Shifting canary traffic" {
		t.Fatalf("expected runtime summary to be cloned defensively, got %+v", detail.Deployments[1].RuntimeSummary)
	}

	detail.Deployments[1].Rollout.Execution[0].Name = "detail-mutated"
	detail.Deployments[1].Conditions[0].Reason = "detail-mutated"
	detail.Deployments[1].RuntimeSummary.Summary = "detail-mutated"
	if accounts[0].Deployments[1].Rollout.Execution[0].Name != "payments-api-prod-candidate" {
		t.Fatalf("expected account detail rollout execution to avoid aliasing account overviews, got %+v", accounts[0].Deployments[1].Rollout.Execution)
	}
	if accounts[0].Deployments[1].Conditions[0].Reason != "RolloutInFlight" {
		t.Fatalf("expected account detail conditions to avoid aliasing account overviews, got %+v", accounts[0].Deployments[1].Conditions)
	}
	if accounts[0].Deployments[1].RuntimeSummary.Summary != "Shifting canary traffic" {
		t.Fatalf("expected account detail runtime summary to avoid aliasing account overviews, got %+v", accounts[0].Deployments[1].RuntimeSummary)
	}
}

func TestBuildAccountOverviewDetailReturnsErrorForDuplicateAWSAccountIDs(t *testing.T) {
	catalog := manifest.Catalog{
		Accounts: []domain.Account{
			{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}},
			{Name: "prod-shadow", AWSAccountID: "123456789012", AllowedRegions: []string{"us-west-2"}},
		},
		Environments: []domain.Environment{
			{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"},
			{Name: "prod-usw2", AccountRef: "prod-shadow", Region: "us-west-2"},
		},
		Deployments: []domain.Deployment{
			{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"},
			{Name: "shadow-api-prod", AppRef: "shadow-api", EnvironmentRef: "prod-usw2"},
		},
	}

	_, err := BuildAccountOverviewDetail(catalog, nil, "123456789012")
	if !errors.Is(err, errAmbiguousAccountOverviewDetail) {
		t.Fatalf("expected ambiguous account detail error, got %v", err)
	}
	if errors.Is(err, statusstore.ErrNotFound) {
		t.Fatalf("expected duplicate account ID to stay distinct from not found, got %v", err)
	}
}

func TestBuildAccountOverviewDetailReturnsNotFound(t *testing.T) {
	catalog := manifest.Catalog{Accounts: []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012"}}}

	_, err := BuildAccountOverviewDetail(catalog, nil, "missing")
	if !errors.Is(err, statusstore.ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestBuildProjectEnvironmentOverviewDetail(t *testing.T) {
	snapshots := []statusstore.Snapshot{
		{DeploymentName: "payments-api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 7, Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"}, DeployedArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6"}}},
		{DeploymentName: "payments-worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 7, Sync: domain.Sync{State: domain.SyncStatePaused, Reason: "manual pause"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "3/3 healthy"}}},
		{DeploymentName: "billing-api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 9, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "stable"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "3/3 healthy"}}},
	}
	catalog := manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2", "us-west-2"}}},
		Environments: []domain.Environment{{Name: "prod", AccountRef: "prod-core", Region: "ap-northeast-2"}, {Name: "staging", AccountRef: "prod-core", Region: "us-west-2"}},
		Deployments: []domain.Deployment{
			{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod", TargetType: "ec2", Strategy: "canary", ExecutionMode: "asgWeightedTargetGroups", DesiredArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7"}},
			{Name: "payments-worker-prod", AppRef: "payments-api", EnvironmentRef: "prod", TargetType: "ec2", Strategy: "linear", ExecutionMode: "asgWeightedTargetGroups"},
			{Name: "billing-api-prod", AppRef: "billing-api", EnvironmentRef: "prod", TargetType: "ec2", Strategy: "blueGreen", ExecutionMode: "asgWeightedTargetGroups"},
		},
	}

	detail, err := BuildProjectEnvironmentOverviewDetail(catalog, snapshots, "123456789012", "payments-api", "prod")
	if err != nil {
		t.Fatalf("BuildProjectEnvironmentOverviewDetail returned error: %v", err)
	}
	if detail.AccountName != "prod-core" || detail.AWSAccountID != "123456789012" || detail.AppRef != "payments-api" || detail.EnvironmentRef != "prod" || detail.Region != "ap-northeast-2" {
		t.Fatalf("expected grouped identity metadata, got %+v", detail)
	}
	if detail.DeploymentCount != 2 || detail.SyncedCount != 0 || detail.ProgressingCount != 1 || detail.PausedCount != 1 || detail.ErrorCount != 0 {
		t.Fatalf("expected grouped status counters, got %+v", detail)
	}
	if len(detail.Deployments) != 2 || detail.Deployments[0].Name != "payments-api-prod" || detail.Deployments[1].Name != "payments-worker-prod" {
		t.Fatalf("expected sorted grouped deployments, got %+v", detail.Deployments)
	}
	if !reflect.DeepEqual(detail.AllowedRegions, []string{"ap-northeast-2", "us-west-2"}) {
		t.Fatalf("expected grouped allowed regions, got %+v", detail.AllowedRegions)
	}

	detail.Deployments[0].Sync.Reason = "mutated"
	refetched, err := BuildProjectEnvironmentOverviewDetail(catalog, snapshots, "123456789012", "payments-api", "prod")
	if err != nil {
		t.Fatalf("BuildProjectEnvironmentOverviewDetail returned error on refetch: %v", err)
	}
	if refetched.Deployments[0].Sync.Reason != "new artifact" {
		t.Fatalf("expected grouped deployment detail to be cloned defensively, got %+v", refetched.Deployments[0])
	}
}

func TestBuildProjectEnvironmentOverviewDetailReturnsNotFound(t *testing.T) {
	catalog := manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012"}},
		Environments: []domain.Environment{{Name: "prod", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments:  []domain.Deployment{{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod"}},
	}

	_, err := BuildProjectEnvironmentOverviewDetail(catalog, nil, "123456789012", "missing", "prod")
	if !errors.Is(err, statusstore.ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
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
						Timestamp:          "2026-04-16T06:15:00Z",
						Source:             "controller",
						Summary:            "Accepted rollout for r7",
						Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"},
						Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "candidate is warming"},
						DesiredGeneration:  7,
						ObservedGeneration: 6,
						RolloutPhase:       domain.RolloutPhaseAccepted,
						CurrentStep:        "CreateCandidateCapacity",
						ChangeDetails:      []string{"Summary changed to Accepted rollout for r7"},
					},
					{
						Timestamp:          "2026-04-16T06:20:00Z",
						Source:             "controller",
						Summary:            "Shifting canary traffic",
						Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"},
						Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"},
						DesiredGeneration:  8,
						ObservedGeneration: 7,
						RolloutPhase:       domain.RolloutPhasePromoting,
						CurrentStep:        "ShiftTrafficCanary",
						ChangeDetails:      []string{"Sync changed to Reconciling: new artifact", "Current step changed from CreateCandidateCapacity to ShiftTrafficCanary"},
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
						{Resource: "candidateAutoScalingGroup", Name: "payments-api-prod-candidate", Description: "Candidate Auto Scaling Group Tenet will create for weighted target group rollout"},
						{Resource: "activeTargetGroup", Name: "payments-api-prod-active", Description: "Currently active target group receiving production traffic"},
						{Resource: "candidateTargetGroup", Name: "payments-api-prod-candidate", Description: "Candidate target group Tenet will register and warm before traffic shifting"},
						{Resource: "productionListenerRule", Name: "payments-api-prod-prod-listener", Description: "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout"},
						{Resource: "trafficShiftPlan", Name: "payments-api-prod-weighted-routing", Description: "Weighted routing plan Tenet follows to move production traffic from the active target group to the candidate target group"},
						{Resource: "userDataBootstrapSource", Name: "file:userdata/payments-api-prod.sh", Description: "Normalized userdata bootstrap source for candidate EC2 capacity"},
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
			{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", TargetType: "ec2", Strategy: "canary", ExecutionMode: "asgWeightedTargetGroups", DesiredArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7"}, UserData: domain.DeploymentUserData{Type: "file", Path: "userdata/payments-api-prod.sh"}, RolloutConfig: domain.DeploymentRolloutConfig{Strategy: domain.DeploymentRolloutStrategy{WarmupSeconds: 30, MinHealthyPercent: 90}}},
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
	if detail.UserData.Type != "file" || detail.UserData.Path != "userdata/payments-api-prod.sh" {
		t.Fatalf("expected userdata metadata, got %+v", detail.UserData)
	}
	if detail.RolloutConfig.Strategy.WarmupSeconds != 30 || detail.RolloutConfig.Strategy.MinHealthyPercent != 90 {
		t.Fatalf("expected rollout config metadata, got %+v", detail.RolloutConfig)
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
	if got := detail.Rollout.Execution; len(got) != 6 || got[0].Resource != "candidateAutoScalingGroup" || got[0].Name != "payments-api-prod-candidate" || got[2].Resource != "candidateTargetGroup" || got[3].Name != "payments-api-prod-prod-listener" || got[4].Description != "Weighted routing plan Tenet follows to move production traffic from the active target group to the candidate target group" || got[5].Resource != "userDataBootstrapSource" {
		t.Fatalf("expected rollout execution details, got %+v", got)
	}
	if len(detail.Conditions) != 1 || detail.Conditions[0].Reason != "RolloutInFlight" {
		t.Fatalf("expected conditions to be surfaced, got %+v", detail.Conditions)
	}
	if len(detail.History) != 2 {
		t.Fatalf("expected runtime history to be surfaced, got %+v", detail.History)
	}
	if got := detail.History; got[0].Timestamp != "2026-04-16T06:15:00Z" || got[0].Source != "controller" || got[0].Summary != "Accepted rollout for r7" || got[0].Sync.Reason != "new artifact" || got[0].Health.Summary != "candidate is warming" || got[0].DesiredGeneration != 7 || got[0].ObservedGeneration != 6 || got[0].RolloutPhase != domain.RolloutPhaseAccepted || got[0].CurrentStep != "CreateCandidateCapacity" || got[1].DesiredGeneration != 8 || got[1].ObservedGeneration != 7 || got[1].RolloutPhase != domain.RolloutPhasePromoting || got[1].CurrentStep != "ShiftTrafficCanary" {
		t.Fatalf("expected runtime history details, got %+v", got)
	}
	assertStringSliceContainsAll(t, detail.History[0].ChangeDetails, "Summary changed to Accepted rollout for r7")
	assertStringSliceContainsAll(t, detail.History[1].ChangeDetails,
		"Sync changed to Reconciling: new artifact",
		"Current step changed from CreateCandidateCapacity to ShiftTrafficCanary",
	)

	snapshots[0].Status.Rollout.Steps[0].Kind = "mutated"
	snapshots[0].Status.Rollout.Execution[0].Name = "mutated"
	snapshots[0].Status.Conditions[0].Reason = "Mutated"
	snapshots[0].Status.History[0].Summary = "mutated"
	snapshots[0].Status.History[1].ChangeDetails[0] = "mutated"
	if detail.Rollout.Steps[0].Kind != "CreateCandidateCapacity" {
		t.Fatalf("expected rollout summary to be cloned defensively, got %+v", detail.Rollout)
	}
	if detail.Rollout.Execution[0].Name != "payments-api-prod-candidate" {
		t.Fatalf("expected rollout execution details to be cloned defensively, got %+v", detail.Rollout.Execution)
	}
	if detail.Conditions[0].Reason != "RolloutInFlight" {
		t.Fatalf("expected conditions to be cloned defensively, got %+v", detail.Conditions)
	}
	if detail.History[0].Summary != "Accepted rollout for r7" {
		t.Fatalf("expected runtime history to be cloned from stored status, got %+v", detail.History)
	}
	if detail.History[1].ChangeDetails[0] != "Sync changed to Reconciling: new artifact" {
		t.Fatalf("expected runtime history changeDetails to be cloned from stored status, got %+v", detail.History[1].ChangeDetails)
	}
	detail.History[1].CurrentStep = "mutated-by-caller"
	detail.History[1].ChangeDetails[0] = "mutated-by-caller"
	if snapshots[0].Status.History[1].CurrentStep != "ShiftTrafficCanary" {
		t.Fatalf("expected caller mutations to avoid mutating stored history, got %+v", snapshots[0].Status.History)
	}
	if snapshots[0].Status.History[1].ChangeDetails[0] != "mutated" {
		t.Fatalf("expected caller mutations to avoid mutating stored history changeDetails, got %+v", snapshots[0].Status.History[1].ChangeDetails)
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
				{Timestamp: "2026-04-16T06:15:00Z", Source: "controller", Summary: "Accepted rollout for r7", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "candidate warming"}, DesiredGeneration: 7, ObservedGeneration: 6, RolloutPhase: domain.RolloutPhaseAccepted, CurrentStep: "CreateCandidateCapacity", TargetType: "ec2", Strategy: "blueGreen", ExecutionMode: "asgInstanceRefresh", Changes: []string{"initial"}},
				{Timestamp: "2026-04-16T06:20:00Z", Source: "controller", Summary: "Shifting canary traffic", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "progressing"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"}, DesiredGeneration: 8, ObservedGeneration: 7, RolloutPhase: domain.RolloutPhasePromoting, CurrentStep: "ShiftTrafficCanary", TargetType: "ec2", Strategy: "canary", ExecutionMode: "asgWeightedTargetGroups", Changes: []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep", "strategy", "executionMode"}, ChangeDetails: []string{"Sync changed to Reconciling: progressing", "Strategy changed from blueGreen to canary", "Execution mode changed from asgInstanceRefresh to asgWeightedTargetGroups"}},
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
	if detail.RuntimeSummary.Timestamp != "2026-04-16T06:20:00Z" || detail.RuntimeSummary.Source != "controller" || detail.RuntimeSummary.Summary != "Shifting canary traffic" || detail.RuntimeSummary.Sync.Reason != "progressing" || detail.RuntimeSummary.Health.Summary != "2/3 targets healthy" || detail.RuntimeSummary.DesiredGeneration != 8 || detail.RuntimeSummary.ObservedGeneration != 7 || detail.RuntimeSummary.RolloutPhase != domain.RolloutPhasePromoting || detail.RuntimeSummary.CurrentStep != "ShiftTrafficCanary" || detail.RuntimeSummary.TargetType != "ec2" || detail.RuntimeSummary.Strategy != "canary" || detail.RuntimeSummary.ExecutionMode != "asgWeightedTargetGroups" || detail.RuntimeSummary.HistoryCount != 2 || !reflect.DeepEqual(detail.RuntimeSummary.Changes, []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep", "strategy", "executionMode"}) {
		t.Fatalf("expected latest history entry runtime summary, got %+v", detail.RuntimeSummary)
	}
	assertStringSliceContainsAll(t, detail.RuntimeSummary.ChangeDetails,
		"Sync changed to Reconciling: progressing",
		"Strategy changed from blueGreen to canary",
		"Execution mode changed from asgInstanceRefresh to asgWeightedTargetGroups",
	)

	snapshots[0].Status.History[1].CurrentStep = "mutated"
	snapshots[0].Status.History[1].ChangeDetails[0] = "mutated"
	if detail.RuntimeSummary.CurrentStep != "ShiftTrafficCanary" {
		t.Fatalf("expected runtime summary to be cloned defensively, got %+v", detail.RuntimeSummary)
	}
	if detail.RuntimeSummary.ChangeDetails[0] != "Sync changed to Reconciling: progressing" {
		t.Fatalf("expected runtime summary changeDetails to be cloned defensively, got %+v", detail.RuntimeSummary.ChangeDetails)
	}
	detail.RuntimeSummary.ChangeDetails[0] = "mutated-by-caller"
	if snapshots[0].Status.History[1].ChangeDetails[0] != "mutated" {
		t.Fatalf("expected caller mutations to avoid mutating stored history changeDetails, got %+v", snapshots[0].Status.History[1].ChangeDetails)
	}
}

func TestBuildDeploymentOverviewDetailRuntimeSummaryBackfillsLegacyHistoryContextFromCurrentRollout(t *testing.T) {
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
				DesiredGeneration:  8,
				ObservedGeneration: 7,
				Phase:              domain.RolloutPhasePromoting,
				CurrentStep:        "ShiftTrafficCanary",
				Summary:            "stale rollout",
				TargetType:         "ec2",
				Strategy:           "canary",
				ExecutionMode:      "asgWeightedTargetGroups",
			},
			History: []domain.StatusHistoryEntry{
				{Timestamp: "2026-04-16T06:20:00Z", Source: "controller", Summary: "Shifting canary traffic", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "progressing"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"}, DesiredGeneration: 8, ObservedGeneration: 7, RolloutPhase: domain.RolloutPhasePromoting, CurrentStep: "ShiftTrafficCanary", Changes: []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"}},
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
	if detail.RuntimeSummary.TargetType != "ec2" || detail.RuntimeSummary.Strategy != "canary" || detail.RuntimeSummary.ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("expected legacy latest history entry to backfill rollout context from current rollout, got %+v", detail.RuntimeSummary)
	}
	if snapshots[0].Status.History[0].TargetType != "" || snapshots[0].Status.History[0].Strategy != "" || snapshots[0].Status.History[0].ExecutionMode != "" {
		t.Fatalf("expected backfill to avoid mutating stored legacy history, got %+v", snapshots[0].Status.History[0])
	}
	if !reflect.DeepEqual(detail.RuntimeSummary.Changes, []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"}) {
		t.Fatalf("expected legacy history change labels to stay intact, got %+v", detail.RuntimeSummary.Changes)
	}
	if detail.RuntimeSummary.HistoryCount != 1 || detail.RuntimeSummary.Timestamp != "2026-04-16T06:20:00Z" || detail.RuntimeSummary.Source != "controller" || detail.RuntimeSummary.Summary != "Shifting canary traffic" || detail.RuntimeSummary.Sync.Reason != "progressing" || detail.RuntimeSummary.Health.Summary != "2/3 targets healthy" || detail.RuntimeSummary.DesiredGeneration != 8 || detail.RuntimeSummary.ObservedGeneration != 7 || detail.RuntimeSummary.RolloutPhase != domain.RolloutPhasePromoting || detail.RuntimeSummary.CurrentStep != "ShiftTrafficCanary" {
		t.Fatalf("expected latest history fields to remain authoritative after backfill, got %+v", detail.RuntimeSummary)
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
			ObservedGeneration: 6,
			Sync:               domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
			Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "3/3 healthy"},
			Rollout: &domain.RolloutSummary{
				DesiredGeneration:  7,
				ObservedGeneration: 6,
				Phase:              domain.RolloutPhaseCompleted,
				CurrentStep:        "Complete",
				Summary:            "Serving stable capacity",
				TargetType:         "ecs",
				Strategy:           "linear",
				ExecutionMode:      "ecsCodeDeploy",
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
	if detail.RuntimeSummary.Timestamp != "" || detail.RuntimeSummary.Source != "" || detail.RuntimeSummary.Summary != "Serving stable capacity" || detail.RuntimeSummary.Sync.Reason != "steady" || detail.RuntimeSummary.Health.Summary != "3/3 healthy" || detail.RuntimeSummary.RolloutPhase != domain.RolloutPhaseCompleted || detail.RuntimeSummary.CurrentStep != "Complete" || detail.RuntimeSummary.HistoryCount != 0 {
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

func assertStringSliceContainsAll(t *testing.T, got []string, want ...string) {
	t.Helper()
	for _, needle := range want {
		found := false
		for _, candidate := range got {
			if candidate == needle {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %q in %v", needle, got)
		}
	}
}

func TestBuildApplicationOverviewDetail(t *testing.T) {
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
						Timestamp:          "2026-04-16T06:15:00Z",
						Source:             "controller",
						Summary:            "Accepted rollout for r7",
						Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"},
						Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "candidate is warming"},
						DesiredGeneration:  7,
						ObservedGeneration: 6,
						RolloutPhase:       domain.RolloutPhaseAccepted,
						CurrentStep:        "CreateCandidateCapacity",
						ChangeDetails:      []string{"Summary changed to Accepted rollout for r7"},
					},
					{
						Timestamp:          "2026-04-16T06:20:00Z",
						Source:             "controller",
						Summary:            "Shifting canary traffic",
						Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "progressing"},
						Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"},
						DesiredGeneration:  8,
						ObservedGeneration: 7,
						RolloutPhase:       domain.RolloutPhasePromoting,
						CurrentStep:        "ShiftTrafficCanary",
						ChangeDetails:      []string{"Sync changed to Reconciling: progressing", "Current step changed from CreateCandidateCapacity to ShiftTrafficCanary"},
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
						{Resource: "candidateAutoScalingGroup", Name: "payments-api-prod-candidate", Description: "Candidate Auto Scaling Group Tenet will create for weighted target group rollout"},
						{Resource: "activeTargetGroup", Name: "payments-api-prod-active", Description: "Currently active target group receiving production traffic"},
					},
				},
			},
		},
	}
	catalog := manifest.Catalog{
		Accounts: []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2", "us-west-2"}}},
		Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments: []domain.Deployment{{
			Name:            "payments-api-prod",
			AppRef:          "payments-api",
			EnvironmentRef:  "prod-apne2",
			TargetType:      "ec2",
			Strategy:        "canary",
			ExecutionMode:   "asgWeightedTargetGroups",
			DesiredArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7"},
			UserData:        domain.DeploymentUserData{Type: "file", Path: "userdata/payments-api-prod.sh"},
			RolloutConfig:   domain.DeploymentRolloutConfig{Strategy: domain.DeploymentRolloutStrategy{WarmupSeconds: 30, MinHealthyPercent: 90}},
		}},
	}

	detail, err := BuildApplicationOverviewDetail(catalog, snapshots, "payments-api-prod")
	if err != nil {
		t.Fatalf("BuildApplicationOverviewDetail returned error: %v", err)
	}
	deploymentDetail, err := BuildDeploymentOverviewDetail(catalog, snapshots, "payments-api-prod")
	if err != nil {
		t.Fatalf("BuildDeploymentOverviewDetail returned error: %v", err)
	}
	if detail.Name != "payments-api-prod" || detail.AppRef != "payments-api" || detail.EnvironmentRef != "prod-apne2" {
		t.Fatalf("expected application identity fields, got %+v", detail)
	}
	if detail.AccountName != "prod-core" || detail.AWSAccountID != "123456789012" || detail.Region != "ap-northeast-2" {
		t.Fatalf("expected placement metadata, got %+v", detail)
	}
	if !reflect.DeepEqual(detail.AllowedRegions, deploymentDetail.AllowedRegions) {
		t.Fatalf("expected application detail allowed regions to match deployment detail, got application=%+v deployment=%+v", detail.AllowedRegions, deploymentDetail.AllowedRegions)
	}
	if detail.TargetType != "ec2" || detail.Strategy != "canary" || detail.ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("expected execution metadata, got %+v", detail)
	}
	if detail.DesiredArtifact.Binary != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7" || detail.DeployedArtifact.Binary != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6" {
		t.Fatalf("expected desired/deployed artifacts, got %+v", detail)
	}
	if detail.UserData.Type != "file" || detail.UserData.Path != "userdata/payments-api-prod.sh" {
		t.Fatalf("expected userdata metadata for application page, got %+v", detail.UserData)
	}
	if detail.RolloutConfig.Strategy.WarmupSeconds != 30 || detail.RolloutConfig.Strategy.MinHealthyPercent != 90 {
		t.Fatalf("expected rollout config metadata for application page, got %+v", detail.RolloutConfig)
	}
	if detail.ObservedGeneration != 7 || detail.Sync.Reason != "new artifact" || detail.Health.Summary != "2/3 targets healthy" {
		t.Fatalf("expected runtime status summary fields, got %+v", detail)
	}
	if detail.RuntimeSummary == nil || detail.RuntimeSummary.CurrentStep != "ShiftTrafficCanary" || detail.RuntimeSummary.HistoryCount != 2 {
		t.Fatalf("expected runtime summary fields for application page, got %+v", detail.RuntimeSummary)
	}
	if detail.Rollout == nil || detail.Rollout.Phase != domain.RolloutPhasePromoting || detail.Rollout.CurrentStep != "ShiftTrafficCanary" || len(detail.Rollout.Steps) != 2 {
		t.Fatalf("expected rollout summary fields for application page, got %+v", detail.Rollout)
	}
	if len(detail.Conditions) != 1 || detail.Conditions[0].Reason != "RolloutInFlight" {
		t.Fatalf("expected conditions for application page, got %+v", detail.Conditions)
	}
	if len(detail.History) != 2 || detail.History[1].CurrentStep != "ShiftTrafficCanary" {
		t.Fatalf("expected history for application page, got %+v", detail.History)
	}
	expected := ApplicationOverviewDetail{
		Name:               deploymentDetail.Name,
		AppRef:             deploymentDetail.AppRef,
		EnvironmentRef:     deploymentDetail.EnvironmentRef,
		AccountName:        deploymentDetail.AccountName,
		AWSAccountID:       deploymentDetail.AWSAccountID,
		AllowedRegions:     append([]string(nil), deploymentDetail.AllowedRegions...),
		Region:             deploymentDetail.Region,
		TargetType:         deploymentDetail.TargetType,
		Strategy:           deploymentDetail.Strategy,
		ExecutionMode:      deploymentDetail.ExecutionMode,
		DesiredArtifact:    deploymentDetail.DesiredArtifact,
		UserData:           deploymentDetail.UserData,
		RolloutConfig:      cloneDeploymentRolloutConfig(deploymentDetail.RolloutConfig),
		DeployedArtifact:   deploymentDetail.DeployedArtifact,
		ObservedGeneration: deploymentDetail.ObservedGeneration,
		Sync:               deploymentDetail.Sync,
		Health:             deploymentDetail.Health,
		RuntimeSummary:     cloneRuntimeSummary(deploymentDetail.RuntimeSummary),
		Rollout:            cloneRolloutSummary(deploymentDetail.Rollout),
		Conditions:         append([]domain.Condition(nil), deploymentDetail.Conditions...),
		History:            cloneStatusHistory(deploymentDetail.History),
	}
	if !reflect.DeepEqual(detail, expected) {
		t.Fatalf("expected application detail to preserve deployment detail semantics, got application=%+v expected=%+v", detail, expected)
	}

	snapshots[0].Status.History[1].CurrentStep = "mutated"
	snapshots[0].Status.History[1].ChangeDetails[0] = "mutated"
	snapshots[0].Status.Rollout.Steps[0].Kind = "mutated"
	snapshots[0].Status.Rollout.Execution[0].Name = "mutated"
	snapshots[0].Status.Conditions[0].Reason = "mutated"
	if detail.RuntimeSummary.CurrentStep != "ShiftTrafficCanary" || detail.RuntimeSummary.ChangeDetails[0] != "Sync changed to Reconciling: progressing" {
		t.Fatalf("expected runtime summary to be cloned defensively, got %+v", detail.RuntimeSummary)
	}
	if detail.Rollout.Steps[0].Kind != "CreateCandidateCapacity" || detail.Rollout.Execution[0].Name != "payments-api-prod-candidate" {
		t.Fatalf("expected rollout summary to be cloned defensively, got %+v", detail.Rollout)
	}
	if detail.Conditions[0].Reason != "RolloutInFlight" {
		t.Fatalf("expected conditions to be cloned defensively, got %+v", detail.Conditions)
	}
	if detail.History[1].CurrentStep != "ShiftTrafficCanary" || detail.History[1].ChangeDetails[0] != "Sync changed to Reconciling: progressing" {
		t.Fatalf("expected history to be cloned defensively, got %+v", detail.History)
	}

	detail.RuntimeSummary.ChangeDetails[0] = "mutated-by-caller"
	detail.Rollout.Execution[0].Name = "mutated-by-caller"
	detail.Conditions[0].Reason = "mutated-by-caller"
	detail.History[1].CurrentStep = "mutated-by-caller"
	refetched, err := BuildApplicationOverviewDetail(catalog, snapshots, "payments-api-prod")
	if err != nil {
		t.Fatalf("BuildApplicationOverviewDetail returned error on refetch: %v", err)
	}
	if refetched.RuntimeSummary.ChangeDetails[0] != "mutated" {
		t.Fatalf("expected application detail runtime summary to avoid caller aliasing, got %+v", refetched.RuntimeSummary.ChangeDetails)
	}
	if refetched.Rollout.Execution[0].Name != "mutated" {
		t.Fatalf("expected application detail rollout to avoid caller aliasing, got %+v", refetched.Rollout.Execution)
	}
	if refetched.Conditions[0].Reason != "mutated" {
		t.Fatalf("expected application detail conditions to avoid caller aliasing, got %+v", refetched.Conditions)
	}
	if refetched.History[1].CurrentStep != "mutated" {
		t.Fatalf("expected application detail history to avoid caller aliasing, got %+v", refetched.History)
	}
}

func TestBuildApplicationOverviewDetailReturnsNotFound(t *testing.T) {
	_, err := BuildApplicationOverviewDetail(manifest.Catalog{}, nil, "missing")
	if !errors.Is(err, statusstore.ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestBuildApplicationOverviewDetailPropagatesPlacementErrors(t *testing.T) {
	catalog := manifest.Catalog{
		Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"us-west-2"}}},
		Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}},
		Deployments: []domain.Deployment{{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
	}

	_, err := BuildApplicationOverviewDetail(catalog, nil, "payments-api-prod")
	if err == nil {
		t.Fatal("expected placement resolution error, got nil")
	}
	if errors.Is(err, statusstore.ErrNotFound) {
		t.Fatalf("expected placement error to be preserved, got %v", err)
	}
	if err.Error() != "region \"ap-northeast-2\" is not allowed for account \"prod-core\"" {
		t.Fatalf("expected disallowed region error, got %v", err)
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
