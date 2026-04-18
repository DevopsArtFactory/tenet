package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/auditlog"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/DevopsArtFactory/tenet/internal/resourceinspection"
	"github.com/DevopsArtFactory/tenet/internal/resourcelogs"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

func assertInternalServerErrorResponse(t *testing.T, resp *httptest.ResponseRecorder, leaked string) {
	t.Helper()
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
	body := resp.Body.String()
	if body != http.StatusText(http.StatusInternalServerError)+"\n" {
		t.Fatalf("expected generic 500 body %q, got %q", http.StatusText(http.StatusInternalServerError)+"\n", body)
	}
	if strings.Contains(body, leaked) {
		t.Fatalf("expected 500 body to avoid leaking %q, got %q", leaked, body)
	}
}

func assertResponseRequestID(t *testing.T, resp *httptest.ResponseRecorder) string {
	t.Helper()
	requestID := resp.Header().Get(RequestIDHeader)
	if requestID == "" {
		t.Fatalf("expected %s response header to be set", RequestIDHeader)
	}
	return requestID
}

func TestRouterHealthz(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	assertResponseRequestID(t, resp)
}

func TestRouterStructuredRequestLoggingCoversAPIAndUIRequests(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		Logger:    logger,
	})

	for _, target := range []string{"/healthz", "/"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set(RequestIDHeader, "client-request-id")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", target, resp.Code)
		}
		if got := resp.Header().Get(RequestIDHeader); got != "client-request-id" {
			t.Fatalf("expected Request-Id echo for %s, got %q", target, got)
		}
	}

	entries := decodeStructuredLogLines(t, output.String())
	if len(entries) != 2 {
		t.Fatalf("expected one structured log line per request, got %d entries: %#v", len(entries), entries)
	}

	for index, target := range []string{"/healthz", "/"} {
		entry := entries[index]
		if entry["msg"] != "http_request" {
			t.Fatalf("expected http_request log message, got %#v", entry)
		}
		if entry["request_id"] != "client-request-id" || entry["method"] != http.MethodGet || entry["path"] != target {
			t.Fatalf("expected stable request fields for %s, got %#v", target, entry)
		}
		if entry["status"] != float64(http.StatusOK) {
			t.Fatalf("expected 200 status for %s, got %#v", target, entry["status"])
		}
		if entry["result"] != "success" {
			t.Fatalf("expected success result for %s, got %#v", target, entry["result"])
		}
		if _, ok := entry["duration_ms"]; !ok {
			t.Fatalf("expected duration_ms field for %s, got %#v", target, entry)
		}
	}
}

func TestRouterRecoversPanicsWithCorrelatedResponseAndLogs(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		Logger:    logger,
		CatalogSummary: func() (catalog.Summary, error) {
			panic("boom")
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/summary", nil)
	req.Header.Set(RequestIDHeader, "panic-request-id")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
	if got := resp.Header().Get(RequestIDHeader); got != "panic-request-id" {
		t.Fatalf("expected Request-Id echo, got %q", got)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "request_id=panic-request-id") {
		t.Fatalf("expected correlated panic response body, got %q", body)
	}
	if strings.Contains(body, "goroutine") || strings.Contains(body, "panic(") {
		t.Fatalf("expected panic response body to avoid stack traces, got %q", body)
	}

	entries := decodeStructuredLogLines(t, output.String())
	if len(entries) != 2 {
		t.Fatalf("expected panic log and completion log, got %d entries: %#v", len(entries), entries)
	}
	if entries[0]["msg"] != "http_request_panic" || entries[0]["request_id"] != "panic-request-id" {
		t.Fatalf("expected panic log entry first, got %#v", entries[0])
	}
	if entries[1]["msg"] != "http_request" || entries[1]["result"] != "panic" || entries[1]["status"] != float64(http.StatusInternalServerError) {
		t.Fatalf("expected completion log to classify panic, got %#v", entries[1])
	}
}

func TestRouterReturnsExplicitTimeoutResponseAndLogsCorrelation(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	router := NewRouter(Options{
		BuildInfo:      buildinfo.Info{Version: "dev", Commit: "abc123"},
		Logger:         logger,
		RequestTimeout: 10 * time.Millisecond,
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{Name: name}, nil
		},
		OverviewDeploymentResourceLogs: func(ctx context.Context, name, resourceType, resourceName string) (resourcelogs.Result, error) {
			<-ctx.Done()
			return resourcelogs.Result{}, ctx.Err()
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing", nil)
	req.Header.Set(RequestIDHeader, "timeout-request-id")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 timeout response, got %d", resp.Code)
	}
	if got := resp.Header().Get(RequestIDHeader); got != "timeout-request-id" {
		t.Fatalf("expected Request-Id echo, got %q", got)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "request timed out") || !strings.Contains(body, "request_id=timeout-request-id") {
		t.Fatalf("expected explicit correlated timeout response, got %q", body)
	}

	entries := decodeStructuredLogLines(t, output.String())
	if len(entries) != 1 {
		t.Fatalf("expected one completion log for timeout request, got %d entries: %#v", len(entries), entries)
	}
	if entries[0]["msg"] != "http_request" || entries[0]["result"] != "timeout" || entries[0]["error"] != "request timed out" {
		t.Fatalf("expected timeout classification in access log, got %#v", entries[0])
	}
	if entries[0]["request_id"] != "timeout-request-id" || entries[0]["status"] != float64(http.StatusServiceUnavailable) {
		t.Fatalf("expected correlated timeout log entry, got %#v", entries[0])
	}
}

func TestRouterSetsRequestIDHeaderAcrossRepresentativeResponses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		options    Options
		method     string
		target     string
		wantStatus int
	}{
		{
			name:       "not found",
			options:    Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}},
			method:     http.MethodGet,
			target:     "/missing",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "method not allowed",
			options:    Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}},
			method:     http.MethodPost,
			target:     "/healthz",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "service unavailable",
			options:    Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}},
			method:     http.MethodGet,
			target:     "/v1/catalog/summary",
			wantStatus: http.StatusServiceUnavailable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(tc.options)
			req := httptest.NewRequest(tc.method, tc.target, nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != tc.wantStatus {
				t.Fatalf("expected %d, got %d", tc.wantStatus, resp.Code)
			}
			assertResponseRequestID(t, resp)
		})
	}
}

func decodeStructuredLogLines(t *testing.T, output string) []map[string]any {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(output), "\n")
	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("failed to decode structured log line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func TestRouterInfo(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var info buildinfo.Info
	if err := json.Unmarshal(resp.Body.Bytes(), &info); err != nil {
		t.Fatalf("failed to decode info: %v", err)
	}

	if info.Commit != "abc123" {
		t.Fatalf("unexpected commit: %s", info.Commit)
	}
}

