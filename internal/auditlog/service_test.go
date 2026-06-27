package auditlog

import (
	"context"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/controllerrefreshaudit"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

func TestServiceListProjectsImmutableRecordsIntoNewestFirstTimeline(t *testing.T) {
	service := Service{
		ListPlanPreviews: func() ([]planpreview.Record, error) {
			return []planpreview.Record{
				{
					ID:             "plan_old",
					DeploymentName: "api-prod",
					Status:         planpreview.StatusReady,
					CreatedAt:      "2026-04-17T08:00:00Z",
					Risk:           planpreview.Risk{Level: "medium"},
					Approval:       planpreview.Approval{Required: true},
					Source:         planpreview.SourceContext{Route: "/v1/overview/deployments/api-prod", Fingerprint: "plan_fp_old"},
				},
				{
					ID:             "plan_new",
					DeploymentName: "worker-prod",
					Status:         planpreview.StatusFailed,
					CreatedAt:      "2026-04-18T07:59:00Z",
					Risk:           planpreview.Risk{Level: "high"},
					Approval:       planpreview.Approval{Required: true},
					Errors:         []string{"stale source fingerprint"},
					Source:         planpreview.SourceContext{Route: "/v1/overview/deployments/worker-prod", Fingerprint: "plan_fp_new"},
				},
			}, nil
		},
		ListApplyRequests: func() ([]applyrequest.Record, error) {
			return []applyrequest.Record{
				{
					ID:             "apply_newest",
					PlanID:         "plan_old",
					DeploymentName: "api-prod",
					Status:         applyrequest.StatusFailed,
					CreatedAt:      "2026-04-18T08:00:00Z",
					Error:          "apply execution is disabled",
					Details:        []string{"did not contact AWS"},
					Actor: requestactor.Actor{
						Available: true,
						ID:        "operator-123",
						Name:      "Jane Operator",
						Email:     "jane@example.com",
					},
					Source: applyrequest.SourceContext{
						Route:       "/v1/plans/plan_old",
						PlanStatus:  "ready",
						Fingerprint: "plan_fp_old",
					},
				},
			}, nil
		},
		ListDeployments: func(_ context.Context) ([]catalog.DeploymentView, error) {
			return []catalog.DeploymentView{
				{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"},
				{Name: "worker-prod", AppRef: "worker", EnvironmentRef: "prod-apne2"},
			}, nil
		},
	}

	result, err := service.List(context.Background(), Query{Limit: 2})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected two limited entries, got %+v", result.Entries)
	}
	if result.ResultScope.ReturnedEntryCount != 2 || result.ResultScope.ReturnedCount != 2 || result.ResultScope.TotalEntryCount != 3 || !result.ResultScope.Limited {
		t.Fatalf("expected capped result scope for limited audit window, got %+v", result.ResultScope)
	}
	if result.ResultScope.Limit == nil || *result.ResultScope.Limit != 2 || result.ResultScope.RequestedLimit == nil || *result.ResultScope.RequestedLimit != 2 || result.ResultScope.EffectiveLimit != 2 || result.ResultScope.TruncationStatus != "truncated" || !result.ResultScope.Truncated {
		t.Fatalf("expected applied audit limit in result scope, got %+v", result.ResultScope)
	}

	newest := result.Entries[0]
	if newest.Action != "apply.requested" || newest.ApplyID != "apply_newest" {
		t.Fatalf("expected newest apply entry first, got %+v", newest)
	}
	if newest.AppRef != "payments-api" || newest.EnvironmentRef != "prod-apne2" {
		t.Fatalf("expected deployment context enrichment, got %+v", newest)
	}
	if newest.Metadata.PlanStatus != "ready" || newest.Metadata.Error != "apply execution is disabled" {
		t.Fatalf("unexpected apply metadata: %+v", newest.Metadata)
	}
	if newest.Source.RecordRoute != "/v1/applies/apply_newest" || newest.Source.SourceRoute != "/v1/plans/plan_old" {
		t.Fatalf("unexpected apply source details: %+v", newest.Source)
	}
	if !newest.Actor.Available || newest.Actor.Name != "Jane Operator" || newest.Actor.Email != "jane@example.com" {
		t.Fatalf("expected projected available actor contract, got %+v", newest.Actor)
	}
	if !strings.HasPrefix(newest.EntryID, "audit_apply_") {
		t.Fatalf("expected backend-authored apply audit entryId, got %q", newest.EntryID)
	}

	second := result.Entries[1]
	if second.Action != "plan.created" || second.PlanID != "plan_new" {
		t.Fatalf("expected newer plan entry second, got %+v", second)
	}
	if second.Metadata.RiskLevel != "high" || len(second.Metadata.Errors) != 1 {
		t.Fatalf("unexpected plan metadata: %+v", second.Metadata)
	}
	if second.Source.RecordRoute != "/v1/plans/plan_new" || second.Source.SourceRoute != "/v1/overview/deployments/worker-prod" {
		t.Fatalf("unexpected plan source details: %+v", second.Source)
	}
	if second.Actor.Available || second.Actor.Reason != requestactor.UnavailableReason {
		t.Fatalf("expected explicit unavailable fallback actor, got %+v", second.Actor)
	}
	if !strings.HasPrefix(second.EntryID, "audit_plan_") || second.EntryID == newest.EntryID {
		t.Fatalf("expected unique backend-authored plan audit entryId, newest=%q second=%q", newest.EntryID, second.EntryID)
	}
}

func TestServiceListAssignsStableEntryIDsAcrossFiltersAndLimits(t *testing.T) {
	service := Service{
		ListPlanPreviews: func() ([]planpreview.Record, error) {
			return []planpreview.Record{{
				ID:             "shared_source",
				DeploymentName: "api-prod",
				Status:         planpreview.StatusReady,
				CreatedAt:      "2026-04-17T08:00:00Z",
				Approval:       planpreview.Approval{Required: true},
				Source:         planpreview.SourceContext{Route: "/v1/overview/deployments/api-prod", Fingerprint: "fp_plan"},
			}}, nil
		},
		ListApplyRequests: func() ([]applyrequest.Record, error) {
			return []applyrequest.Record{{
				ID:             "shared_source",
				PlanID:         "shared_source",
				DeploymentName: "api-prod",
				Status:         applyrequest.StatusFailed,
				CreatedAt:      "2026-04-18T08:30:00Z",
				Source: applyrequest.SourceContext{
					Route:       "/v1/plans/shared_source",
					PlanStatus:  "ready",
					Fingerprint: "fp_plan",
				},
			}}, nil
		},
	}

	all, err := service.List(context.Background(), Query{})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	filtered, err := service.List(context.Background(), Query{DeploymentName: "api-prod", Action: ActionApplyRequested, Limit: 1})
	if err != nil {
		t.Fatalf("filtered List returned error: %v", err)
	}
	if len(all.Entries) != 2 || len(filtered.Entries) != 1 {
		t.Fatalf("expected two full entries and one filtered entry, all=%+v filtered=%+v", all.Entries, filtered.Entries)
	}
	if all.ResultScope.ReturnedEntryCount != 2 || all.ResultScope.ReturnedCount != 2 || all.ResultScope.TotalEntryCount != 2 || all.ResultScope.Limited || all.ResultScope.EffectiveLimit != 0 || all.ResultScope.RequestedLimit != nil || all.ResultScope.TruncationStatus != "not_truncated" || all.ResultScope.Truncated {
		t.Fatalf("expected uncapped all-entry result scope, got %+v", all.ResultScope)
	}
	if filtered.ResultScope.ReturnedEntryCount != 1 || filtered.ResultScope.ReturnedCount != 1 || filtered.ResultScope.TotalEntryCount != 1 || filtered.ResultScope.Limited || filtered.ResultScope.EffectiveLimit != 1 || filtered.ResultScope.RequestedLimit == nil || *filtered.ResultScope.RequestedLimit != 1 || filtered.ResultScope.TruncationStatus != "not_truncated" || filtered.ResultScope.Truncated {
		t.Fatalf("expected filtered result scope to describe matched rows before cap, got %+v", filtered.ResultScope)
	}
	if filtered.ResultScope.DeploymentName != "api-prod" || filtered.ResultScope.Action != string(ActionApplyRequested) || filtered.ResultScope.Limit == nil || *filtered.ResultScope.Limit != 1 {
		t.Fatalf("expected applied filters in filtered result scope, got %+v", filtered.ResultScope)
	}
	if all.Entries[0].EntryID != filtered.Entries[0].EntryID {
		t.Fatalf("expected filtered apply entryId to stay stable, all=%q filtered=%q", all.Entries[0].EntryID, filtered.Entries[0].EntryID)
	}
	if all.Entries[0].EntryID == all.Entries[1].EntryID {
		t.Fatalf("expected action-specific entryIds to disambiguate shared immutable IDs, got %+v", all.Entries)
	}
	for _, entry := range all.Entries {
		if !strings.HasPrefix(entry.EntryID, "audit_") {
			t.Fatalf("expected audit entryId prefix, got %+v", entry)
		}
	}
}

func TestServiceListRejectsNegativeLimit(t *testing.T) {
	service := Service{
		ListPlanPreviews:  func() ([]planpreview.Record, error) { return nil, nil },
		ListApplyRequests: func() ([]applyrequest.Record, error) { return nil, nil },
	}

	_, err := service.List(context.Background(), Query{Limit: -1})
	if err == nil || !strings.Contains(err.Error(), "greater than zero") {
		t.Fatalf("expected invalid limit error, got %v", err)
	}
}

func TestServiceListFiltersByDeploymentBeforeSortingAndLimit(t *testing.T) {
	service := Service{
		ListPlanPreviews: func() ([]planpreview.Record, error) {
			return []planpreview.Record{
				{
					ID:             "plan_api_old",
					DeploymentName: "api-prod",
					Status:         planpreview.StatusReady,
					CreatedAt:      "2026-04-17T08:00:00Z",
					Approval:       planpreview.Approval{Required: true},
					Source:         planpreview.SourceContext{Route: "/v1/overview/deployments/api-prod", Fingerprint: "fp_api_old"},
				},
				{
					ID:             "plan_worker_new",
					DeploymentName: "worker-prod",
					Status:         planpreview.StatusReady,
					CreatedAt:      "2026-04-18T09:00:00Z",
					Approval:       planpreview.Approval{Required: true},
					Source:         planpreview.SourceContext{Route: "/v1/overview/deployments/worker-prod", Fingerprint: "fp_worker_new"},
				},
			}, nil
		},
		ListApplyRequests: func() ([]applyrequest.Record, error) {
			return []applyrequest.Record{
				{
					ID:             "apply_api_newest",
					PlanID:         "plan_api_old",
					DeploymentName: "api-prod",
					Status:         applyrequest.StatusFailed,
					CreatedAt:      "2026-04-18T08:30:00Z",
					Source: applyrequest.SourceContext{
						Route:       "/v1/plans/plan_api_old",
						PlanStatus:  "ready",
						Fingerprint: "fp_api_old",
					},
				},
			}, nil
		},
	}

	result, err := service.List(context.Background(), Query{DeploymentName: "api-prod", Limit: 1})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected one filtered entry, got %+v", result.Entries)
	}
	if entry := result.Entries[0]; entry.DeploymentName != "api-prod" || entry.ApplyID != "apply_api_newest" {
		t.Fatalf("expected newest api-prod entry after filtering, got %+v", entry)
	}
}

