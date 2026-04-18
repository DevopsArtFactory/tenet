package auditlog

import (
	"context"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
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
		ListDeployments: func() ([]catalog.DeploymentView, error) {
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
}

func TestProjectApplyPreservesQueuedLifecycleMetadata(t *testing.T) {
	entry := projectApply(applyrequest.Record{
		ID:             "apply_queued",
		PlanID:         "plan_ready",
		DeploymentName: "api-prod",
		Status:         applyrequest.StatusQueued,
		CreatedAt:      "2026-04-18T08:30:00Z",
		Details: []string{
			"Tenet persisted this immutable apply request as queued because TENET_AWS_ENABLE_APPLY=true.",
			"No AWS mutation has been implemented for this backend slice yet.",
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