func TestRouterCatalogSummary(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogSummary: func() (catalog.Summary, error) {
			return catalog.Summary{AccountCount: 1, EnvironmentCount: 2, DeploymentCount: 3}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/summary", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var summary catalog.Summary
	if err := json.Unmarshal(resp.Body.Bytes(), &summary); err != nil {
		t.Fatalf("failed to decode summary: %v", err)
	}
	if summary.DeploymentCount != 3 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestRouterCatalogSummaryUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/summary", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterCatalogSummaryPropagatesErrors(t *testing.T) {
	const upstreamErr = "boom: summary provider details"
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogSummary: func() (catalog.Summary, error) {
			return catalog.Summary{}, errors.New(upstreamErr)
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/summary", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertInternalServerErrorResponse(t, resp, upstreamErr)
}

func TestRouterCatalogDeployments(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogDeployments: func() ([]catalog.DeploymentView, error) {
			return []catalog.DeploymentView{{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var deployments []catalog.DeploymentView
	if err := json.Unmarshal(resp.Body.Bytes(), &deployments); err != nil {
		t.Fatalf("failed to decode deployments: %v", err)
	}
	if len(deployments) != 1 || deployments[0].Name != "api-prod" {
		t.Fatalf("unexpected deployments: %+v", deployments)
	}
}

func TestRouterCatalogDeploymentsUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterCatalogDeploymentsPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogDeployments: func() ([]catalog.DeploymentView, error) {
			return nil, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterCatalogEnvironments(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogEnvironments: func() ([]catalog.EnvironmentView, error) {
			return []catalog.EnvironmentView{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2", NetworkRef: "prod-main"}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/environments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var environments []catalog.EnvironmentView
	if err := json.Unmarshal(resp.Body.Bytes(), &environments); err != nil {
		t.Fatalf("failed to decode environments: %v", err)
	}
	if len(environments) != 1 || environments[0].Name != "prod-apne2" {
		t.Fatalf("unexpected environments: %+v", environments)
	}
}

func TestRouterCatalogEnvironmentsUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/environments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterCatalogEnvironmentsPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogEnvironments: func() ([]catalog.EnvironmentView, error) {
			return nil, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/environments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterCatalogAccounts(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogAccounts: func() ([]catalog.AccountView, error) {
			return []catalog.AccountView{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var accounts []catalog.AccountView
	if err := json.Unmarshal(resp.Body.Bytes(), &accounts); err != nil {
		t.Fatalf("failed to decode accounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Name != "prod-core" {
		t.Fatalf("unexpected accounts: %+v", accounts)
	}
}

func TestRouterCatalogAccountsUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterCatalogAccountsPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogAccounts: func() ([]catalog.AccountView, error) {
			return nil, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterCatalogPlacements(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogPlacements: func() ([]catalog.PlacementView, error) {
			return []catalog.PlacementView{{DeploymentName: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", AccountName: "prod-core", AWSAccountID: "123456789012", Region: "ap-northeast-2", NetworkRef: "prod-main"}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/placements", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var placements []catalog.PlacementView
	if err := json.Unmarshal(resp.Body.Bytes(), &placements); err != nil {
		t.Fatalf("failed to decode placements: %v", err)
	}
	if len(placements) != 1 || placements[0].DeploymentName != "api-prod" {
		t.Fatalf("unexpected placements: %+v", placements)
	}
}

func TestRouterCatalogPlacementsUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/placements", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterCatalogPlacementsPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogPlacements: func() ([]catalog.PlacementView, error) {
			return nil, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/placements", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterCatalogTopology(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogTopology: func() (catalog.TopologySnapshot, error) {
			return catalog.TopologySnapshot{Summary: catalog.Summary{AccountCount: 1, EnvironmentCount: 1, DeploymentCount: 1}, Accounts: []catalog.AccountView{{Name: "prod-core"}}, Environments: []catalog.EnvironmentView{{Name: "prod-apne2"}}, Deployments: []catalog.DeploymentView{{Name: "api-prod"}}, Placements: []catalog.PlacementView{{DeploymentName: "api-prod", AccountName: "prod-core"}}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/topology", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var topology catalog.TopologySnapshot
	if err := json.Unmarshal(resp.Body.Bytes(), &topology); err != nil {
		t.Fatalf("failed to decode topology: %v", err)
	}
	if topology.Summary.DeploymentCount != 1 || len(topology.Placements) != 1 {
		t.Fatalf("unexpected topology: %+v", topology)
	}
}

func TestRouterCatalogTopologyUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/topology", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterCatalogTopologyPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogTopology: func() (catalog.TopologySnapshot, error) {
			return catalog.TopologySnapshot{}, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/topology", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterAuditLogs(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		AuditLogs: func(ctx context.Context, query auditlog.Query) (auditlog.Result, error) {
			if query.Limit != 2 || query.DeploymentName != "api-prod" || query.Action != auditlog.ActionApplyRequested {
				t.Fatalf("expected filtered audit query, got %+v", query)
			}
			return auditlog.Result{
				Entries: []auditlog.Entry{{
					Timestamp:      "2026-04-18T08:00:00Z",
					Action:         "apply.requested",
					DeploymentName: "api-prod",
					AppRef:         "payments-api",
					EnvironmentRef: "prod-apne2",
					PlanID:         "plan_1234",
					ApplyID:        "apply_5678",
					Actor:          auditlog.Actor{Available: false, Reason: "actor information is unavailable"},
					Metadata:       auditlog.Metadata{Status: "failed", PlanStatus: "ready", Error: "apply execution is disabled"},
					Source:         auditlog.Source{Kind: "applyRequest", Store: "applyRequestStore", RecordRoute: "/v1/applies/apply_5678", SourceRoute: "/v1/plans/plan_1234"},
				}},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-logs?deployment=api-prod&action=apply.requested&limit=2", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var result auditlog.Result
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode audit logs: %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].ApplyID != "apply_5678" {
		t.Fatalf("unexpected audit logs payload: %+v", result)
	}
}

func TestRouterAuditLogsUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-logs", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterAuditLogsRejectsInvalidLimit(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		AuditLogs: func(context.Context, auditlog.Query) (auditlog.Result, error) {
			t.Fatal("expected invalid limit to short-circuit before provider")
			return auditlog.Result{}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-logs?limit=0", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestRouterAuditLogsRejectsInvalidDeploymentQuery(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		AuditLogs: func(context.Context, auditlog.Query) (auditlog.Result, error) {
			t.Fatal("expected invalid deployment query to short-circuit before provider")
			return auditlog.Result{}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-logs?deployment=api/prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestRouterAuditLogsRejectsInvalidActionQuery(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		AuditLogs: func(context.Context, auditlog.Query) (auditlog.Result, error) {
			t.Fatal("expected invalid action query to short-circuit before provider")
			return auditlog.Result{}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-logs?action=plan.deleted", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestRouterAuditLogsMethodNotAllowed(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		AuditLogs: func(context.Context, auditlog.Query) (auditlog.Result, error) {
			return auditlog.Result{}, nil
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/audit-logs", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.Code)
	}
	if allow := resp.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow GET, got %q", allow)
	}
}

func TestRouterDeploymentStatuses(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatuses: func() ([]statusstore.Snapshot, error) {
			return []statusstore.Snapshot{{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "RolloutCompleted"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "all checks passed"}, Conditions: []domain.Condition{{Type: "Ready", Status: true, Reason: "Stable", Message: "deployment is healthy"}}}}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var snapshots []statusstore.Snapshot
	if err := json.Unmarshal(resp.Body.Bytes(), &snapshots); err != nil {
		t.Fatalf("failed to decode snapshots: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].DeploymentName != "api-prod" {
		t.Fatalf("unexpected snapshots: %+v", snapshots)
	}
	if snapshots[0].Status.Sync.Reason != "RolloutCompleted" || len(snapshots[0].Status.Conditions) != 1 {
		t.Fatalf("unexpected status payload: %+v", snapshots[0].Status)
	}
}

func TestRouterDeploymentStatusesUseStableRolloutAndHistoryJSONFieldNames(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatuses: func() ([]statusstore.Snapshot, error) {
			return []statusstore.Snapshot{{DeploymentName: "api-prod", Status: domain.DeploymentStatus{
				ObservedGeneration: 3,
				Sync:               domain.Sync{State: domain.SyncStateSynced, Reason: "RolloutCompleted"},
				Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "all checks passed"},
				Rollout: &domain.RolloutSummary{
					DesiredGeneration:  3,
					ObservedGeneration: 3,
					Phase:              domain.RolloutPhaseCompleted,
					Summary:            "Deployment is synced and healthy",
					Steps:              []domain.RolloutStepSummary{{Kind: "MarkSynced", Status: domain.RolloutStepStatusCompleted}},
				},
				History: []domain.StatusHistoryEntry{{
					Timestamp:    "2026-04-16T07:00:00Z",
					Source:       "controller",
					Summary:      "Deployment is synced and healthy",
					Sync:         domain.Sync{State: domain.SyncStateSynced, Reason: "RolloutCompleted"},
					Health:       domain.Health{Overall: domain.HealthHealthy, Summary: "all checks passed"},
					RolloutPhase: domain.RolloutPhaseCompleted,
				}},
			}}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw status payload: %v", err)
	}
	status, ok := payload[0]["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested status object, got %#v", payload[0]["status"])
	}
	rollout, ok := status["rollout"].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout object in raw status payload, got %#v", status["rollout"])
	}
	if rollout["phase"] != string(domain.RolloutPhaseCompleted) {
		t.Fatalf("expected rollout.phase field, got %#v", rollout)
	}
	if _, exists := rollout["currentStep"]; exists {
		t.Fatalf("expected completed rollout currentStep to be omitted, got %#v", rollout)
	}
	if _, exists := rollout["CurrentStep"]; exists {
		t.Fatalf("unexpected Go-style rollout field name in %#v", rollout)
	}
	steps, ok := rollout["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected rollout.steps array in raw status payload, got %#v", rollout["steps"])
	}
	step, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[0] object, got %#v", steps[0])
	}
	if step["kind"] != "MarkSynced" || step["status"] != string(domain.RolloutStepStatusCompleted) {
		t.Fatalf("expected rollout.steps[0] field names/values, got %#v", step)
	}
	if _, exists := step["current"]; exists {
		t.Fatalf("expected completed rollout step current flag to be omitted, got %#v", step)
	}
	if _, exists := step["Current"]; exists {
		t.Fatalf("unexpected Go-style step field name in %#v", step)
	}
	history, ok := status["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("expected history array in raw status payload, got %#v", status["history"])
	}
	historyEntry, ok := history[0].(map[string]any)
	if !ok {
		t.Fatalf("expected history[0] object, got %#v", history[0])
	}
	if historyEntry["rolloutPhase"] != string(domain.RolloutPhaseCompleted) {
		t.Fatalf("expected history[0].rolloutPhase field, got %#v", historyEntry)
	}
	if _, exists := historyEntry["currentStep"]; exists {
		t.Fatalf("expected completed history currentStep to be omitted, got %#v", historyEntry)
	}
	if _, exists := historyEntry["RolloutPhase"]; exists {
		t.Fatalf("unexpected Go-style history field name in %#v", historyEntry)
	}
}

func TestRouterDeploymentStatusesUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterDeploymentStatusesPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatuses: func() ([]statusstore.Snapshot, error) {
			return nil, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterDeploymentStatusByName(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatus: func(name string) (statusstore.Snapshot, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected lookup name: %s", name)
			}
			return statusstore.Snapshot{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "RolloutCompleted"}}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var snapshot statusstore.Snapshot
	if err := json.Unmarshal(resp.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("failed to decode snapshot: %v", err)
	}
	if snapshot.DeploymentName != "api-prod" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestRouterDeploymentStatusByNameUsesStableJSONFieldNames(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatus: func(name string) (statusstore.Snapshot, error) {
			return statusstore.Snapshot{DeploymentName: "api-prod", Status: domain.DeploymentStatus{
				ObservedGeneration: 3,
				Sync:               domain.Sync{State: domain.SyncStateSynced, Reason: "RolloutCompleted"},
				Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "all checks passed"},
				ManagedDrift: []domain.ManagedDriftItem{
					{Field: "artifact", Desired: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r4", Actual: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r3", Severity: domain.ManagedDriftSeverityHigh},
				},
				Conditions: []domain.Condition{{Type: "Ready", Status: true, Reason: "Stable", Message: "deployment is healthy"}},
				Rollout: &domain.RolloutSummary{
					DesiredGeneration:  4,
					ObservedGeneration: 3,
					Phase:              domain.RolloutPhaseProvisioning,
					CurrentStep:        "CreateCandidateCapacity",
					Summary:            "Creating candidate capacity for weighted target groups",
					Steps:              []domain.RolloutStepSummary{{Kind: "ResolvePlacement", Status: domain.RolloutStepStatusCompleted}, {Kind: "CreateCandidateCapacity", Current: true, Status: domain.RolloutStepStatusCurrent}},
				},
				History: []domain.StatusHistoryEntry{{
					Timestamp:    "2026-04-16T07:05:00Z",
					Source:       "controller",
					Summary:      "Creating candidate capacity for weighted target groups",
					Sync:         domain.Sync{State: domain.SyncStateReconciling, Reason: "CandidateProvisioning"},
					Health:       domain.Health{Overall: domain.HealthProgressing, Summary: "warming"},
					ManagedDrift: []domain.ManagedDriftItem{{Field: "artifact", Desired: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r4", Actual: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r3", Severity: domain.ManagedDriftSeverityHigh}},
					RolloutPhase: domain.RolloutPhaseProvisioning,
					CurrentStep:  "CreateCandidateCapacity",
				}},
			}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw payload: %v", err)
	}
	status, ok := payload["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested status object, got %#v", payload["status"])
	}
	if _, ok := status["observedGeneration"]; !ok {
		t.Fatalf("expected observedGeneration field in %#v", status)
	}
	sync, ok := status["sync"].(map[string]any)
	if !ok {
		t.Fatalf("expected sync object, got %#v", status["sync"])
	}
	if sync["state"] != string(domain.SyncStateSynced) {
		t.Fatalf("unexpected sync payload: %#v", sync)
	}
	rollout, ok := status["rollout"].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout object, got %#v", status["rollout"])
	}
	if rollout["phase"] != string(domain.RolloutPhaseProvisioning) || rollout["currentStep"] != "CreateCandidateCapacity" {
		t.Fatalf("expected rollout phase/currentStep fields, got %#v", rollout)
	}
	steps, ok := rollout["steps"].([]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("expected rollout.steps array, got %#v", rollout["steps"])
	}
	firstStep, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[0] object, got %#v", steps[0])
	}
	if firstStep["kind"] != "ResolvePlacement" || firstStep["status"] != string(domain.RolloutStepStatusCompleted) {
		t.Fatalf("expected rollout.steps[0] field names/values, got %#v", firstStep)
	}
	if _, exists := firstStep["current"]; exists {
		t.Fatalf("expected completed rollout step current flag to be omitted, got %#v", firstStep)
	}
	currentStep, ok := steps[1].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[1] object, got %#v", steps[1])
	}
	if currentStep["kind"] != "CreateCandidateCapacity" || currentStep["current"] != true || currentStep["status"] != string(domain.RolloutStepStatusCurrent) {
		t.Fatalf("expected rollout.steps[1] field names/values, got %#v", currentStep)
	}
	history, ok := status["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("expected history array in raw payload, got %#v", status["history"])
	}
	historyEntry, ok := history[0].(map[string]any)
	if !ok {
		t.Fatalf("expected history[0] object, got %#v", history[0])
	}
	if historyEntry["currentStep"] != "CreateCandidateCapacity" || historyEntry["rolloutPhase"] != string(domain.RolloutPhaseProvisioning) {
		t.Fatalf("expected history field names/values, got %#v", historyEntry)
	}
	managedDrift, ok := status["managedDrift"].([]any)
	if !ok || len(managedDrift) != 1 {
		t.Fatalf("expected managedDrift array in raw payload, got %#v", status["managedDrift"])
	}
	driftItem, ok := managedDrift[0].(map[string]any)
	if !ok || driftItem["field"] != "artifact" || driftItem["desired"] != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r4" || driftItem["actual"] != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r3" || driftItem["severity"] != string(domain.ManagedDriftSeverityHigh) {
		t.Fatalf("expected managedDrift item fields, got %#v", managedDrift[0])
	}
	historyManagedDrift, ok := historyEntry["managedDrift"].([]any)
	if !ok || len(historyManagedDrift) != 1 {
		t.Fatalf("expected history managedDrift array, got %#v", historyEntry["managedDrift"])
	}
	if _, exists := status["ManagedDrift"]; exists {
		t.Fatalf("unexpected Go-style managedDrift field name in %#v", status)
	}
	if _, exists := status["ObservedGeneration"]; exists {
		t.Fatalf("unexpected Go-style field name in %#v", status)
	}
}

func TestRouterDeploymentStatusByNameUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterDeploymentStatusByNameNotFound(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatus: func(name string) (statusstore.Snapshot, error) {
			return statusstore.Snapshot{}, statusstore.ErrNotFound
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments/missing", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestRouterDeploymentStatusByNameRejectsInvalidNames(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatus: func(name string) (statusstore.Snapshot, error) {
			return statusstore.Snapshot{}, nil
		},
	})
	for _, path := range []string{"/v1/status/deployments/", "/v1/status/deployments/a/b"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
		}
	}
}

func TestRouterDeploymentStatusByNamePropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatus: func(name string) (statusstore.Snapshot, error) {
			return statusstore.Snapshot{}, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterOverviewAccounts(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewAccounts: func() ([]catalog.AccountOverview, error) {
			return []catalog.AccountOverview{{
				Name:             "prod-core",
				AWSAccountID:     "123456789012",
				DeploymentCount:  3,
				ProjectCount:     2,
				EnvironmentCount: 2,
				SyncedCount:      1,
				ProgressingCount: 1,
				PausedCount:      1,
				ErrorCount:       0,
				Deployments: []catalog.DeploymentOverview{
					{
						Name:          "api-prod",
						Strategy:      "canary",
						ExecutionMode: "asgWeightedTargetGroups",
						RuntimeSummary: &domain.RuntimeSummary{
							Timestamp:          "2026-04-16T06:49:00Z",
							Source:             "controller",
							Summary:            "Traffic shift is progressing",
							Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "Traffic shift running"},
							Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
							DesiredGeneration:  8,
							ObservedGeneration: 7,
							RolloutPhase:       domain.RolloutPhaseAccepted,
							CurrentStep:        "PromoteCandidateCapacity",
							TargetType:         "ec2",
							Strategy:           "canary",
							ExecutionMode:      "asgWeightedTargetGroups",
							HistoryCount:       4,
							Changes:            []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"},
						},
						Rollout: &domain.RolloutSummary{
							DesiredGeneration:  4,
							ObservedGeneration: 3,
							Phase:              domain.RolloutPhaseAccepted,
							CurrentStep:        "PromoteCandidateCapacity",
							Summary:            "Observing current state",
							Steps: []domain.RolloutStepSummary{
								{Kind: "CreateCandidateCapacity", Status: "completed"},
								{Kind: "ShiftTrafficCanary", Status: "pending"},
								{Kind: "PromoteCandidateCapacity", Current: true, Status: "current"},
							},
						},
					},
					{
						Name:          "api-staging",
						Strategy:      "linear",
						ExecutionMode: "ecsCodeDeploy",
						RuntimeSummary: &domain.RuntimeSummary{
							Summary:            "Waiting for service stability",
							Sync:               domain.Sync{State: domain.SyncStateOutOfSync, Reason: "PendingPromotion"},
							Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "warming up"},
							DesiredGeneration:  8,
							ObservedGeneration: 7,
							RolloutPhase:       domain.RolloutPhaseAccepted,
							CurrentStep:        "VerifyServiceStability",
							HistoryCount:       0,
						},
						Rollout: &domain.RolloutSummary{
							DesiredGeneration:  8,
							ObservedGeneration: 7,
							Phase:              domain.RolloutPhaseAccepted,
							CurrentStep:        "VerifyServiceStability",
							Summary:            "Observing current state",
							Steps: []domain.RolloutStepSummary{
								{Kind: "CreateCodeDeployDeployment", Status: "completed"},
								{Kind: "ShiftTrafficLinear", Status: "pending"},
								{Kind: "VerifyServiceStability", Current: true, Status: "current"},
							},
						},
					},
				},
			}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var accounts []catalog.AccountOverview
	if err := json.Unmarshal(resp.Body.Bytes(), &accounts); err != nil {
		t.Fatalf("failed to decode overview accounts: %v", err)
	}
	if len(accounts) != 1 || len(accounts[0].Deployments) != 2 || accounts[0].Deployments[0].ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("unexpected overview accounts: %+v", accounts)
	}
	if accounts[0].DeploymentCount != 3 || accounts[0].ProjectCount != 2 || accounts[0].EnvironmentCount != 2 || accounts[0].SyncedCount != 1 || accounts[0].ProgressingCount != 1 || accounts[0].PausedCount != 1 || accounts[0].ErrorCount != 0 {
		t.Fatalf("expected additive account counts in overview response, got %+v", accounts[0])
	}
	if accounts[0].Deployments[0].Rollout == nil || accounts[0].Deployments[0].Rollout.Phase != domain.RolloutPhaseAccepted || accounts[0].Deployments[0].Rollout.CurrentStep != "PromoteCandidateCapacity" || len(accounts[0].Deployments[0].Rollout.Steps) != 3 {
		t.Fatalf("expected rollout summary in overview accounts response, got %+v", accounts)
	}
	if got := accounts[0].Deployments[0].Rollout.Steps; got[0].Kind != "CreateCandidateCapacity" || got[0].Current || got[1].Kind != "ShiftTrafficCanary" || got[1].Current || got[2].Kind != "PromoteCandidateCapacity" || !got[2].Current {
		t.Fatalf("expected strategy-aware EC2 rollout payload, got %+v", got)
	}
	if accounts[0].Deployments[1].Rollout == nil || accounts[0].Deployments[1].ExecutionMode != "ecsCodeDeploy" || accounts[0].Deployments[1].Rollout.Phase != domain.RolloutPhaseAccepted || accounts[0].Deployments[1].Rollout.CurrentStep != "VerifyServiceStability" {
		t.Fatalf("expected strategy-aware ECS rollout payload, got %+v", accounts[0].Deployments[1])
	}
	if accounts[0].Deployments[0].RuntimeSummary == nil || accounts[0].Deployments[0].RuntimeSummary.Timestamp != "2026-04-16T06:49:00Z" || accounts[0].Deployments[0].RuntimeSummary.Source != "controller" || accounts[0].Deployments[0].RuntimeSummary.Summary != "Traffic shift is progressing" || accounts[0].Deployments[0].RuntimeSummary.Sync.Reason != "Traffic shift running" || accounts[0].Deployments[0].RuntimeSummary.DesiredGeneration != 8 || accounts[0].Deployments[0].RuntimeSummary.ObservedGeneration != 7 || accounts[0].Deployments[0].RuntimeSummary.CurrentStep != "PromoteCandidateCapacity" || accounts[0].Deployments[0].RuntimeSummary.TargetType != "ec2" || accounts[0].Deployments[0].RuntimeSummary.Strategy != "canary" || accounts[0].Deployments[0].RuntimeSummary.ExecutionMode != "asgWeightedTargetGroups" || accounts[0].Deployments[0].RuntimeSummary.HistoryCount != 4 || !reflect.DeepEqual(accounts[0].Deployments[0].RuntimeSummary.Changes, []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"}) {
		t.Fatalf("expected runtime summary in overview accounts response, got %+v", accounts[0].Deployments[0])
	}
	if got := accounts[0].Deployments[1].Rollout.Steps; got[0].Kind != "CreateCodeDeployDeployment" || got[0].Current || got[1].Kind != "ShiftTrafficLinear" || got[1].Current || got[2].Kind != "VerifyServiceStability" || !got[2].Current {
		t.Fatalf("expected strategy-aware ECS rollout steps, got %+v", got)
	}

	var payload []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview accounts payload: %v", err)
	}
	if payload[0]["deploymentCount"] != float64(3) || payload[0]["projectCount"] != float64(2) || payload[0]["environmentCount"] != float64(2) || payload[0]["syncedCount"] != float64(1) || payload[0]["progressingCount"] != float64(1) || payload[0]["pausedCount"] != float64(1) || payload[0]["errorCount"] != float64(0) {
		t.Fatalf("expected additive JSON count fields, got %+v", payload[0])
	}
	rollout, ok := payload[0]["deployments"].([]any)[0].(map[string]any)["rollout"].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout object in raw overview payload, got %#v", payload[0])
	}
	steps, ok := rollout["steps"].([]any)
	if !ok || len(steps) != 3 {
		t.Fatalf("expected rollout.steps array in raw overview payload, got %#v", rollout["steps"])
	}
	firstStep, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[0] object, got %#v", steps[0])
	}
	if firstStep["kind"] != "CreateCandidateCapacity" || firstStep["status"] != "completed" {
		t.Fatalf("expected rollout.steps[0] status field names, got %#v", firstStep)
	}
	lastStep, ok := steps[2].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[2] object, got %#v", steps[2])
	}
	if lastStep["kind"] != "PromoteCandidateCapacity" || lastStep["current"] != true || lastStep["status"] != "current" {
		t.Fatalf("expected rollout.steps[2] current/status field names, got %#v", lastStep)
	}
	runtimeSummary, ok := payload[0]["deployments"].([]any)[0].(map[string]any)["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in raw overview payload, got %#v", payload[0])
	}
	if runtimeSummary["timestamp"] != "2026-04-16T06:49:00Z" || runtimeSummary["source"] != "controller" || runtimeSummary["summary"] != "Traffic shift is progressing" || runtimeSummary["desiredGeneration"] != float64(8) || runtimeSummary["observedGeneration"] != float64(7) || runtimeSummary["currentStep"] != "PromoteCandidateCapacity" || runtimeSummary["targetType"] != "ec2" || runtimeSummary["strategy"] != "canary" || runtimeSummary["executionMode"] != "asgWeightedTargetGroups" || runtimeSummary["historyCount"] != float64(4) || runtimeSummary["rolloutPhase"] != string(domain.RolloutPhaseAccepted) {
		t.Fatalf("expected runtimeSummary JSON fields in raw overview payload, got %#v", runtimeSummary)
	}
	changes, ok := runtimeSummary["changes"].([]any)
	if !ok || len(changes) != 7 || changes[0] != "summary" || changes[6] != "currentStep" {
		t.Fatalf("expected runtimeSummary.changes field in raw overview payload, got %#v", runtimeSummary["changes"])
	}
	syncSummary, ok := runtimeSummary["sync"].(map[string]any)
	if !ok || syncSummary["state"] != string(domain.SyncStateReconciling) || syncSummary["reason"] != "Traffic shift running" {
		t.Fatalf("expected runtimeSummary.sync fields in raw overview payload, got %#v", runtimeSummary["sync"])
	}
	healthSummary, ok := runtimeSummary["health"].(map[string]any)
	if !ok || healthSummary["overall"] != string(domain.HealthHealthy) || healthSummary["summary"] != "steady" {
		t.Fatalf("expected runtimeSummary.health fields in raw overview payload, got %#v", runtimeSummary["health"])
	}
}