func TestServiceListFiltersByActionBeforeSortingAndLimit(t *testing.T) {
	service := Service{
		ListPlanPreviews: func() ([]planpreview.Record, error) {
			return []planpreview.Record{
				{
					ID:             "plan_api_old",
					DeploymentName: "api-prod",
					Status:         planpreview.StatusReady,
					CreatedAt:      "2026-04-17T08:00:00Z",
					Approval:       planpreview.Approval{Required: true},
					Source:         planpreview.SourceContext{Route: "/v1/overview/deployments/api-prod", Fingerprint: "fp_api_old"},
				},
			}, nil
		},
		ListApplyRequests: func() ([]applyrequest.Record, error) {
			return []applyrequest.Record{
				{
					ID:             "apply_api_newest",
					PlanID:         "plan_api_old",
					DeploymentName: "api-prod",
					Status:         applyrequest.StatusFailed,
					CreatedAt:      "2026-04-18T08:30:00Z",
					Source: applyrequest.SourceContext{
						Route:       "/v1/plans/plan_api_old",
						PlanStatus:  "ready",
						Fingerprint: "fp_api_old",
					},
				},
			}, nil
		},
	}

	result, err := service.List(context.Background(), Query{Action: ActionApplyRequested, Limit: 1})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected one filtered entry, got %+v", result.Entries)
	}
	if entry := result.Entries[0]; entry.Action != string(ActionApplyRequested) || entry.ApplyID != "apply_api_newest" {
		t.Fatalf("expected newest apply entry after action filtering, got %+v", entry)
	}
}