func TestOverviewAccountsResponseIncludesRuntimeSummary(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewAccounts: func() ([]catalog.AccountOverview, error) {
			return []catalog.AccountOverview{{
				Name:         "prod-core",
				AWSAccountID: "123456789012",
				Deployments: []catalog.DeploymentOverview{{
					Name: "api-prod",
					RuntimeSummary: &domain.RuntimeSummary{
						Timestamp:          "2026-04-16T06:49:00Z",
						Source:             "controller",
						Summary:            "Traffic shift is progressing",
						Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "Traffic shift running"},
						Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
						DesiredGeneration:  8,
						ObservedGeneration: 7,
						RolloutPhase:       domain.RolloutPhaseAccepted,
						CurrentStep:        "ShiftTrafficCanary",
						TargetType:         "ec2",
						Strategy:           "canary",
						ExecutionMode:      "asgWeightedTargetGroups",
						DeployedArtifact:   domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7"},
						Conditions:         []domain.Condition{{Type: "Ready", Status: true, Reason: "TrafficShiftComplete", Message: "candidate is serving traffic"}},
						HistoryCount:       2,
						Changes:            []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep", "deployedArtifact", "conditions"},
						ChangeDetails:      []string{"Sync changed to Reconciling: Traffic shift running", "Health changed to Healthy: steady", "Desired generation changed from 7 to 8", "Observed generation changed from 6 to 7", "Current step changed from CreateCandidateCapacity to ShiftTrafficCanary", "Artifact changed to s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7", "Conditions changed to Ready=True (TrafficShiftComplete)"},
					},
				}},
			}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview accounts payload: %v", err)
	}
	runtimeSummary, ok := payload[0]["deployments"].([]any)[0].(map[string]any)["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in overview accounts payload, got %#v", payload)
	}
	if runtimeSummary["summary"] != "Traffic shift is progressing" || runtimeSummary["timestamp"] != "2026-04-16T06:49:00Z" || runtimeSummary["source"] != "controller" || runtimeSummary["desiredGeneration"] != float64(8) || runtimeSummary["observedGeneration"] != float64(7) || runtimeSummary["currentStep"] != "ShiftTrafficCanary" || runtimeSummary["targetType"] != "ec2" || runtimeSummary["strategy"] != "canary" || runtimeSummary["executionMode"] != "asgWeightedTargetGroups" || runtimeSummary["historyCount"] != float64(2) || runtimeSummary["rolloutPhase"] != string(domain.RolloutPhaseAccepted) {
		t.Fatalf("expected runtimeSummary fields in overview accounts payload, got %#v", runtimeSummary)
	}
	changes, ok := runtimeSummary["changes"].([]any)
	if !ok || len(changes) != 9 || changes[1] != "sync" || changes[5] != "rolloutPhase" || changes[7] != "deployedArtifact" || changes[8] != "conditions" {
		t.Fatalf("expected runtimeSummary.changes in overview accounts payload, got %#v", runtimeSummary["changes"])
	}
	changeDetails, ok := runtimeSummary["changeDetails"].([]any)
	if !ok || len(changeDetails) != 7 || changeDetails[0] != "Sync changed to Reconciling: Traffic shift running" || changeDetails[5] != "Artifact changed to s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7" || changeDetails[6] != "Conditions changed to Ready=True (TrafficShiftComplete)" {
		t.Fatalf("expected runtimeSummary.changeDetails in overview accounts payload, got %#v", runtimeSummary["changeDetails"])
	}
	syncSummary, ok := runtimeSummary["sync"].(map[string]any)
	if !ok || syncSummary["state"] != string(domain.SyncStateReconciling) || syncSummary["reason"] != "Traffic shift running" {
		t.Fatalf("expected runtimeSummary.sync fields in overview accounts payload, got %#v", runtimeSummary["sync"])
	}
	healthSummary, ok := runtimeSummary["health"].(map[string]any)
	if !ok || healthSummary["overall"] != string(domain.HealthHealthy) || healthSummary["summary"] != "steady" {
		t.Fatalf("expected runtimeSummary.health fields in overview accounts payload, got %#v", runtimeSummary["health"])
	}
	artifact, ok := runtimeSummary["deployedArtifact"].(map[string]any)
	if !ok || artifact["binary"] != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7" {
		t.Fatalf("expected runtimeSummary.deployedArtifact fields in overview accounts payload, got %#v", runtimeSummary["deployedArtifact"])
	}
	conditions, ok := runtimeSummary["conditions"].([]any)
	if !ok || len(conditions) != 1 {
		t.Fatalf("expected runtimeSummary.conditions array in overview accounts payload, got %#v", runtimeSummary["conditions"])
	}
	condition, ok := conditions[0].(map[string]any)
	if !ok || condition["type"] != "Ready" || condition["status"] != true || condition["reason"] != "TrafficShiftComplete" {
		t.Fatalf("expected runtimeSummary.conditions fields in overview accounts payload, got %#v", conditions)
	}
}

func TestOverviewAccountsResponseOmitsRuntimeSummarySourceWithoutHistory(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewAccounts: func() ([]catalog.AccountOverview, error) {
			return []catalog.AccountOverview{{
				Name:         "prod-core",
				AWSAccountID: "123456789012",
				Deployments: []catalog.DeploymentOverview{{
					Name: "api-prod",
					RuntimeSummary: &domain.RuntimeSummary{
						Summary:            "Serving stable capacity",
						Sync:               domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
						Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
						DesiredGeneration:  8,
						ObservedGeneration: 8,
						RolloutPhase:       domain.RolloutPhaseCompleted,
						CurrentStep:        "Complete",
						HistoryCount:       0,
					},
				}},
			}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview accounts payload: %v", err)
	}
	runtimeSummary, ok := payload[0]["deployments"].([]any)[0].(map[string]any)["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in overview accounts payload, got %#v", payload)
	}
	if _, ok := runtimeSummary["source"]; ok {
		t.Fatalf("expected runtimeSummary.source to be omitted without history, got %#v", runtimeSummary)
	}
}

func TestRouterOverviewApplications(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewApplications: func() ([]catalog.ApplicationOverview, error) {
			return []catalog.ApplicationOverview{{
				Name:               "payments-api-prod",
				AppRef:             "payments-api",
				EnvironmentRef:     "prod-apne2",
				AccountName:        "prod-core",
				AWSAccountID:       "123456789012",
				Region:             "ap-northeast-2",
				TargetType:         "ec2",
				Strategy:           "canary",
				ExecutionMode:      "asgWeightedTargetGroups",
				DesiredArtifact:    domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7"},
				DeployedArtifact:   domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6"},
				ObservedGeneration: 7,
				Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"},
				Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"},
				RuntimeSummary: &domain.RuntimeSummary{
					Timestamp:          "2026-04-16T06:20:00Z",
					Source:             "controller",
					Summary:            "Shifting canary traffic",
					ObservedGeneration: 7,
					CurrentStep:        "ShiftTrafficCanary",
					TargetType:         "ec2",
					Strategy:           "canary",
					ExecutionMode:      "asgWeightedTargetGroups",
				},
				Rollout: &domain.RolloutSummary{
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					Phase:              domain.RolloutPhasePromoting,
					CurrentStep:        "ShiftTrafficCanary",
				},
				Conditions: []domain.Condition{{Type: "Progressing", Status: true, Reason: "RolloutInFlight", Message: "candidate receiving traffic"}},
			}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var applications []catalog.ApplicationOverview
	if err := json.Unmarshal(resp.Body.Bytes(), &applications); err != nil {
		t.Fatalf("failed to decode overview applications: %v", err)
	}
	if len(applications) != 1 || applications[0].Name != "payments-api-prod" || applications[0].ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("unexpected overview applications: %+v", applications)
	}
	if applications[0].RuntimeSummary == nil || applications[0].Rollout == nil || len(applications[0].Conditions) != 1 {
		t.Fatalf("expected joined status metadata in overview applications response, got %+v", applications[0])
	}
	if applications[0].RuntimeSummary.TargetType != "ec2" || applications[0].RuntimeSummary.Strategy != "canary" || applications[0].RuntimeSummary.ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("expected runtimeSummary rollout-context fields in typed overview applications response, got %+v", applications[0].RuntimeSummary)
	}

	var rawPayload []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &rawPayload); err != nil {
		t.Fatalf("failed to decode raw overview applications payload: %v", err)
	}
	runtimeSummary, ok := rawPayload[0]["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in raw overview applications payload, got %#v", rawPayload)
	}
	if runtimeSummary["targetType"] != "ec2" || runtimeSummary["strategy"] != "canary" || runtimeSummary["executionMode"] != "asgWeightedTargetGroups" {
		t.Fatalf("expected runtimeSummary rollout-context fields in raw overview applications payload, got %#v", runtimeSummary)
	}
}

func TestRouterOverviewApplicationsUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterOverviewApplicationsPropagatesErrors(t *testing.T) {
	const upstreamErr = "boom: overview applications provider details"
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewApplications: func() ([]catalog.ApplicationOverview, error) {
			return nil, errors.New(upstreamErr)
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertInternalServerErrorResponse(t, resp, upstreamErr)
}

func TestRouterOverviewApplication(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewApplication: func(name string) (catalog.ApplicationOverviewDetail, error) {
				if name != "payments-api-prod" {
					t.Fatalf("unexpected deployment name: %s", name)
				}
				return catalog.ApplicationOverviewDetail{
					Name:               "payments-api-prod",
					AppRef:             "payments-api",
					EnvironmentRef:     "prod-apne2",
					AccountName:        "prod-core",
					AWSAccountID:       "123456789012",
					AllowedRegions:     []string{"ap-northeast-2", "us-west-2"},
					Region:             "ap-northeast-2",
					TargetType:         "ec2",
					Strategy:           "canary",
					ExecutionMode:      "asgWeightedTargetGroups",
					DesiredArtifact:    domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7"},
					DeployedArtifact:   domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6"},
					ObservedGeneration: 7,
					SourceFingerprint:  "plan_1234abcd5678ef",
					Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"},
					Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"},
					RuntimeSummary:     &domain.RuntimeSummary{Timestamp: "2026-04-16T06:20:00Z", Source: "controller", Summary: "Shifting canary traffic", ObservedGeneration: 7, CurrentStep: "ShiftTrafficCanary", TargetType: "ec2", Strategy: "canary", ExecutionMode: "asgWeightedTargetGroups"},
					Rollout:            &domain.RolloutSummary{DesiredGeneration: 8, ObservedGeneration: 7, Phase: domain.RolloutPhasePromoting, CurrentStep: "ShiftTrafficCanary"},
					Conditions:         []domain.Condition{{Type: "Progressing", Status: true, Reason: "RolloutInFlight", Message: "candidate receiving traffic"}},
					History:            []domain.StatusHistoryEntry{{Timestamp: "2026-04-16T06:15:00Z", Source: "controller", Summary: "Created candidate capacity"}},
				}, nil
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications/payments-api-prod", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.Code)
		}

		var detail catalog.ApplicationOverviewDetail
		if err := json.Unmarshal(resp.Body.Bytes(), &detail); err != nil {
			t.Fatalf("failed to decode overview application detail: %v", err)
		}
		if detail.Name != "payments-api-prod" || detail.ExecutionMode != "asgWeightedTargetGroups" {
			t.Fatalf("unexpected application overview detail: %+v", detail)
		}
		if detail.SourceFingerprint != "plan_1234abcd5678ef" {
			t.Fatalf("expected source fingerprint in overview application detail response, got %+v", detail)
		}
		if detail.RuntimeSummary == nil || detail.Rollout == nil || len(detail.Conditions) != 1 || len(detail.History) != 1 {
			t.Fatalf("expected joined status metadata in overview application detail response, got %+v", detail)
		}
		var payload map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode raw overview application detail payload: %v", err)
		}
		if payload["sourceFingerprint"] != "plan_1234abcd5678ef" {
			t.Fatalf("expected sourceFingerprint field in raw overview application detail payload, got %#v", payload)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications/payments-api-prod", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewApplication: func(string) (catalog.ApplicationOverviewDetail, error) {
				return catalog.ApplicationOverviewDetail{}, statusstore.ErrNotFound
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications/missing", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})

	t.Run("propagates unexpected errors", func(t *testing.T) {
		const upstreamErr = "boom: overview application detail"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewApplication: func(string) (catalog.ApplicationOverviewDetail, error) {
				return catalog.ApplicationOverviewDetail{}, errors.New(upstreamErr)
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications/payments-api-prod", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, upstreamErr)
	})

	t.Run("rejects invalid names", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewApplication: func(string) (catalog.ApplicationOverviewDetail, error) {
				return catalog.ApplicationOverviewDetail{}, nil
			},
		})
		for _, path := range []string{"/v1/overview/applications/", "/v1/overview/applications/payments-api-prod/extra"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
			}
		}
	})
}

func TestRouterOverviewAccountsUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterOverviewAccountsPropagatesErrors(t *testing.T) {
	const upstreamErr = "boom: overview accounts provider details"
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewAccounts: func() ([]catalog.AccountOverview, error) {
			return nil, errors.New(upstreamErr)
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertInternalServerErrorResponse(t, resp, upstreamErr)
}

func TestRouterOverviewAccountDetail(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewAccount: func(awsAccountID string) (catalog.AccountOverviewDetail, error) {
			if awsAccountID != "123456789012" {
				t.Fatalf("unexpected lookup aws account id: %s", awsAccountID)
			}
			return catalog.AccountOverviewDetail{
				Name:             "prod-core",
				AWSAccountID:     "123456789012",
				AllowedRegions:   []string{"ap-northeast-2", "us-west-2"},
				DeploymentCount:  1,
				ProjectCount:     1,
				EnvironmentCount: 1,
				SyncedCount:      1,
				Deployments: []catalog.DeploymentOverview{{
					Name:           "api-prod",
					EnvironmentRef: "prod-apne2",
					Region:         "ap-northeast-2",
					Strategy:       "canary",
					ExecutionMode:  "asgWeightedTargetGroups",
					Sync:           domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
					Health:         domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
				}},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/123456789012", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var detail catalog.AccountOverviewDetail
	if err := json.Unmarshal(resp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode overview account detail: %v", err)
	}
	if detail.Name != "prod-core" || detail.AWSAccountID != "123456789012" || len(detail.Deployments) != 1 {
		t.Fatalf("unexpected overview account detail: %+v", detail)
	}
	if detail.ProjectCount != 1 || detail.EnvironmentCount != 1 {
		t.Fatalf("expected topology count fields in overview account detail: %+v", detail)
	}
	if detail.Deployments[0].Name != "api-prod" || detail.Deployments[0].ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("unexpected overview account deployment detail: %+v", detail.Deployments[0])
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview account detail payload: %v", err)
	}
	if payload["projectCount"] != float64(1) || payload["environmentCount"] != float64(1) {
		t.Fatalf("expected topology count fields in raw overview account detail payload, got %#v", payload)
	}
}

func TestOverviewAccountDetailRoute(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/123456789012", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewAccount: func(string) (catalog.AccountOverviewDetail, error) {
				return catalog.AccountOverviewDetail{}, statusstore.ErrNotFound
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/missing", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != statusstore.ErrNotFound.Error()+"\n" {
			t.Fatalf("expected not-found body %q, got %q", statusstore.ErrNotFound.Error()+"\n", body)
		}
	})

	t.Run("provider error", func(t *testing.T) {
		const upstreamErr = "boom: overview account detail provider details"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewAccount: func(string) (catalog.AccountOverviewDetail, error) {
				return catalog.AccountOverviewDetail{}, errors.New(upstreamErr)
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/123456789012", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, upstreamErr)
	})

	t.Run("rejects invalid names", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewAccount: func(string) (catalog.AccountOverviewDetail, error) {
				return catalog.AccountOverviewDetail{}, nil
			},
		})
		for _, path := range []string{"/v1/overview/accounts/", "/v1/overview/accounts/a/b"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
			}
		}
	})
}

func TestRouterProjectEnvironmentOverview(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewProjectEnvironment: func(accountID, appRef, environmentRef string) (catalog.ProjectEnvironmentOverviewDetail, error) {
			if accountID != "123456789012" || appRef != "payments-api" || environmentRef != "prod" {
				t.Fatalf("unexpected grouped lookup: %s %s %s", accountID, appRef, environmentRef)
			}
			return catalog.ProjectEnvironmentOverviewDetail{
				AccountName:      "prod-core",
				AWSAccountID:     "123456789012",
				AllowedRegions:   []string{"ap-northeast-2", "us-west-2"},
				AppRef:           "payments-api",
				EnvironmentRef:   "prod",
				Region:           "ap-northeast-2",
				DeploymentCount:  2,
				SyncedCount:      1,
				ProgressingCount: 1,
				PausedCount:      0,
				ErrorCount:       0,
				Deployments:      []catalog.DeploymentOverview{{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "ap-northeast-2", ExecutionMode: "asgWeightedTargetGroups", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "warming"}}, {Name: "payments-worker-prod", AppRef: "payments-api", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "ap-northeast-2", ExecutionMode: "asgWeightedTargetGroups", Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "steady"}}},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/123456789012/projects/payments-api/environments/prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var detail catalog.ProjectEnvironmentOverviewDetail
	if err := json.Unmarshal(resp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode grouped overview detail: %v", err)
	}
	if detail.AccountName != "prod-core" || detail.AppRef != "payments-api" || detail.EnvironmentRef != "prod" || detail.Region != "ap-northeast-2" || len(detail.Deployments) != 2 {
		t.Fatalf("unexpected grouped overview detail: %+v", detail)
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw grouped overview payload: %v", err)
	}
	if payload["accountName"] != "prod-core" || payload["appRef"] != "payments-api" || payload["environmentRef"] != "prod" || payload["deploymentCount"] != float64(2) {
		t.Fatalf("expected grouped overview JSON fields, got %#v", payload)
	}
}

func TestOverviewProjectEnvironmentRoute(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/123456789012/projects/payments-api/environments/prod", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}, OverviewProjectEnvironment: func(string, string, string) (catalog.ProjectEnvironmentOverviewDetail, error) {
			return catalog.ProjectEnvironmentOverviewDetail{}, statusstore.ErrNotFound
		}})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/123456789012/projects/missing/environments/prod", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})

	t.Run("invalid paths", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}, OverviewProjectEnvironment: func(string, string, string) (catalog.ProjectEnvironmentOverviewDetail, error) {
			return catalog.ProjectEnvironmentOverviewDetail{}, nil
		}})
		for _, path := range []string{"/v1/overview/accounts/", "/v1/overview/accounts/123456789012/projects", "/v1/overview/accounts/123456789012/projects/payments-api", "/v1/overview/accounts/123456789012/projects/payments-api/environments", "/v1/overview/accounts/123456789012/projects/payments-api/environments/prod/extra"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
			}
		}
	})
}