func TestServiceListReturnsExplicitEmptyEntriesForDeploymentWithoutAuditRecords(t *testing.T) {
	service := Service{
		ListPlanPreviews: func() ([]planpreview.Record, error) {
			return []planpreview.Record{{
				ID:             "plan_api_only",
				DeploymentName: "api-prod",
				Status:         planpreview.StatusReady,
				CreatedAt:      "2026-04-17T08:00:00Z",
				Approval:       planpreview.Approval{Required: true},
				Source:         planpreview.SourceContext{Route: "/v1/overview/deployments/api-prod", Fingerprint: "fp_api_only"},
			}}, nil
		},
		ListApplyRequests: func() ([]applyrequest.Record, error) { return nil, nil },
	}

	result, err := service.List(context.Background(), Query{DeploymentName: "worker-prod"})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("expected explicit empty audit result, got %+v", result.Entries)
	}
	if result.ResultScope.ReturnedEntryCount != 0 || result.ResultScope.ReturnedCount != 0 || result.ResultScope.TotalEntryCount != 0 || result.ResultScope.DeploymentName != "worker-prod" || result.ResultScope.Limited || result.ResultScope.EffectiveLimit != 0 || result.ResultScope.TruncationStatus != "not_truncated" || result.ResultScope.Truncated {
		t.Fatalf("expected empty scoped result metadata, got %+v", result.ResultScope)
	}
}