func TestRouterOverviewDeployment(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			if name != "api-prod" {
				return catalog.DeploymentOverviewDetail{}, statusstore.ErrNotFound
			}
			return catalog.DeploymentOverviewDetail{
				Name:               "api-prod",
				AppRef:             "payments-api",
				EnvironmentRef:     "prod-apne2",
				AccountName:        "prod-core",
				AWSAccountID:       "123456789012",
				AllowedRegions:     []string{"ap-northeast-2"},
				Region:             "ap-northeast-2",
				TargetType:         "ecs-service",
				Strategy:           "linear",
				ExecutionMode:      "ecsCodeDeploy",
				DesiredArtifact:    domain.DeploymentArtifact{Image: "payments-api:v2"},
				UserData:           domain.DeploymentUserData{Type: "file", Path: "userdata/payments-api-prod.sh"},
				RolloutConfig:      domain.DeploymentRolloutConfig{Strategy: domain.DeploymentRolloutStrategy{MinHealthyPercent: 90, WarmupSeconds: 30}},
				DeployedArtifact:   domain.DeploymentArtifact{Image: "payments-api:v1"},
				ObservedGeneration: 7,
				SourceFingerprint:  "plan_feedcafe1234ab",
				Sync:               domain.Sync{State: domain.SyncStateOutOfSync, Reason: "PendingPromotion"},
				Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "warming up"},
				RuntimeSummary:     &domain.RuntimeSummary{Timestamp: "2026-04-16T06:49:00Z", Source: "controller", Summary: "Traffic shift is progressing", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "Traffic shift running"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "steady"}, DesiredGeneration: 8, ObservedGeneration: 7, RolloutPhase: domain.RolloutPhaseAccepted, CurrentStep: "ShiftTrafficLinear", TargetType: "ecs-service", Strategy: "linear", ExecutionMode: "ecsCodeDeploy", HistoryCount: 3, Changes: []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"}},
				Rollout: &domain.RolloutSummary{
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					Phase:              domain.RolloutPhaseStabilizing,
					CurrentStep:        "VerifyServiceStability",
					Summary:            "Stabilizing ECS service after CodeDeploy traffic shift",
					Steps:              []domain.RolloutStepSummary{{Kind: "CreateCodeDeployDeployment", Status: "completed"}, {Kind: "ShiftTrafficLinear", Status: "pending"}, {Kind: "VerifyServiceStability", Current: true, Status: "current"}},
					Execution:          []domain.RolloutExecutionDetail{{Resource: "ecsService", Name: "payments-api-prod-service", Description: "ECS service Tenet updates during deployment"}, {Resource: "codeDeployApplication", Name: "payments-api-prod-codedeploy-app", Description: "CodeDeploy application coordinating ECS traffic shifting"}, {Resource: "codeDeployDeploymentGroup", Name: "payments-api-prod-codedeploy-group", Description: "CodeDeploy deployment group Tenet targets for ECS rollout"}, {Resource: "productionListener", Name: "payments-api-prod-prod-listener", Description: "Production listener CodeDeploy shifts during ECS blue/green rollout"}, {Resource: "testListener", Name: "payments-api-prod-test-listener", Description: "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion"}, {Resource: "blueTaskSetTargetGroup", Name: "payments-api-prod-blue", Description: "Blue task set target group CodeDeploy treats as the live ECS traffic destination"}, {Resource: "greenTaskSetTargetGroup", Name: "payments-api-prod-green", Description: "Green task set target group CodeDeploy warms before shifting ECS traffic"}},
				},
				Conditions: []domain.Condition{{Type: "Ready", Status: false, Reason: "Updating", Message: "service is progressing"}},
				History: []domain.StatusHistoryEntry{{
					Timestamp:          "2026-04-16T06:49:00Z",
					Source:             "controller",
					Summary:            "Shifted 10 percent of traffic",
					Sync:               domain.Sync{State: domain.SyncStateReconciling},
					Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					Changes:            []string{"summary", "currentStep"},
					RolloutPhase:       domain.RolloutPhaseAccepted,
					CurrentStep:        "ShiftTrafficLinear",
					TargetType:         "ecs-service",
					Strategy:           "linear",
					ExecutionMode:      "ecsCodeDeploy",
				}},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var detail catalog.DeploymentOverviewDetail
	if err := json.Unmarshal(resp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode overview deployment: %v", err)
	}
	if detail.Name != "api-prod" || detail.ExecutionMode != "ecsCodeDeploy" || detail.AccountName != "prod-core" {
		t.Fatalf("unexpected overview deployment detail: %+v", detail)
	}
	if detail.SourceFingerprint != "plan_feedcafe1234ab" {
		t.Fatalf("expected source fingerprint in deployment detail, got %+v", detail)
	}
	if detail.UserData.Type != "file" || detail.UserData.Path != "userdata/payments-api-prod.sh" {
		t.Fatalf("expected userdata metadata in deployment detail, got %+v", detail.UserData)
	}
	if detail.RolloutConfig.Strategy.MinHealthyPercent != 90 || detail.RolloutConfig.Strategy.WarmupSeconds != 30 {
		t.Fatalf("expected rollout config metadata in deployment detail, got %+v", detail.RolloutConfig)
	}
	if detail.Rollout == nil || detail.Rollout.CurrentStep != "VerifyServiceStability" || len(detail.Rollout.Steps) != 3 {
		t.Fatalf("expected rollout summary in deployment detail, got %+v", detail)
	}
	if detail.RuntimeSummary == nil || detail.RuntimeSummary.Timestamp != "2026-04-16T06:49:00Z" || detail.RuntimeSummary.Source != "controller" || detail.RuntimeSummary.Summary != "Traffic shift is progressing" || detail.RuntimeSummary.Sync.Reason != "Traffic shift running" || detail.RuntimeSummary.DesiredGeneration != 8 || detail.RuntimeSummary.ObservedGeneration != 7 || detail.RuntimeSummary.CurrentStep != "ShiftTrafficLinear" || detail.RuntimeSummary.TargetType != "ecs-service" || detail.RuntimeSummary.Strategy != "linear" || detail.RuntimeSummary.ExecutionMode != "ecsCodeDeploy" || detail.RuntimeSummary.HistoryCount != 3 || !reflect.DeepEqual(detail.RuntimeSummary.Changes, []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"}) {
		t.Fatalf("expected runtime summary in deployment detail, got %+v", detail)
	}
	if len(detail.History) != 1 || detail.History[0].Summary != "Shifted 10 percent of traffic" || detail.History[0].DesiredGeneration != 8 || detail.History[0].ObservedGeneration != 7 || detail.History[0].CurrentStep != "ShiftTrafficLinear" || detail.History[0].TargetType != "ecs-service" || detail.History[0].Strategy != "linear" || detail.History[0].ExecutionMode != "ecsCodeDeploy" {
		t.Fatalf("expected runtime history in deployment detail, got %+v", detail.History)
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview deployment payload: %v", err)
	}
	rollout, ok := payload["rollout"].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout object in raw payload, got %#v", payload["rollout"])
	}
	if rollout["currentStep"] != "VerifyServiceStability" {
		t.Fatalf("expected rollout.currentStep field, got %#v", rollout)
	}
	if payload["sourceFingerprint"] != "plan_feedcafe1234ab" {
		t.Fatalf("expected sourceFingerprint field in raw payload, got %#v", payload)
	}
	runtimeSummary, ok := payload["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in raw payload, got %#v", payload["runtimeSummary"])
	}
	userData, ok := payload["userData"].(map[string]any)
	if !ok || userData["type"] != "file" || userData["path"] != "userdata/payments-api-prod.sh" {
		t.Fatalf("expected userData object in raw payload, got %#v", payload["userData"])
	}
	rolloutConfig, ok := payload["rolloutConfig"].(map[string]any)
	if !ok {
		t.Fatalf("expected rolloutConfig object in raw payload, got %#v", payload["rolloutConfig"])
	}
	rolloutStrategy, ok := rolloutConfig["strategy"].(map[string]any)
	if !ok || rolloutStrategy["minHealthyPercent"] != float64(90) || rolloutStrategy["warmupSeconds"] != float64(30) {
		t.Fatalf("expected rolloutConfig.strategy object in raw payload, got %#v", payload["rolloutConfig"])
	}
	if runtimeSummary["timestamp"] != "2026-04-16T06:49:00Z" || runtimeSummary["source"] != "controller" || runtimeSummary["summary"] != "Traffic shift is progressing" || runtimeSummary["desiredGeneration"] != float64(8) || runtimeSummary["observedGeneration"] != float64(7) || runtimeSummary["currentStep"] != "ShiftTrafficLinear" || runtimeSummary["targetType"] != "ecs-service" || runtimeSummary["strategy"] != "linear" || runtimeSummary["executionMode"] != "ecsCodeDeploy" || runtimeSummary["historyCount"] != float64(3) || runtimeSummary["rolloutPhase"] != string(domain.RolloutPhaseAccepted) {
		t.Fatalf("expected runtimeSummary JSON fields in raw payload, got %#v", runtimeSummary)
	}
	changes, ok := runtimeSummary["changes"].([]any)
	if !ok || len(changes) != 7 || changes[2] != "health" || changes[4] != "observedGeneration" {
		t.Fatalf("expected runtimeSummary.changes field in raw payload, got %#v", runtimeSummary["changes"])
	}
	syncSummary, ok := runtimeSummary["sync"].(map[string]any)
	if !ok || syncSummary["state"] != string(domain.SyncStateReconciling) || syncSummary["reason"] != "Traffic shift running" {
		t.Fatalf("expected runtimeSummary.sync fields in raw payload, got %#v", runtimeSummary["sync"])
	}
	healthSummary, ok := runtimeSummary["health"].(map[string]any)
	if !ok || healthSummary["overall"] != string(domain.HealthHealthy) || healthSummary["summary"] != "steady" {
		t.Fatalf("expected runtimeSummary.health fields in raw payload, got %#v", runtimeSummary["health"])
	}
	steps, ok := rollout["steps"].([]any)
	if !ok || len(steps) != 3 {
		t.Fatalf("expected rollout.steps array in raw payload, got %#v", rollout["steps"])
	}
	lastStep, ok := steps[2].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[2] object, got %#v", steps[2])
	}
	if lastStep["kind"] != "VerifyServiceStability" || lastStep["current"] != true || lastStep["status"] != "current" {
		t.Fatalf("expected rollout.steps[2] field names, got %#v", lastStep)
	}
	firstStep, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[0] object, got %#v", steps[0])
	}
	if firstStep["kind"] != "CreateCodeDeployDeployment" || firstStep["status"] != "completed" {
		t.Fatalf("expected rollout.steps[0] status field names, got %#v", firstStep)
	}
	execution, ok := rollout["execution"].([]any)
	if !ok || len(execution) != 7 {
		t.Fatalf("expected rollout.execution array in raw payload, got %#v", rollout["execution"])
	}
	firstExecution, ok := execution[0].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.execution[0] object, got %#v", execution[0])
	}
	if firstExecution["resource"] != "ecsService" || firstExecution["name"] != "payments-api-prod-service" || firstExecution["description"] != "ECS service Tenet updates during deployment" {
		t.Fatalf("expected rollout.execution[0] field names, got %#v", firstExecution)
	}
	lastExecution, ok := execution[6].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.execution[6] object, got %#v", execution[6])
	}
	if lastExecution["resource"] != "greenTaskSetTargetGroup" || lastExecution["name"] != "payments-api-prod-green" || lastExecution["description"] != "Green task set target group CodeDeploy warms before shifting ECS traffic" {
		t.Fatalf("expected rollout.execution[6] field names, got %#v", lastExecution)
	}
	history, ok := payload["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("expected history array in raw payload, got %#v", payload["history"])
	}
	historyEntry, ok := history[0].(map[string]any)
	if !ok {
		t.Fatalf("expected history[0] object, got %#v", history[0])
	}
	sync, ok := historyEntry["sync"].(map[string]any)
	if !ok {
		t.Fatalf("expected history[0].sync object, got %#v", historyEntry["sync"])
	}
	if sync["state"] != string(domain.SyncStateReconciling) {
		t.Fatalf("expected history[0].sync.state field, got %#v", sync)
	}
	historyChanges, ok := historyEntry["changes"].([]any)
	if !ok || len(historyChanges) != 2 || historyChanges[0] != "summary" || historyChanges[1] != "currentStep" {
		t.Fatalf("expected history[0].changes string array in raw payload, got %#v", historyEntry["changes"])
	}
	if historyEntry["currentStep"] != "ShiftTrafficLinear" || historyEntry["desiredGeneration"] != float64(8) || historyEntry["observedGeneration"] != float64(7) || historyEntry["rolloutPhase"] != string(domain.RolloutPhaseAccepted) || historyEntry["targetType"] != "ecs-service" || historyEntry["strategy"] != "linear" || historyEntry["executionMode"] != "ecsCodeDeploy" {
		t.Fatalf("expected history[0] field names, got %#v", historyEntry)
	}
}

func TestDeploymentOverviewDetailRouteOmitsRuntimeSummarySourceWithoutHistory(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected lookup name: %s", name)
			}
			return catalog.DeploymentOverviewDetail{
				Name: "api-prod",
				RuntimeSummary: &domain.RuntimeSummary{
					Summary:            "Serving stable capacity",
					Sync:               domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
					Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
					DesiredGeneration:  8,
					ObservedGeneration: 8,
					RolloutPhase:       domain.RolloutPhaseCompleted,
					CurrentStep:        "Complete",
					HistoryCount:       0,
				},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview deployment payload: %v", err)
	}
	runtimeSummary, ok := payload["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in raw payload, got %#v", payload["runtimeSummary"])
	}
	if _, ok := runtimeSummary["source"]; ok {
		t.Fatalf("expected runtimeSummary.source to be omitted without history, got %#v", runtimeSummary)
	}
}

func TestRouterOverviewDeploymentChangeDetails(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected lookup name: %s", name)
			}
			return catalog.DeploymentOverviewDetail{
				Name: "api-prod",
				RuntimeSummary: &domain.RuntimeSummary{
					Timestamp:          "2026-04-16T06:49:00Z",
					Source:             "controller",
					Summary:            "Traffic shift is progressing",
					Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "Traffic shift running"},
					Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					RolloutPhase:       domain.RolloutPhaseAccepted,
					CurrentStep:        "ShiftTrafficLinear",
					TargetType:         "ecs-service",
					Strategy:           "linear",
					ExecutionMode:      "ecsCodeDeploy",
					HistoryCount:       3,
					Changes:            []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"},
					ChangeDetails:      []string{"Sync changed to Reconciling: Traffic shift running", "Health changed to Healthy: steady", "Desired generation changed from 7 to 8", "Observed generation changed from 6 to 7", "Rollout phase changed from Provisioning to Accepted", "Current step changed from CreateCodeDeployDeployment to ShiftTrafficLinear"},
				},
				History: []domain.StatusHistoryEntry{{
					Timestamp:          "2026-04-16T06:49:00Z",
					Source:             "controller",
					Summary:            "Shifted 10 percent of traffic",
					Sync:               domain.Sync{State: domain.SyncStateReconciling},
					Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					Changes:            []string{"summary", "currentStep"},
					ChangeDetails:      []string{"Summary changed to Shifted 10 percent of traffic", "Current step changed from CreateCodeDeployDeployment to ShiftTrafficLinear"},
					RolloutPhase:       domain.RolloutPhaseAccepted,
					CurrentStep:        "ShiftTrafficLinear",
					TargetType:         "ecs-service",
					Strategy:           "linear",
					ExecutionMode:      "ecsCodeDeploy",
				}},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview deployment payload: %v", err)
	}
	runtimeSummary, ok := payload["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in raw payload, got %#v", payload["runtimeSummary"])
	}
	changeDetails, ok := runtimeSummary["changeDetails"].([]any)
	if !ok || len(changeDetails) != 6 || changeDetails[0] != "Sync changed to Reconciling: Traffic shift running" || changeDetails[5] != "Current step changed from CreateCodeDeployDeployment to ShiftTrafficLinear" {
		t.Fatalf("expected runtimeSummary.changeDetails in raw payload, got %#v", runtimeSummary["changeDetails"])
	}
	history, ok := payload["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("expected history array in raw payload, got %#v", payload["history"])
	}
	historyEntry, ok := history[0].(map[string]any)
	if !ok {
		t.Fatalf("expected history[0] object, got %#v", history[0])
	}
	historyChangeDetails, ok := historyEntry["changeDetails"].([]any)
	if !ok || len(historyChangeDetails) != 2 || historyChangeDetails[0] != "Summary changed to Shifted 10 percent of traffic" || historyChangeDetails[1] != "Current step changed from CreateCodeDeployDeployment to ShiftTrafficLinear" {
		t.Fatalf("expected history[0].changeDetails in raw payload, got %#v", historyEntry["changeDetails"])
	}
}

func TestDeploymentOverviewDetailRouteIncludesRolloutStepDetail(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected lookup name: %s", name)
			}
			return catalog.DeploymentOverviewDetail{
				Name: "api-prod",
				Rollout: &domain.RolloutSummary{
					Steps: []domain.RolloutStepSummary{
						{Kind: "CreateCodeDeployDeployment", Detail: "Creates CodeDeploy deployment for service <api>", Status: domain.RolloutStepStatusCompleted},
						{Kind: "ShiftTrafficLinear", Detail: "Shifts 10% of production traffic", Current: true, Status: domain.RolloutStepStatusCurrent},
					},
				},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var detail catalog.DeploymentOverviewDetail
	if err := json.Unmarshal(resp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode overview deployment: %v", err)
	}
	if detail.Rollout == nil || len(detail.Rollout.Steps) != 2 {
		t.Fatalf("expected rollout steps in deployment detail, got %+v", detail)
	}
	if detail.Rollout.Steps[0].Detail != "Creates CodeDeploy deployment for service <api>" {
		t.Fatalf("expected rollout step detail to survive typed decode, got %+v", detail.Rollout.Steps)
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview deployment payload: %v", err)
	}
	rollout, ok := payload["rollout"].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout object in raw payload, got %#v", payload["rollout"])
	}
	steps, ok := rollout["steps"].([]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("expected rollout.steps array in raw payload, got %#v", rollout["steps"])
	}
	firstStep, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[0] object, got %#v", steps[0])
	}
	if firstStep["detail"] != "Creates CodeDeploy deployment for service <api>" {
		t.Fatalf("expected rollout.steps[0].detail field, got %#v", firstStep)
	}
	secondStep, ok := steps[1].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[1] object, got %#v", steps[1])
	}
	if secondStep["detail"] != "Shifts 10% of production traffic" || secondStep["current"] != true || secondStep["status"] != string(domain.RolloutStepStatusCurrent) {
		t.Fatalf("expected rollout.steps[1] detail/current/status fields, got %#v", secondStep)
	}
	if _, exists := firstStep["Detail"]; exists {
		t.Fatalf("unexpected Go-style detail field name in %#v", firstStep)
	}
}

func TestOverviewDeploymentResponseIncludesRuntimeSummary(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{
				Name: "api-prod",
				ManagedDrift: []domain.ManagedDriftItem{
					{Field: "artifact", Desired: "ghcr.io/devopsartfactory/payments-api:2026-04-16.2", Actual: "ghcr.io/devopsartfactory/payments-api:2026-04-16.1", Severity: domain.ManagedDriftSeverityHigh},
					{Field: "generation", Desired: "8", Actual: "7", Severity: domain.ManagedDriftSeverityMedium},
				},
				RuntimeSummary: &domain.RuntimeSummary{
					Timestamp:          "2026-04-16T06:49:00Z",
					Summary:            "Traffic shift is progressing",
					Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "Traffic shift running"},
					Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					RolloutPhase:       domain.RolloutPhaseAccepted,
					CurrentStep:        "ShiftTrafficLinear",
					TargetType:         "ecs-service",
					Strategy:           "linear",
					ExecutionMode:      "ecsCodeDeploy",
					ManagedDrift: []domain.ManagedDriftItem{
						{Field: "artifact", Desired: "ghcr.io/devopsartfactory/payments-api:2026-04-16.2", Actual: "ghcr.io/devopsartfactory/payments-api:2026-04-16.1", Severity: domain.ManagedDriftSeverityHigh},
						{Field: "generation", Desired: "8", Actual: "7", Severity: domain.ManagedDriftSeverityMedium},
					},
					DeployedArtifact: domain.DeploymentArtifact{Image: "ghcr.io/devopsartfactory/payments-api:2026-04-16.1"},
					Conditions:       []domain.Condition{{Type: "Ready", Status: true, Reason: "Stable", Message: "deployment is healthy"}},
					HistoryCount:     3,
				},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview deployment payload: %v", err)
	}
	runtimeSummary, ok := payload["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in overview deployment payload, got %#v", payload["runtimeSummary"])
	}
	if runtimeSummary["summary"] != "Traffic shift is progressing" || runtimeSummary["timestamp"] != "2026-04-16T06:49:00Z" || runtimeSummary["desiredGeneration"] != float64(8) || runtimeSummary["observedGeneration"] != float64(7) || runtimeSummary["currentStep"] != "ShiftTrafficLinear" || runtimeSummary["targetType"] != "ecs-service" || runtimeSummary["strategy"] != "linear" || runtimeSummary["executionMode"] != "ecsCodeDeploy" || runtimeSummary["historyCount"] != float64(3) || runtimeSummary["rolloutPhase"] != string(domain.RolloutPhaseAccepted) {
		t.Fatalf("expected runtimeSummary fields in overview deployment payload, got %#v", runtimeSummary)
	}
	syncSummary, ok := runtimeSummary["sync"].(map[string]any)
	if !ok || syncSummary["state"] != string(domain.SyncStateReconciling) || syncSummary["reason"] != "Traffic shift running" {
		t.Fatalf("expected runtimeSummary.sync fields in overview deployment payload, got %#v", runtimeSummary["sync"])
	}
	healthSummary, ok := runtimeSummary["health"].(map[string]any)
	if !ok || healthSummary["overall"] != string(domain.HealthHealthy) || healthSummary["summary"] != "steady" {
		t.Fatalf("expected runtimeSummary.health fields in overview deployment payload, got %#v", runtimeSummary["health"])
	}
	artifact, ok := runtimeSummary["deployedArtifact"].(map[string]any)
	if !ok || artifact["image"] != "ghcr.io/devopsartfactory/payments-api:2026-04-16.1" {
		t.Fatalf("expected runtimeSummary.deployedArtifact fields in overview deployment payload, got %#v", runtimeSummary["deployedArtifact"])
	}
	conditions, ok := runtimeSummary["conditions"].([]any)
	if !ok || len(conditions) != 1 {
		t.Fatalf("expected runtimeSummary.conditions array in overview deployment payload, got %#v", runtimeSummary["conditions"])
	}
	condition, ok := conditions[0].(map[string]any)
	if !ok || condition["type"] != "Ready" || condition["status"] != true || condition["reason"] != "Stable" {
		t.Fatalf("expected runtimeSummary.conditions fields in overview deployment payload, got %#v", conditions)
	}
	managedDrift, ok := payload["managedDrift"].([]any)
	if !ok || len(managedDrift) != 2 {
		t.Fatalf("expected managedDrift array in overview deployment payload, got %#v", payload["managedDrift"])
	}
	driftItem, ok := managedDrift[0].(map[string]any)
	if !ok || driftItem["field"] != "artifact" || driftItem["severity"] != string(domain.ManagedDriftSeverityHigh) || driftItem["desired"] != "ghcr.io/devopsartfactory/payments-api:2026-04-16.2" {
		t.Fatalf("expected managedDrift item fields in overview deployment payload, got %#v", managedDrift[0])
	}
	runtimeManagedDrift, ok := runtimeSummary["managedDrift"].([]any)
	if !ok || len(runtimeManagedDrift) != 2 {
		t.Fatalf("expected runtimeSummary.managedDrift array in overview deployment payload, got %#v", runtimeSummary["managedDrift"])
	}
	if _, exists := payload["ManagedDrift"]; exists {
		t.Fatalf("unexpected Go-style managedDrift field name in %#v", payload)
	}
}

func TestRouterOverviewDeploymentUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterOverviewDeploymentNotFound(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{}, statusstore.ErrNotFound
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/missing", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestRouterOverviewDeploymentProviderErrorReturnsInternalServerError(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{}, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterOverviewDeploymentRejectsInvalidNames(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{}, nil
		},
	})
	for _, path := range []string{"/v1/overview/deployments/", "/v1/overview/deployments/a/b"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
		}
	}
}

func TestRouterOverviewDeploymentResourceLogs(t *testing.T) {
	t.Run("returns resource log payload", func(t *testing.T) {
		expected := resourcelogs.Result{
			DeploymentName: "api-prod",
			ResourceType:   "ecsService",
			ResourceName:   "payments-api-prod-service",
			LookupContext: resourcelogs.LookupContext{
				Region:                 "ap-northeast-2",
				ControlAccountID:       "123456789012",
				TargetAccountID:        "123456789012",
				CrossAccount:           false,
				ObserverRoleConfigured: true,
			},
			Available: true,
			Source:    resourcelogs.SourceCloudWatch,
			Entries: []resourcelogs.Entry{
				{Timestamp: "2026-04-16T07:10:00Z", Message: "service became healthy", Stream: "ecs/payments-api/service/abc123"},
				{Timestamp: "2026-04-16T07:09:00Z", Message: "registered 2/2 targets"},
			},
			UnavailableReason: "",
		}

		var gotRequest struct {
			deploymentName string
			resourceType   string
			resourceName   string
			requestID      string
		}
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourcelogs.Result, error) {
				gotRequest.deploymentName = deploymentName
				gotRequest.resourceType = resourceType
				gotRequest.resourceName = resourceName
				gotRequest.requestID = RequestIDFromContext(ctx)
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		responseRequestID := assertResponseRequestID(t, resp)
		if !reflect.DeepEqual(gotRequest, struct {
			deploymentName string
			resourceType   string
			resourceName   string
			requestID      string
		}{deploymentName: "api-prod", resourceType: "ecsService", resourceName: "payments-api-prod-service", requestID: responseRequestID}) {
			t.Fatalf("unexpected provider request: %+v", gotRequest)
		}

		var got resourcelogs.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode resource logs payload: %v", err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("unexpected resource logs payload: %+v", got)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw resource logs payload: %v", err)
		}
		for _, key := range []string{"deploymentName", "resourceType", "resourceName", "lookupContext", "available", "source", "entries", "unavailableReason"} {
			if _, ok := raw[key]; !ok {
				t.Fatalf("expected %q field in payload, got %#v", key, raw)
			}
		}
		lookupContext, ok := raw["lookupContext"].(map[string]any)
		if !ok {
			t.Fatalf("expected lookupContext object in payload, got %#v", raw["lookupContext"])
		}
		for key, want := range map[string]any{
			"region":                 "ap-northeast-2",
			"controlAccountId":       "123456789012",
			"targetAccountId":        "123456789012",
			"crossAccount":           false,
			"observerRoleConfigured": true,
		} {
			if got := lookupContext[key]; !reflect.DeepEqual(got, want) {
				t.Fatalf("expected lookupContext[%q]=%v, got %#v", key, want, got)
			}
		}
		entries, ok := raw["entries"].([]any)
		if !ok || len(entries) != 2 {
			t.Fatalf("expected entries array in payload, got %#v", raw["entries"])
		}
		entry, ok := entries[0].(map[string]any)
		if !ok || entry["timestamp"] != "2026-04-16T07:10:00Z" || entry["message"] != "service became healthy" || entry["stream"] != "ecs/payments-api/service/abc123" {
			t.Fatalf("expected entry shape in payload, got %#v", entries[0])
		}
		entry, ok = entries[1].(map[string]any)
		if !ok {
			t.Fatalf("expected second entry shape in payload, got %#v", entries[1])
		}
		if _, ok := entry["stream"]; ok {
			t.Fatalf("expected optional stream to be omitted when empty, got %#v", entries[1])
		}
	})

	t.Run("honors inbound request-id header", func(t *testing.T) {
		const inboundRequestID = "operator-correlation-123"

		var providerRequestID string
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourcelogs.Result, error) {
				providerRequestID = RequestIDFromContext(ctx)
				return resourcelogs.Result{
					DeploymentName: deploymentName,
					ResourceType:   resourceType,
					ResourceName:   resourceName,
					Available:      false,
					Entries:        []resourcelogs.Entry{},
				}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		req.Header.Set("request-id", inboundRequestID)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if got := resp.Header().Get(RequestIDHeader); got != inboundRequestID {
			t.Fatalf("expected response %s %q, got %q", RequestIDHeader, inboundRequestID, got)
		}
		if providerRequestID != inboundRequestID {
			t.Fatalf("expected provider context request ID %q, got %q", inboundRequestID, providerRequestID)
		}
	})

	for _, tc := range []struct {
		name         string
		query        string
		wantBodyPart string
	}{
		{name: "missing resourceType", query: "resourceName=payments-api-prod-service", wantBodyPart: "resourceType"},
		{name: "missing resourceName", query: "resourceType=ecsService", wantBodyPart: "resourceName"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
					t.Fatalf("overview deployment lookup should not run for invalid query")
					return catalog.DeploymentOverviewDetail{}, nil
				},
				OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourcelogs.Result, error) {
					t.Fatalf("resource logs provider should not run for invalid query")
					return resourcelogs.Result{}, nil
				},
			})

			req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?"+tc.query, nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d with body %q", resp.Code, resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), tc.wantBodyPart) {
				t.Fatalf("expected 400 body to mention %q, got %q", tc.wantBodyPart, resp.Body.String())
			}
		})
	}

	t.Run("returns 503 when provider is unavailable", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d with body %q", resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), "resource logs") {
			t.Fatalf("expected 503 body to mention resource logs, got %q", resp.Body.String())
		}
	})

	t.Run("propagates deployment not found as 404", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "missing" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{}, statusstore.ErrNotFound
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourcelogs.Result, error) {
				t.Fatalf("resource logs provider should not run when deployment lookup fails")
				return resourcelogs.Result{}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/missing/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d with body %q", resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), statusstore.ErrNotFound.Error()) {
			t.Fatalf("expected 404 body to include %q, got %q", statusstore.ErrNotFound.Error(), resp.Body.String())
		}
	})

	t.Run("propagates deployment resource binding failures as 404", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourcelogs.Result, error) {
				if deploymentName != "api-prod" || resourceType != "ecsService" || resourceName != "payments-api-prod-service" {
					t.Fatalf("unexpected resource logs request: deployment=%q type=%q name=%q", deploymentName, resourceType, resourceName)
				}
				return resourcelogs.Result{}, statusstore.ErrNotFound
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d with body %q", resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), statusstore.ErrNotFound.Error()) {
			t.Fatalf("expected 404 body to include %q, got %q", statusstore.ErrNotFound.Error(), resp.Body.String())
		}
	})

	t.Run("returns generic 500 when resource logs provider errors", func(t *testing.T) {
		const providerErr = "boom: resource logs provider internals"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourcelogs.Result, error) {
				if deploymentName != "api-prod" || resourceType != "ecsService" || resourceName != "payments-api-prod-service" {
					t.Fatalf("unexpected resource logs request: deployment=%q type=%q name=%q", deploymentName, resourceType, resourceName)
				}
				return resourcelogs.Result{}, errors.New(providerErr)
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, providerErr)
	})

	t.Run("returns generic 500 when deployment lookup errors on resource logs route", func(t *testing.T) {
		const providerErr = "boom: deployment lookup internals"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{}, errors.New(providerErr)
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourcelogs.Result, error) {
				t.Fatalf("resource logs provider should not run when deployment lookup errors")
				return resourcelogs.Result{}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, providerErr)
	})
}