func TestProjectApplyPreservesQueuedLifecycleMetadata(t *testing.T) {
	entry := projectApply(applyrequest.Record{
		ID:             "apply_queued",
		PlanID:         "plan_ready",
		DeploymentName: "api-prod",
		Status:         applyrequest.StatusQueued,
		CreatedAt:      "2026-04-18T08:30:00Z",
		Details: []string{
			"Tenet persisted this immutable apply request as queued because TENET_AWS_ENABLE_APPLY=true enabled the local-safe executor handoff path.",
			"Tenet will advance this same applyId locally, but live AWS mutation is still not implemented for this backend slice.",
		},
		Source: applyrequest.SourceContext{
			Route:       "/v1/plans/plan_ready",
			PlanStatus:  "ready",
			Fingerprint: "fp_plan_ready",
		},
	}, catalog.DeploymentView{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"})

	if entry.Metadata.Status != "queued" || entry.Metadata.Error != "" {
		t.Fatalf("expected queued lifecycle metadata without disabled error, got %+v", entry.Metadata)
	}
	if len(entry.Metadata.Details) != 2 || entry.Source.RecordRoute != "/v1/applies/apply_queued" {
		t.Fatalf("unexpected queued audit projection: %+v", entry)
	}
}

func TestServiceListProjectsApplyTransitionsForAuditTimeline(t *testing.T) {
	service := Service{
		ListPlanPreviews: func() ([]planpreview.Record, error) { return nil, nil },
		ListApplyRequests: func() ([]applyrequest.Record, error) {
			return []applyrequest.Record{{
				ID:             "apply_running",
				PlanID:         "plan_ready",
				DeploymentName: "api-prod",
				Status:         applyrequest.StatusRunning,
				CreatedAt:      "2026-04-18T08:30:00Z",
				Source: applyrequest.SourceContext{
					Route:       "/v1/plans/plan_ready",
					PlanStatus:  "ready",
					Fingerprint: "fp_plan_ready",
				},
				Transitions: []applyrequest.Transition{{
					TransitionedAt: "2026-04-18T08:35:00Z",
					PreviousStatus: applyrequest.StatusQueued,
					Status:         applyrequest.StatusRunning,
					Step:           1,
					Actor:          requestactor.Actor{Available: true, ID: "operator-123", Name: "Release Operator", Email: "release@example.com"},
					Details:        []string{"executor picked up queued apply request"},
					Lifecycle: applyrequest.Lifecycle{
						UpdatedAt:      "2026-04-18T08:35:00Z",
						StartedAt:      "2026-04-18T08:35:00Z",
						CurrentStep:    1,
						CompletedSteps: 0,
						TotalSteps:     2,
					},
				}},
			}}, nil
		},
		ListDeployments: func(context.Context) ([]catalog.DeploymentView, error) {
			return []catalog.DeploymentView{{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}}, nil
		},
	}

	result, err := service.List(context.Background(), Query{Action: ActionApplyTransitioned})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected one transition audit entry, got %+v", result.Entries)
	}
	entry := result.Entries[0]
	if entry.Action != string(ActionApplyTransitioned) || entry.Timestamp != "2026-04-18T08:35:00Z" || entry.ApplyID != "apply_running" || entry.PlanID != "plan_ready" {
		t.Fatalf("unexpected transition audit entry identity: %+v", entry)
	}
	if entry.Actor.ID != "operator-123" || entry.Metadata.PreviousStatus != "queued" || entry.Metadata.NewStatus != "running" || entry.Metadata.Step != 1 || entry.Metadata.TotalSteps != 2 {
		t.Fatalf("expected transition actor and lifecycle metadata, got %+v", entry)
	}
	if entry.Source.Kind != "applyTransition" || entry.Source.RecordRoute != "/v1/applies/apply_running" || entry.Source.SourceRoute != "/v1/plans/plan_ready" {
		t.Fatalf("expected transition source handoff to the immutable apply and plan records, got %+v", entry.Source)
	}
	if result.ResultScope.Action != string(ActionApplyTransitioned) || result.ResultScope.TotalEntryCount != 1 {
		t.Fatalf("expected filtered result scope for transition action, got %+v", result.ResultScope)
	}
}

func TestServiceListProjectsManualControllerRefreshAuditRows(t *testing.T) {
	service := Service{
		ListPlanPreviews:  func() ([]planpreview.Record, error) { return nil, nil },
		ListApplyRequests: func() ([]applyrequest.Record, error) { return nil, nil },
		ListControllerRefreshAuditRows: func() ([]controllerrefreshaudit.Record, error) {
			return []controllerrefreshaudit.Record{{
				ID:             "api-controller-refresh:manual-refresh-123",
				Route:          "POST /v1/controller/refresh",
				Status:         "published",
				Result:         "published",
				RequestedAt:    "2026-04-20T10:45:00Z",
				RequestID:      "api-controller-refresh:manual-refresh-123",
				DeliveryID:     "manual-refresh-123",
				Backend:        "redis",
				Channel:        "tenet:test:controller-refresh",
				HTTPStatusCode: 202,
				Actor: requestactor.Actor{
					Available: true,
					ID:        "operator-123",
					Name:      "Release Operator",
					Email:     "release@example.com",
				},
			}}, nil
		},
	}

	result, err := service.List(context.Background(), Query{Action: ActionControllerRefresh})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected one controller refresh audit entry, got %+v", result.Entries)
	}
	entry := result.Entries[0]
	if entry.Action != string(ActionControllerRefresh) || entry.Timestamp != "2026-04-20T10:45:00Z" {
		t.Fatalf("unexpected controller refresh audit identity: %+v", entry)
	}
	if entry.DeploymentName != "" || entry.AppRef != "" || entry.EnvironmentRef != "" {
		t.Fatalf("controller refresh audit rows should remain global, got %+v", entry)
	}
	if entry.Metadata.Route != "POST /v1/controller/refresh" || entry.Metadata.Status != "published" || entry.Metadata.Result != "published" || entry.Metadata.RequestID != "api-controller-refresh:manual-refresh-123" || entry.Metadata.DeliveryID != "manual-refresh-123" || entry.Metadata.Backend != "redis" || entry.Metadata.Channel != "tenet:test:controller-refresh" || entry.Metadata.HTTPStatusCode != 202 {
		t.Fatalf("unexpected controller refresh audit metadata: %+v", entry.Metadata)
	}
	if entry.Source.Kind != "controllerRefreshRequest" || entry.Source.Store != "controllerRefreshAuditStore" || entry.Source.RecordRoute != "POST /v1/controller/refresh" {
		t.Fatalf("unexpected controller refresh audit source: %+v", entry.Source)
	}
	if !entry.Actor.Available || entry.Actor.ID != "operator-123" {
		t.Fatalf("expected controller refresh audit actor projection, got %+v", entry.Actor)
	}
	if result.ResultScope.Action != string(ActionControllerRefresh) || result.ResultScope.TotalEntryCount != 1 {
		t.Fatalf("expected controller refresh filtered result scope, got %+v", result.ResultScope)
	}
}