func TestRouterOverviewDeploymentResourceInspection(t *testing.T) {
	t.Run("returns resource inspection payload", func(t *testing.T) {
		expected := resourceinspection.Result{
			DeploymentName: "api-prod",
			ResourceType:   "ecsService",
			ResourceName:   "payments-api-prod-service",
			LookupContext: resourceinspection.LookupContext{
				Region:                 "ap-northeast-2",
				ControlAccountID:       "123456789012",
				TargetAccountID:        "123456789012",
				CrossAccount:           false,
				ObserverRoleConfigured: true,
			},
			Available: true,
			Source:    resourceinspection.SourceECSService,
			ECSService: &resourceinspection.ECSServiceInspection{
				ClusterARN:     "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments",
				ServiceARN:     "arn:aws:ecs:ap-northeast-2:123456789012:service/payments/payments-api-prod-service",
				ServiceName:    "payments-api-prod-service",
				Status:         "ACTIVE",
				DesiredCount:   2,
				RunningCount:   2,
				PendingCount:   0,
				TaskDefinition: "payments-api:42",
				Deployments: []resourceinspection.ECSDeploymentSummary{{
					ID:             "ecs-svc/123",
					Status:         "PRIMARY",
					RolloutState:   "COMPLETED",
					TaskDefinition: "payments-api:42",
					DesiredCount:   2,
					RunningCount:   2,
					PendingCount:   0,
				}},
				Events: []resourceinspection.ECSServiceEvent{{
					CreatedAt: "2026-04-18T00:12:00Z",
					Message:   "service reached steady state",
				}},
			},
		}

		var gotRequest struct {
			deploymentName string
			resourceType   string
			resourceName   string
			requestID      string
		}
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourceinspection.Result, error) {
				gotRequest.deploymentName = deploymentName
				gotRequest.resourceType = resourceType
				gotRequest.resourceName = resourceName
				gotRequest.requestID = RequestIDFromContext(ctx)
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		responseRequestID := assertResponseRequestID(t, resp)
		if !reflect.DeepEqual(gotRequest, struct {
			deploymentName string
			resourceType   string
			resourceName   string
			requestID      string
		}{deploymentName: "api-prod", resourceType: "ecsService", resourceName: "payments-api-prod-service", requestID: responseRequestID}) {
			t.Fatalf("unexpected provider request: %+v", gotRequest)
		}

		var got resourceinspection.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode resource inspection payload: %v", err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("unexpected resource inspection payload: %+v", got)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw resource inspection payload: %v", err)
		}
		for _, key := range []string{"deploymentName", "resourceType", "resourceName", "lookupContext", "available", "source", "ecsService", "unavailableReason"} {
			if _, ok := raw[key]; !ok {
				t.Fatalf("expected %q field in payload, got %#v", key, raw)
			}
		}
	})

	t.Run("returns traffic routing summary payload", func(t *testing.T) {
		expected := resourceinspection.Result{
			DeploymentName: "api-prod",
			ResourceType:   "trafficShiftPlan",
			ResourceName:   "api-prod-weighted-routing",
			LookupContext: resourceinspection.LookupContext{
				Region:                 "ap-northeast-2",
				ControlAccountID:       "123456789012",
				TargetAccountID:        "210987654321",
				CrossAccount:           true,
				ObserverRoleConfigured: false,
			},
			Available: true,
			Source:    resourceinspection.SourceRolloutSummary,
			TrafficRoutingResource: &resourceinspection.TrafficRoutingInspection{
				Kind:           "trafficShiftPlan",
				DisplayName:    "Traffic shift plan",
				InspectionMode: "summary_only",
				Summary:        "Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s",
			},
		}

		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string) (resourceinspection.Result, error) {
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=trafficShiftPlan&resourceName=api-prod-weighted-routing", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}

		var got resourceinspection.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode traffic routing inspection payload: %v", err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("unexpected traffic routing inspection payload: %+v", got)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw traffic routing inspection payload: %v", err)
		}
		for _, key := range []string{"deploymentName", "resourceType", "resourceName", "lookupContext", "available", "source", "trafficRoutingResource", "unavailableReason"} {
			if _, ok := raw[key]; !ok {
				t.Fatalf("expected %q field in payload, got %#v", key, raw)
			}
		}
		trafficRoutingResource, ok := raw["trafficRoutingResource"].(map[string]any)
		if !ok {
			t.Fatalf("expected trafficRoutingResource object in payload, got %#v", raw["trafficRoutingResource"])
		}
		for key, want := range map[string]any{
			"kind":           "trafficShiftPlan",
			"displayName":    "Traffic shift plan",
			"inspectionMode": "summary_only",
			"summary":        "Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s",
		} {
			if got := trafficRoutingResource[key]; !reflect.DeepEqual(got, want) {
				t.Fatalf("expected trafficRoutingResource[%q]=%v, got %#v", key, want, got)
			}
		}
	})

	t.Run("returns auto scaling inspection payload with launch template metadata", func(t *testing.T) {
		expected := resourceinspection.Result{
			DeploymentName: "api-prod",
			ResourceType:   "candidateAutoScalingGroup",
			ResourceName:   "api-prod-candidate",
			LookupContext: resourceinspection.LookupContext{
				Region:                 "ap-northeast-2",
				ControlAccountID:       "123456789012",
				TargetAccountID:        "123456789012",
				CrossAccount:           false,
				ObserverRoleConfigured: true,
			},
			Available: true,
			Source:    resourceinspection.SourceAutoScalingGroup,
			AutoScalingGroup: &resourceinspection.AutoScalingInspection{
				AutoScalingGroupARN:    "arn:aws:autoscaling:ap-northeast-2:123456789012:autoScalingGroup:*:autoScalingGroupName/api-prod-candidate",
				AutoScalingGroupName:   "api-prod-candidate",
				MinSize:                2,
				MaxSize:                8,
				DesiredCapacity:        4,
				DefaultCooldown:        300,
				HealthCheckType:        "EC2",
				HealthCheckGracePeriod: 120,
				LaunchTemplate: &resourceinspection.AutoScalingLaunchTemplateSummary{
					ID:      "lt-abc123",
					Name:    "api-prod-candidate",
					Version: "$Default",
				},
				MixedInstancesPolicy: &resourceinspection.AutoScalingMixedInstancesPolicy{
					LaunchTemplate: &resourceinspection.AutoScalingLaunchTemplateSummary{
						ID:      "lt-base123",
						Name:    "api-prod-candidate-base",
						Version: "7",
					},
					Overrides: []resourceinspection.AutoScalingLaunchTemplateOverride{{
						InstanceType:     "c7g.large",
						WeightedCapacity: "2",
						LaunchTemplate: &resourceinspection.AutoScalingLaunchTemplateSummary{
							ID:      "lt-arm123",
							Name:    "api-prod-candidate-arm",
							Version: "3",
						},
					}},
				},
				TargetGroupARNs: []string{
					"arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/api-prod-candidate/abc123",
				},
				Instances: []resourceinspection.AutoScalingGroupInstanceSummary{{
					InstanceID:           "i-abc123",
					AvailabilityZone:     "ap-northeast-2a",
					LifecycleState:       "InService",
					HealthStatus:         "Healthy",
					ProtectedFromScaleIn: true,
					WeightedCapacity:     "1",
				}},
			},
		}

		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string) (resourceinspection.Result, error) {
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=api-prod-candidate", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}

		var got resourceinspection.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode auto scaling inspection payload: %v", err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("unexpected auto scaling inspection payload: %+v", got)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw auto scaling inspection payload: %v", err)
		}
		autoScalingGroup, ok := raw["autoScalingGroup"].(map[string]any)
		if !ok {
			t.Fatalf("expected autoScalingGroup object in payload, got %#v", raw["autoScalingGroup"])
		}
		launchTemplate, ok := autoScalingGroup["launchTemplate"].(map[string]any)
		if !ok {
			t.Fatalf("expected launchTemplate object in payload, got %#v", autoScalingGroup["launchTemplate"])
		}
		if got := launchTemplate["name"]; got != "api-prod-candidate" {
			t.Fatalf("expected launchTemplate name, got %#v", got)
		}
		mixedInstancesPolicy, ok := autoScalingGroup["mixedInstancesPolicy"].(map[string]any)
		if !ok {
			t.Fatalf("expected mixedInstancesPolicy object in payload, got %#v", autoScalingGroup["mixedInstancesPolicy"])
		}
		overrides, ok := mixedInstancesPolicy["overrides"].([]any)
		if !ok || len(overrides) != 1 {
			t.Fatalf("expected mixedInstancesPolicy overrides in payload, got %#v", mixedInstancesPolicy["overrides"])
		}
	})

	t.Run("returns live traffic routing payload", func(t *testing.T) {
		expected := resourceinspection.Result{
			DeploymentName: "api-prod",
			ResourceType:   "codeDeployDeploymentGroup",
			ResourceName:   "api-prod-codedeploy-group",
			LookupContext: resourceinspection.LookupContext{
				Region:                 "ap-northeast-2",
				ControlAccountID:       "123456789012",
				TargetAccountID:        "123456789012",
				CrossAccount:           false,
				ObserverRoleConfigured: true,
			},
			Available: true,
			Source:    resourceinspection.SourceTrafficRoutingAWS,
			TrafficRoutingResource: &resourceinspection.TrafficRoutingInspection{
				Kind:           "codeDeployDeploymentGroup",
				DisplayName:    "CodeDeploy deployment group",
				InspectionMode: "live",
				Summary:        "CodeDeploy deployment group Tenet targets for ECS rollout (style: linear)",
				CodeDeployDeploymentGroup: &resourceinspection.CodeDeployDeploymentGroupInspection{
					ApplicationName:       "api-prod-codedeploy-app",
					DeploymentGroupName:   "api-prod-codedeploy-group",
					DeploymentConfigName:  "CodeDeployDefault.ECSLinear10PercentEvery1Minutes",
					ServiceRoleARN:        "arn:aws:iam::123456789012:role/TenetCodeDeployRole",
					DeploymentType:        "BLUE_GREEN",
					DeploymentOption:      "WITH_TRAFFIC_CONTROL",
					ECSServices:           []resourceinspection.CodeDeployECSServiceBinding{{ClusterName: "payments-prod", ServiceName: "api-prod-service"}},
					TargetGroups:          []string{"api-prod-blue", "api-prod-green"},
					ProductionListenerARN: "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:listener/app/payments/abc123/prod456",
					TestListenerARN:       "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:listener/app/payments/abc123/test789",
				},
			},
		}

		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string) (resourceinspection.Result, error) {
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=codeDeployDeploymentGroup&resourceName=api-prod-codedeploy-group", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}

		var got resourceinspection.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode live traffic routing inspection payload: %v", err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("unexpected live traffic routing inspection payload: %+v", got)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw live traffic routing inspection payload: %v", err)
		}
		trafficRoutingResource, ok := raw["trafficRoutingResource"].(map[string]any)
		if !ok {
			t.Fatalf("expected trafficRoutingResource object in payload, got %#v", raw["trafficRoutingResource"])
		}
		if got := trafficRoutingResource["inspectionMode"]; got != "live" {
			t.Fatalf("expected live inspectionMode, got %#v", got)
		}
		if _, ok := trafficRoutingResource["codeDeployDeploymentGroup"].(map[string]any); !ok {
			t.Fatalf("expected codeDeployDeploymentGroup object in payload, got %#v", trafficRoutingResource["codeDeployDeploymentGroup"])
		}
	})

	for _, tc := range []struct {
		name         string
		query        string
		wantBodyPart string
	}{
		{name: "missing resourceType", query: "resourceName=payments-api-prod-service", wantBodyPart: "resourceType"},
		{name: "missing resourceName", query: "resourceType=ecsService", wantBodyPart: "resourceName"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
					t.Fatalf("overview deployment lookup should not run for invalid query")
					return catalog.DeploymentOverviewDetail{}, nil
				},
				OverviewDeploymentResourceInspection: func(context.Context, string, string, string) (resourceinspection.Result, error) {
					t.Fatalf("resource inspection provider should not run for invalid query")
					return resourceinspection.Result{}, nil
				},
			})

			req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?"+tc.query, nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d with body %q", resp.Code, resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), tc.wantBodyPart) {
				t.Fatalf("expected 400 body to mention %q, got %q", tc.wantBodyPart, resp.Body.String())
			}
		})
	}

	t.Run("returns 503 when provider is unavailable", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d with body %q", resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), "resource inspection") {
			t.Fatalf("expected 503 body to mention resource inspection, got %q", resp.Body.String())
		}
	})

	t.Run("propagates deployment not found as 404", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{}, statusstore.ErrNotFound
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string) (resourceinspection.Result, error) {
				t.Fatalf("resource inspection provider should not run when deployment lookup fails")
				return resourceinspection.Result{}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/missing/resource-inspection?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d with body %q", resp.Code, resp.Body.String())
		}
	})

	t.Run("propagates deployment resource binding failures as 404", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string) (resourceinspection.Result, error) {
				return resourceinspection.Result{}, statusstore.ErrNotFound
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d with body %q", resp.Code, resp.Body.String())
		}
	})

	t.Run("returns generic 500 when resource inspection provider errors", func(t *testing.T) {
		const providerErr = "boom: resource inspection provider internals"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string) (resourceinspection.Result, error) {
				return resourceinspection.Result{}, errors.New(providerErr)
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, providerErr)
	})
}

func TestRouterCreateDeploymentPlanPreview(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CreateDeploymentPlanPreview: func(ctx context.Context, deploymentName string, req planpreview.CreateRequest) (planpreview.CreateResult, error) {
			if deploymentName != "api-prod" {
				t.Fatalf("unexpected deployment plan preview create for %s", deploymentName)
			}
			if req.IfMatch != "plan_input" {
				t.Fatalf("unexpected create request: %+v", req)
			}
			return planpreview.CreateResult{
				Created: true,
				Record: planpreview.Record{
					ID:             "plan_1234abcd",
					DeploymentName: deploymentName,
					Status:         planpreview.StatusReady,
					CreatedAt:      "2026-04-17T08:00:00Z",
					Operations: []planpreview.Operation{{
						Kind:    "CreateCodeDeployDeployment",
						Status:  "completed",
						Phase:   "Provisioning",
						Target:  "api-prod-codedeploy-group (codeDeployDeploymentGroup)",
						Summary: "Create the CodeDeploy deployment Tenet will use for traffic shifting.",
					}},
					Risk:     planpreview.Risk{Level: "medium", Label: "medium risk", Guidance: "check runtime summary"},
					Approval: planpreview.Approval{Required: true, Summary: "check runtime summary"},
					Source:   planpreview.SourceContext{DeploymentName: deploymentName, Route: "/v1/overview/deployments/api-prod", Fingerprint: "plan_input"},
				},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{"ifMatch":"plan_input"}`))
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
	}
	if location := resp.Header().Get("Location"); location != "/v1/plans/plan_1234abcd" {
		t.Fatalf("expected Location header, got %q", location)
	}
	var record planpreview.Record
	if err := json.Unmarshal(resp.Body.Bytes(), &record); err != nil {
		t.Fatalf("failed to decode plan preview: %v", err)
	}
	if record.ID != "plan_1234abcd" || record.Status != planpreview.StatusReady || len(record.Operations) != 1 {
		t.Fatalf("unexpected plan preview payload: %+v", record)
	}
}

func TestRouterCreateDeploymentPlanPreviewPropagatesExplicitRequestActor(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CreateDeploymentPlanPreview: func(ctx context.Context, deploymentName string, req planpreview.CreateRequest) (planpreview.CreateResult, error) {
			actor := requestactor.FromContext(ctx)
			return planpreview.CreateResult{
				Created: true,
				Record: planpreview.Record{
					ID:             "plan_1234abcd",
					DeploymentName: deploymentName,
					Status:         planpreview.StatusReady,
					CreatedAt:      "2026-04-17T08:00:00Z",
					Actor:          actor,
					Source:         planpreview.SourceContext{DeploymentName: deploymentName, Route: "/v1/overview/deployments/api-prod", Fingerprint: req.IfMatch},
				},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{"ifMatch":"plan_input"}`))
	req.Header.Set(requestactor.HeaderID, "operator-123")
	req.Header.Set(requestactor.HeaderName, "Jane Operator")
	req.Header.Set(requestactor.HeaderEmail, "jane@example.com")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
	}
	var record planpreview.Record
	if err := json.Unmarshal(resp.Body.Bytes(), &record); err != nil {
		t.Fatalf("failed to decode plan preview: %v", err)
	}
	if !record.Actor.Available || record.Actor.Name != "Jane Operator" || record.Actor.Email != "jane@example.com" {
		t.Fatalf("expected propagated request actor, got %+v", record.Actor)
	}
}

func TestRouterCreateDeploymentPlanPreviewStatusCodes(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			CreateDeploymentPlanPreview: func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
				t.Fatalf("create provider should not run on invalid body")
				return planpreview.CreateResult{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{"ifMatch":true}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			CreateDeploymentPlanPreview: func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
				return planpreview.CreateResult{}, statusstore.ErrNotFound
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/missing", strings.NewReader(`{}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})

	t.Run("provider error", func(t *testing.T) {
		const upstreamErr = "boom: create details"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			CreateDeploymentPlanPreview: func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
				return planpreview.CreateResult{}, errors.New(upstreamErr)
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, upstreamErr)
	})

	for _, path := range []string{"/v1/plans/deployments/", "/v1/plans/deployments/a/b"} {
		t.Run("invalid path "+path, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				CreateDeploymentPlanPreview: func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
					t.Fatalf("create provider should not run on invalid path")
					return planpreview.CreateResult{}, nil
				},
			})
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
			}
		})
	}
}

func TestRouterPlanPreview(t *testing.T) {
	record := planpreview.Record{
		ID:             "plan_1234abcd",
		DeploymentName: "api-prod",
		Status:         planpreview.StatusFailed,
		CreatedAt:      "2026-04-17T08:05:00Z",
		Errors:         []string{"stale source fingerprint"},
		Source:         planpreview.SourceContext{DeploymentName: "api-prod", Route: "/v1/overview/deployments/api-prod", Fingerprint: "plan_current"},
	}

	t.Run("get payload", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			PlanPreview: func(context.Context, string) (planpreview.Record, error) {
				return record, nil
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/plans/plan_1234abcd", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.Code)
		}
		var got planpreview.Record
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode plan preview payload: %v", err)
		}
		if got.ID != record.ID || got.Status != planpreview.StatusFailed || got.Errors[0] != "stale source fingerprint" {
			t.Fatalf("unexpected plan preview payload: %+v", got)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
		req := httptest.NewRequest(http.MethodGet, "/v1/plans/plan_1234abcd", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			PlanPreview: func(context.Context, string) (planpreview.Record, error) {
				return planpreview.Record{}, planpreview.ErrNotFound
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/plans/plan_missing", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})

	t.Run("provider error", func(t *testing.T) {
		const upstreamErr = "boom: plan store"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			PlanPreview: func(context.Context, string) (planpreview.Record, error) {
				return planpreview.Record{}, errors.New(upstreamErr)
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/plans/plan_1234abcd", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, upstreamErr)
	})

	for _, path := range []string{"/v1/plans/", "/v1/plans/a/b"} {
		t.Run("invalid path "+path, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				PlanPreview: func(context.Context, string) (planpreview.Record, error) {
					t.Fatalf("plan preview provider should not run on invalid path")
					return planpreview.Record{}, nil
				},
			})
			req := httptest.NewRequest(http.MethodGet, path, nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
			}
		})
	}
}

func TestRouterCreateApplyRequest(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CreateApplyRequest: func(ctx context.Context, planID string) (applyrequest.CreateResult, error) {
			if planID != "plan_1234abcd" {
				t.Fatalf("unexpected apply create for %s", planID)
			}
			return applyrequest.CreateResult{
				Created: true,
				Record: applyrequest.Record{
					ID:             "apply_1234abcd",
					PlanID:         planID,
					DeploymentName: "api-prod",
					Status:         applyrequest.StatusFailed,
					CreatedAt:      "2026-04-18T08:00:00Z",
					Error:          "apply execution is disabled",
					Details:        []string{"Tenet persisted this immutable apply request in read-only mode and did not contact AWS."},
					Lifecycle:      applyrequest.Lifecycle{UpdatedAt: "2026-04-18T08:00:00Z", FinishedAt: "2026-04-18T08:00:00Z", TotalSteps: 1},
					Operations: []applyrequest.Operation{
						{Status: applyrequest.OperationStatusPending, Phase: "Provisioning", Target: "api-prod-codedeploy-group (codeDeployDeploymentGroup)", Summary: "Create the CodeDeploy deployment for api-prod."},
					},
					Source: applyrequest.SourceContext{PlanID: planID, Route: "/v1/plans/plan_1234abcd", PlanStatus: "ready", Fingerprint: "plan_input"},
				},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/applies/plans/plan_1234abcd", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
	}
	if location := resp.Header().Get("Location"); location != "/v1/applies/apply_1234abcd" {
		t.Fatalf("expected Location header, got %q", location)
	}
	var record applyrequest.Record
	if err := json.Unmarshal(resp.Body.Bytes(), &record); err != nil {
		t.Fatalf("failed to decode apply request: %v", err)
	}
	if record.ID != "apply_1234abcd" || record.Status != applyrequest.StatusFailed || record.Error != "apply execution is disabled" {
		t.Fatalf("unexpected apply request payload: %+v", record)
	}
	if record.Lifecycle.UpdatedAt != "2026-04-18T08:00:00Z" || record.Lifecycle.TotalSteps != 1 {
		t.Fatalf("expected lifecycle metadata in create response, got %+v", record.Lifecycle)
	}
	if len(record.Operations) != 1 || record.Operations[0].Status != applyrequest.OperationStatusPending {
		t.Fatalf("expected additive operations in apply request payload, got %+v", record)
	}
}

func TestRouterCreateApplyRequestPropagatesExplicitRequestActor(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CreateApplyRequest: func(ctx context.Context, planID string) (applyrequest.CreateResult, error) {
			return applyrequest.CreateResult{
				Created: true,
				Record: applyrequest.Record{
					ID:             "apply_1234abcd",
					PlanID:         planID,
					DeploymentName: "api-prod",
					Status:         applyrequest.StatusFailed,
					CreatedAt:      "2026-04-18T08:00:00Z",
					Actor:          requestactor.FromContext(ctx),
					Error:          "apply execution is disabled",
					Lifecycle:      applyrequest.Lifecycle{UpdatedAt: "2026-04-18T08:00:00Z", FinishedAt: "2026-04-18T08:00:00Z", TotalSteps: 1},
					Operations: []applyrequest.Operation{
						{Status: applyrequest.OperationStatusPending, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
					},
					Source: applyrequest.SourceContext{PlanID: planID, Route: "/v1/plans/plan_1234abcd", PlanStatus: "ready"},
				},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/applies/plans/plan_1234abcd", nil)
	req.Header.Set(requestactor.HeaderID, "operator-123")
	req.Header.Set(requestactor.HeaderName, "Jane Operator")
	req.Header.Set(requestactor.HeaderEmail, "jane@example.com")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
	}
	var record applyrequest.Record
	if err := json.Unmarshal(resp.Body.Bytes(), &record); err != nil {
		t.Fatalf("failed to decode apply request: %v", err)
	}
	if !record.Actor.Available || record.Actor.Name != "Jane Operator" || record.Actor.Email != "jane@example.com" {
		t.Fatalf("expected propagated request actor, got %+v", record.Actor)
	}
}

func TestRouterCreateApplyRequestStatusCodes(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/plans/plan_1234abcd", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			CreateApplyRequest: func(context.Context, string) (applyrequest.CreateResult, error) {
				return applyrequest.CreateResult{}, planpreview.ErrNotFound
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/plans/plan_missing", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})

	t.Run("plan not ready", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			CreateApplyRequest: func(context.Context, string) (applyrequest.CreateResult, error) {
				return applyrequest.CreateResult{}, applyrequest.ErrPlanNotReady
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/plans/plan_failed_1234abcd", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", resp.Code)
		}
	})

	t.Run("stale plan fingerprint", func(t *testing.T) {
		const conflictBody = "apply request plan source fingerprint conflict: reviewed plan plan_1234abcd expects deployment detail fingerprint plan_deadbeef but current fingerprint for api-prod is plan_feedface; create a new plan preview before applying\n"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			CreateApplyRequest: func(context.Context, string) (applyrequest.CreateResult, error) {
				return applyrequest.CreateResult{}, fmt.Errorf("%w: reviewed plan plan_1234abcd expects deployment detail fingerprint plan_deadbeef but current fingerprint for api-prod is plan_feedface; create a new plan preview before applying", applyrequest.ErrSourceFingerprintConflict)
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/plans/plan_1234abcd", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != conflictBody {
			t.Fatalf("expected conflict body %q, got %q", conflictBody, body)
		}
	})

	t.Run("provider error", func(t *testing.T) {
		const upstreamErr = "boom: apply request details"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			CreateApplyRequest: func(context.Context, string) (applyrequest.CreateResult, error) {
				return applyrequest.CreateResult{}, errors.New(upstreamErr)
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/plans/plan_1234abcd", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, upstreamErr)
	})

	for _, path := range []string{"/v1/applies/plans/", "/v1/applies/plans/a/b"} {
		t.Run("invalid path "+path, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				CreateApplyRequest: func(context.Context, string) (applyrequest.CreateResult, error) {
					t.Fatalf("apply create provider should not run on invalid path")
					return applyrequest.CreateResult{}, nil
				},
			})
			req := httptest.NewRequest(http.MethodPost, path, nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
			}
		})
	}
}

func TestRouterApplyRequest(t *testing.T) {
	record := applyrequest.Record{
		ID:             "apply_1234abcd",
		PlanID:         "plan_1234abcd",
		DeploymentName: "api-prod",
		Status:         applyrequest.StatusFailed,
		CreatedAt:      "2026-04-18T08:05:00Z",
		Error:          "apply execution is disabled",
		Details:        []string{"Tenet persisted this immutable apply request in read-only mode and did not contact AWS."},
		Lifecycle:      applyrequest.Lifecycle{UpdatedAt: "2026-04-18T08:05:00Z", FinishedAt: "2026-04-18T08:05:00Z", TotalSteps: 1},
		Operations: []applyrequest.Operation{
			{Status: applyrequest.OperationStatusPending, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
		},
		Source: applyrequest.SourceContext{PlanID: "plan_1234abcd", Route: "/v1/plans/plan_1234abcd", PlanStatus: "ready", Fingerprint: "plan_input"},
	}

	t.Run("get payload", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
				return record, nil
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/applies/apply_1234abcd", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.Code)
		}
		var got applyrequest.Record
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode apply request payload: %v", err)
		}
		if got.ID != record.ID || got.PlanID != record.PlanID || got.Error != record.Error {
			t.Fatalf("unexpected apply request payload: %+v", got)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw apply request payload: %v", err)
		}
		for _, key := range []string{"id", "planId", "deploymentName", "status", "createdAt", "error", "details", "lifecycle", "operations", "source"} {
			if _, ok := raw[key]; !ok {
				t.Fatalf("expected apply request payload to include %q, got %#v", key, raw)
			}
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
		req := httptest.NewRequest(http.MethodGet, "/v1/applies/apply_1234abcd", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
				return applyrequest.Record{}, applyrequest.ErrNotFound
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/applies/apply_missing", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})

	t.Run("provider error", func(t *testing.T) {
		const upstreamErr = "boom: apply store"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
				return applyrequest.Record{}, errors.New(upstreamErr)
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/applies/apply_1234abcd", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, upstreamErr)
	})

	for _, path := range []string{"/v1/applies/", "/v1/applies/a/b"} {
		t.Run("invalid path "+path, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
					t.Fatalf("apply request provider should not run on invalid path")
					return applyrequest.Record{}, nil
				},
			})
			req := httptest.NewRequest(http.MethodGet, path, nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
			}
		})
	}
}

func TestRouterRootServesUI(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if got := resp.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", got)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "/v1/overview/accounts") {
		t.Fatalf("expected UI to reference overview endpoint, got %q", body)
	}
	for _, fragment := range []string{"Rollout phase", "Current step", "Planned steps", "ShiftTrafficCanary", "CreateCodeDeployDeployment", "deployments", "synced", "progressing", "paused", "error"} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected UI to render rollout summary label %q, got %q", fragment, body)
		}
	}
	for _, unsafe := range []string{
		"root.innerHTML = accounts.map",
		"document.getElementById('accounts').innerHTML = `<div class=\"empty\">Failed to load overview: ${error.message}</div>`",
		"['Progressing', deployments.length - synced]",
		"const progressing = deployments.filter((deployment) => deployment.sync?.state === 'Reconciling' || deployment.sync?.state === 'OutOfSync').length;",
	} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("expected UI to avoid unsafe rendering or misclassified summary logic %q, got %q", unsafe, body)
		}
	}
	for _, fragment := range []string{
		"textContent",
		"createElement",
		"const progressingStates = new Set(['Reconciling', 'OutOfSync', 'Drifted']);",
		"progressingStates.has(deployment.sync?.state)",
		"case 'Reconciling':",
		"case 'OutOfSync':",
		"case 'Drifted':\n            return 'progress';",
		"case 'Paused':\n            return 'paused';",
		"case 'Error':\n            return 'error';",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected UI to include safe rendering and precise sync-state pill mapping %q, got %q", fragment, body)
		}
	}
}

func TestRouterRejectsNonGETMethods(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogSummary: func() (catalog.Summary, error) {
			return catalog.Summary{}, nil
		},
		CatalogDeployments: func() ([]catalog.DeploymentView, error) {
			return []catalog.DeploymentView{}, nil
		},
		CatalogEnvironments: func() ([]catalog.EnvironmentView, error) {
			return []catalog.EnvironmentView{}, nil
		},
		CatalogAccounts: func() ([]catalog.AccountView, error) {
			return []catalog.AccountView{}, nil
		},
		CatalogPlacements: func() ([]catalog.PlacementView, error) {
			return []catalog.PlacementView{}, nil
		},
		CatalogTopology: func() (catalog.TopologySnapshot, error) {
			return catalog.TopologySnapshot{}, nil
		},
		OverviewAccounts: func() ([]catalog.AccountOverview, error) {
			return []catalog.AccountOverview{}, nil
		},
		OverviewAccount: func(string) (catalog.AccountOverviewDetail, error) {
			return catalog.AccountOverviewDetail{}, nil
		},
		OverviewApplication: func(string) (catalog.ApplicationOverviewDetail, error) {
			return catalog.ApplicationOverviewDetail{}, nil
		},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{}, nil
		},
		OverviewDeploymentResourceLogs: func(context.Context, string, string, string) (resourcelogs.Result, error) {
			return resourcelogs.Result{}, nil
		},
		OverviewDeploymentResourceInspection: func(context.Context, string, string, string) (resourceinspection.Result, error) {
			return resourceinspection.Result{}, nil
		},
		CreateDeploymentPlanPreview: func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
			return planpreview.CreateResult{Record: planpreview.Record{ID: "plan_1234abcd"}}, nil
		},
		PlanPreview: func(context.Context, string) (planpreview.Record, error) {
			return planpreview.Record{ID: "plan_1234abcd"}, nil
		},
		CreateApplyRequest: func(context.Context, string) (applyrequest.CreateResult, error) {
			return applyrequest.CreateResult{Record: applyrequest.Record{ID: "apply_1234abcd"}}, nil
		},
		ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
			return applyrequest.Record{ID: "apply_1234abcd"}, nil
		},
		DeploymentStatuses: func() ([]statusstore.Snapshot, error) {
			return []statusstore.Snapshot{}, nil
		},
		DeploymentStatus: func(name string) (statusstore.Snapshot, error) {
			return statusstore.Snapshot{}, nil
		},
	})

	getPaths := []string{"/", "/healthz", "/v1/info", "/v1/catalog/summary", "/v1/catalog/deployments", "/v1/catalog/environments", "/v1/catalog/accounts", "/v1/catalog/placements", "/v1/catalog/topology", "/v1/overview/applications", "/v1/overview/applications/api-prod", "/v1/overview/accounts", "/v1/overview/accounts/123456789012", "/v1/overview/accounts/123456789012/projects/payments-api/environments/prod", "/v1/overview/deployments/api-prod", "/v1/overview/deployments/api-prod/events", "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=payments-api-prod-service", "/v1/plans/plan_1234abcd", "/v1/applies/apply_1234abcd", "/v1/status/deployments", "/v1/status/deployments/api-prod"}
	for _, path := range getPaths {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for %s, got %d", path, resp.Code)
		}
		if allow := resp.Header().Get("Allow"); allow != http.MethodGet {
			t.Fatalf("expected Allow header for %s, got %q", path, allow)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/plans/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for /v1/plans/deployments/api-prod, got %d", resp.Code)
	}
	if allow := resp.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow header for /v1/plans/deployments/api-prod, got %q", allow)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/applies/plans/plan_1234abcd", nil)
	resp = httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for /v1/applies/plans/plan_1234abcd, got %d", resp.Code)
	}
	if allow := resp.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow header for /v1/applies/plans/plan_1234abcd, got %q", allow)
	}
}
