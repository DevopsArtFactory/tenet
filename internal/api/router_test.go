package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/auditlog"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/controllerrefresh"
	"github.com/DevopsArtFactory/tenet/internal/controllerrefreshaudit"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/githubwebhook"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/rbacpolicy"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/DevopsArtFactory/tenet/internal/resourceaws"
	"github.com/DevopsArtFactory/tenet/internal/resourceinspection"
	"github.com/DevopsArtFactory/tenet/internal/resourcelogs"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

type contextKey string

type recordingControllerRefreshPublisher struct {
	requests []controllerrefresh.Request
	receipt  controllerrefresh.Receipt
	err      error
}

func (p *recordingControllerRefreshPublisher) Publish(_ context.Context, request controllerrefresh.Request) (controllerrefresh.Receipt, error) {
	p.requests = append(p.requests, request)
	return p.receipt, p.err
}

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

func requireManagedDriftItemByField(t *testing.T, items []any, field string) map[string]any {
	t.Helper()

	for _, item := range items {
		driftItem, ok := item.(map[string]any)
		if ok && driftItem["field"] == field {
			return driftItem
		}
	}

	t.Fatalf("expected managedDrift item for field %q in %#v", field, items)
	return nil
}

func requireManagedDriftSource(t *testing.T, item map[string]any, kind string, label string, route string, resourceType string, resourceName string) map[string]any {
	t.Helper()

	source, ok := item["source"].(map[string]any)
	if !ok {
		t.Fatalf("expected managedDrift source object in %#v", item)
	}
	if source["kind"] != kind || source["label"] != label || source["route"] != route || source["resourceType"] != resourceType || source["resourceName"] != resourceName {
		t.Fatalf("expected managedDrift source kind=%q label=%q route=%q resourceType=%q resourceName=%q, got %#v", kind, label, route, resourceType, resourceName, source)
	}
	return source
}

func boolPtr(value bool) *bool {
	return &value
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

func TestRouterFaviconRequestStaysOutOfClientErrorLogs(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected favicon request to return 204, got %d", resp.Code)
	}
	assertResponseRequestID(t, resp)
	if cacheControl := resp.Header().Get("Cache-Control"); cacheControl != "public, max-age=86400" {
		t.Fatalf("expected cacheable empty favicon response, got Cache-Control %q", cacheControl)
	}
}

type diagnosticRefreshPublisher struct {
	diagnostic controllerrefresh.Diagnostic
}

func (p diagnosticRefreshPublisher) Publish(context.Context, controllerrefresh.Request) (controllerrefresh.Receipt, error) {
	return controllerrefresh.Receipt{}, nil
}

func (p diagnosticRefreshPublisher) Diagnostic() controllerrefresh.Diagnostic {
	return p.diagnostic
}

func TestRouterOperatorContextReportsFilesystemDesiredStateSourceDiagnostics(t *testing.T) {
	router := NewRouter(Options{
		GitHubWebhookDesiredStateSource: githubwebhook.DesiredStateSourceConfig{Kind: githubwebhook.DesiredStateSourceKindFilesystem},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	desiredStateSource, ok := body["desiredStateSource"].(map[string]any)
	if !ok {
		t.Fatalf("expected desiredStateSource object, got %#v", body["desiredStateSource"])
	}
	if desiredStateSource["configured"] != true || desiredStateSource["kind"] != githubwebhook.DesiredStateSourceKindFilesystem {
		t.Fatalf("unexpected filesystem desired-state source flags: %#v", desiredStateSource)
	}
	filesystem, ok := desiredStateSource["filesystem"].(map[string]any)
	if !ok || filesystem["mode"] != "local_repository" {
		t.Fatalf("unexpected filesystem source diagnostic: %#v", desiredStateSource["filesystem"])
	}
	if _, ok := desiredStateSource["github"]; ok {
		t.Fatalf("filesystem diagnostic must not include github source fields: %#v", desiredStateSource)
	}
	refresh, ok := desiredStateSource["controllerRefresh"].(map[string]any)
	if !ok {
		t.Fatalf("expected controllerRefresh object, got %#v", desiredStateSource["controllerRefresh"])
	}
	if refresh["configured"] != false || refresh["available"] != false || refresh["status"] != "filesystem_noop" || refresh["diagnosticOnly"] != true {
		t.Fatalf("unexpected filesystem refresh diagnostic: %#v", refresh)
	}
	if !strings.Contains(fmt.Sprint(refresh["reason"]), "filesystem catalog") || !strings.Contains(fmt.Sprint(refresh["reason"]), "no-op") {
		t.Fatalf("expected operator-friendly filesystem no-op reason, got %#v", refresh)
	}
	statuses := jsonStringSlice(t, desiredStateSource["webhookMatchStatuses"])
	for _, status := range []string{githubwebhook.DesiredStateSourceMatchStatusMatched, githubwebhook.DesiredStateSourceMatchStatusRepositoryMismatch, githubwebhook.DesiredStateSourceMatchStatusRefMismatch, githubwebhook.DesiredStateSourceMatchStatusPathUntouched} {
		if !slicesContain(statuses, status) {
			t.Fatalf("expected webhook match status %q in %#v", status, statuses)
		}
	}
}

func TestRouterOperatorContextReportsGitHubDesiredStateSourceRefreshDiagnostics(t *testing.T) {
	router := NewRouter(Options{
		GitHubWebhookDesiredStateSource: githubwebhook.DesiredStateSourceConfig{
			Kind:       githubwebhook.DesiredStateSourceKindGitHub,
			Repository: "acme/platform-config",
			Ref:        "main",
			Path:       "tenet",
			BaseURL:    "https://github.example/api/v3",
		},
		GitHubWebhookRefreshPublisher: diagnosticRefreshPublisher{diagnostic: controllerrefresh.Diagnostic{
			Configured: true,
			Available:  true,
			Backend:    controllerrefresh.BackendRedis,
			Channel:    "tenet:test:controller-refresh",
		}},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	desiredStateSource, ok := body["desiredStateSource"].(map[string]any)
	if !ok {
		t.Fatalf("expected desiredStateSource object, got %#v", body["desiredStateSource"])
	}
	if desiredStateSource["configured"] != true || desiredStateSource["kind"] != githubwebhook.DesiredStateSourceKindGitHub {
		t.Fatalf("unexpected github desired-state source flags: %#v", desiredStateSource)
	}
	github, ok := desiredStateSource["github"].(map[string]any)
	if !ok {
		t.Fatalf("expected github source diagnostic, got %#v", desiredStateSource["github"])
	}
	if github["repository"] != "acme/platform-config" || github["ref"] != "main" || github["path"] != "tenet" || github["baseUrl"] != "https://github.example/api/v3" {
		t.Fatalf("unexpected non-secret github source diagnostic: %#v", github)
	}
	encodedDesiredStateSource, err := json.Marshal(desiredStateSource)
	if err != nil {
		t.Fatalf("encode desired-state source diagnostic: %v", err)
	}
	if got := string(encodedDesiredStateSource); strings.Contains(got, "TOKEN") || strings.Contains(strings.ToLower(got), "secret") {
		t.Fatalf("desired-state source diagnostic leaked token/secret-shaped data: %s", got)
	}
	refresh, ok := desiredStateSource["controllerRefresh"].(map[string]any)
	if !ok {
		t.Fatalf("expected controllerRefresh object, got %#v", desiredStateSource["controllerRefresh"])
	}
	if refresh["configured"] != true || refresh["available"] != true || refresh["backend"] != controllerrefresh.BackendRedis || refresh["channel"] != "tenet:test:controller-refresh" || refresh["status"] != "available" || refresh["diagnosticOnly"] != true {
		t.Fatalf("unexpected github refresh diagnostic: %#v", refresh)
	}
	if !strings.Contains(fmt.Sprint(refresh["reason"]), "accepted matching push") || !strings.Contains(fmt.Sprint(refresh["reason"]), "Redis controller-refresh") {
		t.Fatalf("expected operator-friendly Redis refresh reason, got %#v", refresh)
	}
	reasons := jsonStringSlice(t, desiredStateSource["webhookProcessedFalseReasons"])
	for _, reason := range []string{"deleted_ref_ignored", githubwebhook.DesiredStateSourceMatchStatusRepositoryMismatch, githubwebhook.DesiredStateSourceMatchStatusRefMismatch, githubwebhook.DesiredStateSourceMatchStatusPathUntouched, "controller_refresh_unavailable"} {
		if !slicesContain(reasons, reason) {
			t.Fatalf("expected processed=false reason %q in %#v", reason, reasons)
		}
	}
}

func TestRouterControllerRefreshFilesystemNoop(t *testing.T) {
	publisher := &recordingControllerRefreshPublisher{}
	var records []controllerrefreshaudit.Record
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		GitHubWebhookDesiredStateSource: githubwebhook.DesiredStateSourceConfig{
			Kind: githubwebhook.DesiredStateSourceKindFilesystem,
		},
		GitHubWebhookRefreshPublisher: publisher,
		RecordControllerRefreshAudit: func(_ context.Context, record controllerrefreshaudit.Record) error {
			records = append(records, record)
			return nil
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/controller/refresh", nil)
	req.Header.Set(RequestIDHeader, "manual-noop-1")
	req.Header.Set(requestactor.HeaderID, "explicit-456")
	req.Header.Set(requestactor.HeaderName, "Alex Explicit")
	req.Header.Set(requestactor.HeaderEmail, "alex.explicit@example.com")
	req.Header.Set(requestactor.HeaderAliasID, "alias-123")
	req.Header.Set(requestactor.HeaderAliasName, "Alias Actor")
	req.Header.Set(requestactor.HeaderAliasEmail, "alias@example.com")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", resp.Code, resp.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode controller refresh response: %v", err)
	}
	if got["status"] != "filesystem_noop" || got["processed"] != false {
		t.Fatalf("unexpected filesystem no-op response: %#v", got)
	}
	desiredStateSource, ok := got["desiredStateSource"].(map[string]any)
	if !ok {
		t.Fatalf("expected desiredStateSource diagnostics, got %#v", got["desiredStateSource"])
	}
	if desiredStateSource["kind"] != githubwebhook.DesiredStateSourceKindFilesystem {
		t.Fatalf("expected filesystem desired-state diagnostic, got %#v", desiredStateSource)
	}
	refresh, ok := desiredStateSource["controllerRefresh"].(map[string]any)
	if !ok || refresh["status"] != "filesystem_noop" {
		t.Fatalf("expected filesystem controller refresh diagnostic, got %#v", desiredStateSource["controllerRefresh"])
	}
	if len(publisher.requests) != 0 {
		t.Fatalf("expected filesystem refresh to skip publisher, got %+v", publisher.requests)
	}
	if len(records) != 1 {
		t.Fatalf("expected one controller refresh audit record, got %+v", records)
	}
	record := records[0]
	if record.Route != "POST /v1/controller/refresh" || record.Status != "filesystem_noop" || record.Result != "noop" || record.RequestID != "manual-noop-1" || record.DeliveryID != "" || record.Channel != "" || record.HTTPStatusCode != http.StatusOK {
		t.Fatalf("unexpected filesystem refresh audit record: %+v", record)
	}
	if !strings.Contains(record.NoopReason, "filesystem catalog") {
		t.Fatalf("expected safe no-op reason in audit record, got %+v", record)
	}
	if !record.Actor.Available || record.Actor.ID != "explicit-456" || record.Actor.Name != "Alex Explicit" || record.Actor.Email != "alex.explicit@example.com" {
		t.Fatalf("expected explicit X-Tenet-Actor-* audit actor to win, got %+v", record.Actor)
	}
}

func TestRouterControllerRefreshPublishesGitHubDesiredStateRefresh(t *testing.T) {
	previousNow := githubWebhookNow
	githubWebhookNow = func() time.Time { return time.Date(2026, time.April, 20, 10, 45, 0, 0, time.UTC) }
	defer func() {
		githubWebhookNow = previousNow
	}()

	publisher := &recordingControllerRefreshPublisher{
		receipt: controllerrefresh.Receipt{
			Backend:   controllerrefresh.BackendRedis,
			Channel:   "tenet:test:controller-refresh",
			RequestID: "api-controller-refresh:manual-refresh-123",
		},
	}
	var records []controllerrefreshaudit.Record
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		GitHubWebhookDesiredStateSource: githubwebhook.DesiredStateSourceConfig{
			Kind:       githubwebhook.DesiredStateSourceKindGitHub,
			Repository: "acme/platform-config",
			Ref:        "main",
			Path:       "tenet",
		},
		GitHubWebhookRefreshPublisher: publisher,
		RecordControllerRefreshAudit: func(_ context.Context, record controllerrefreshaudit.Record) error {
			records = append(records, record)
			return nil
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/controller/refresh", nil)
	req.Header.Set(RequestIDHeader, "manual-refresh-123")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%q", resp.Code, resp.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode controller refresh response: %v", err)
	}
	if got["status"] != "published" || got["processed"] != true {
		t.Fatalf("unexpected published response: %#v", got)
	}
	refresh, ok := got["controllerRefresh"].(map[string]any)
	if !ok {
		t.Fatalf("expected controllerRefresh summary, got %#v", got["controllerRefresh"])
	}
	if refresh["backend"] != controllerrefresh.BackendRedis || refresh["channel"] != "tenet:test:controller-refresh" || refresh["requestId"] != "api-controller-refresh:manual-refresh-123" || refresh["publishedAt"] != "2026-04-20T10:45:00Z" {
		t.Fatalf("unexpected controller refresh summary: %#v", refresh)
	}
	if len(publisher.requests) != 1 {
		t.Fatalf("expected one controller refresh publish, got %d", len(publisher.requests))
	}
	request := publisher.requests[0]
	if request.RequestID != "api-controller-refresh:manual-refresh-123" || request.DeliveryID != "manual-refresh-123" || request.Repository != "acme/platform-config" || request.Ref != "main" || request.After != "" || request.RequestedAt != "2026-04-20T10:45:00Z" {
		t.Fatalf("unexpected controller refresh publish request: %+v", request)
	}
	if len(records) != 1 {
		t.Fatalf("expected one controller refresh audit record, got %+v", records)
	}
	record := records[0]
	if record.ID != "api-controller-refresh:manual-refresh-123" || record.Route != "POST /v1/controller/refresh" || record.Status != "published" || record.Result != "published" || record.RequestID != "api-controller-refresh:manual-refresh-123" || record.DeliveryID != "manual-refresh-123" || record.Backend != controllerrefresh.BackendRedis || record.Channel != "tenet:test:controller-refresh" || record.NoopReason != "" || record.HTTPStatusCode != http.StatusAccepted {
		t.Fatalf("unexpected published refresh audit record: %+v", record)
	}
}

func TestRouterControllerRefreshRequiresPost(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/controller/refresh", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%q", resp.Code, resp.Body.String())
	}
	if allow := resp.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow header %q, got %q", http.MethodPost, allow)
	}
}

func TestRouterControllerRefreshStatusReportsLatestReceipt(t *testing.T) {
	auditService := auditlog.Service{
		ListPlanPreviews:  func() ([]planpreview.Record, error) { return nil, nil },
		ListApplyRequests: func() ([]applyrequest.Record, error) { return nil, nil },
		ListControllerRefreshAuditRows: func() ([]controllerrefreshaudit.Record, error) {
			return []controllerrefreshaudit.Record{
				{
					ID:             "api-controller-refresh:manual-refresh-older",
					Route:          "POST /v1/controller/refresh",
					Status:         "published",
					Result:         "published",
					RequestedAt:    "2026-04-20T10:44:00Z",
					RequestID:      "api-controller-refresh:manual-refresh-older",
					DeliveryID:     "manual-refresh-older",
					Backend:        controllerrefresh.BackendRedis,
					Channel:        "tenet:test:controller-refresh",
					HTTPStatusCode: http.StatusAccepted,
				},
				{
					ID:             "api-controller-refresh:manual-refresh-123",
					Route:          "POST /v1/controller/refresh",
					Status:         "published",
					Result:         "published",
					RequestedAt:    "2026-04-20T10:45:00Z",
					RequestID:      "api-controller-refresh:manual-refresh-123",
					DeliveryID:     "manual-refresh-123",
					Backend:        controllerrefresh.BackendRedis,
					Channel:        "tenet:test:controller-refresh",
					HTTPStatusCode: http.StatusAccepted,
					Actor: requestactor.Actor{
						Available: true,
						ID:        "operator-123",
						Name:      "Jane Operator",
						Email:     "jane@example.com",
					},
				},
				{
					ID:             "api-controller-refresh:manual-refresh-noop",
					Route:          "POST /v1/controller/refresh",
					Status:         "noop",
					Result:         "noop",
					RequestedAt:    "2026-04-20T10:43:00Z",
					RequestID:      "api-controller-refresh:manual-refresh-noop",
					DeliveryID:     "manual-refresh-noop",
					Backend:        controllerrefresh.BackendRedis,
					Channel:        "tenet:test:controller-refresh",
					HTTPStatusCode: http.StatusAccepted,
				},
			}, nil
		},
	}
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		AuditLogs: func(ctx context.Context, query auditlog.Query) (auditlog.Result, error) {
			if query.Limit != 1 || query.DeploymentName != "" || query.Action != auditlog.ActionControllerRefresh {
				t.Fatalf("expected latest controller-refresh audit query, got %+v", query)
			}
			return auditService.List(ctx, query)
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/controller/refresh/status", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", resp.Code, resp.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode controller refresh status response: %v", err)
	}
	if raw["status"] != "published" || raw["available"] != true || raw["hasHistory"] != true || raw["readOnly"] != true || raw["route"] != "GET /v1/controller/refresh/status" {
		t.Fatalf("unexpected controller refresh status response: %#v", raw)
	}
	latest, ok := raw["latestReceipt"].(map[string]any)
	if !ok {
		t.Fatalf("expected latestReceipt object, got %#v", raw["latestReceipt"])
	}
	if latest["requestId"] != "api-controller-refresh:manual-refresh-123" || latest["deliveryId"] != "manual-refresh-123" || latest["backend"] != controllerrefresh.BackendRedis || latest["channel"] != "tenet:test:controller-refresh" || latest["requestedAt"] != "2026-04-20T10:45:00Z" || latest["httpStatusCode"] != float64(http.StatusAccepted) {
		t.Fatalf("unexpected latest receipt: %#v", latest)
	}
	actor, ok := latest["actor"].(map[string]any)
	if !ok || actor["available"] != true || actor["id"] != "operator-123" || actor["name"] != "Jane Operator" || actor["email"] != "jane@example.com" {
		t.Fatalf("expected latest receipt actor projection, got %#v", latest["actor"])
	}
	scope, ok := raw["resultScope"].(map[string]any)
	if !ok || scope["requestedLimit"] != float64(1) || scope["effectiveLimit"] != float64(1) || scope["returnedEntryCount"] != float64(1) || scope["totalEntryCount"] != float64(3) || scope["truncated"] != true {
		t.Fatalf("expected latest receipt result scope, got %#v", raw["resultScope"])
	}
}

func TestRouterControllerRefreshStatusNoHistory(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		AuditLogs: func(ctx context.Context, query auditlog.Query) (auditlog.Result, error) {
			if query.Limit != 1 || query.Action != auditlog.ActionControllerRefresh {
				t.Fatalf("expected latest controller-refresh audit query, got %+v", query)
			}
			return auditlog.Result{
				Entries:     []auditlog.Entry{},
				ResultScope: auditlog.BuildResultScope(query, 0, 0),
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/controller/refresh/status", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", resp.Code, resp.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode controller refresh status response: %v", err)
	}
	if raw["status"] != "no_history" || raw["available"] != true || raw["hasHistory"] != false || raw["latestReceipt"] != nil {
		t.Fatalf("unexpected no-history controller refresh status: %#v", raw)
	}
	if reason := fmt.Sprint(raw["reason"]); !strings.Contains(reason, "No manual controller refresh receipt history") {
		t.Fatalf("expected explicit no-history reason, got %q", reason)
	}
}

func TestRouterControllerRefreshStatusUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/controller/refresh/status", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%q", resp.Code, resp.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode controller refresh status response: %v", err)
	}
	if raw["status"] != "unavailable" || raw["available"] != false || raw["hasHistory"] != false || raw["latestReceipt"] != nil {
		t.Fatalf("unexpected unavailable controller refresh status: %#v", raw)
	}
}

func TestRouterControllerRefreshStatusAuthGuards(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		RequireOperatorRoleForRead:         true,
		AuditLogs: func(context.Context, auditlog.Query) (auditlog.Result, error) {
			t.Fatal("auth guard should short-circuit controller refresh status provider")
			return auditlog.Result{}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/controller/refresh/status", nil)
	req.Header.Set("X-Tenet-Authenticated-Id", "operator-123")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%q", resp.Code, resp.Body.String())
	}
	if body := resp.Body.String(); body != "forbidden: authenticated viewer-capable role is required for this operator read route\n" {
		t.Fatalf("unexpected forbidden response body: %q", body)
	}
}

func TestRouterControllerRefreshStatusRequiresGet(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodPost, "/v1/controller/refresh/status", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%q", resp.Code, resp.Body.String())
	}
	if allow := resp.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow header %q, got %q", http.MethodGet, allow)
	}
}

func TestRouterControllerRefreshAuthGuards(t *testing.T) {
	t.Run("unauthenticated when identity is enforced", func(t *testing.T) {
		publisher := &recordingControllerRefreshPublisher{}
		router := NewRouter(Options{
			BuildInfo:                    buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireAuthenticatedIdentity: true,
			GitHubWebhookDesiredStateSource: githubwebhook.DesiredStateSourceConfig{
				Kind:       githubwebhook.DesiredStateSourceKindGitHub,
				Repository: "acme/platform-config",
				Ref:        "main",
			},
			GitHubWebhookRefreshPublisher: publisher,
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/controller/refresh", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator write route\n" {
			t.Fatalf("unexpected unauthenticated response body: %q", body)
		}
		if len(publisher.requests) != 0 {
			t.Fatalf("expected identity guard to short-circuit publisher, got %+v", publisher.requests)
		}
	})

	t.Run("forbidden when operator role is enforced", func(t *testing.T) {
		publisher := &recordingControllerRefreshPublisher{}
		router := NewRouter(Options{
			BuildInfo:                    buildinfo.Info{Version: "dev", Commit: "abc123"},
			TrustAuthenticatedHeaders:    true,
			RequireAuthenticatedIdentity: true,
			RequireOperatorRoleForWrite:  true,
			GitHubWebhookDesiredStateSource: githubwebhook.DesiredStateSourceConfig{
				Kind:       githubwebhook.DesiredStateSourceKindGitHub,
				Repository: "acme/platform-config",
				Ref:        "main",
			},
			GitHubWebhookRefreshPublisher: publisher,
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/controller/refresh", nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Viewer")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "viewer@example.com")
		req.Header.Set(requestactor.AuthenticatedRolesHeader, "viewer")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "forbidden: authenticated operator role is required for this operator write route\n" {
			t.Fatalf("unexpected forbidden response body: %q", body)
		}
		if len(publisher.requests) != 0 {
			t.Fatalf("expected role guard to short-circuit publisher, got %+v", publisher.requests)
		}
	})
}

func TestRouterOperatorContextReportsManagedLoginRuntimeDisabledByDefault(t *testing.T) {
	router := NewRouter(Options{})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	authRuntime, ok := body["authRuntime"].(map[string]any)
	if !ok {
		t.Fatalf("expected authRuntime object, got %#v", body["authRuntime"])
	}
	managedLogin, ok := authRuntime["tenetManagedLogin"].(map[string]any)
	if !ok {
		t.Fatalf("expected tenetManagedLogin object, got %#v", authRuntime["tenetManagedLogin"])
	}
	if managedLogin["configured"] != false || managedLogin["active"] != false || managedLogin["provider"] != "" {
		t.Fatalf("expected disabled unconfigured managed-login diagnostic, got %#v", managedLogin)
	}
	if managedLogin["unavailableReason"] != "tenet-managed login is disabled because OAuth runtime configuration is incomplete or absent" {
		t.Fatalf("unexpected managed-login unavailable reason: %#v", managedLogin)
	}
	if managedLogin["sessionCookieSecureOverride"] != false || managedLogin["sessionCookieSecureForRequest"] != false {
		t.Fatalf("expected disabled managed-login cookie security diagnostics to stay false, got %#v", managedLogin)
	}
	assertManagedLoginRawLifecycleDiagnostics(t, managedLogin, false)
	if !reflect.DeepEqual(jsonStringSlice(t, managedLogin["requiredSettings"]), RequiredTenetManagedLoginSettings()) || !reflect.DeepEqual(jsonStringSlice(t, managedLogin["missingSettings"]), RequiredTenetManagedLoginSettings()) {
		t.Fatalf("unexpected managed-login required/missing settings: %#v", managedLogin)
	}
}

func TestRouterOperatorContextReportsConfiguredManagedLoginRuntimeGroundwork(t *testing.T) {
	router := NewRouter(Options{
		AuthRuntime: AuthRuntimeDiagnostic{
			TenetManagedLogin: TenetManagedLoginDiagnostic{
				Configured:              true,
				Provider:                "github",
				LoginRoute:              "/auth/login/github",
				CallbackRoute:           "/auth/callback/github",
				LogoutRoute:             "/auth/logout",
				SessionCookieName:       "tenet_session",
				StateStoreConfigured:    true,
				IdentityStoreConfigured: true,
				SessionSecretConfigured: true,
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	authRuntime := body["authRuntime"].(map[string]any)
	managedLogin := authRuntime["tenetManagedLogin"].(map[string]any)
	if managedLogin["configured"] != true || managedLogin["active"] != false || managedLogin["provider"] != "github" {
		t.Fatalf("expected configured but inactive managed-login diagnostic, got %#v", managedLogin)
	}
	if managedLogin["loginRoute"] != "/auth/login/github" || managedLogin["callbackRoute"] != "/auth/callback/github" || managedLogin["logoutRoute"] != "/auth/logout" || managedLogin["sessionCookieName"] != "tenet_session" {
		t.Fatalf("expected stable route/session diagnostics including logout, got %#v", managedLogin)
	}
	if managedLogin["stateStoreConfigured"] != true || managedLogin["identityStoreConfigured"] != true || managedLogin["sessionSecretConfigured"] != true {
		t.Fatalf("expected store/secret diagnostics, got %#v", managedLogin)
	}
	if managedLogin["sessionCookieSecureOverride"] != false || managedLogin["sessionCookieSecureForRequest"] != false {
		t.Fatalf("expected configured inactive local request to report non-secure cookie mode, got %#v", managedLogin)
	}
	assertManagedLoginRawLifecycleDiagnostics(t, managedLogin, false)
	if managedLogin["unavailableReason"] != "tenet-managed login configuration is present, but login/callback/session routes are not active for this API instance" {
		t.Fatalf("unexpected configured managed-login unavailable reason: %#v", managedLogin)
	}
	if !reflect.DeepEqual(jsonStringSlice(t, managedLogin["requiredSettings"]), RequiredTenetManagedLoginSettings()) || len(jsonStringSlice(t, managedLogin["missingSettings"])) != 0 {
		t.Fatalf("unexpected configured managed-login required/missing settings: %#v", managedLogin)
	}
}

func TestRouterOperatorContextReportsEnterpriseSSORuntimeDiagnostic(t *testing.T) {
	router := NewRouter(Options{
		AuthRuntime: AuthRuntimeDiagnostic{
			EnterpriseSSO: EnterpriseSSODiagnostic{
				Configured:              true,
				Provider:                "google_oidc",
				RequiredSettings:        []string{"TENET_AUTH_PROVIDER", "TENET_GOOGLE_OIDC_CLIENT_ID"},
				MissingSettings:         []string{},
				LoginRoute:              "/auth/login/google",
				CallbackRoute:           "/auth/callback/google",
				StateStoreConfigured:    true,
				IdentityStoreConfigured: true,
				SessionSecretConfigured: true,
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	authRuntime := body["authRuntime"].(map[string]any)
	enterpriseSSO := authRuntime["enterpriseSso"].(map[string]any)
	if enterpriseSSO["configured"] != true || enterpriseSSO["active"] != false || enterpriseSSO["provider"] != "google" {
		t.Fatalf("expected configured inactive enterprise SSO diagnostic, got %#v", enterpriseSSO)
	}
	if enterpriseSSO["loginRoute"] != "/auth/login/google" || enterpriseSSO["callbackRoute"] != "/auth/callback/google" || enterpriseSSO["logoutRoute"] != "/auth/logout" {
		t.Fatalf("expected enterprise SSO route diagnostics, got %#v", enterpriseSSO)
	}
	if enterpriseSSO["stateStoreConfigured"] != true || enterpriseSSO["identityStoreConfigured"] != true || enterpriseSSO["sessionSecretConfigured"] != true {
		t.Fatalf("expected enterprise SSO shared session-store diagnostics, got %#v", enterpriseSSO)
	}
	if got := resp.Body.String(); strings.Contains(got, "client-secret") || strings.Contains(got, "assertion") || strings.Contains(got, "access_token") {
		t.Fatalf("enterprise SSO diagnostic leaked secret/token-shaped data: %s", got)
	}
}

func TestRouterOperatorContextReportsEnterpriseSSOInvalidSettings(t *testing.T) {
	router := NewRouter(Options{
		AuthRuntime: AuthRuntimeDiagnostic{
			EnterpriseSSO: EnterpriseSSODiagnostic{
				Configured:              true,
				Provider:                "okta_saml",
				RequiredSettings:        []string{"TENET_AUTH_PROVIDER", "TENET_OKTA_SAML_ACS_URL"},
				MissingSettings:         []string{},
				InvalidSettings:         []string{"TENET_OKTA_SAML_ACS_URL"},
				LoginRoute:              "/auth/login/okta",
				CallbackRoute:           "/auth/callback/okta/saml",
				StateStoreConfigured:    true,
				IdentityStoreConfigured: true,
				SessionSecretConfigured: true,
				UnavailableReason:       "enterprise SSO provider configuration contains invalid non-secret URL settings",
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	authRuntime := body["authRuntime"].(map[string]any)
	enterpriseSSO := authRuntime["enterpriseSso"].(map[string]any)
	if enterpriseSSO["configured"] != false || enterpriseSSO["active"] != false || enterpriseSSO["provider"] != "okta_saml" {
		t.Fatalf("expected invalid enterprise SSO settings to force inactive diagnostic, got %#v", enterpriseSSO)
	}
	invalid, ok := enterpriseSSO["invalidSettings"].([]any)
	if !ok || len(invalid) != 1 || invalid[0] != "TENET_OKTA_SAML_ACS_URL" {
		t.Fatalf("expected raw JSON invalidSettings diagnostic, got %#v", enterpriseSSO["invalidSettings"])
	}
}

func TestRouterManagedLoginRoutesRequireActiveRuntime(t *testing.T) {
	router := NewRouter(Options{
		AuthRuntime: AuthRuntimeDiagnostic{
			TenetManagedLogin: TenetManagedLoginDiagnostic{
				Configured:              true,
				Provider:                "github",
				LoginRoute:              "/auth/login/github",
				CallbackRoute:           "/auth/callback/github",
				SessionCookieName:       "tenet_session",
				StateStoreConfigured:    true,
				IdentityStoreConfigured: true,
				SessionSecretConfigured: true,
			},
		},
	})

	for _, path := range []string{"/auth/login/github", "/auth/callback/github"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusServiceUnavailable || !strings.Contains(resp.Body.String(), "tenet-managed GitHub OAuth login is not configured") {
				t.Fatalf("expected inactive managed-login runtime to return 503 for %s, got %d: %s", path, resp.Code, resp.Body.String())
			}
		})
	}

	logoutReq := httptest.NewRequest(http.MethodGet, "/auth/logout?returnUrl=/", nil)
	logoutResp := httptest.NewRecorder()
	router.ServeHTTP(logoutResp, logoutReq)
	if logoutResp.Code != http.StatusNotFound {
		t.Fatalf("expected inactive managed-login runtime to keep logout route unavailable, got %d: %s", logoutResp.Code, logoutResp.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected legacy auth route to remain unregistered, got %d: %s", resp.Code, resp.Body.String())
	}
}

func jsonStringSlice(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("expected JSON string array, got %#v", value)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("expected JSON string array item, got %#v", item)
		}
		result = append(result, text)
	}
	return result
}

func slicesContain(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func assertManagedLoginRawLifecycleDiagnostics(t *testing.T, managedLogin map[string]any, expectLogout bool) {
	t.Helper()
	if managedLogin["oauthStateTtlSeconds"] != float64(DefaultTenetManagedLoginOAuthStateTTLSeconds()) {
		t.Fatalf("expected default OAuth state TTL diagnostic, got %#v", managedLogin)
	}
	if managedLogin["sessionTtlSeconds"] != float64(DefaultTenetManagedLoginSessionTTLSeconds()) || managedLogin["sessionExpiryEnforced"] != true {
		t.Fatalf("expected signed session TTL/expiry diagnostics, got %#v", managedLogin)
	}
	if managedLogin["sessionCookiePath"] != defaultAuthSessionCookiePath || managedLogin["sessionCookieHttpOnly"] != true || managedLogin["sessionCookieSameSite"] != defaultAuthSessionCookieSameSite {
		t.Fatalf("expected stable session cookie attribute diagnostics, got %#v", managedLogin)
	}
	if managedLogin["providerAccessTokenStored"] != false || managedLogin["providerAccessTokenReturned"] != false || managedLogin["providerTokenRefreshSupported"] != false || managedLogin["providerTokenRevocationSupported"] != false {
		t.Fatalf("expected provider-token diagnostics to remain non-secret/non-refreshing/non-revoking, got %#v", managedLogin)
	}
	if managedLogin["logoutClearsSessionCookie"] != expectLogout || managedLogin["logoutRevokesProviderToken"] != false {
		t.Fatalf("unexpected logout lifecycle diagnostics: %#v", managedLogin)
	}
}

func TestRouterOperatorContextReportsTrustedFrontDoorState(t *testing.T) {
	router := NewRouter(Options{
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		RequireOperatorRoleForRead:         true,
		RequireOperatorRoleForWrite:        true,
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
	req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
	req.Header.Add(requestactor.AuthenticatedRolesHeader, "operator, admin, operator")
	req.Header.Add(requestactor.AuthorizedDeploymentsHeader, "payments-api-prod, worker-prod")
	req.Header.Add(requestactor.AuthorizedDeploymentsHeader, "payments-api-prod")
	req.Header.Add(requestactor.AuthorizedProjectsHeader, "payments, platform, payments")
	req.Header.Add(requestactor.AuthorizedEnvironmentsHeader, "prod, staging")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	trustedHeaders, ok := body["trustedHeaders"].(map[string]any)
	if !ok {
		t.Fatalf("expected trustedHeaders object, got %#v", body["trustedHeaders"])
	}
	if trustedHeaders["enabled"] != true {
		t.Fatalf("expected trustedHeaders.enabled=true, got %#v", trustedHeaders)
	}
	if trustedHeaders["deploymentReadAuthorizationEnforced"] != true || trustedHeaders["operatorReadRoleEnforced"] != true || trustedHeaders["operatorWriteRoleEnforced"] != true {
		t.Fatalf("expected auth enforcement flags to be present, got %#v", trustedHeaders)
	}
	assertOperatorContextIdentityHeaderSupport(t, body)
	enforcement, ok := body["enforcement"].(map[string]any)
	if !ok {
		t.Fatalf("expected enforcement object, got %#v", body["enforcement"])
	}
	if enforcement["mode"] != "trusted_front_door_read_write" || enforcement["trustedHeadersEnabled"] != true || enforcement["readRoutesGuarded"] != true || enforcement["writeRoutesGuarded"] != true {
		t.Fatalf("expected read/write trusted-front-door enforcement summary, got %#v", enforcement)
	}
	if enforcement["deploymentScopedRoutesRequireAllowlist"] != true || enforcement["collectionReadsFiltered"] != true || enforcement["summary"] != "trusted front-door identity, roles, and deployment allowlists are enforcing guarded operator read/write routes" {
		t.Fatalf("expected allowlist/filtering enforcement details, got %#v", enforcement)
	}
	identity, ok := body["identity"].(map[string]any)
	if !ok {
		t.Fatalf("expected identity object, got %#v", body["identity"])
	}
	if identity["authenticated"] != true || identity["id"] != "front-door-123" || identity["name"] != "Front Door Operator" || identity["email"] != "frontdoor@example.com" {
		t.Fatalf("unexpected identity payload: %#v", identity)
	}
	identityProvenance, ok := body["identityProvenance"].(map[string]any)
	if !ok {
		t.Fatalf("expected identityProvenance object, got %#v", body["identityProvenance"])
	}
	if identityProvenance["idSource"] != "canonical" || identityProvenance["nameSource"] != "canonical" || identityProvenance["emailSource"] != "canonical" {
		t.Fatalf("expected canonical identity provenance, got %#v", identityProvenance)
	}
	roles, ok := body["roles"].(map[string]any)
	if !ok {
		t.Fatalf("expected roles object, got %#v", body["roles"])
	}
	if !reflect.DeepEqual(roles["names"], []any{"operator", "admin"}) || roles["canRead"] != true || roles["canWrite"] != true {
		t.Fatalf("unexpected roles payload: %#v", roles)
	}
	if !reflect.DeepEqual(roles["supportedNames"], []any{"viewer", "operator", "admin"}) || !reflect.DeepEqual(roles["recognizedNames"], []any{"operator", "admin"}) || !reflect.DeepEqual(roles["unrecognizedNames"], []any{}) {
		t.Fatalf("unexpected role diagnostics: %#v", roles)
	}
	if !reflect.DeepEqual(roles["supportedHeaderNames"], []any{"X-Tenet-Authenticated-Roles"}) {
		t.Fatalf("unexpected role header support: %#v", roles["supportedHeaderNames"])
	}
	rolesProvenance, ok := roles["provenance"].(map[string]any)
	if !ok || rolesProvenance["source"] != "canonical" {
		t.Fatalf("expected canonical role provenance, got %#v", roles["provenance"])
	}
	deploymentAccess, ok := body["deploymentAccess"].(map[string]any)
	if !ok {
		t.Fatalf("expected deploymentAccess object, got %#v", body["deploymentAccess"])
	}
	if deploymentAccess["enforced"] != true || deploymentAccess["forwarded"] != true || !reflect.DeepEqual(deploymentAccess["names"], []any{"payments-api-prod", "worker-prod"}) {
		t.Fatalf("unexpected deploymentAccess payload: %#v", deploymentAccess)
	}
	if !reflect.DeepEqual(deploymentAccess["supportedHeaderNames"], []any{"X-Tenet-Authorized-Deployments", "X-Tenet-Authorized-Projects", "X-Tenet-Authorized-Environments"}) {
		t.Fatalf("unexpected deployment access header support: %#v", deploymentAccess["supportedHeaderNames"])
	}
	scopes, ok := deploymentAccess["scopes"].(map[string]any)
	if !ok {
		t.Fatalf("expected deploymentAccess scopes object, got %#v", deploymentAccess["scopes"])
	}
	if scopes["forwarded"] != true || !reflect.DeepEqual(scopes["projects"], []any{"payments", "platform"}) || !reflect.DeepEqual(scopes["environments"], []any{"prod", "staging"}) {
		t.Fatalf("unexpected deploymentAccess scopes payload: %#v", scopes)
	}
	summary, ok := deploymentAccess["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected explicit unavailable deploymentAccess summary, got %#v", deploymentAccess["summary"])
	}
	if summary["available"] != false || summary["unavailableReason"] != "catalog placement provider is unavailable" || summary["forwardedCount"] != float64(2) {
		t.Fatalf("unexpected unavailable deploymentAccess summary: %#v", summary)
	}
	accessVerdicts, ok := body["accessVerdicts"].(map[string]any)
	if !ok {
		t.Fatalf("expected accessVerdicts object, got %#v", body["accessVerdicts"])
	}
	operatorRead := accessVerdicts["operatorRead"].(map[string]any)
	if operatorRead["allowed"] != true || operatorRead["requiresAuthenticatedIdentity"] != true || operatorRead["requiresViewerCapableRole"] != true {
		t.Fatalf("expected allowed read verdict with identity/read-role requirements, got %#v", operatorRead)
	}
	if !reflect.DeepEqual(operatorRead["exampleRoutes"], []any{"GET /v1/overview/applications", "GET /v1/status/deployments", "GET /v1/catalog/deployments", "GET /v1/audit-logs", "GET /v1/controller/refresh/status"}) {
		t.Fatalf("expected operator read route examples, got %#v", operatorRead["exampleRoutes"])
	}
	if !reflect.DeepEqual(operatorRead["requiredTrustedHeaders"], []any{"X-Tenet-Authenticated-Id", "X-User-ID", "X-Tenet-Authenticated-Roles"}) {
		t.Fatalf("expected operator read trusted headers, got %#v", operatorRead["requiredTrustedHeaders"])
	}
	if !reflect.DeepEqual(operatorRead["acceptedRoleNames"], []any{"viewer", "operator", "admin"}) {
		t.Fatalf("expected operator read accepted roles, got %#v", operatorRead["acceptedRoleNames"])
	}
	operatorWrite := accessVerdicts["operatorWrite"].(map[string]any)
	if operatorWrite["allowed"] != true || operatorWrite["requiresOperatorRole"] != true || operatorWrite["reason"] != "guarded operator write prerequisites are satisfied for this request" {
		t.Fatalf("expected allowed write verdict, got %#v", operatorWrite)
	}
	if !reflect.DeepEqual(operatorWrite["exampleRoutes"], []any{"POST /v1/plans/deployments/{deploymentName}", "POST /v1/applies/plans/{planId}", "POST /v1/applies/{applyId}/claim", "POST /v1/applies/{applyId}/heartbeat", "POST /v1/applies/{applyId}/transitions", "POST /v1/controller/refresh"}) {
		t.Fatalf("expected operator write route examples, got %#v", operatorWrite["exampleRoutes"])
	}
	if !reflect.DeepEqual(operatorWrite["requiredTrustedHeaders"], []any{"X-Tenet-Authenticated-Id", "X-User-ID", "X-Tenet-Authenticated-Roles"}) {
		t.Fatalf("expected operator write trusted headers, got %#v", operatorWrite["requiredTrustedHeaders"])
	}
	if !reflect.DeepEqual(operatorWrite["acceptedRoleNames"], []any{"operator", "admin"}) {
		t.Fatalf("expected operator write accepted roles, got %#v", operatorWrite["acceptedRoleNames"])
	}
	deploymentScopedRead := accessVerdicts["deploymentScopedRead"].(map[string]any)
	if deploymentScopedRead["allowed"] != true || deploymentScopedRead["requiresExactDeploymentAllowlistMatch"] != true || deploymentScopedRead["acceptsProjectEnvironmentScopes"] != true || deploymentScopedRead["reason"] != "deployment-scoped reads can pass for deployments named in X-Tenet-Authorized-Deployments or matched by X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments" {
		t.Fatalf("expected conditional deployment read verdict, got %#v", deploymentScopedRead)
	}
	if !reflect.DeepEqual(deploymentScopedRead["exampleRoutes"], []any{"GET /v1/overview/deployments/{deploymentName}", "GET /v1/status/deployments/{deploymentName}", "GET /v1/applies/{applyId}/preflight", "GET /v1/audit-logs?deployment={deploymentName}"}) {
		t.Fatalf("expected deployment-scoped read route examples, got %#v", deploymentScopedRead["exampleRoutes"])
	}
	if !reflect.DeepEqual(deploymentScopedRead["requiredTrustedHeaders"], []any{"X-Tenet-Authenticated-Id", "X-User-ID", "X-Tenet-Authenticated-Roles", "X-Tenet-Authorized-Deployments", "X-Tenet-Authorized-Projects", "X-Tenet-Authorized-Environments"}) {
		t.Fatalf("expected deployment-scoped read trusted headers, got %#v", deploymentScopedRead["requiredTrustedHeaders"])
	}
	deploymentScopedWrite := accessVerdicts["deploymentScopedWrite"].(map[string]any)
	if deploymentScopedWrite["allowed"] != true || deploymentScopedWrite["requiresExactDeploymentAllowlistMatch"] != true || deploymentScopedWrite["acceptsProjectEnvironmentScopes"] != true {
		t.Fatalf("expected conditional deployment write verdict, got %#v", deploymentScopedWrite)
	}
	if !reflect.DeepEqual(deploymentScopedWrite["exampleRoutes"], []any{"POST /v1/plans/deployments/{deploymentName}", "POST /v1/applies/plans/{planId}", "POST /v1/applies/{applyId}/claim", "POST /v1/applies/{applyId}/heartbeat", "POST /v1/applies/{applyId}/transitions"}) {
		t.Fatalf("expected deployment-scoped write route examples, got %#v", deploymentScopedWrite["exampleRoutes"])
	}
	if !reflect.DeepEqual(deploymentScopedWrite["requiredTrustedHeaders"], []any{"X-Tenet-Authenticated-Id", "X-User-ID", "X-Tenet-Authenticated-Roles", "X-Tenet-Authorized-Deployments", "X-Tenet-Authorized-Projects", "X-Tenet-Authorized-Environments"}) {
		t.Fatalf("expected deployment-scoped write trusted headers, got %#v", deploymentScopedWrite["requiredTrustedHeaders"])
	}
	collectionRead := accessVerdicts["collectionRead"].(map[string]any)
	if collectionRead["allowed"] != true || collectionRead["filtersCollectionsByDeploymentAccess"] != true {
		t.Fatalf("expected collection read verdict, got %#v", collectionRead)
	}
	if !reflect.DeepEqual(collectionRead["exampleRoutes"], []any{"GET /v1/overview/applications", "GET /v1/status/deployments", "GET /v1/catalog/deployments", "GET /v1/overview/accounts", "GET /v1/applies?status=active", "GET /v1/applies?status=active&claim=stale", "GET /v1/applies?status=queued", "GET /v1/audit-logs"}) {
		t.Fatalf("expected collection read route examples, got %#v", collectionRead["exampleRoutes"])
	}
	if !reflect.DeepEqual(collectionRead["requiredTrustedHeaders"], []any{"X-Tenet-Authenticated-Id", "X-User-ID", "X-Tenet-Authenticated-Roles", "X-Tenet-Authorized-Deployments", "X-Tenet-Authorized-Projects", "X-Tenet-Authorized-Environments"}) {
		t.Fatalf("expected collection read trusted headers, got %#v", collectionRead["requiredTrustedHeaders"])
	}
	catalogRead := accessVerdicts["catalogRead"].(map[string]any)
	if catalogRead["allowed"] != true || catalogRead["filtersCollectionsByDeploymentAccess"] != true || catalogRead["reason"] != "raw catalog reads are filtered to deployment-visible desired-state metadata before returning catalog collections or summaries" {
		t.Fatalf("expected raw catalog read verdict, got %#v", catalogRead)
	}
	if !reflect.DeepEqual(catalogRead["exampleRoutes"], []any{"GET /v1/catalog/deployments", "GET /v1/catalog/accounts", "GET /v1/catalog/placements", "GET /v1/catalog/topology", "GET /v1/catalog/summary"}) {
		t.Fatalf("expected raw catalog read route examples, got %#v", catalogRead["exampleRoutes"])
	}
	if !reflect.DeepEqual(catalogRead["requiredTrustedHeaders"], []any{"X-Tenet-Authenticated-Id", "X-User-ID", "X-Tenet-Authenticated-Roles", "X-Tenet-Authorized-Deployments", "X-Tenet-Authorized-Projects", "X-Tenet-Authorized-Environments"}) {
		t.Fatalf("expected raw catalog read trusted headers, got %#v", catalogRead["requiredTrustedHeaders"])
	}
}

func TestRouterOperatorContextReportsRepositoryPolicyEvaluation(t *testing.T) {
	called := false
	router := NewRouter(Options{
		TrustAuthenticatedHeaders: true,
		RepositoryPolicy: func(_ context.Context, identity requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
			called = true
			if identity.ID != "operator-123" || !reflect.DeepEqual(groups.Names, []string{"platform-admins", "sre"}) {
				t.Fatalf("unexpected repository-policy inputs: identity=%+v groups=%+v", identity, groups)
			}
			return rbacpolicy.Evaluate(rbacpolicy.Policy{
				Version: rbacpolicy.VersionV1,
				Users:   []rbacpolicy.Subject{{ID: "operator-123", Roles: []string{"viewer"}, Deployments: []string{"payments-api-prod"}}},
				Groups:  []rbacpolicy.GroupGrant{{Name: "platform-admins", Roles: []string{"operator"}, Projects: []string{"payments"}, Environments: []string{"prod"}}},
			}, identity, groups)
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "operator-123")
	req.Header.Set(requestactor.AuthenticatedHeaderEmail, "operator@example.com")
	req.Header.Add(requestactor.AuthenticatedGroupsHeader, "platform-admins, sre, platform-admins")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !called {
		t.Fatalf("expected repository policy provider to be called")
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	repositoryPolicy, ok := body["repositoryPolicy"].(map[string]any)
	if !ok {
		t.Fatalf("expected repositoryPolicy object, got %#v", body["repositoryPolicy"])
	}
	if repositoryPolicy["configured"] != true || repositoryPolicy["available"] != true || repositoryPolicy["source"] != "repository" || repositoryPolicy["diagnosticOnly"] != true || repositoryPolicy["canRead"] != true || repositoryPolicy["canWrite"] != true {
		t.Fatalf("unexpected repositoryPolicy flags: %#v", repositoryPolicy)
	}
	if !reflect.DeepEqual(repositoryPolicy["supportedGroupHeaderNames"], []any{"X-Tenet-Authenticated-Groups"}) {
		t.Fatalf("unexpected supported group header names: %#v", repositoryPolicy["supportedGroupHeaderNames"])
	}
	rolesPayload, ok := repositoryPolicy["roles"].(map[string]any)
	if !ok || !reflect.DeepEqual(rolesPayload["names"], []any{"viewer", "operator"}) {
		t.Fatalf("unexpected repositoryPolicy roles: %#v", repositoryPolicy["roles"])
	}
	deploymentAccess, ok := repositoryPolicy["deploymentAccess"].(map[string]any)
	if !ok || !reflect.DeepEqual(deploymentAccess["names"], []any{"payments-api-prod"}) {
		t.Fatalf("unexpected repositoryPolicy deployment access: %#v", repositoryPolicy["deploymentAccess"])
	}
	scopes, ok := repositoryPolicy["projectEnvironmentScopes"].(map[string]any)
	if !ok || !reflect.DeepEqual(scopes["projects"], []any{"payments"}) || !reflect.DeepEqual(scopes["environments"], []any{"prod"}) {
		t.Fatalf("unexpected repositoryPolicy scopes: %#v", repositoryPolicy["projectEnvironmentScopes"])
	}
	if matchedUsers, ok := repositoryPolicy["matchedUsers"].([]any); !ok || len(matchedUsers) != 1 {
		t.Fatalf("expected one matched user, got %#v", repositoryPolicy["matchedUsers"])
	}
	if matchedGroups, ok := repositoryPolicy["matchedGroups"].([]any); !ok || len(matchedGroups) != 1 {
		t.Fatalf("expected one matched group, got %#v", repositoryPolicy["matchedGroups"])
	}
}

func TestRouterOperatorContextReportsIdentityProvenanceSources(t *testing.T) {
	for _, tc := range []struct {
		name            string
		headers         map[string]string
		wantIDSource    string
		wantNameSource  string
		wantEmailSource string
		wantIdentity    map[string]any
	}{
		{
			name: "canonical only",
			headers: map[string]string{
				requestactor.AuthenticatedHeaderID:    "front-door-123",
				requestactor.AuthenticatedHeaderName:  "Front Door Operator",
				requestactor.AuthenticatedHeaderEmail: "frontdoor@example.com",
			},
			wantIDSource:    "canonical",
			wantNameSource:  "canonical",
			wantEmailSource: "canonical",
			wantIdentity: map[string]any{
				"authenticated": true,
				"id":            "front-door-123",
				"name":          "Front Door Operator",
				"email":         "frontdoor@example.com",
			},
		},
		{
			name: "alias only",
			headers: map[string]string{
				requestactor.HeaderAliasID:    "alias-456",
				requestactor.HeaderAliasName:  "Alias Operator",
				requestactor.HeaderAliasEmail: "alias@example.com",
			},
			wantIDSource:    "alias",
			wantNameSource:  "alias",
			wantEmailSource: "alias",
			wantIdentity: map[string]any{
				"authenticated": true,
				"id":            "alias-456",
				"name":          "Alias Operator",
				"email":         "alias@example.com",
			},
		},
		{
			name: "canonical wins over alias per field",
			headers: map[string]string{
				requestactor.AuthenticatedHeaderID: "front-door-123",
				requestactor.HeaderAliasID:         "alias-456",
				requestactor.HeaderAliasName:       "Alias Operator",
			},
			wantIDSource:    "canonical",
			wantNameSource:  "alias",
			wantEmailSource: "absent",
			wantIdentity: map[string]any{
				"authenticated": true,
				"id":            "front-door-123",
				"name":          "Alias Operator",
			},
		},
		{
			name:            "absent",
			wantIDSource:    "absent",
			wantNameSource:  "absent",
			wantEmailSource: "absent",
			wantIdentity: map[string]any{
				"authenticated": false,
				"reason":        requestactor.AuthenticatedIdentityUnavailableReason,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(Options{TrustAuthenticatedHeaders: true})
			req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
			for name, value := range tc.headers {
				req.Header.Set(name, value)
			}
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode operator context response: %v", err)
			}
			identity := body["identity"].(map[string]any)
			for key, want := range tc.wantIdentity {
				if identity[key] != want {
					t.Fatalf("identity[%s] = %#v, want %#v in %#v", key, identity[key], want, identity)
				}
			}
			provenance := body["identityProvenance"].(map[string]any)
			if provenance["idSource"] != tc.wantIDSource || provenance["nameSource"] != tc.wantNameSource || provenance["emailSource"] != tc.wantEmailSource {
				t.Fatalf("unexpected identity provenance: %#v", provenance)
			}
			assertOperatorContextIdentityHeaderSupport(t, body)
		})
	}
}

func TestRouterOperatorContextReportsActorAttributionDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name               string
		trustHeaders       bool
		headers            map[string]string
		wantActor          map[string]any
		wantIDSource       string
		wantNameSource     string
		wantEmailSource    string
		wantSource         string
		wantAvailable      bool
		wantSummarySnippet string
	}{
		{
			name: "no actor or trusted identity",
			wantActor: map[string]any{
				"available": false,
				"reason":    requestactor.UnavailableReason,
			},
			wantIDSource:       "absent",
			wantNameSource:     "absent",
			wantEmailSource:    "absent",
			wantSource:         "unavailable",
			wantAvailable:      false,
			wantSummarySnippet: "actor attribution does not authorize routes",
		},
		{
			name: "explicit actor headers win over aliases",
			headers: map[string]string{
				requestactor.HeaderID:         "operator-123",
				requestactor.HeaderName:       "Jane Operator",
				requestactor.HeaderEmail:      "jane@example.com",
				requestactor.HeaderAliasID:    "alias-456",
				requestactor.HeaderAliasName:  "Alias Operator",
				requestactor.HeaderAliasEmail: "alias@example.com",
			},
			wantActor: map[string]any{
				"available": true,
				"id":        "operator-123",
				"name":      "Jane Operator",
				"email":     "jane@example.com",
			},
			wantIDSource:       "canonical",
			wantNameSource:     "canonical",
			wantEmailSource:    "canonical",
			wantSource:         "explicit_actor_headers",
			wantAvailable:      true,
			wantSummarySnippet: "X-User-* actor aliases only filling missing actor fields",
		},
		{
			name: "alias actor headers",
			headers: map[string]string{
				requestactor.HeaderAliasID:    "alias-456",
				requestactor.HeaderAliasName:  "Alias Operator",
				requestactor.HeaderAliasEmail: "alias@example.com",
			},
			wantActor: map[string]any{
				"available": true,
				"id":        "alias-456",
				"name":      "Alias Operator",
				"email":     "alias@example.com",
			},
			wantIDSource:       "alias",
			wantNameSource:     "alias",
			wantEmailSource:    "alias",
			wantSource:         "explicit_actor_headers",
			wantAvailable:      true,
			wantSummarySnippet: "explicit X-Tenet-Actor-* headers",
		},
		{
			name:         "trusted authenticated identity projection",
			trustHeaders: true,
			headers: map[string]string{
				requestactor.AuthenticatedHeaderID:    "front-door-123",
				requestactor.AuthenticatedHeaderName:  "Front Door Operator",
				requestactor.AuthenticatedHeaderEmail: "frontdoor@example.com",
			},
			wantActor: map[string]any{
				"available": true,
				"id":        "front-door-123",
				"name":      "Front Door Operator",
				"email":     "frontdoor@example.com",
			},
			wantIDSource:       "absent",
			wantNameSource:     "absent",
			wantEmailSource:    "absent",
			wantSource:         "trusted_authenticated_identity",
			wantAvailable:      true,
			wantSummarySnippet: "project from trusted authenticated identity",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(Options{TrustAuthenticatedHeaders: tc.trustHeaders})
			req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
			for name, value := range tc.headers {
				req.Header.Set(name, value)
			}
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode operator context response: %v", err)
			}
			actor := body["actor"].(map[string]any)
			for key, want := range tc.wantActor {
				if actor[key] != want {
					t.Fatalf("actor[%s] = %#v, want %#v in %#v", key, actor[key], want, actor)
				}
			}
			assertOperatorContextActorHeaderSupport(t, body)
			provenance := body["actorProvenance"].(map[string]any)
			if provenance["idSource"] != tc.wantIDSource || provenance["nameSource"] != tc.wantNameSource || provenance["emailSource"] != tc.wantEmailSource {
				t.Fatalf("unexpected actor provenance: %#v", provenance)
			}
			attribution := body["actorAttribution"].(map[string]any)
			if attribution["available"] != tc.wantAvailable || attribution["source"] != tc.wantSource {
				t.Fatalf("unexpected actor attribution: %#v", attribution)
			}
			wantPrecedence := []any{"explicit_actor_headers", "actor_alias_headers", "trusted_authenticated_identity", "unavailable"}
			if !reflect.DeepEqual(attribution["precedence"], wantPrecedence) {
				t.Fatalf("unexpected actor attribution precedence: %#v", attribution)
			}
			if summary, _ := attribution["summary"].(string); !strings.Contains(summary, tc.wantSummarySnippet) {
				t.Fatalf("expected actor attribution summary to contain %q, got %#v", tc.wantSummarySnippet, attribution)
			}
		})
	}
}

func assertOperatorContextActorHeaderSupport(t *testing.T, body map[string]any) {
	t.Helper()
	support, ok := body["actorHeaderSupport"].(map[string]any)
	if !ok {
		t.Fatalf("expected actorHeaderSupport object, got %#v", body["actorHeaderSupport"])
	}
	if !reflect.DeepEqual(support["canonical"], []any{"X-Tenet-Actor-Id", "X-Tenet-Actor-Name", "X-Tenet-Actor-Email"}) {
		t.Fatalf("unexpected canonical actor header support: %#v", support["canonical"])
	}
	if !reflect.DeepEqual(support["aliases"], []any{"X-User-ID", "X-User-Name", "X-User-Email"}) {
		t.Fatalf("unexpected alias actor header support: %#v", support["aliases"])
	}
}

func assertOperatorContextIdentityHeaderSupport(t *testing.T, body map[string]any) {
	t.Helper()
	support, ok := body["identityHeaderSupport"].(map[string]any)
	if !ok {
		t.Fatalf("expected identityHeaderSupport object, got %#v", body["identityHeaderSupport"])
	}
	if !reflect.DeepEqual(support["canonical"], []any{"X-Tenet-Authenticated-Id", "X-Tenet-Authenticated-Name", "X-Tenet-Authenticated-Email"}) {
		t.Fatalf("unexpected canonical identity header support: %#v", support["canonical"])
	}
	if !reflect.DeepEqual(support["aliases"], []any{"X-User-ID", "X-User-Name", "X-User-Email"}) {
		t.Fatalf("unexpected alias identity header support: %#v", support["aliases"])
	}
}

func TestRouterOperatorContextReportsRoleProvenance(t *testing.T) {
	for _, tc := range []struct {
		name             string
		header           string
		wantSource       string
		wantRoles        []any
		wantRecognized   []any
		wantUnrecognized []any
		wantCanRead      bool
		wantCanWrite     bool
		wantNamesOmitted bool
	}{
		{
			name:             "only supported roles",
			header:           "viewer, operator",
			wantSource:       "canonical",
			wantRoles:        []any{"viewer", "operator"},
			wantRecognized:   []any{"viewer", "operator"},
			wantUnrecognized: []any{},
			wantCanRead:      true,
			wantCanWrite:     true,
		},
		{
			name:             "only unsupported roles",
			header:           "auditor, security-reviewer",
			wantSource:       "canonical",
			wantRoles:        []any{"auditor", "security-reviewer"},
			wantRecognized:   []any{},
			wantUnrecognized: []any{"auditor", "security-reviewer"},
		},
		{
			name:             "mixed supported and unsupported roles",
			header:           "viewer, auditor, admin",
			wantSource:       "canonical",
			wantRoles:        []any{"viewer", "auditor", "admin"},
			wantRecognized:   []any{"viewer", "admin"},
			wantUnrecognized: []any{"auditor"},
			wantCanRead:      true,
			wantCanWrite:     true,
		},
		{
			name:             "empty roles",
			header:           " , ",
			wantSource:       "absent",
			wantRecognized:   []any{},
			wantUnrecognized: []any{},
			wantNamesOmitted: true,
		},
		{
			name:             "absent roles",
			wantSource:       "absent",
			wantRecognized:   []any{},
			wantUnrecognized: []any{},
			wantNamesOmitted: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(Options{TrustAuthenticatedHeaders: true})
			req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
			if tc.header != "" {
				req.Header.Set(requestactor.AuthenticatedRolesHeader, tc.header)
			}
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode operator context response: %v", err)
			}
			roles := body["roles"].(map[string]any)
			provenance := roles["provenance"].(map[string]any)
			if provenance["source"] != tc.wantSource {
				t.Fatalf("unexpected role provenance: %#v", provenance)
			}
			if tc.wantNamesOmitted {
				if _, ok := roles["names"]; ok {
					t.Fatalf("expected role names to be omitted, got %#v", roles["names"])
				}
			} else if !reflect.DeepEqual(roles["names"], tc.wantRoles) {
				t.Fatalf("unexpected role names: got %#v want %#v", roles["names"], tc.wantRoles)
			}
			if !reflect.DeepEqual(roles["supportedNames"], []any{"viewer", "operator", "admin"}) {
				t.Fatalf("unexpected supported role names: %#v", roles["supportedNames"])
			}
			if !reflect.DeepEqual(roles["supportedHeaderNames"], []any{"X-Tenet-Authenticated-Roles"}) {
				t.Fatalf("unexpected supported role header names: %#v", roles["supportedHeaderNames"])
			}
			if !reflect.DeepEqual(roles["recognizedNames"], tc.wantRecognized) {
				t.Fatalf("unexpected recognized role names: got %#v want %#v", roles["recognizedNames"], tc.wantRecognized)
			}
			if !reflect.DeepEqual(roles["unrecognizedNames"], tc.wantUnrecognized) {
				t.Fatalf("unexpected unrecognized role names: got %#v want %#v", roles["unrecognizedNames"], tc.wantUnrecognized)
			}
			if roles["canRead"] != tc.wantCanRead || roles["canWrite"] != tc.wantCanWrite {
				t.Fatalf("unexpected role capability booleans: %#v", roles)
			}
		})
	}
}

func TestRouterOperatorContextSummarizesMatchedAndUnmatchedDeploymentAccess(t *testing.T) {
	router := NewRouter(Options{
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		CatalogPlacements: func(context.Context) ([]catalog.PlacementView, error) {
			return []catalog.PlacementView{
				{DeploymentName: "payments-api-prod", AppRef: "payments", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "us-west-2"},
				{DeploymentName: "payments-worker-prod", AppRef: "payments", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "us-west-2"},
				{DeploymentName: "unforwarded-api", AppRef: "internal", EnvironmentRef: "staging", AWSAccountID: "210987654321", Region: "us-east-1"},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Add(requestactor.AuthorizedDeploymentsHeader, "payments-api-prod, stale-api")
	req.Header.Add(requestactor.AuthorizedDeploymentsHeader, "payments-worker-prod, archived-api")
	req.Header.Set(requestactor.AuthorizedProjectsHeader, "payments")
	req.Header.Set(requestactor.AuthorizedEnvironmentsHeader, "prod")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	deploymentAccess := body["deploymentAccess"].(map[string]any)
	if !reflect.DeepEqual(deploymentAccess["names"], []any{"payments-api-prod", "stale-api", "payments-worker-prod", "archived-api"}) {
		t.Fatalf("expected forwarded deployment names to stay raw and ordered, got %#v", deploymentAccess["names"])
	}
	summary, ok := deploymentAccess["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected deploymentAccess summary object, got %#v", deploymentAccess["summary"])
	}
	if summary["available"] != true {
		t.Fatalf("expected available summary, got %#v", summary)
	}
	expected := map[string]any{
		"forwardedCount":                 float64(4),
		"matchedCount":                   float64(2),
		"exactDeploymentMatchedCount":    float64(2),
		"projectEnvironmentMatchedCount": float64(2),
		"unmatchedCount":                 float64(2),
		"accountCount":                   float64(1),
		"projectCount":                   float64(1),
		"environmentCount":               float64(1),
	}
	for key, want := range expected {
		if summary[key] != want {
			t.Fatalf("expected summary[%s]=%#v, got %#v in %#v", key, want, summary[key], summary)
		}
	}
	if !reflect.DeepEqual(summary["unmatchedNames"], []any{"archived-api", "stale-api"}) {
		t.Fatalf("expected deterministic unmatched names, got %#v", summary["unmatchedNames"])
	}
	if _, ok := summary["unmatchedNamesTruncated"]; ok {
		t.Fatalf("expected unmatched names truncation flag to be omitted when the sample is complete, got %#v", summary)
	}
	examples, ok := summary["projectEnvironmentExamples"].([]any)
	if !ok || len(examples) != 1 {
		t.Fatalf("expected one project/environment diagnostic example, got %#v", summary["projectEnvironmentExamples"])
	}
	example := examples[0].(map[string]any)
	if example["appRef"] != "payments" || example["environmentRef"] != "prod" || example["region"] != "us-west-2" || example["awsAccountId"] != "123456789012" || example["deploymentCount"] != float64(2) {
		t.Fatalf("unexpected project/environment example metadata: %#v", example)
	}
	if !reflect.DeepEqual(example["exampleDeployments"], []any{"payments-api-prod", "payments-worker-prod"}) {
		t.Fatalf("expected deterministic visible deployment examples, got %#v", example["exampleDeployments"])
	}
	visibleDeployments, ok := summary["visibleDeploymentExamples"].([]any)
	if !ok || len(visibleDeployments) != 2 {
		t.Fatalf("expected two visible deployment examples, got %#v", summary["visibleDeploymentExamples"])
	}
	firstVisible := visibleDeployments[0].(map[string]any)
	if firstVisible["deploymentName"] != "payments-api-prod" || firstVisible["appRef"] != "payments" || firstVisible["environmentRef"] != "prod" || firstVisible["region"] != "us-west-2" || firstVisible["awsAccountId"] != "123456789012" {
		t.Fatalf("unexpected first visible deployment example metadata: %#v", firstVisible)
	}
	if !reflect.DeepEqual(firstVisible["matchSources"], []any{"exactDeployment", "projectEnvironment"}) {
		t.Fatalf("expected combined exact deployment and project/environment match sources, got %#v", firstVisible["matchSources"])
	}
	if _, ok := summary["emptyReason"]; ok {
		t.Fatalf("expected no empty reason when forwarded deployments match, got %#v", summary)
	}
	if _, ok := summary["unavailableReason"]; ok {
		t.Fatalf("expected no unavailable reason for available summary, got %#v", summary)
	}
	accessVerdicts := body["accessVerdicts"].(map[string]any)
	collectionRead := accessVerdicts["collectionRead"].(map[string]any)
	if collectionRead["allowed"] != true || collectionRead["filtersCollectionsByDeploymentAccess"] != true || collectionRead["emptyCollectionLikely"] != nil {
		t.Fatalf("expected collection read verdict to report filtering without likely-empty warning, got %#v", collectionRead)
	}
}

func TestRouterOperatorContextSummarizesProjectEnvironmentScopedAccess(t *testing.T) {
	router := NewRouter(Options{
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		CatalogPlacements: func(context.Context) ([]catalog.PlacementView, error) {
			return []catalog.PlacementView{
				{DeploymentName: "payments-api-prod", AppRef: "payments", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "us-west-2"},
				{DeploymentName: "payments-api-staging", AppRef: "payments", EnvironmentRef: "staging", AWSAccountID: "210987654321", Region: "us-east-1"},
				{DeploymentName: "worker-prod", AppRef: "workers", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "us-west-2"},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Set(requestactor.AuthorizedProjectsHeader, "payments")
	req.Header.Set(requestactor.AuthorizedEnvironmentsHeader, "prod, staging")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	deploymentAccess := body["deploymentAccess"].(map[string]any)
	scopes := deploymentAccess["scopes"].(map[string]any)
	if scopes["forwarded"] != true || !reflect.DeepEqual(scopes["projects"], []any{"payments"}) || !reflect.DeepEqual(scopes["environments"], []any{"prod", "staging"}) {
		t.Fatalf("unexpected forwarded project/environment scopes: %#v", scopes)
	}
	summary := deploymentAccess["summary"].(map[string]any)
	if summary["available"] != true || summary["forwardedCount"] != float64(0) || summary["matchedCount"] != float64(2) || summary["projectEnvironmentMatchedCount"] != float64(2) || summary["projectCount"] != float64(1) || summary["environmentCount"] != float64(2) {
		t.Fatalf("unexpected project/environment access summary: %#v", summary)
	}
	if _, ok := summary["exactDeploymentMatchedCount"]; ok {
		t.Fatalf("expected exact-deployment match count to be omitted when no exact deployment grants matched, got %#v", summary)
	}
	examples := summary["projectEnvironmentExamples"].([]any)
	if len(examples) != 2 {
		t.Fatalf("expected two scoped project/environment examples, got %#v", examples)
	}
	visibleDeployments := summary["visibleDeploymentExamples"].([]any)
	if len(visibleDeployments) != 2 {
		t.Fatalf("expected two scoped visible deployment examples, got %#v", visibleDeployments)
	}
	firstVisible := visibleDeployments[0].(map[string]any)
	if firstVisible["deploymentName"] != "payments-api-prod" || !reflect.DeepEqual(firstVisible["matchSources"], []any{"projectEnvironment"}) {
		t.Fatalf("expected project/environment match source for scoped deployment, got %#v", firstVisible)
	}
}

func TestRouterOperatorContextSummaryReportsUnmatchedProjectEnvironmentScopes(t *testing.T) {
	router := NewRouter(Options{
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		CatalogPlacements: func(context.Context) ([]catalog.PlacementView, error) {
			return []catalog.PlacementView{
				{DeploymentName: "payments-api-prod", AppRef: "payments", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "us-west-2"},
				{DeploymentName: "payments-api-staging", AppRef: "payments", EnvironmentRef: "staging", AWSAccountID: "210987654321", Region: "us-east-1"},
				{DeploymentName: "worker-prod", AppRef: "workers", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "us-west-2"},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Set(requestactor.AuthorizedProjectsHeader, "payments, archived")
	req.Header.Set(requestactor.AuthorizedEnvironmentsHeader, "prod, qa")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	deploymentAccess := body["deploymentAccess"].(map[string]any)
	summary := deploymentAccess["summary"].(map[string]any)
	expected := map[string]any{
		"forwardedProjectCount":          float64(2),
		"forwardedEnvironmentCount":      float64(2),
		"matchedCount":                   float64(1),
		"projectEnvironmentMatchedCount": float64(1),
		"unmatchedProjectCount":          float64(1),
		"unmatchedEnvironmentCount":      float64(1),
		"projectCount":                   float64(1),
		"environmentCount":               float64(1),
	}
	for key, want := range expected {
		if summary[key] != want {
			t.Fatalf("expected summary[%s]=%#v, got %#v in %#v", key, want, summary[key], summary)
		}
	}
	if !reflect.DeepEqual(summary["unmatchedProjects"], []any{"archived"}) {
		t.Fatalf("expected deterministic unmatched project scope sample, got %#v", summary["unmatchedProjects"])
	}
	if !reflect.DeepEqual(summary["unmatchedEnvironments"], []any{"qa"}) {
		t.Fatalf("expected deterministic unmatched environment scope sample, got %#v", summary["unmatchedEnvironments"])
	}
	if _, ok := summary["unmatchedProjectsTruncated"]; ok {
		t.Fatalf("expected unmatched project truncation flag to be omitted when sample is complete, got %#v", summary)
	}
	if _, ok := summary["unmatchedEnvironmentsTruncated"]; ok {
		t.Fatalf("expected unmatched environment truncation flag to be omitted when sample is complete, got %#v", summary)
	}
}

func TestRouterOperatorContextSummaryReportsBoundedUnmatchedDeploymentSample(t *testing.T) {
	router := NewRouter(Options{
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		CatalogPlacements: func(context.Context) ([]catalog.PlacementView, error) {
			return []catalog.PlacementView{
				{DeploymentName: "payments-api-prod", AppRef: "payments", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "us-west-2"},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Add(requestactor.AuthorizedDeploymentsHeader, "payments-api-prod")
	for i := 0; i < operatorContextUnmatchedDeploymentLimit+3; i++ {
		req.Header.Add(requestactor.AuthorizedDeploymentsHeader, fmt.Sprintf("stale-api-%02d", i))
	}
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	deploymentAccess := body["deploymentAccess"].(map[string]any)
	summary, ok := deploymentAccess["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected deploymentAccess summary object, got %#v", deploymentAccess["summary"])
	}
	if summary["available"] != true || summary["forwardedCount"] != float64(operatorContextUnmatchedDeploymentLimit+4) || summary["matchedCount"] != float64(1) || summary["unmatchedCount"] != float64(operatorContextUnmatchedDeploymentLimit+3) {
		t.Fatalf("unexpected bounded unmatched summary counts: %#v", summary)
	}
	unmatchedNames, ok := summary["unmatchedNames"].([]any)
	if !ok || len(unmatchedNames) != operatorContextUnmatchedDeploymentLimit {
		t.Fatalf("expected bounded unmatchedNames sample of %d entries, got %#v", operatorContextUnmatchedDeploymentLimit, summary["unmatchedNames"])
	}
	if unmatchedNames[0] != "stale-api-00" || unmatchedNames[len(unmatchedNames)-1] != "stale-api-24" {
		t.Fatalf("expected deterministic sorted unmatchedNames sample, got %#v", unmatchedNames)
	}
	if summary["unmatchedNamesTruncated"] != true {
		t.Fatalf("expected unmatchedNamesTruncated=true for bounded sample, got %#v", summary)
	}
}

func TestRouterOperatorContextVerdictsExplainDeniedAndEmptyCollectionCases(t *testing.T) {
	t.Run("missing identity matches guarded read unauthorized path", func(t *testing.T) {
		router := NewRouter(Options{
			TrustAuthenticatedHeaders:          true,
			RequireDeploymentReadAuthorization: true,
			RequireOperatorRoleForRead:         true,
			RequireOperatorRoleForWrite:        true,
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected diagnostic context to stay readable, got %d: %s", resp.Code, resp.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode operator context response: %v", err)
		}
		accessVerdicts := body["accessVerdicts"].(map[string]any)
		operatorRead := accessVerdicts["operatorRead"].(map[string]any)
		if operatorRead["allowed"] != false || operatorRead["reason"] != "authenticated identity is required for guarded operator read routes" {
			t.Fatalf("expected read verdict to mirror guarded 401 reason, got %#v", operatorRead)
		}
		operatorWrite := accessVerdicts["operatorWrite"].(map[string]any)
		if operatorWrite["allowed"] != false || operatorWrite["reason"] != "authenticated identity is required for guarded operator write routes" {
			t.Fatalf("expected write verdict to mirror guarded 401 reason, got %#v", operatorWrite)
		}
	})

	t.Run("missing allowlist explains empty collection and deployment 403 risk", func(t *testing.T) {
		router := NewRouter(Options{
			TrustAuthenticatedHeaders:          true,
			RequireDeploymentReadAuthorization: true,
			RequireOperatorRoleForRead:         true,
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedRolesHeader, "viewer")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected diagnostic context to stay readable, got %d: %s", resp.Code, resp.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode operator context response: %v", err)
		}
		accessVerdicts := body["accessVerdicts"].(map[string]any)
		deploymentScopedRead := accessVerdicts["deploymentScopedRead"].(map[string]any)
		if deploymentScopedRead["allowed"] != false || deploymentScopedRead["reason"] != "deployment-scoped reads will return 403 unless X-Tenet-Authorized-Deployments names the deployment or project/environment scopes match catalog placement" {
			t.Fatalf("expected deployment read verdict to explain deployment or scope requirement, got %#v", deploymentScopedRead)
		}
		collectionRead := accessVerdicts["collectionRead"].(map[string]any)
		if collectionRead["allowed"] != true || collectionRead["emptyCollectionLikely"] != true || collectionRead["reason"] != "collection reads will be allowed but return empty collections until deployment or project/environment scopes forward at least one visible deployment" {
			t.Fatalf("expected collection verdict to explain empty filtered collections, got %#v", collectionRead)
		}
	})

	t.Run("partial project environment scopes explain deployment 403 risk", func(t *testing.T) {
		router := NewRouter(Options{
			TrustAuthenticatedHeaders:          true,
			RequireDeploymentReadAuthorization: true,
			RequireOperatorRoleForRead:         true,
			CatalogPlacements: func(context.Context) ([]catalog.PlacementView, error) {
				return []catalog.PlacementView{
					{DeploymentName: "payments-api-prod", AppRef: "payments", EnvironmentRef: "prod"},
				}, nil
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedRolesHeader, "viewer")
		req.Header.Set(requestactor.AuthorizedProjectsHeader, "payments")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected diagnostic context to stay readable, got %d: %s", resp.Code, resp.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode operator context response: %v", err)
		}
		accessVerdicts := body["accessVerdicts"].(map[string]any)
		deploymentScopedRead := accessVerdicts["deploymentScopedRead"].(map[string]any)
		if deploymentScopedRead["allowed"] != false || deploymentScopedRead["reason"] != "deployment-scoped reads need both X-Tenet-Authorized-Projects and X-Tenet-Authorized-Environments when no exact deployment allowlist is forwarded" {
			t.Fatalf("expected partial project/environment scopes to explain missing paired scope, got %#v", deploymentScopedRead)
		}
		collectionRead := accessVerdicts["collectionRead"].(map[string]any)
		if collectionRead["allowed"] != true || collectionRead["emptyCollectionLikely"] != true || collectionRead["reason"] != "collection reads will be allowed but likely empty because the forwarded deployment grants or project/environment scopes do not match catalog deployments" {
			t.Fatalf("expected partial scope collection verdict to warn likely empty, got %#v", collectionRead)
		}
	})

	t.Run("nonmatching project environment scopes explain deployment 403 risk", func(t *testing.T) {
		router := NewRouter(Options{
			TrustAuthenticatedHeaders:          true,
			RequireDeploymentReadAuthorization: true,
			RequireOperatorRoleForRead:         true,
			CatalogPlacements: func(context.Context) ([]catalog.PlacementView, error) {
				return []catalog.PlacementView{
					{DeploymentName: "payments-api-prod", AppRef: "payments", EnvironmentRef: "prod"},
				}, nil
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedRolesHeader, "viewer")
		req.Header.Set(requestactor.AuthorizedProjectsHeader, "payments")
		req.Header.Set(requestactor.AuthorizedEnvironmentsHeader, "dev")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected diagnostic context to stay readable, got %d: %s", resp.Code, resp.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode operator context response: %v", err)
		}
		accessVerdicts := body["accessVerdicts"].(map[string]any)
		deploymentScopedRead := accessVerdicts["deploymentScopedRead"].(map[string]any)
		if deploymentScopedRead["allowed"] != false || deploymentScopedRead["reason"] != "deployment-scoped reads using only project/environment scopes will return 403 because those scopes do not match catalog deployments" {
			t.Fatalf("expected nonmatching project/environment scopes to explain no catalog match, got %#v", deploymentScopedRead)
		}
		collectionRead := accessVerdicts["collectionRead"].(map[string]any)
		if collectionRead["allowed"] != true || collectionRead["emptyCollectionLikely"] != true || collectionRead["reason"] != "collection reads will be allowed but likely empty because the forwarded deployment grants or project/environment scopes do not match catalog deployments" {
			t.Fatalf("expected nonmatching scope collection verdict to warn likely empty, got %#v", collectionRead)
		}
	})
}

func TestRouterOperatorContextSummaryReportsEmptyVisibleProjectEnvironmentSet(t *testing.T) {
	router := NewRouter(Options{
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		CatalogPlacements: func(context.Context) ([]catalog.PlacementView, error) {
			return []catalog.PlacementView{
				{DeploymentName: "payments-api-prod", AppRef: "payments", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "us-west-2"},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Add(requestactor.AuthorizedDeploymentsHeader, "stale-api")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	deploymentAccess := body["deploymentAccess"].(map[string]any)
	summary, ok := deploymentAccess["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected deploymentAccess summary object, got %#v", deploymentAccess["summary"])
	}
	if summary["available"] != true || summary["forwardedCount"] != float64(1) || summary["matchedCount"] != float64(0) || summary["unmatchedCount"] != float64(1) || summary["emptyReason"] != "no forwarded deployment names or project/environment scopes match catalog placements" {
		t.Fatalf("unexpected empty visible summary: %#v", summary)
	}
	if !reflect.DeepEqual(summary["unmatchedNames"], []any{"stale-api"}) {
		t.Fatalf("expected stale forwarded deployment as unmatched, got %#v", summary["unmatchedNames"])
	}
	if _, ok := summary["projectEnvironmentExamples"]; ok {
		t.Fatalf("expected omitted examples when no project/environment pair is visible, got %#v", summary["projectEnvironmentExamples"])
	}
	accessVerdicts := body["accessVerdicts"].(map[string]any)
	collectionRead := accessVerdicts["collectionRead"].(map[string]any)
	if collectionRead["emptyCollectionLikely"] != true || collectionRead["reason"] != "collection reads will be allowed but likely empty because the forwarded deployment grants or project/environment scopes do not match catalog deployments" {
		t.Fatalf("expected likely-empty collection verdict, got %#v", collectionRead)
	}
}

func TestRouterOperatorContextSummaryReportsCatalogLookupFailure(t *testing.T) {
	router := NewRouter(Options{
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		CatalogPlacements: func(context.Context) ([]catalog.PlacementView, error) {
			return nil, errors.New("catalog unavailable")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Add(requestactor.AuthorizedDeploymentsHeader, "payments-api-prod")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	deploymentAccess := body["deploymentAccess"].(map[string]any)
	summary, ok := deploymentAccess["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected deploymentAccess summary object, got %#v", deploymentAccess["summary"])
	}
	if summary["available"] != false || summary["unavailableReason"] != "catalog placement lookup failed" || summary["forwardedCount"] != float64(1) {
		t.Fatalf("unexpected catalog failure summary: %#v", summary)
	}
	accessVerdicts := body["accessVerdicts"].(map[string]any)
	collectionRead := accessVerdicts["collectionRead"].(map[string]any)
	if collectionRead["emptyCollectionLikely"] != nil {
		t.Fatalf("exact deployment grant should keep collection verdict from claiming empty despite catalog lookup failure, got %#v", collectionRead)
	}
}

func TestRouterOperatorContextVerdictsReportProjectEnvironmentScopesWithoutCatalogPlacementData(t *testing.T) {
	tests := []struct {
		name                  string
		catalogPlacements     func(context.Context) ([]catalog.PlacementView, error)
		wantUnavailableReason string
	}{
		{
			name:                  "missing catalog placement provider",
			wantUnavailableReason: "catalog placement provider is unavailable",
		},
		{
			name: "catalog placement provider fails",
			catalogPlacements: func(context.Context) ([]catalog.PlacementView, error) {
				return nil, errors.New("catalog unavailable")
			},
			wantUnavailableReason: "catalog placement lookup failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(Options{
				TrustAuthenticatedHeaders:          true,
				RequireDeploymentReadAuthorization: true,
				RequireOperatorRoleForRead:         true,
				CatalogPlacements:                  tt.catalogPlacements,
			})
			req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
			req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
			req.Header.Set(requestactor.AuthenticatedRolesHeader, "viewer")
			req.Header.Set(requestactor.AuthorizedProjectsHeader, "payments")
			req.Header.Set(requestactor.AuthorizedEnvironmentsHeader, "prod")
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode operator context response: %v", err)
			}
			deploymentAccess := body["deploymentAccess"].(map[string]any)
			summary, ok := deploymentAccess["summary"].(map[string]any)
			if !ok {
				t.Fatalf("expected deploymentAccess summary object, got %#v", deploymentAccess["summary"])
			}
			if summary["available"] != false || summary["unavailableReason"] != tt.wantUnavailableReason || summary["forwardedProjectCount"] != float64(1) || summary["forwardedEnvironmentCount"] != float64(1) {
				t.Fatalf("unexpected unavailable project/environment summary: %#v", summary)
			}
			accessVerdicts := body["accessVerdicts"].(map[string]any)
			collectionRead := accessVerdicts["collectionRead"].(map[string]any)
			if collectionRead["allowed"] != true || collectionRead["filtersCollectionsByDeploymentAccess"] != true || collectionRead["emptyCollectionLikely"] != true || collectionRead["reason"] != "collection reads will be allowed but likely empty because catalog placement data is unavailable to match the forwarded project/environment scopes" {
				t.Fatalf("expected collection verdict to report likely-empty unavailable placement data, got %#v", collectionRead)
			}
			catalogRead := accessVerdicts["catalogRead"].(map[string]any)
			if catalogRead["allowed"] != true || catalogRead["filtersCollectionsByDeploymentAccess"] != true || catalogRead["emptyCollectionLikely"] != true || catalogRead["reason"] != "collection reads will be allowed but likely empty because catalog placement data is unavailable to match the forwarded project/environment scopes" {
				t.Fatalf("expected catalog verdict to report likely-empty unavailable placement data, got %#v", catalogRead)
			}
		})
	}
}

func TestRouterOperatorContextEnforcementReportsTrustedHeaderModes(t *testing.T) {
	tests := []struct {
		name       string
		options    Options
		wantMode   string
		wantRead   bool
		wantWrite  bool
		wantFilter bool
	}{
		{
			name:     "observed only",
			options:  Options{TrustAuthenticatedHeaders: true},
			wantMode: "trusted_headers_observed_only",
		},
		{
			name:       "write only",
			options:    Options{TrustAuthenticatedHeaders: true, RequireAuthenticatedIdentity: true, RequireOperatorRoleForWrite: true},
			wantMode:   "trusted_front_door_write_only",
			wantWrite:  true,
			wantFilter: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(tt.options)
			req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode operator context response: %v", err)
			}
			enforcement, ok := body["enforcement"].(map[string]any)
			if !ok {
				t.Fatalf("expected enforcement object, got %#v", body["enforcement"])
			}
			if enforcement["mode"] != tt.wantMode || enforcement["readRoutesGuarded"] != tt.wantRead || enforcement["writeRoutesGuarded"] != tt.wantWrite || enforcement["collectionReadsFiltered"] != tt.wantFilter {
				t.Fatalf("unexpected enforcement payload for %s: %#v", tt.name, enforcement)
			}
		})
	}
}

func TestRouterOperatorContextIsExplicitWhenTrustedHeadersAreUnavailable(t *testing.T) {
	router := NewRouter(Options{})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "ignored-when-untrusted")
	req.Header.Set(requestactor.AuthenticatedRolesHeader, "admin")
	req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "payments-api-prod")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context response: %v", err)
	}
	trustedHeaders := body["trustedHeaders"].(map[string]any)
	if trustedHeaders["enabled"] != false || trustedHeaders["deploymentReadAuthorizationEnforced"] != false {
		t.Fatalf("expected disabled trusted-header flags, got %#v", trustedHeaders)
	}
	assertOperatorContextIdentityHeaderSupport(t, body)
	enforcement := body["enforcement"].(map[string]any)
	if enforcement["mode"] != "local_unrestricted" || enforcement["trustedHeadersEnabled"] != false || enforcement["readRoutesGuarded"] != false || enforcement["writeRoutesGuarded"] != false {
		t.Fatalf("expected local unrestricted enforcement summary, got %#v", enforcement)
	}
	identity := body["identity"].(map[string]any)
	if identity["authenticated"] != false || identity["reason"] != requestactor.AuthenticatedIdentityUnavailableReason {
		t.Fatalf("expected explicit unavailable identity, got %#v", identity)
	}
	roles := body["roles"].(map[string]any)
	if roles["canRead"] != false || roles["canWrite"] != false {
		t.Fatalf("expected unavailable roles without capabilities, got %#v", roles)
	}
	if _, ok := roles["names"]; ok {
		t.Fatalf("expected omitted role names without trusted headers, got %#v", roles)
	}
	deploymentAccess := body["deploymentAccess"].(map[string]any)
	if deploymentAccess["enforced"] != false || deploymentAccess["forwarded"] != false {
		t.Fatalf("expected explicit unforwarded deployment access, got %#v", deploymentAccess)
	}
	if _, ok := deploymentAccess["names"]; ok {
		t.Fatalf("expected omitted deployment names without trusted headers, got %#v", deploymentAccess)
	}
}

func TestRouterOperatorContextRejectsWrongMethod(t *testing.T) {
	router := NewRouter(Options{})
	req := httptest.NewRequest(http.MethodPost, "/v1/operator/context", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.Code)
	}
	if got := resp.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("expected Allow %q, got %q", http.MethodGet, got)
	}
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
		CatalogSummary: func(_ context.Context) (catalog.Summary, error) {
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
		OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{Name: name}, nil
		},
		OverviewDeploymentResourceLogs: func(ctx context.Context, name, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
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

func TestRouterCatalogSummaryPassesRequestContextToProvider(t *testing.T) {
	const key contextKey = "catalog-summary"
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/summary", nil)
	req = req.WithContext(context.WithValue(req.Context(), key, "trace-value"))

	var got any
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogSummary: func(ctx context.Context) (catalog.Summary, error) {
			got = ctx.Value(key)
			return catalog.Summary{DeploymentCount: 1}, nil
		},
	})
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if got != "trace-value" {
		t.Fatalf("expected request context value to reach provider, got %#v", got)
	}
}

func TestRouterOverviewApplicationPassesRequestContextToProvider(t *testing.T) {
	const key contextKey = "overview-application"
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications/payments-api-prod", nil)
	req = req.WithContext(context.WithValue(req.Context(), key, "trace-value"))

	var got any
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewApplication: func(ctx context.Context, name string) (catalog.ApplicationOverviewDetail, error) {
			if name != "payments-api-prod" {
				t.Fatalf("unexpected deployment name: %s", name)
			}
			got = ctx.Value(key)
			return catalog.ApplicationOverviewDetail{Name: name}, nil
		},
	})
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if got != "trace-value" {
		t.Fatalf("expected request context value to reach overview provider, got %#v", got)
	}
}

func TestRouterOverviewDetailManagedDriftSummaryJSON(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "deployment detail", path: "/v1/overview/deployments/payments-api-prod"},
		{name: "application detail", path: "/v1/overview/applications/payments-api-prod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
					if name != "payments-api-prod" {
						t.Fatalf("unexpected deployment name: %s", name)
					}
					return catalog.DeploymentOverviewDetail{
						Name: "payments-api-prod",
						ManagedDrift: []domain.ManagedDriftItem{
							{Field: "artifact", Severity: domain.ManagedDriftSeverityHigh},
							{Field: "generation", Severity: domain.ManagedDriftSeverityMedium},
							{Field: "runtimeContext.capacity.desired", Severity: domain.ManagedDriftSeverityLow},
						},
						ManagedDriftSummary: catalog.ManagedDriftSummary{Total: 3, High: 1, Medium: 1, Low: 1},
					}, nil
				},
				OverviewApplication: func(_ context.Context, name string) (catalog.ApplicationOverviewDetail, error) {
					if name != "payments-api-prod" {
						t.Fatalf("unexpected deployment name: %s", name)
					}
					return catalog.ApplicationOverviewDetail{
						Name: "payments-api-prod",
						ManagedDrift: []domain.ManagedDriftItem{
							{Field: "artifact", Severity: domain.ManagedDriftSeverityHigh},
							{Field: "generation", Severity: domain.ManagedDriftSeverityMedium},
							{Field: "runtimeContext.capacity.desired", Severity: domain.ManagedDriftSeverityLow},
						},
						ManagedDriftSummary: catalog.ManagedDriftSummary{Total: 3, High: 1, Medium: 1, Low: 1},
					}, nil
				},
			})

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			summary, ok := payload["managedDriftSummary"].(map[string]any)
			if !ok || summary["total"] != float64(3) || summary["high"] != float64(1) || summary["medium"] != float64(1) || summary["low"] != float64(1) {
				t.Fatalf("expected managedDriftSummary counts in payload, got %#v", payload["managedDriftSummary"])
			}
			if _, exists := payload["ManagedDriftSummary"]; exists {
				t.Fatalf("unexpected Go-style managed drift summary field in %#v", payload)
			}
			items, ok := payload["managedDrift"].([]any)
			if !ok || len(items) != 3 {
				t.Fatalf("expected managedDrift rows to be preserved, got %#v", payload["managedDrift"])
			}
		})
	}
}

func TestRouterCatalogSummaryTreatsCanceledContextAsUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/summary", nil).WithContext(ctx)
	resp := httptest.NewRecorder()

	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogSummary: func(ctx context.Context) (catalog.Summary, error) {
			return catalog.Summary{}, ctx.Err()
		},
	})

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for canceled catalog request, got %d", resp.Code)
	}
	if body := resp.Body.String(); body != "catalog summary unavailable\n" {
		t.Fatalf("expected unavailable body, got %q", body)
	}
}

func TestRouterDeploymentStatusProviderObservesCanceledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments/api-prod", nil).WithContext(ctx)

	var got error
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatus: func(ctx context.Context, name string) (statusstore.Snapshot, error) {
			if name != "api-prod" {
				t.Fatalf("expected deployment name api-prod, got %q", name)
			}
			got = ctx.Err()
			return statusstore.Snapshot{}, ctx.Err()
		},
	})
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for canceled provider error, got %d", resp.Code)
	}
	if body := resp.Body.String(); body != "deployment status unavailable\n" {
		t.Fatalf("expected unavailable body, got %q", body)
	}
	if !errors.Is(got, context.Canceled) {
		t.Fatalf("expected provider to observe context cancellation, got %v", got)
	}
}

func TestRouterDeploymentStatusesProviderObservesRequestTimeout(t *testing.T) {
	seenErr := make(chan error, 1)
	router := NewRouter(Options{
		BuildInfo:      buildinfo.Info{Version: "dev", Commit: "abc123"},
		RequestTimeout: 10 * time.Millisecond,
		DeploymentStatuses: func(ctx context.Context) ([]statusstore.Snapshot, error) {
			<-ctx.Done()
			seenErr <- ctx.Err()
			return nil, ctx.Err()
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 timeout response, got %d", resp.Code)
	}
	select {
	case err := <-seenErr:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected provider to observe deadline exceeded, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for provider to observe request timeout")
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

func TestRouterGitHubWebhookRequiresPost(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo:           buildinfo.Info{Version: "dev", Commit: "abc123"},
		GitHubWebhookSecret: "top-secret",
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/webhook", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.Code)
	}
	assertResponseRequestID(t, resp)
	if allow := resp.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow POST, got %q", allow)
	}
}

func TestRouterGitHubWebhookUnavailableWithoutSecret(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", strings.NewReader(`{}`))
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
	assertResponseRequestID(t, resp)
	if body := resp.Body.String(); body != "github webhook unavailable\n" {
		t.Fatalf("unexpected unavailable body: %q", body)
	}
}

func TestRouterGitHubWebhookAcceptedPushEvent(t *testing.T) {
	const secret = "top-secret"
	body := `{
		"ref":"refs/heads/main",
		"before":"1111111111111111111111111111111111111111",
		"after":"2222222222222222222222222222222222222222",
		"repository":{"full_name":"acme/platform-config"},
		"head_commit":{"id":"2222222222222222222222222222222222222222","message":"update deployment","timestamp":"2026-04-18T15:00:00Z"},
		"commits":[
			{"id":"a","added":["deployments/api.yaml"],"modified":["environments/prod.yaml"],"removed":[]},
			{"id":"b","added":["accounts/prod.yaml"],"modified":["deployments/worker.yaml"],"removed":["deployments/old.yaml"]}
		]
	}`

	router := NewRouter(Options{
		BuildInfo:           buildinfo.Info{Version: "dev", Commit: "abc123"},
		GitHubWebhookSecret: secret,
		GitHubWebhookDesiredStateSource: githubwebhook.DesiredStateSourceConfig{
			Kind:       githubwebhook.DesiredStateSourceKindGitHub,
			Repository: "acme/platform-config",
			Ref:        "main",
			Path:       "deployments",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", strings.NewReader(body))
	req.Header = signedGitHubWebhookHeader(secret, "push", body)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%q", resp.Code, resp.Body.String())
	}
	assertResponseRequestID(t, resp)

	var got githubwebhook.Response
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Status != githubwebhook.StatusAccepted || got.Processed {
		t.Fatalf("unexpected status payload: %+v", got)
	}
	if got.Reason != "github push event validated, but controller refresh handoff is unavailable because this API runtime is not using the redis status backend" {
		t.Fatalf("unexpected reason: %q", got.Reason)
	}
	if got.Repository != "acme/platform-config" || got.Ref != "refs/heads/main" || got.After != "2222222222222222222222222222222222222222" || got.Before != "1111111111111111111111111111111111111111" {
		t.Fatalf("unexpected push summary: %+v", got)
	}
	if got.HeadCommit == nil || got.HeadCommit.ID != "2222222222222222222222222222222222222222" {
		t.Fatalf("expected head commit summary, got %+v", got.HeadCommit)
	}
	if got.ChangedFiles == nil || !reflect.DeepEqual(got.ChangedFiles.All, []string{"deployments/api.yaml", "accounts/prod.yaml", "environments/prod.yaml", "deployments/worker.yaml", "deployments/old.yaml"}) {
		t.Fatalf("unexpected changed files: %+v", got.ChangedFiles)
	}
	if got.DesiredStateSourceMatch == nil {
		t.Fatalf("expected desired-state source match summary, got %+v", got)
	}
	if got.DesiredStateSourceMatch.MatchStatus != githubwebhook.DesiredStateSourceMatchStatusMatched || !got.DesiredStateSourceMatch.Matches {
		t.Fatalf("unexpected desired-state source match summary: %+v", got.DesiredStateSourceMatch)
	}
	if got.DesiredStateSourceMatch.Path != "deployments" || got.DesiredStateSourceMatch.Repository != "acme/platform-config" || got.DesiredStateSourceMatch.Ref != "main" {
		t.Fatalf("unexpected desired-state source config echo: %+v", got.DesiredStateSourceMatch)
	}
	if got.ControllerRefresh != nil {
		t.Fatalf("expected no controller refresh handoff summary, got %+v", got.ControllerRefresh)
	}
}

func TestRouterGitHubWebhookPublishesControllerRefreshForMatchedPush(t *testing.T) {
	const secret = "top-secret"
	body := `{
		"ref":"refs/heads/main",
		"before":"1111111111111111111111111111111111111111",
		"after":"2222222222222222222222222222222222222222",
		"repository":{"full_name":"acme/platform-config"},
		"commits":[{"id":"a","modified":["tenet/deployments/api.yaml"]}]
	}`

	previousNow := githubWebhookNow
	githubWebhookNow = func() time.Time { return time.Date(2026, time.April, 19, 9, 30, 0, 0, time.UTC) }
	defer func() {
		githubWebhookNow = previousNow
	}()

	publisher := &recordingControllerRefreshPublisher{
		receipt: controllerrefresh.Receipt{
			Backend:   controllerrefresh.BackendRedis,
			Channel:   "tenet:test:controller-refresh",
			RequestID: "github-webhook:delivery-1:2222222222222222222222222222222222222222",
		},
	}
	router := NewRouter(Options{
		BuildInfo:           buildinfo.Info{Version: "dev", Commit: "abc123"},
		GitHubWebhookSecret: secret,
		GitHubWebhookDesiredStateSource: githubwebhook.DesiredStateSourceConfig{
			Kind:       githubwebhook.DesiredStateSourceKindGitHub,
			Repository: "acme/platform-config",
			Ref:        "main",
			Path:       "tenet",
		},
		GitHubWebhookRefreshPublisher: publisher,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", strings.NewReader(body))
	req.Header = signedGitHubWebhookHeader(secret, "push", body)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%q", resp.Code, resp.Body.String())
	}

	var got githubwebhook.Response
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Status != githubwebhook.StatusAccepted || !got.Processed {
		t.Fatalf("unexpected processed payload: %+v", got)
	}
	if got.Reason != "github push event validated and controller refresh handoff published" {
		t.Fatalf("unexpected reason: %q", got.Reason)
	}
	if got.ControllerRefresh == nil {
		t.Fatalf("expected controller refresh handoff summary, got %+v", got)
	}
	if got.ControllerRefresh.Backend != controllerrefresh.BackendRedis || got.ControllerRefresh.Channel != "tenet:test:controller-refresh" || got.ControllerRefresh.RequestID != "github-webhook:delivery-1:2222222222222222222222222222222222222222" || got.ControllerRefresh.PublishedAt != "2026-04-19T09:30:00Z" {
		t.Fatalf("unexpected controller refresh handoff summary: %+v", got.ControllerRefresh)
	}
	if len(publisher.requests) != 1 {
		t.Fatalf("expected exactly one controller refresh publish, got %d", len(publisher.requests))
	}
	request := publisher.requests[0]
	if request.RequestID != "github-webhook:delivery-1:2222222222222222222222222222222222222222" || request.DeliveryID != "delivery-1" || request.Repository != "acme/platform-config" || request.Ref != "refs/heads/main" || request.After != "2222222222222222222222222222222222222222" || request.RequestedAt != "2026-04-19T09:30:00Z" {
		t.Fatalf("unexpected controller refresh publish request: %+v", request)
	}
}

func TestRouterGitHubWebhookIgnoresDeletedRefPushWithoutPublishingRefresh(t *testing.T) {
	const secret = "top-secret"
	body := `{
		"ref":"refs/heads/main",
		"before":"1111111111111111111111111111111111111111",
		"after":"0000000000000000000000000000000000000000",
		"repository":{"full_name":"acme/platform-config","default_branch":"main"},
		"commits":[]
	}`

	publisher := &recordingControllerRefreshPublisher{
		receipt: controllerrefresh.Receipt{
			Backend:   controllerrefresh.BackendRedis,
			Channel:   "tenet:test:controller-refresh",
			RequestID: "github-webhook:delivery-1:0000000000000000000000000000000000000000",
		},
	}
	router := NewRouter(Options{
		BuildInfo:           buildinfo.Info{Version: "dev", Commit: "abc123"},
		GitHubWebhookSecret: secret,
		GitHubWebhookDesiredStateSource: githubwebhook.DesiredStateSourceConfig{
			Kind:       githubwebhook.DesiredStateSourceKindGitHub,
			Repository: "acme/platform-config",
			Ref:        "main",
		},
		GitHubWebhookRefreshPublisher: publisher,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", strings.NewReader(body))
	req.Header = signedGitHubWebhookHeader(secret, "push", body)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 ignored response, got %d body=%q", resp.Code, resp.Body.String())
	}

	var got githubwebhook.Response
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Status != githubwebhook.StatusIgnored || got.Processed || !got.Deleted {
		t.Fatalf("unexpected deleted-ref payload: %+v", got)
	}
	if got.ControllerRefresh != nil {
		t.Fatalf("expected no controller refresh handoff summary, got %+v", got.ControllerRefresh)
	}
	if got.DesiredStateSourceMatch == nil || !got.DesiredStateSourceMatch.Matches {
		t.Fatalf("expected matched desired-state source summary to remain visible, got %+v", got.DesiredStateSourceMatch)
	}
	if len(publisher.requests) != 0 {
		t.Fatalf("expected no controller refresh publish for deleted ref, got %+v", publisher.requests)
	}
}

func TestRouterGitHubWebhookIgnoresNonPushEvent(t *testing.T) {
	const secret = "top-secret"
	body := `{"zen":"keep it logically awesome"}`

	router := NewRouter(Options{
		BuildInfo:           buildinfo.Info{Version: "dev", Commit: "abc123"},
		GitHubWebhookSecret: secret,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", strings.NewReader(body))
	req.Header = signedGitHubWebhookHeader(secret, "ping", body)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", resp.Code, resp.Body.String())
	}

	var got githubwebhook.Response
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Status != githubwebhook.StatusIgnored || got.Processed {
		t.Fatalf("unexpected ignored payload: %+v", got)
	}
	if got.Reason != `github event "ping" is ignored because reconcile wiring is not implemented in this slice` {
		t.Fatalf("unexpected ignored reason: %q", got.Reason)
	}
	if got.DesiredStateSourceMatch != nil {
		t.Fatalf("expected no desired-state source match summary for ignored event, got %+v", got.DesiredStateSourceMatch)
	}
}

func TestRouterGitHubWebhookRejectsMalformedRequest(t *testing.T) {
	const secret = "top-secret"
	body := `{"ref":"refs/heads/main"}`

	router := NewRouter(Options{
		BuildInfo:           buildinfo.Info{Version: "dev", Commit: "abc123"},
		GitHubWebhookSecret: secret,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", strings.NewReader(body))
	req.Header = signedGitHubWebhookHeader("wrong-secret", "push", body)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	assertResponseRequestID(t, resp)
	if body := resp.Body.String(); body != "github webhook signature mismatch\n" {
		t.Fatalf("unexpected bad request body: %q", body)
	}
}

func signedGitHubWebhookHeader(secret, event, body string) http.Header {
	header := http.Header{}
	header.Set(githubwebhook.HeaderDeliveryID, "delivery-1")
	header.Set(githubwebhook.HeaderEvent, event)
	header.Set(githubwebhook.HeaderSignature, gitHubWebhookSignature(secret, body))
	return header
}

func gitHubWebhookSignature(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
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
		CatalogSummary: func(_ context.Context) (catalog.Summary, error) {
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
		CatalogSummary: func(_ context.Context) (catalog.Summary, error) {
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
		CatalogDeployments: func(_ context.Context) ([]catalog.DeploymentView, error) {
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
		CatalogDeployments: func(_ context.Context) ([]catalog.DeploymentView, error) {
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
		CatalogEnvironments: func(_ context.Context) ([]catalog.EnvironmentView, error) {
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
		CatalogEnvironments: func(_ context.Context) ([]catalog.EnvironmentView, error) {
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
		CatalogAccounts: func(_ context.Context) ([]catalog.AccountView, error) {
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
		CatalogAccounts: func(_ context.Context) ([]catalog.AccountView, error) {
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
		CatalogPlacements: func(_ context.Context) ([]catalog.PlacementView, error) {
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
		CatalogPlacements: func(_ context.Context) ([]catalog.PlacementView, error) {
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
		CatalogTopology: func(_ context.Context) (catalog.TopologySnapshot, error) {
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
		CatalogTopology: func(_ context.Context) (catalog.TopologySnapshot, error) {
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

func TestRouterCatalogRoutesRequireTrustedReadIdentity(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		CatalogDeployments: func(_ context.Context) ([]catalog.DeploymentView, error) {
			t.Fatal("catalog provider should not be called without trusted identity")
			return nil, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "authenticated identity is required for this operator read route") {
		t.Fatalf("expected trusted read identity guidance, got %q", resp.Body.String())
	}
}

func TestRouterCatalogRoutesRequireViewerCapableRole(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		RequireOperatorRoleForRead:         true,
		CatalogDeployments: func(_ context.Context) ([]catalog.DeploymentView, error) {
			t.Fatal("catalog provider should not be called without viewer-capable role")
			return nil, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/deployments", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Set(requestactor.AuthenticatedRolesHeader, "auditor")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "authenticated viewer-capable role is required for this operator read route") {
		t.Fatalf("expected viewer role guidance, got %q", resp.Body.String())
	}
}

func TestRouterCatalogDeploymentCollectionFiltersExactAllowlist(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		RequireOperatorRoleForRead:         true,
		CatalogDeployments: func(_ context.Context) ([]catalog.DeploymentView, error) {
			return []catalog.DeploymentView{
				{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"},
				{Name: "worker-prod", AppRef: "worker", EnvironmentRef: "prod-apne2"},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/deployments", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Set(requestactor.AuthenticatedRolesHeader, "viewer")
	req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "api-prod")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var deployments []catalog.DeploymentView
	if err := json.Unmarshal(resp.Body.Bytes(), &deployments); err != nil {
		t.Fatalf("decode catalog deployments: %v", err)
	}
	if len(deployments) != 1 || deployments[0].Name != "api-prod" {
		t.Fatalf("expected only exact allowlisted deployment, got %+v", deployments)
	}
}

func TestRouterCatalogTopologyFiltersProjectEnvironmentScopes(t *testing.T) {
	topology := catalog.TopologySnapshot{
		Summary: catalog.Summary{AccountCount: 2, EnvironmentCount: 2, DeploymentCount: 2},
		Accounts: []catalog.AccountView{
			{Name: "prod-core", AWSAccountID: "111111111111"},
			{Name: "stage-core", AWSAccountID: "222222222222"},
		},
		Environments: []catalog.EnvironmentView{
			{Name: "prod-apne2", AccountRef: "prod-core"},
			{Name: "staging-apne2", AccountRef: "stage-core"},
		},
		Deployments: []catalog.DeploymentView{
			{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"},
			{Name: "api-staging", AppRef: "payments-api", EnvironmentRef: "staging-apne2"},
		},
		Placements: []catalog.PlacementView{
			{DeploymentName: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", AccountName: "prod-core", AWSAccountID: "111111111111", Region: "ap-northeast-2"},
			{DeploymentName: "api-staging", AppRef: "payments-api", EnvironmentRef: "staging-apne2", AccountName: "stage-core", AWSAccountID: "222222222222", Region: "ap-northeast-2"},
		},
	}
	router := NewRouter(Options{
		BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		CatalogPlacements: func(_ context.Context) ([]catalog.PlacementView, error) {
			return topology.Placements, nil
		},
		CatalogTopology: func(_ context.Context) (catalog.TopologySnapshot, error) {
			return topology, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/topology", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Set(requestactor.AuthorizedProjectsHeader, "payments-api")
	req.Header.Set(requestactor.AuthorizedEnvironmentsHeader, "staging-apne2")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var filtered catalog.TopologySnapshot
	if err := json.Unmarshal(resp.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("decode catalog topology: %v", err)
	}
	if filtered.Summary.DeploymentCount != 1 || filtered.Summary.AccountCount != 1 || filtered.Summary.EnvironmentCount != 1 {
		t.Fatalf("expected filtered topology summary, got %+v", filtered.Summary)
	}
	if len(filtered.Deployments) != 1 || filtered.Deployments[0].Name != "api-staging" {
		t.Fatalf("expected project/environment-scoped deployment only, got %+v", filtered.Deployments)
	}
	if len(filtered.Accounts) != 1 || filtered.Accounts[0].Name != "stage-core" {
		t.Fatalf("expected account tied to visible placement only, got %+v", filtered.Accounts)
	}
}

func TestRouterCatalogAccountsFailClosedWhenPlacementsUnavailable(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		CatalogAccounts: func(_ context.Context) ([]catalog.AccountView, error) {
			return []catalog.AccountView{{Name: "prod-core", AWSAccountID: "111111111111"}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/accounts", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "api-prod")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var accounts []catalog.AccountView
	if err := json.Unmarshal(resp.Body.Bytes(), &accounts); err != nil {
		t.Fatalf("decode catalog accounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected catalog accounts to fail closed without placement data, got %+v", accounts)
	}
}

func TestRouterCatalogEnvironmentsFailClosedWhenPlacementsUnavailable(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		CatalogEnvironments: func(_ context.Context) ([]catalog.EnvironmentView, error) {
			return []catalog.EnvironmentView{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2"}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/environments", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "api-prod")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var environments []catalog.EnvironmentView
	if err := json.Unmarshal(resp.Body.Bytes(), &environments); err != nil {
		t.Fatalf("decode catalog environments: %v", err)
	}
	if len(environments) != 0 {
		t.Fatalf("expected catalog environments to fail closed without placement data, got %+v", environments)
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
					EntryID:        "audit_apply_testopaque",
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
				ResultScope: auditlog.BuildResultScope(query, 1, 1),
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
	if result.ResultScope.ReturnedEntryCount != 1 || result.ResultScope.ReturnedCount != 1 || result.ResultScope.TotalEntryCount != 1 || result.ResultScope.Limited || result.ResultScope.DeploymentName != "api-prod" || result.ResultScope.Action != string(auditlog.ActionApplyRequested) {
		t.Fatalf("unexpected audit result scope: %+v", result.ResultScope)
	}
	if result.ResultScope.Limit == nil || *result.ResultScope.Limit != 2 || result.ResultScope.RequestedLimit == nil || *result.ResultScope.RequestedLimit != 2 || result.ResultScope.EffectiveLimit != 2 || result.ResultScope.TruncationStatus != "not_truncated" || result.ResultScope.Truncated {
		t.Fatalf("expected audit result scope to echo applied limit, got %+v", result.ResultScope)
	}
	var raw map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode raw audit logs: %v", err)
	}
	rawEntries, ok := raw["entries"].([]any)
	if !ok || len(rawEntries) != 1 {
		t.Fatalf("expected raw entries array, got %#v", raw["entries"])
	}
	rawEntry, ok := rawEntries[0].(map[string]any)
	if !ok || rawEntry["entryId"] != "audit_apply_testopaque" {
		t.Fatalf("expected raw audit entryId field, got %#v", rawEntries[0])
	}
	rawScope, ok := raw["resultScope"].(map[string]any)
	if !ok || rawScope["returnedEntryCount"] != float64(1) || rawScope["returnedCount"] != float64(1) || rawScope["totalEntryCount"] != float64(1) || rawScope["limited"] != false || rawScope["limit"] != float64(2) || rawScope["requestedLimit"] != float64(2) || rawScope["effectiveLimit"] != float64(2) || rawScope["truncationStatus"] != "not_truncated" || rawScope["truncated"] != false || rawScope["deploymentName"] != "api-prod" || rawScope["action"] != "apply.requested" {
		t.Fatalf("expected raw audit resultScope contract, got %#v", raw["resultScope"])
	}
}

func TestRouterAuditLogsFiltersAuthorizationBeforeApplyingCollectionLimit(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
		TrustAuthenticatedHeaders:          true,
		RequireDeploymentReadAuthorization: true,
		AuditLogs: func(ctx context.Context, query auditlog.Query) (auditlog.Result, error) {
			if query.Limit != 0 || query.DeploymentName != "" || query.Action != auditlog.ActionApplyRequested {
				t.Fatalf("expected unbounded action-filtered provider query before authorization filter, got %+v", query)
			}
			return auditlog.Result{Entries: []auditlog.Entry{
				{Timestamp: "2026-04-18T09:00:00Z", Action: "apply.requested", DeploymentName: "other-prod", ApplyID: "apply_hidden"},
				{Timestamp: "2026-04-18T08:00:00Z", Action: "apply.requested", DeploymentName: "api-prod", ApplyID: "apply_visible_newer"},
				{Timestamp: "2026-04-18T07:00:00Z", Action: "apply.requested", DeploymentName: "api-prod", ApplyID: "apply_visible_older"},
			}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-logs?action=apply.requested&limit=1", nil)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "api-prod")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
	}
	var result auditlog.Result
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode audit logs: %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].ApplyID != "apply_visible_newer" {
		t.Fatalf("expected limit to apply after authorization filtering, got %+v", result.Entries)
	}
	if result.ResultScope.ReturnedEntryCount != 1 || result.ResultScope.ReturnedCount != 1 || result.ResultScope.TotalEntryCount != 2 || !result.ResultScope.Limited || result.ResultScope.Action != string(auditlog.ActionApplyRequested) {
		t.Fatalf("expected authorized collection result scope after filtering and limit, got %+v", result.ResultScope)
	}
	if result.ResultScope.Limit == nil || *result.ResultScope.Limit != 1 || result.ResultScope.RequestedLimit == nil || *result.ResultScope.RequestedLimit != 1 || result.ResultScope.EffectiveLimit != 1 || result.ResultScope.TruncationStatus != "truncated" || !result.ResultScope.Truncated {
		t.Fatalf("expected authorized collection result scope limit echo, got %+v", result.ResultScope)
	}
}

func TestRouterAuditLogsAcceptsApplyTransitionAction(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		AuditLogs: func(ctx context.Context, query auditlog.Query) (auditlog.Result, error) {
			if query.Limit != 0 || query.DeploymentName != "api-prod" || query.Action != auditlog.ActionApplyTransitioned {
				t.Fatalf("expected transition-filtered audit query, got %+v", query)
			}
			return auditlog.Result{
				Entries: []auditlog.Entry{{
					EntryID:        "audit_transition_testopaque",
					Timestamp:      "2026-04-18T08:35:00Z",
					Action:         "apply.transitioned",
					DeploymentName: "api-prod",
					PlanID:         "plan_1234",
					ApplyID:        "apply_5678",
					Metadata: auditlog.Metadata{
						PreviousStatus: "queued",
						NewStatus:      "running",
						Step:           1,
						CurrentStep:    1,
						TotalSteps:     3,
					},
					Source: auditlog.Source{Kind: "applyTransition", Store: "applyRequestStore", RecordRoute: "/v1/applies/apply_5678", SourceRoute: "/v1/plans/plan_1234"},
				}},
				ResultScope: auditlog.BuildResultScope(query, 1, 1),
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-logs?deployment=api-prod&action=apply.transitioned", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode raw audit logs: %v", err)
	}
	rawEntries, ok := raw["entries"].([]any)
	if !ok || len(rawEntries) != 1 {
		t.Fatalf("expected one transition entry, got %#v", raw["entries"])
	}
	rawEntry, ok := rawEntries[0].(map[string]any)
	if !ok || rawEntry["action"] != "apply.transitioned" || rawEntry["entryId"] != "audit_transition_testopaque" {
		t.Fatalf("expected raw transition audit entry, got %#v", rawEntries[0])
	}
	rawMetadata, ok := rawEntry["metadata"].(map[string]any)
	if !ok || rawMetadata["previousStatus"] != "queued" || rawMetadata["newStatus"] != "running" || rawMetadata["step"] != float64(1) || rawMetadata["currentStep"] != float64(1) || rawMetadata["totalSteps"] != float64(3) {
		t.Fatalf("expected raw transition metadata, got %#v", rawEntry["metadata"])
	}
	rawScope, ok := raw["resultScope"].(map[string]any)
	if !ok || rawScope["action"] != "apply.transitioned" || rawScope["deploymentName"] != "api-prod" || rawScope["returnedEntryCount"] != float64(1) || rawScope["returnedCount"] != float64(1) || rawScope["truncated"] != false {
		t.Fatalf("expected raw transition result scope, got %#v", raw["resultScope"])
	}
}

func TestRouterAuditLogsAcceptsControllerRefreshAction(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		AuditLogs: func(ctx context.Context, query auditlog.Query) (auditlog.Result, error) {
			if query.Limit != 0 || query.DeploymentName != "" || query.Action != auditlog.ActionControllerRefresh {
				t.Fatalf("expected controller-refresh-filtered audit query, got %+v", query)
			}
			return auditlog.Result{
				Entries: []auditlog.Entry{{
					EntryID:   "audit_controller_refresh_testopaque",
					Timestamp: "2026-04-20T10:45:00Z",
					Action:    string(auditlog.ActionControllerRefresh),
					Actor:     auditlog.Actor{Available: false, Reason: requestactor.UnavailableReason},
					Metadata: auditlog.Metadata{
						Status:         "published",
						Route:          "POST /v1/controller/refresh",
						Result:         "published",
						RequestID:      "api-controller-refresh:manual-refresh-123",
						DeliveryID:     "manual-refresh-123",
						Backend:        controllerrefresh.BackendRedis,
						Channel:        "tenet:test:controller-refresh",
						HTTPStatusCode: http.StatusAccepted,
					},
					Source: auditlog.Source{Kind: "controllerRefreshRequest", Store: "controllerRefreshAuditStore", RecordRoute: "POST /v1/controller/refresh"},
				}},
				ResultScope: auditlog.BuildResultScope(query, 1, 1),
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-logs?action=controller.refresh.requested", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode raw audit logs: %v", err)
	}
	rawEntries, ok := raw["entries"].([]any)
	if !ok || len(rawEntries) != 1 {
		t.Fatalf("expected one controller refresh entry, got %#v", raw["entries"])
	}
	rawEntry, ok := rawEntries[0].(map[string]any)
	if !ok || rawEntry["action"] != "controller.refresh.requested" || rawEntry["entryId"] != "audit_controller_refresh_testopaque" {
		t.Fatalf("expected raw controller refresh audit entry, got %#v", rawEntries[0])
	}
	rawMetadata, ok := rawEntry["metadata"].(map[string]any)
	if !ok || rawMetadata["route"] != "POST /v1/controller/refresh" || rawMetadata["status"] != "published" || rawMetadata["result"] != "published" || rawMetadata["requestId"] != "api-controller-refresh:manual-refresh-123" || rawMetadata["deliveryId"] != "manual-refresh-123" || rawMetadata["channel"] != "tenet:test:controller-refresh" {
		t.Fatalf("expected safe controller refresh metadata, got %#v", rawEntry["metadata"])
	}
	rawScope, ok := raw["resultScope"].(map[string]any)
	if !ok || rawScope["action"] != "controller.refresh.requested" || rawScope["returnedEntryCount"] != float64(1) {
		t.Fatalf("expected raw controller refresh result scope, got %#v", raw["resultScope"])
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

func TestDeploymentOverviewEventsRouteIncludesManagedDriftSummary(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected deployment name %q", name)
			}
			return catalog.DeploymentOverviewEvents{
				DeploymentName: name,
				Events: []domain.StatusHistoryEntry{{
					EventID:   "evt_test_runtime_event",
					Timestamp: "2026-04-16T06:15:00Z",
					Source:    "controller",
					Summary:   "Deployment has managed drift",
					ControllerRefresh: &domain.ControllerRefreshEvidence{
						RequestID:    "req-123",
						DeliveryID:   "delivery-456",
						Repository:   "DevopsArtFactory/tenet-config",
						Ref:          "refs/heads/main",
						After:        "abc123def456",
						RequestedAt:  "2026-04-16T06:00:00Z",
						ReceivedAt:   "2026-04-16T06:00:03Z",
						ReconciledAt: "2026-04-16T06:15:00Z",
					},
					ManagedDrift: []domain.ManagedDriftItem{
						{Field: "artifact", Severity: domain.ManagedDriftSeverityHigh},
						{Field: "runtimeContext.capacity.min", Severity: domain.ManagedDriftSeverityMedium},
						{Field: "runtimeContext.tags.owner", Severity: domain.ManagedDriftSeverityLow},
					},
				}},
				ManagedDriftSummary: catalog.ManagedDriftSummary{Total: 3, High: 1, Medium: 1, Low: 1},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/events", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode deployment events payload: %v", err)
	}
	if payload["deploymentName"] != "api-prod" {
		t.Fatalf("expected deploymentName in events payload, got %#v", payload)
	}
	events, ok := payload["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected events array in payload, got %#v", payload["events"])
	}
	event, ok := events[0].(map[string]any)
	if !ok {
		t.Fatalf("expected event object, got %#v", events[0])
	}
	if event["eventId"] != "evt_test_runtime_event" {
		t.Fatalf("expected eventId JSON field in runtime event payload, got %#v", event)
	}
	refresh, ok := event["controllerRefresh"].(map[string]any)
	if !ok {
		t.Fatalf("expected controllerRefresh object in event payload, got %#v", event)
	}
	for key, want := range map[string]any{
		"requestId":    "req-123",
		"deliveryId":   "delivery-456",
		"repository":   "DevopsArtFactory/tenet-config",
		"ref":          "refs/heads/main",
		"after":        "abc123def456",
		"requestedAt":  "2026-04-16T06:00:00Z",
		"receivedAt":   "2026-04-16T06:00:03Z",
		"reconciledAt": "2026-04-16T06:15:00Z",
	} {
		if refresh[key] != want {
			t.Fatalf("expected controllerRefresh.%s=%#v, got %#v", key, want, refresh[key])
		}
	}
	summary, ok := payload["managedDriftSummary"].(map[string]any)
	if !ok || summary["total"] != float64(3) || summary["high"] != float64(1) || summary["medium"] != float64(1) || summary["low"] != float64(1) {
		t.Fatalf("expected managedDriftSummary counts in events payload, got %#v", payload["managedDriftSummary"])
	}
	scope, ok := payload["resultScope"].(map[string]any)
	if !ok || scope["effectiveLimit"] != float64(0) || scope["returnedEventCount"] != float64(1) || scope["returnedCount"] != float64(1) || scope["totalEventCount"] != float64(1) || scope["limited"] != false || scope["newestEventTimestamp"] != "2026-04-16T06:15:00Z" || scope["oldestEventTimestamp"] != "2026-04-16T06:15:00Z" || scope["truncationStatus"] != "not_truncated" || scope["truncated"] != false {
		t.Fatalf("expected default events resultScope to describe the full returned window, got %#v", payload["resultScope"])
	}
	if _, exists := scope["requestedLimit"]; exists {
		t.Fatalf("expected default events resultScope to omit requestedLimit, got %#v", scope)
	}
	if _, exists := payload["ManagedDriftSummary"]; exists {
		t.Fatalf("unexpected Go-style managed drift summary field in %#v", payload)
	}
	if _, exists := payload["ResultScope"]; exists {
		t.Fatalf("unexpected Go-style result scope field in %#v", payload)
	}
}

func TestDeploymentOverviewEventsRouteAppliesLimitToEventsAndSummary(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected deployment name %q", name)
			}
			return catalog.DeploymentOverviewEvents{
				DeploymentName: name,
				Events: []domain.StatusHistoryEntry{
					{
						Timestamp:    "2026-05-06T03:00:00Z",
						Summary:      "Newest controller event",
						ManagedDrift: []domain.ManagedDriftItem{{Field: "artifact", Severity: domain.ManagedDriftSeverityHigh}},
					},
					{
						Timestamp:    "2026-05-06T02:00:00Z",
						Summary:      "Older controller event",
						ManagedDrift: []domain.ManagedDriftItem{{Field: "runtimeContext.capacity.min", Severity: domain.ManagedDriftSeverityMedium}},
					},
				},
				ManagedDriftSummary: catalog.ManagedDriftSummary{Total: 2, High: 1, Medium: 1},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/events?limit=1", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode deployment events payload: %v", err)
	}
	events, ok := payload["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected bounded events array in payload, got %#v", payload["events"])
	}
	event, ok := events[0].(map[string]any)
	if !ok || event["summary"] != "Newest controller event" {
		t.Fatalf("expected newest visible event, got %#v", events[0])
	}
	summary, ok := payload["managedDriftSummary"].(map[string]any)
	if !ok || summary["total"] != float64(1) || summary["high"] != float64(1) || summary["medium"] != float64(0) || summary["low"] != float64(0) {
		t.Fatalf("expected managedDriftSummary counts for returned events only, got %#v", payload["managedDriftSummary"])
	}
	scope, ok := payload["resultScope"].(map[string]any)
	if !ok || scope["requestedLimit"] != float64(1) || scope["effectiveLimit"] != float64(1) || scope["returnedEventCount"] != float64(1) || scope["returnedCount"] != float64(1) || scope["totalEventCount"] != float64(2) || scope["retentionLimit"] != float64(domain.RuntimeHistoryMaxEntries) || scope["retainedEventCount"] != float64(2) || scope["retentionStatus"] != "within_retention_limit" || scope["limited"] != true || scope["newestEventTimestamp"] != "2026-05-06T03:00:00Z" || scope["oldestEventTimestamp"] != "2026-05-06T03:00:00Z" || scope["truncationStatus"] != "truncated" || scope["truncated"] != true {
		t.Fatalf("expected bounded resultScope for returned events only, got %#v", payload["resultScope"])
	}
}

func TestDeploymentOverviewEventsRouteFiltersBySourceBeforeApplyingLimit(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected deployment name %q", name)
			}
			return catalog.DeploymentOverviewEvents{
				DeploymentName: name,
				Events: []domain.StatusHistoryEntry{
					{Timestamp: "2026-05-06T04:00:00Z", Source: "controller", Summary: "new controller reconcile"},
					{Timestamp: "2026-05-06T03:00:00Z", Source: "github", Summary: "new github refresh", ManagedDrift: []domain.ManagedDriftItem{{Field: "artifact", Severity: domain.ManagedDriftSeverityHigh}}},
					{Timestamp: "2026-05-06T02:00:00Z", Source: "github", Summary: "older github refresh", ManagedDrift: []domain.ManagedDriftItem{{Field: "runtimeContext.capacity.min", Severity: domain.ManagedDriftSeverityMedium}}},
				},
				ManagedDriftSummary: catalog.ManagedDriftSummary{Total: 2, High: 1, Medium: 1},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/events?source=github&limit=1", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode deployment events payload: %v", err)
	}
	events, ok := payload["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected one source-filtered event, got %#v", payload["events"])
	}
	event, ok := events[0].(map[string]any)
	if !ok || event["source"] != "github" || event["summary"] != "new github refresh" {
		t.Fatalf("expected newest github event after source-before-limit filtering, got %#v", events[0])
	}
	summary, ok := payload["managedDriftSummary"].(map[string]any)
	if !ok || summary["total"] != float64(1) || summary["high"] != float64(1) || summary["medium"] != float64(0) {
		t.Fatalf("expected managedDriftSummary to describe returned source-filtered events, got %#v", payload["managedDriftSummary"])
	}
	scope, ok := payload["resultScope"].(map[string]any)
	if !ok || scope["sourceFilter"] != "github" || scope["totalEventCount"] != float64(2) || scope["returnedEventCount"] != float64(1) || scope["retainedEventCount"] != float64(3) || scope["limited"] != true || scope["truncationStatus"] != "truncated" {
		t.Fatalf("expected resultScope to echo source filter and count filtered events before limit, got %#v", payload["resultScope"])
	}
	sourceSummary, ok := payload["sourceSummary"].([]any)
	if !ok || len(sourceSummary) != 2 {
		t.Fatalf("expected sourceSummary for retained event window, got %#v", payload["sourceSummary"])
	}
	wantSources := []map[string]any{
		{"source": "controller", "count": float64(1)},
		{"source": "github", "count": float64(2)},
	}
	for i, want := range wantSources {
		got, ok := sourceSummary[i].(map[string]any)
		if !ok {
			t.Fatalf("expected sourceSummary[%d] object, got %#v", i, sourceSummary[i])
		}
		for key, wantValue := range want {
			if got[key] != wantValue {
				t.Fatalf("expected sourceSummary[%d][%q]=%#v, got %#v in %#v", i, key, wantValue, got[key], sourceSummary)
			}
		}
	}
}

func TestDeploymentOverviewEventsRouteFiltersBlankSourcesAsUnknown(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
			return catalog.DeploymentOverviewEvents{
				DeploymentName: name,
				Events: []domain.StatusHistoryEntry{
					{Timestamp: "2026-05-06T04:00:00Z", Source: "controller", Summary: "controller reconcile"},
					{Timestamp: "2026-05-06T03:00:00Z", Source: "", Summary: "legacy status import", ManagedDrift: []domain.ManagedDriftItem{{Field: "artifact", Severity: domain.ManagedDriftSeverityLow}}},
					{Timestamp: "2026-05-06T02:00:00Z", Source: "  ", Summary: "whitespace source import"},
				},
				ManagedDriftSummary: catalog.ManagedDriftSummary{Total: 1, Low: 1},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/events?source=unknown&limit=1", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode deployment events payload: %v", err)
	}
	events, ok := payload["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected one source-filtered unknown event, got %#v", payload["events"])
	}
	event, ok := events[0].(map[string]any)
	if !ok || event["summary"] != "legacy status import" || event["source"] != "" {
		t.Fatalf("expected newest blank-source event to be selected without mutating event payload, got %#v", events[0])
	}
	sourceSummary, ok := payload["sourceSummary"].([]any)
	if !ok || len(sourceSummary) != 2 {
		t.Fatalf("expected sourceSummary to include controller and unknown buckets, got %#v", payload["sourceSummary"])
	}
	wantSources := []map[string]any{
		{"source": "controller", "count": float64(1)},
		{"source": "unknown", "count": float64(2)},
	}
	for i, want := range wantSources {
		got, ok := sourceSummary[i].(map[string]any)
		if !ok {
			t.Fatalf("expected sourceSummary[%d] object, got %#v", i, sourceSummary[i])
		}
		for key, wantValue := range want {
			if got[key] != wantValue {
				t.Fatalf("expected sourceSummary[%d][%q]=%#v, got %#v in %#v", i, key, wantValue, got[key], sourceSummary)
			}
		}
	}
	summary, ok := payload["managedDriftSummary"].(map[string]any)
	if !ok || summary["total"] != float64(1) || summary["low"] != float64(1) {
		t.Fatalf("expected managedDriftSummary to describe returned unknown-source events, got %#v", payload["managedDriftSummary"])
	}
	scope, ok := payload["resultScope"].(map[string]any)
	if !ok || scope["sourceFilter"] != "unknown" || scope["totalEventCount"] != float64(2) || scope["returnedEventCount"] != float64(1) || scope["retainedEventCount"] != float64(3) || scope["limited"] != true || scope["truncationStatus"] != "truncated" {
		t.Fatalf("expected resultScope to describe unknown-source filtered window, got %#v", payload["resultScope"])
	}
}

func TestDeploymentOverviewEventsRouteReturnsEmptySourceFilterWindow(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
			return catalog.DeploymentOverviewEvents{
				DeploymentName: name,
				Events:         []domain.StatusHistoryEntry{{Timestamp: "2026-05-06T03:00:00Z", Source: "controller", Summary: "controller reconcile"}},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/events?source=github", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode deployment events payload: %v", err)
	}
	events, ok := payload["events"].([]any)
	if !ok || len(events) != 0 {
		t.Fatalf("expected explicit empty events array for unmatched source filter, got %#v", payload["events"])
	}
	scope, ok := payload["resultScope"].(map[string]any)
	if !ok || scope["sourceFilter"] != "github" || scope["totalEventCount"] != float64(0) || scope["returnedEventCount"] != float64(0) || scope["retainedEventCount"] != float64(1) || scope["truncationStatus"] != "not_truncated" {
		t.Fatalf("expected resultScope to describe empty source-filtered window, got %#v", payload["resultScope"])
	}
}

func TestDeploymentOverviewEventsRouteRejectsInvalidSourceFilter(t *testing.T) {
	providerCalls := 0
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
			providerCalls++
			return catalog.DeploymentOverviewEvents{DeploymentName: name}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/events?source=github%20refresh", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if got := strings.TrimSpace(resp.Body.String()); got != "invalid source query" {
		t.Fatalf("expected invalid source query response, got %q", got)
	}
	if providerCalls != 0 {
		t.Fatalf("expected invalid source to fail before provider call, got %d calls", providerCalls)
	}
}

func TestDeploymentOverviewEventsRouteReportsRetainedHistoryCapSeparatelyFromQueryLimit(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
			events := make([]domain.StatusHistoryEntry, domain.RuntimeHistoryMaxEntries)
			for i := range events {
				events[i] = domain.StatusHistoryEntry{
					Timestamp: fmt.Sprintf("2026-05-06T03:%02d:00Z", i),
					Summary:   fmt.Sprintf("retained event %d", i),
				}
			}
			return catalog.DeploymentOverviewEvents{DeploymentName: name, Events: events}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/events", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode deployment events payload: %v", err)
	}
	scope, ok := payload["resultScope"].(map[string]any)
	if !ok || scope["limited"] != false || scope["truncationStatus"] != "not_truncated" || scope["truncated"] != false || scope["retentionLimit"] != float64(domain.RuntimeHistoryMaxEntries) || scope["retainedEventCount"] != float64(domain.RuntimeHistoryMaxEntries) || scope["retentionStatus"] != "at_retention_limit" {
		t.Fatalf("expected retained-history cap metadata separate from query-limit truncation, got %#v", payload["resultScope"])
	}
}

func TestDeploymentOverviewEventsRouteReportsConfiguredRetentionLimit(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
			return catalog.DeploymentOverviewEvents{
				DeploymentName: name,
				Events: []domain.StatusHistoryEntry{
					{Timestamp: "2026-05-06T01:00:00Z", Summary: "older event"},
					{Timestamp: "2026-05-06T02:00:00Z", Summary: "newer event"},
				},
				Retention: catalog.RuntimeEventRetention{
					Limit:              3,
					RetainedEventCount: 2,
					Status:             domain.RuntimeHistoryRetentionStatusWithinLimit,
				},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/events", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode deployment events payload: %v", err)
	}
	retention, ok := payload["retention"].(map[string]any)
	if !ok || retention["limit"] != float64(3) || retention["retainedEventCount"] != float64(2) || retention["status"] != domain.RuntimeHistoryRetentionStatusWithinLimit {
		t.Fatalf("expected configured top-level retention metadata, got %#v", payload["retention"])
	}
	scope, ok := payload["resultScope"].(map[string]any)
	if !ok || scope["retentionLimit"] != float64(3) || scope["retainedEventCount"] != float64(2) || scope["retentionStatus"] != domain.RuntimeHistoryRetentionStatusWithinLimit {
		t.Fatalf("expected configured resultScope retention metadata, got %#v", payload["resultScope"])
	}
}

func TestDeploymentOverviewEventsRouteReportsTimestampEnvelopeForReturnedWindow(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
			return catalog.DeploymentOverviewEvents{
				DeploymentName: name,
				Events: []domain.StatusHistoryEntry{
					{Timestamp: "2026-05-06T01:00:00Z", Summary: "older returned event"},
					{Timestamp: "2026-05-06T03:00:00Z", Summary: "newest returned event"},
					{Timestamp: "2026-05-06T02:00:00Z", Summary: "middle returned event"},
				},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/events", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode deployment events payload: %v", err)
	}
	scope, ok := payload["resultScope"].(map[string]any)
	if !ok || scope["newestEventTimestamp"] != "2026-05-06T03:00:00Z" || scope["oldestEventTimestamp"] != "2026-05-06T01:00:00Z" {
		t.Fatalf("expected resultScope timestamp envelope for returned events, got %#v", payload["resultScope"])
	}
}

func TestDeploymentOverviewEventsRouteReportsExactLimitAsUnknown(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected deployment name %q", name)
			}
			return catalog.DeploymentOverviewEvents{
				DeploymentName: name,
				Events: []domain.StatusHistoryEntry{{
					Timestamp: "2026-05-06T03:00:00Z",
					Summary:   "Only controller event",
				}},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/events?limit=1", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode deployment events payload: %v", err)
	}
	events, ok := payload["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected exact-limit events array in payload, got %#v", payload["events"])
	}
	scope, ok := payload["resultScope"].(map[string]any)
	if !ok || scope["requestedLimit"] != float64(1) || scope["effectiveLimit"] != float64(1) || scope["returnedEventCount"] != float64(1) || scope["returnedCount"] != float64(1) || scope["totalEventCount"] != float64(1) || scope["limited"] != false || scope["newestEventTimestamp"] != "2026-05-06T03:00:00Z" || scope["oldestEventTimestamp"] != "2026-05-06T03:00:00Z" || scope["truncationStatus"] != "unknown" || scope["truncated"] != false {
		t.Fatalf("expected exact-limit resultScope to report unknown total without truncation, got %#v", payload["resultScope"])
	}
}

func TestDeploymentOverviewEventsRouteReportsUnderLimitAsNotTruncated(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected deployment name %q", name)
			}
			return catalog.DeploymentOverviewEvents{
				DeploymentName: name,
				Events: []domain.StatusHistoryEntry{{
					Timestamp: "2026-05-06T03:00:00Z",
					Summary:   "Only controller event",
				}},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/events?limit=2", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode deployment events payload: %v", err)
	}
	events, ok := payload["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected under-limit events array in payload, got %#v", payload["events"])
	}
	scope, ok := payload["resultScope"].(map[string]any)
	if !ok || scope["requestedLimit"] != float64(2) || scope["effectiveLimit"] != float64(2) || scope["returnedEventCount"] != float64(1) || scope["returnedCount"] != float64(1) || scope["totalEventCount"] != float64(1) || scope["limited"] != false || scope["truncationStatus"] != "not_truncated" || scope["truncated"] != false {
		t.Fatalf("expected under-limit resultScope to be known complete, got %#v", payload["resultScope"])
	}
}

func TestDeploymentOverviewEventsRouteOmitsWindowTimestampsForEmptyEvents(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
			return catalog.DeploymentOverviewEvents{DeploymentName: name, Events: []domain.StatusHistoryEntry{}}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/events", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode deployment events payload: %v", err)
	}
	scope, ok := payload["resultScope"].(map[string]any)
	if !ok || scope["returnedEventCount"] != float64(0) || scope["returnedCount"] != float64(0) || scope["totalEventCount"] != float64(0) || scope["retentionLimit"] != float64(domain.RuntimeHistoryMaxEntries) || scope["retainedEventCount"] != float64(0) || scope["retentionStatus"] != "within_retention_limit" || scope["truncationStatus"] != "not_truncated" || scope["truncated"] != false {
		t.Fatalf("expected empty resultScope counts, got %#v", payload["resultScope"])
	}
	if _, exists := scope["newestEventTimestamp"]; exists {
		t.Fatalf("expected empty event resultScope to omit newestEventTimestamp, got %#v", scope)
	}
	if _, exists := scope["oldestEventTimestamp"]; exists {
		t.Fatalf("expected empty event resultScope to omit oldestEventTimestamp, got %#v", scope)
	}
}

func TestDeploymentOverviewEventsRouteRejectsInvalidLimit(t *testing.T) {
	for _, rawLimit := range []string{"0", "abc"} {
		t.Run(rawLimit, func(t *testing.T) {
			providerCalls := 0
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
					providerCalls++
					return catalog.DeploymentOverviewEvents{DeploymentName: name}, nil
				},
			})

			req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/events?limit="+rawLimit, nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
			}
			if got := strings.TrimSpace(resp.Body.String()); got != "invalid limit query" {
				t.Fatalf("expected invalid limit query response, got %q", got)
			}
			if providerCalls != 0 {
				t.Fatalf("expected invalid limit to fail before provider call, got %d calls", providerCalls)
			}
		})
	}
}

func TestRouterDeploymentReadAuthorization(t *testing.T) {
	newGuardedRouter := func(t *testing.T) (http.Handler, *int) {
		t.Helper()

		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			OverviewApplications: func(context.Context) ([]catalog.ApplicationOverview, error) {
				providerCalls++
				return []catalog.ApplicationOverview{{Name: "api-prod"}}, nil
			},
			CatalogPlacements: func(context.Context) ([]catalog.PlacementView, error) {
				return []catalog.PlacementView{
					{DeploymentName: "api-prod", AppRef: "payments", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "us-west-2"},
					{DeploymentName: "worker-prod", AppRef: "workers", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "us-west-2"},
					{DeploymentName: "payments-api-staging", AppRef: "payments", EnvironmentRef: "staging", AWSAccountID: "210987654321", Region: "us-east-1"},
				}, nil
			},
			OverviewApplication: func(_ context.Context, name string) (catalog.ApplicationOverviewDetail, error) {
				providerCalls++
				return catalog.ApplicationOverviewDetail{Name: name}, nil
			},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				providerCalls++
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
				providerCalls++
				return catalog.DeploymentOverviewEvents{DeploymentName: name}, nil
			},
			OverviewDeploymentResourceLogs: func(_ context.Context, name, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
				providerCalls++
				return resourcelogs.Result{DeploymentName: name, ResourceType: resourceType, ResourceName: resourceName}, nil
			},
			OverviewDeploymentResourceInspection: func(_ context.Context, name, resourceType, resourceName string, _ int) (resourceinspection.Result, error) {
				providerCalls++
				return resourceinspection.Result{DeploymentName: name, ResourceType: resourceType, ResourceName: resourceName, Available: true}, nil
			},
			AuditLogs: func(_ context.Context, query auditlog.Query) (auditlog.Result, error) {
				providerCalls++
				if query.DeploymentName != "" {
					return auditlog.Result{
						Entries: []auditlog.Entry{{
							Action:         string(auditlog.ActionApplyRequested),
							DeploymentName: query.DeploymentName,
						}},
					}, nil
				}
				return auditlog.Result{
					Entries: []auditlog.Entry{
						{
							Action:         string(auditlog.ActionApplyRequested),
							DeploymentName: "api-prod",
						},
						{
							Action:         string(auditlog.ActionApplyRequested),
							DeploymentName: "worker-prod",
						},
						{
							Action:         string(auditlog.ActionApplyRequested),
							DeploymentName: "payments-api-staging",
						},
					},
				}, nil
			},
			DeploymentStatuses: func(context.Context) ([]statusstore.Snapshot, error) {
				providerCalls++
				return []statusstore.Snapshot{
					{DeploymentName: "api-prod"},
					{DeploymentName: "worker-prod"},
					{DeploymentName: "payments-api-staging"},
				}, nil
			},
			DeploymentStatus: func(_ context.Context, name string) (statusstore.Snapshot, error) {
				providerCalls++
				return statusstore.Snapshot{DeploymentName: name}, nil
			},
		})

		return router, &providerCalls
	}

	newAuthenticatedRequest := func(path string, authorizedDeployments string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		if authorizedDeployments != "" {
			req.Header.Set(requestactor.AuthorizedDeploymentsHeader, authorizedDeployments)
		}
		return req
	}

	newProjectEnvironmentRequest := func(path string, projects string, environments string) *http.Request {
		req := newAuthenticatedRequest(path, "")
		if projects != "" {
			req.Header.Set(requestactor.AuthorizedProjectsHeader, projects)
		}
		if environments != "" {
			req.Header.Set(requestactor.AuthorizedEnvironmentsHeader, environments)
		}
		return req
	}

	guardedRoutes := []struct {
		name string
		path string
	}{
		{name: "deployment status detail", path: "/v1/status/deployments/api-prod"},
		{name: "overview application detail", path: "/v1/overview/applications/api-prod"},
		{name: "overview deployment detail", path: "/v1/overview/deployments/api-prod"},
		{name: "overview deployment events", path: "/v1/overview/deployments/api-prod/events"},
		{name: "overview deployment resource logs", path: "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service"},
		{name: "overview deployment resource inspection", path: "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=payments-api-prod-service"},
		{name: "deployment scoped audit logs", path: "/v1/audit-logs?deployment=api-prod&action=apply.requested&limit=1"},
	}

	for _, tc := range guardedRoutes {
		t.Run(tc.name+" unauthenticated", func(t *testing.T) {
			router, providerCalls := newGuardedRouter(t)
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", resp.Code)
			}
			if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator read route\n" {
				t.Fatalf("unexpected 401 response body: %q", body)
			}
			if *providerCalls != 0 {
				t.Fatalf("expected auth guard to short-circuit before providers, got %d calls", *providerCalls)
			}
		})

		t.Run(tc.name+" forbidden", func(t *testing.T) {
			router, providerCalls := newGuardedRouter(t)
			req := newAuthenticatedRequest(tc.path, "worker-prod")
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d", resp.Code)
			}
			if body := resp.Body.String(); body != "forbidden: operator does not have access to this deployment\n" {
				t.Fatalf("unexpected 403 response body: %q", body)
			}
			if *providerCalls != 0 {
				t.Fatalf("expected auth guard to short-circuit before providers, got %d calls", *providerCalls)
			}
		})

		t.Run(tc.name+" allowed", func(t *testing.T) {
			router, providerCalls := newGuardedRouter(t)
			req := newAuthenticatedRequest(tc.path, "api-prod, worker-prod")
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
			}
			if *providerCalls == 0 {
				t.Fatal("expected guarded route provider to run")
			}
		})

		t.Run(tc.name+" allowed by project/environment scope", func(t *testing.T) {
			router, providerCalls := newGuardedRouter(t)
			req := newProjectEnvironmentRequest(tc.path, "payments", "prod")
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
			}
			if *providerCalls == 0 {
				t.Fatal("expected guarded route provider to run")
			}
		})
	}

	t.Run("unscoped audit logs unauthenticated", func(t *testing.T) {
		router, providerCalls := newGuardedRouter(t)
		req := httptest.NewRequest(http.MethodGet, "/v1/audit-logs?action=apply.requested&limit=1", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator read route\n" {
			t.Fatalf("unexpected 401 response body: %q", body)
		}
		if *providerCalls != 0 {
			t.Fatalf("expected auth guard to short-circuit before providers, got %d calls", *providerCalls)
		}
	})

	t.Run("unscoped audit logs filter authorized deployments", func(t *testing.T) {
		router, providerCalls := newGuardedRouter(t)
		req := newAuthenticatedRequest("/v1/audit-logs?action=apply.requested", "api-prod, worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if *providerCalls != 1 {
			t.Fatalf("expected unscoped audit logs provider call, got %d", *providerCalls)
		}

		var result auditlog.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode audit logs: %v", err)
		}
		got := make([]string, 0, len(result.Entries))
		for _, entry := range result.Entries {
			got = append(got, entry.DeploymentName)
		}
		want := []string{"api-prod", "worker-prod"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected filtered unscoped audit deployments: got %+v want %+v", got, want)
		}
	})

	t.Run("unscoped audit logs return empty entries when allowlist has no visible rows", func(t *testing.T) {
		router, providerCalls := newGuardedRouter(t)
		req := newAuthenticatedRequest("/v1/audit-logs?action=apply.requested", "")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if *providerCalls != 1 {
			t.Fatalf("expected unscoped audit logs provider call, got %d", *providerCalls)
		}

		var result auditlog.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode audit logs: %v", err)
		}
		if len(result.Entries) != 0 {
			t.Fatalf("expected empty filtered unscoped audit entries, got %+v", result.Entries)
		}
	})

	t.Run("deployment status collection unauthenticated", func(t *testing.T) {
		router, providerCalls := newGuardedRouter(t)
		req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator read route\n" {
			t.Fatalf("unexpected 401 response body: %q", body)
		}
		if *providerCalls != 0 {
			t.Fatalf("expected auth guard to short-circuit before providers, got %d calls", *providerCalls)
		}
	})

	t.Run("deployment status collection filters allowed deployments", func(t *testing.T) {
		router, providerCalls := newGuardedRouter(t)
		req := newAuthenticatedRequest("/v1/status/deployments", "payments-api-staging, worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var snapshots []statusstore.Snapshot
		if err := json.Unmarshal(resp.Body.Bytes(), &snapshots); err != nil {
			t.Fatalf("decode deployment statuses: %v", err)
		}
		want := []statusstore.Snapshot{{DeploymentName: "worker-prod"}, {DeploymentName: "payments-api-staging"}}
		if !reflect.DeepEqual(snapshots, want) {
			t.Fatalf("unexpected filtered deployment statuses: got %+v want %+v", snapshots, want)
		}
		if *providerCalls != 1 {
			t.Fatalf("expected deployment statuses provider call, got %d", *providerCalls)
		}
	})

	t.Run("deployment status collection filters project environment scoped deployments", func(t *testing.T) {
		router, providerCalls := newGuardedRouter(t)
		req := newProjectEnvironmentRequest("/v1/status/deployments", "payments", "staging")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var snapshots []statusstore.Snapshot
		if err := json.Unmarshal(resp.Body.Bytes(), &snapshots); err != nil {
			t.Fatalf("decode deployment statuses: %v", err)
		}
		want := []statusstore.Snapshot{{DeploymentName: "payments-api-staging"}}
		if !reflect.DeepEqual(snapshots, want) {
			t.Fatalf("unexpected project/environment filtered deployment statuses: got %+v want %+v", snapshots, want)
		}
		if *providerCalls != 1 {
			t.Fatalf("expected deployment statuses provider call, got %d", *providerCalls)
		}
	})

	t.Run("deployment status collection returns empty array for nonmatching project environment scope", func(t *testing.T) {
		router, providerCalls := newGuardedRouter(t)
		req := newProjectEnvironmentRequest("/v1/status/deployments", "payments", "dev")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if body := strings.TrimSpace(resp.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array, got %q", body)
		}
		if *providerCalls != 1 {
			t.Fatalf("expected deployment statuses provider call, got %d", *providerCalls)
		}
	})

	t.Run("deployment status collection returns empty array when allowlist has no matches", func(t *testing.T) {
		router, providerCalls := newGuardedRouter(t)
		req := newAuthenticatedRequest("/v1/status/deployments", "missing-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if body := strings.TrimSpace(resp.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array, got %q", body)
		}
		if *providerCalls != 1 {
			t.Fatalf("expected deployment statuses provider call, got %d", *providerCalls)
		}
	})

	t.Run("deployment status collection returns empty array when allowlist header is absent", func(t *testing.T) {
		router, providerCalls := newGuardedRouter(t)
		req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments", nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if body := strings.TrimSpace(resp.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array, got %q", body)
		}
		if *providerCalls != 1 {
			t.Fatalf("expected deployment statuses provider call, got %d", *providerCalls)
		}
	})

	t.Run("overview applications collection unauthenticated", func(t *testing.T) {
		router, providerCalls := newGuardedRouter(t)
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator read route\n" {
			t.Fatalf("unexpected 401 response body: %q", body)
		}
		if *providerCalls != 0 {
			t.Fatalf("expected auth guard to short-circuit before providers, got %d calls", *providerCalls)
		}
	})

	t.Run("overview applications collection filters allowed deployments", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			OverviewApplications: func(context.Context) ([]catalog.ApplicationOverview, error) {
				return []catalog.ApplicationOverview{
					{Name: "api-prod"},
					{Name: "worker-prod"},
					{Name: "payments-api-staging"},
				}, nil
			},
		})
		req := newAuthenticatedRequest("/v1/overview/applications", "payments-api-staging, worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var applications []catalog.ApplicationOverview
		if err := json.Unmarshal(resp.Body.Bytes(), &applications); err != nil {
			t.Fatalf("decode overview applications: %v", err)
		}
		want := []catalog.ApplicationOverview{{Name: "worker-prod"}, {Name: "payments-api-staging"}}
		if !reflect.DeepEqual(applications, want) {
			t.Fatalf("unexpected filtered overview applications: got %+v want %+v", applications, want)
		}
	})

	t.Run("overview applications collection returns empty array when allowlist has no matches", func(t *testing.T) {
		router, providerCalls := newGuardedRouter(t)
		req := newAuthenticatedRequest("/v1/overview/applications", "worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if body := strings.TrimSpace(resp.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array, got %q", body)
		}
		if *providerCalls != 1 {
			t.Fatalf("expected overview applications provider call, got %d", *providerCalls)
		}
	})

	t.Run("overview applications collection returns empty array when allowlist header is absent", func(t *testing.T) {
		router, providerCalls := newGuardedRouter(t)
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications", nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if body := strings.TrimSpace(resp.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array, got %q", body)
		}
		if *providerCalls != 1 {
			t.Fatalf("expected overview applications provider call, got %d", *providerCalls)
		}
	})

	t.Run("immutable plan preview reads honor deployment authorization", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			PlanPreview: func(context.Context, string) (planpreview.Record, error) {
				providerCalls++
				return planpreview.Record{
					ID:             "plan_1234abcd",
					DeploymentName: "api-prod",
					Status:         planpreview.StatusReady,
					CreatedAt:      "2026-04-17T08:05:00Z",
					Source:         planpreview.SourceContext{DeploymentName: "api-prod"},
				}, nil
			},
		})

		t.Run("unauthenticated", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/plans/plan_1234abcd", nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", resp.Code)
			}
			if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator read route\n" {
				t.Fatalf("unexpected 401 response body: %q", body)
			}
			if providerCalls != 0 {
				t.Fatalf("expected unauthenticated auth guard to short-circuit before lookup, got %d calls", providerCalls)
			}
		})

		t.Run("forbidden", func(t *testing.T) {
			req := newAuthenticatedRequest("/v1/plans/plan_1234abcd", "worker-prod")
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d", resp.Code)
			}
			if body := resp.Body.String(); body != "forbidden: operator does not have access to this deployment\n" {
				t.Fatalf("unexpected 403 response body: %q", body)
			}
			if providerCalls != 1 {
				t.Fatalf("expected forbidden lookup to load stored record once, got %d calls", providerCalls)
			}
		})

		t.Run("allowed", func(t *testing.T) {
			req := newAuthenticatedRequest("/v1/plans/plan_1234abcd", "api-prod, worker-prod")
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
			}
			if providerCalls != 2 {
				t.Fatalf("expected allowed lookup to load stored record once, got %d calls", providerCalls)
			}
		})
	})

	t.Run("immutable apply request reads honor deployment authorization", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
				providerCalls++
				return applyrequest.Record{
					ID:             "apply_1234abcd",
					PlanID:         "plan_1234abcd",
					DeploymentName: "api-prod",
					Status:         applyrequest.StatusFailed,
					CreatedAt:      "2026-04-18T08:05:00Z",
					Source:         applyrequest.SourceContext{PlanID: "plan_1234abcd"},
				}, nil
			},
		})

		t.Run("unauthenticated", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/applies/apply_1234abcd", nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", resp.Code)
			}
			if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator read route\n" {
				t.Fatalf("unexpected 401 response body: %q", body)
			}
			if providerCalls != 0 {
				t.Fatalf("expected unauthenticated auth guard to short-circuit before lookup, got %d calls", providerCalls)
			}
		})

		t.Run("forbidden", func(t *testing.T) {
			req := newAuthenticatedRequest("/v1/applies/apply_1234abcd", "worker-prod")
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d", resp.Code)
			}
			if body := resp.Body.String(); body != "forbidden: operator does not have access to this deployment\n" {
				t.Fatalf("unexpected 403 response body: %q", body)
			}
			if providerCalls != 1 {
				t.Fatalf("expected forbidden lookup to load stored record once, got %d calls", providerCalls)
			}
		})

		t.Run("allowed", func(t *testing.T) {
			req := newAuthenticatedRequest("/v1/applies/apply_1234abcd", "api-prod, worker-prod")
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
			}
			if providerCalls != 2 {
				t.Fatalf("expected allowed lookup to load stored record once, got %d calls", providerCalls)
			}
		})
	})
}

func TestRouterOperatorReadRoleAuthorization(t *testing.T) {
	newGuardedRouter := func() (http.Handler, *int) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			TrustAuthenticatedHeaders:          true,
			RequireOperatorRoleForRead:         true,
			RequireDeploymentReadAuthorization: true,
			OverviewApplications: func(context.Context) ([]catalog.ApplicationOverview, error) {
				providerCalls++
				return []catalog.ApplicationOverview{{Name: "api-prod"}}, nil
			},
			OverviewApplication: func(_ context.Context, name string) (catalog.ApplicationOverviewDetail, error) {
				providerCalls++
				return catalog.ApplicationOverviewDetail{Name: name}, nil
			},
			OverviewAccounts: func(context.Context) ([]catalog.AccountOverview, error) {
				providerCalls++
				return []catalog.AccountOverview{{
					Name:         "prod-core",
					AWSAccountID: "123456789012",
					Deployments:  []catalog.DeploymentOverview{{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
				}}, nil
			},
			OverviewAccount: func(_ context.Context, awsAccountID string) (catalog.AccountOverviewDetail, error) {
				providerCalls++
				return catalog.AccountOverviewDetail{
					Name:         "prod-core",
					AWSAccountID: awsAccountID,
					Deployments:  []catalog.DeploymentOverview{{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
				}, nil
			},
			OverviewProjectEnvironment: func(_ context.Context, accountID, appRef, environmentRef string) (catalog.ProjectEnvironmentOverviewDetail, error) {
				providerCalls++
				return catalog.ProjectEnvironmentOverviewDetail{
					AccountName:    "prod-core",
					AWSAccountID:   accountID,
					AppRef:         appRef,
					EnvironmentRef: environmentRef,
					Deployments:    []catalog.DeploymentOverview{{Name: "api-prod", AppRef: appRef, EnvironmentRef: environmentRef}},
				}, nil
			},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				providerCalls++
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentEvents: func(_ context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
				providerCalls++
				return catalog.DeploymentOverviewEvents{DeploymentName: name}, nil
			},
			OverviewDeploymentResourceLogs: func(_ context.Context, name, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
				providerCalls++
				return resourcelogs.Result{DeploymentName: name, ResourceType: resourceType, ResourceName: resourceName, Available: true}, nil
			},
			OverviewDeploymentResourceInspection: func(_ context.Context, name, resourceType, resourceName string, _ int) (resourceinspection.Result, error) {
				providerCalls++
				return resourceinspection.Result{DeploymentName: name, ResourceType: resourceType, ResourceName: resourceName, Available: true}, nil
			},
			AuditLogs: func(_ context.Context, query auditlog.Query) (auditlog.Result, error) {
				providerCalls++
				if query.DeploymentName != "" {
					return auditlog.Result{Entries: []auditlog.Entry{{DeploymentName: query.DeploymentName}}}, nil
				}
				return auditlog.Result{Entries: []auditlog.Entry{{DeploymentName: "api-prod"}}}, nil
			},
			PlanPreview: func(_ context.Context, id string) (planpreview.Record, error) {
				providerCalls++
				return planpreview.Record{ID: id, DeploymentName: "api-prod", Status: planpreview.StatusReady, CreatedAt: "2026-04-17T08:05:00Z"}, nil
			},
			ApplyRequest: func(_ context.Context, id string) (applyrequest.Record, error) {
				providerCalls++
				return applyrequest.Record{ID: id, DeploymentName: "api-prod", Status: applyrequest.StatusQueued, CreatedAt: "2026-04-18T08:05:00Z"}, nil
			},
			ApplyRequests: func(context.Context, applyrequest.ListQuery) ([]applyrequest.Record, error) {
				providerCalls++
				return []applyrequest.Record{{ID: "apply_queued", DeploymentName: "api-prod", Status: applyrequest.StatusQueued, CreatedAt: "2026-04-18T08:05:00Z"}}, nil
			},
			DeploymentStatuses: func(context.Context) ([]statusstore.Snapshot, error) {
				providerCalls++
				return []statusstore.Snapshot{{DeploymentName: "api-prod"}}, nil
			},
			DeploymentStatus: func(_ context.Context, name string) (statusstore.Snapshot, error) {
				providerCalls++
				return statusstore.Snapshot{DeploymentName: name}, nil
			},
		})
		return router, &providerCalls
	}

	newAuthenticatedRequest := func(path string, roles string, authorizedDeployments string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		if roles != "" {
			req.Header.Set(requestactor.AuthenticatedRolesHeader, roles)
		}
		if authorizedDeployments != "" {
			req.Header.Set(requestactor.AuthorizedDeploymentsHeader, authorizedDeployments)
		}
		return req
	}

	routes := []struct {
		name                  string
		path                  string
		authorizedDeployments string
	}{
		{name: "overview applications collection", path: "/v1/overview/applications", authorizedDeployments: "api-prod"},
		{name: "overview application detail", path: "/v1/overview/applications/api-prod", authorizedDeployments: "api-prod"},
		{name: "overview accounts collection", path: "/v1/overview/accounts", authorizedDeployments: "api-prod"},
		{name: "overview account detail", path: "/v1/overview/accounts/123456789012", authorizedDeployments: "api-prod"},
		{name: "overview project environment detail", path: "/v1/overview/accounts/123456789012/projects/payments-api/environments/prod-apne2", authorizedDeployments: "api-prod"},
		{name: "overview deployment detail", path: "/v1/overview/deployments/api-prod", authorizedDeployments: "api-prod"},
		{name: "overview deployment events", path: "/v1/overview/deployments/api-prod/events", authorizedDeployments: "api-prod"},
		{name: "overview deployment resource logs", path: "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=api-prod-service", authorizedDeployments: "api-prod"},
		{name: "overview deployment resource inspection", path: "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=api-prod-service", authorizedDeployments: "api-prod"},
		{name: "audit logs collection", path: "/v1/audit-logs?action=apply.requested", authorizedDeployments: "api-prod"},
		{name: "deployment-scoped audit logs", path: "/v1/audit-logs?deployment=api-prod&action=apply.requested", authorizedDeployments: "api-prod"},
		{name: "deployment status collection", path: "/v1/status/deployments", authorizedDeployments: "api-prod"},
		{name: "deployment status detail", path: "/v1/status/deployments/api-prod", authorizedDeployments: "api-prod"},
		{name: "immutable plan preview read", path: "/v1/plans/plan_1234abcd", authorizedDeployments: "api-prod"},
		{name: "immutable apply request read", path: "/v1/applies/apply_1234abcd", authorizedDeployments: "api-prod"},
		{name: "queued immutable apply collection", path: "/v1/applies?status=queued", authorizedDeployments: "api-prod"},
	}

	for _, tc := range routes {
		t.Run(tc.name+" requires viewer-capable role", func(t *testing.T) {
			router, providerCalls := newGuardedRouter()
			req := newAuthenticatedRequest(tc.path, "", tc.authorizedDeployments)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d", resp.Code)
			}
			if body := resp.Body.String(); body != "forbidden: authenticated viewer-capable role is required for this operator read route\n" {
				t.Fatalf("unexpected 403 response body: %q", body)
			}
			if *providerCalls != 0 {
				t.Fatalf("expected role guard to short-circuit before providers, got %d calls", *providerCalls)
			}
		})

		t.Run(tc.name+" allows viewer role", func(t *testing.T) {
			router, providerCalls := newGuardedRouter()
			req := newAuthenticatedRequest(tc.path, "viewer", tc.authorizedDeployments)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
			}
			if *providerCalls == 0 {
				t.Fatal("expected read provider to run for viewer-capable role")
			}
		})
	}
}

func TestRouterDeploymentStatuses(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatuses: func(_ context.Context) ([]statusstore.Snapshot, error) {
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
		DeploymentStatuses: func(_ context.Context) ([]statusstore.Snapshot, error) {
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
		DeploymentStatuses: func(_ context.Context) ([]statusstore.Snapshot, error) {
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
		DeploymentStatus: func(_ context.Context, name string) (statusstore.Snapshot, error) {
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
	desiredCapacitySource := &domain.ManagedDriftSource{
		Kind:         "resourceInspection",
		Label:        "candidateAutoScalingGroup payments-api-prod-candidate",
		Route:        "/v1/overview/deployments/api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate",
		ResourceType: "candidateAutoScalingGroup",
		ResourceName: "payments-api-prod-candidate",
	}
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatus: func(_ context.Context, name string) (statusstore.Snapshot, error) {
			return statusstore.Snapshot{DeploymentName: "api-prod", Status: domain.DeploymentStatus{
				ObservedGeneration: 3,
				Sync:               domain.Sync{State: domain.SyncStateSynced, Reason: "RolloutCompleted"},
				Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "all checks passed"},
				ManagedDrift: []domain.ManagedDriftItem{
					{Field: "artifact", Desired: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r4", Actual: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r3", Severity: domain.ManagedDriftSeverityHigh},
					{Field: "runtimeContext.capacity.desired", Desired: "6", Actual: "4", Severity: domain.ManagedDriftSeverityHigh, ResourceRef: &domain.ManagedDriftResourceRef{ResourceType: "candidateAutoScalingGroup", ResourceName: "payments-api-prod-candidate"}, Source: desiredCapacitySource},
					{Field: "runtimeContext.capacity.min", Desired: "4", Actual: "2", Severity: domain.ManagedDriftSeverityMedium},
					{Field: "runtimeContext.capacity.max", Desired: "12", Actual: "8", Severity: domain.ManagedDriftSeverityLow},
					{Field: "runtimeContext.launchTemplate.imageId", Desired: "ami-1234567890abcdef0", Actual: "ami-fffffffffffffffff", Severity: domain.ManagedDriftSeverityHigh},
					{Field: "runtimeContext.launchTemplate.instanceType", Desired: "t3.medium", Actual: "m6i.large", Severity: domain.ManagedDriftSeverityHigh},
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
					Timestamp: "2026-04-16T07:05:00Z",
					Source:    "controller",
					Summary:   "Creating candidate capacity for weighted target groups",
					Sync:      domain.Sync{State: domain.SyncStateReconciling, Reason: "CandidateProvisioning"},
					Health:    domain.Health{Overall: domain.HealthProgressing, Summary: "warming"},
					ManagedDrift: []domain.ManagedDriftItem{
						{Field: "artifact", Desired: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r4", Actual: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r3", Severity: domain.ManagedDriftSeverityHigh},
						{Field: "runtimeContext.capacity.desired", Desired: "6", Actual: "4", Severity: domain.ManagedDriftSeverityHigh, ResourceRef: &domain.ManagedDriftResourceRef{ResourceType: "candidateAutoScalingGroup", ResourceName: "payments-api-prod-candidate"}, Source: desiredCapacitySource},
						{Field: "runtimeContext.capacity.min", Desired: "4", Actual: "2", Severity: domain.ManagedDriftSeverityMedium},
						{Field: "runtimeContext.capacity.max", Desired: "12", Actual: "8", Severity: domain.ManagedDriftSeverityLow},
						{Field: "runtimeContext.launchTemplate.imageId", Desired: "ami-1234567890abcdef0", Actual: "ami-fffffffffffffffff", Severity: domain.ManagedDriftSeverityHigh},
						{Field: "runtimeContext.launchTemplate.instanceType", Desired: "t3.medium", Actual: "m6i.large", Severity: domain.ManagedDriftSeverityHigh},
					},
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

	raw := resp.Body.String()
	for _, fragment := range []string{
		`"field":"runtimeContext.capacity.desired"`,
		`"field":"runtimeContext.capacity.min"`,
		`"field":"runtimeContext.capacity.max"`,
		`"field":"runtimeContext.launchTemplate.imageId"`,
		`"field":"runtimeContext.launchTemplate.instanceType"`,
	} {
		if !strings.Contains(raw, fragment) {
			t.Fatalf("expected raw deployment status payload to include %s, got %s", fragment, raw)
		}
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
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
	if !ok || len(managedDrift) != 6 {
		t.Fatalf("expected managedDrift array in raw payload, got %#v", status["managedDrift"])
	}
	driftItem := requireManagedDriftItemByField(t, managedDrift, "artifact")
	if driftItem["desired"] != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r4" || driftItem["actual"] != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r3" || driftItem["severity"] != string(domain.ManagedDriftSeverityHigh) {
		t.Fatalf("expected managedDrift artifact item fields, got %#v", driftItem)
	}
	if _, exists := driftItem["resourceRef"]; exists {
		t.Fatalf("expected read-model-only artifact drift to omit resourceRef, got %#v", driftItem)
	}
	if _, exists := driftItem["source"]; exists {
		t.Fatalf("expected drift row without known source to omit source, got %#v", driftItem)
	}
	driftID, ok := driftItem["driftId"].(string)
	if !ok || !strings.HasPrefix(driftID, "drift_") {
		t.Fatalf("expected raw status managedDrift row to include backend-authored driftId, got %#v", driftItem)
	}
	if desiredCapacityDrift := requireManagedDriftItemByField(t, managedDrift, "runtimeContext.capacity.desired"); desiredCapacityDrift["desired"] != "6" || desiredCapacityDrift["actual"] != "4" || desiredCapacityDrift["severity"] != string(domain.ManagedDriftSeverityHigh) {
		t.Fatalf("expected managedDrift desired-capacity item fields, got %#v", desiredCapacityDrift)
	} else {
		ref, ok := desiredCapacityDrift["resourceRef"].(map[string]any)
		if !ok || ref["resourceType"] != "candidateAutoScalingGroup" || ref["resourceName"] != "payments-api-prod-candidate" {
			t.Fatalf("expected managedDrift desired-capacity resourceRef, got %#v", desiredCapacityDrift["resourceRef"])
		}
		requireManagedDriftSource(t, desiredCapacityDrift, "resourceInspection", "candidateAutoScalingGroup payments-api-prod-candidate", "/v1/overview/deployments/api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate", "candidateAutoScalingGroup", "payments-api-prod-candidate")
	}
	if minCapacityDrift := requireManagedDriftItemByField(t, managedDrift, "runtimeContext.capacity.min"); minCapacityDrift["desired"] != "4" || minCapacityDrift["actual"] != "2" || minCapacityDrift["severity"] != string(domain.ManagedDriftSeverityMedium) {
		t.Fatalf("expected managedDrift min-capacity item fields, got %#v", minCapacityDrift)
	}
	if maxCapacityDrift := requireManagedDriftItemByField(t, managedDrift, "runtimeContext.capacity.max"); maxCapacityDrift["desired"] != "12" || maxCapacityDrift["actual"] != "8" || maxCapacityDrift["severity"] != string(domain.ManagedDriftSeverityLow) {
		t.Fatalf("expected managedDrift max-capacity item fields, got %#v", maxCapacityDrift)
	}
	if launchTemplateImageDrift := requireManagedDriftItemByField(t, managedDrift, "runtimeContext.launchTemplate.imageId"); launchTemplateImageDrift["desired"] != "ami-1234567890abcdef0" || launchTemplateImageDrift["actual"] != "ami-fffffffffffffffff" || launchTemplateImageDrift["severity"] != string(domain.ManagedDriftSeverityHigh) {
		t.Fatalf("expected managedDrift launch-template imageId item fields, got %#v", launchTemplateImageDrift)
	}
	if launchTemplateInstanceTypeDrift := requireManagedDriftItemByField(t, managedDrift, "runtimeContext.launchTemplate.instanceType"); launchTemplateInstanceTypeDrift["desired"] != "t3.medium" || launchTemplateInstanceTypeDrift["actual"] != "m6i.large" || launchTemplateInstanceTypeDrift["severity"] != string(domain.ManagedDriftSeverityHigh) {
		t.Fatalf("expected managedDrift launch-template instanceType item fields, got %#v", launchTemplateInstanceTypeDrift)
	}
	historyManagedDrift, ok := historyEntry["managedDrift"].([]any)
	if !ok || len(historyManagedDrift) != 6 {
		t.Fatalf("expected history managedDrift array, got %#v", historyEntry["managedDrift"])
	}
	if historyDesiredCapacityDrift := requireManagedDriftItemByField(t, historyManagedDrift, "runtimeContext.capacity.desired"); historyDesiredCapacityDrift["desired"] != "6" || historyDesiredCapacityDrift["actual"] != "4" {
		t.Fatalf("expected history managedDrift desired-capacity item fields, got %#v", historyDesiredCapacityDrift)
	} else {
		historyDriftID, ok := historyDesiredCapacityDrift["driftId"].(string)
		if !ok || !strings.HasPrefix(historyDriftID, "drift_") {
			t.Fatalf("expected history managedDrift row to include backend-authored driftId, got %#v", historyDesiredCapacityDrift)
		}
		ref, ok := historyDesiredCapacityDrift["resourceRef"].(map[string]any)
		if !ok || ref["resourceType"] != "candidateAutoScalingGroup" || ref["resourceName"] != "payments-api-prod-candidate" {
			t.Fatalf("expected history managedDrift desired-capacity resourceRef, got %#v", historyDesiredCapacityDrift["resourceRef"])
		}
		requireManagedDriftSource(t, historyDesiredCapacityDrift, "resourceInspection", "candidateAutoScalingGroup payments-api-prod-candidate", "/v1/overview/deployments/api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate", "candidateAutoScalingGroup", "payments-api-prod-candidate")
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
		DeploymentStatus: func(_ context.Context, name string) (statusstore.Snapshot, error) {
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
		DeploymentStatus: func(_ context.Context, name string) (statusstore.Snapshot, error) {
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
		DeploymentStatus: func(_ context.Context, name string) (statusstore.Snapshot, error) {
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
		OverviewAccounts: func(_ context.Context) ([]catalog.AccountOverview, error) {
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
				RuntimeEventCue: &catalog.RuntimeEventCue{
					DeploymentName:       "api-prod",
					Timestamp:            "2026-04-16T06:49:00Z",
					Source:               "controller",
					Summary:              "Traffic shift is progressing",
					Sync:                 domain.Sync{State: domain.SyncStateReconciling, Reason: "Traffic shift running"},
					Health:               domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
					RolloutPhase:         domain.RolloutPhaseAccepted,
					CurrentStep:          "PromoteCandidateCapacity",
					RetainedEventCount:   4,
					EventDeploymentCount: 1,
				},
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
	runtimeEventCue, ok := payload[0]["runtimeEventCue"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeEventCue object in raw overview accounts payload, got %#v", payload[0])
	}
	if runtimeEventCue["deploymentName"] != "api-prod" || runtimeEventCue["timestamp"] != "2026-04-16T06:49:00Z" || runtimeEventCue["source"] != "controller" || runtimeEventCue["summary"] != "Traffic shift is progressing" || runtimeEventCue["rolloutPhase"] != string(domain.RolloutPhaseAccepted) || runtimeEventCue["currentStep"] != "PromoteCandidateCapacity" || runtimeEventCue["retainedEventCount"] != float64(4) || runtimeEventCue["eventDeploymentCount"] != float64(1) {
		t.Fatalf("expected runtimeEventCue JSON fields in raw overview accounts payload, got %#v", runtimeEventCue)
	}
	cueSync, ok := runtimeEventCue["sync"].(map[string]any)
	if !ok || cueSync["state"] != string(domain.SyncStateReconciling) || cueSync["reason"] != "Traffic shift running" {
		t.Fatalf("expected runtimeEventCue.sync fields in raw overview accounts payload, got %#v", runtimeEventCue["sync"])
	}
	cueHealth, ok := runtimeEventCue["health"].(map[string]any)
	if !ok || cueHealth["overall"] != string(domain.HealthHealthy) || cueHealth["summary"] != "steady" {
		t.Fatalf("expected runtimeEventCue.health fields in raw overview accounts payload, got %#v", runtimeEventCue["health"])
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
		OverviewAccounts: func(_ context.Context) ([]catalog.AccountOverview, error) {
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
		OverviewAccounts: func(_ context.Context) ([]catalog.AccountOverview, error) {
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
		OverviewApplications: func(_ context.Context) ([]catalog.ApplicationOverview, error) {
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
		OverviewApplications: func(_ context.Context) ([]catalog.ApplicationOverview, error) {
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
			OverviewApplication: func(_ context.Context, name string) (catalog.ApplicationOverviewDetail, error) {
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
			OverviewApplication: func(_ context.Context, _ string) (catalog.ApplicationOverviewDetail, error) {
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
			OverviewApplication: func(_ context.Context, _ string) (catalog.ApplicationOverviewDetail, error) {
				return catalog.ApplicationOverviewDetail{}, errors.New(upstreamErr)
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications/payments-api-prod", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, upstreamErr)
	})

	t.Run("treats request deadline as unavailable", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewApplication: func(ctx context.Context, _ string) (catalog.ApplicationOverviewDetail, error) {
				return catalog.ApplicationOverviewDetail{}, ctx.Err()
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications/payments-api-prod", nil).WithContext(ctx)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "overview application unavailable\n" {
			t.Fatalf("expected unavailable body, got %q", body)
		}
	})

	t.Run("rejects invalid names", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewApplication: func(_ context.Context, _ string) (catalog.ApplicationOverviewDetail, error) {
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
		OverviewAccounts: func(_ context.Context) ([]catalog.AccountOverview, error) {
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
		OverviewAccount: func(_ context.Context, awsAccountID string) (catalog.AccountOverviewDetail, error) {
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
				RuntimeEventCue: &catalog.RuntimeEventCue{
					DeploymentName:       "api-prod",
					Timestamp:            "2026-04-16T06:49:00Z",
					Source:               "controller",
					Summary:              "Traffic shift is progressing",
					Sync:                 domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
					Health:               domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
					RolloutPhase:         domain.RolloutPhaseCompleted,
					CurrentStep:          "Complete",
					RetainedEventCount:   2,
					EventDeploymentCount: 1,
				},
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
	runtimeEventCue, ok := payload["runtimeEventCue"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeEventCue object in raw overview account detail payload, got %#v", payload)
	}
	if runtimeEventCue["deploymentName"] != "api-prod" || runtimeEventCue["timestamp"] != "2026-04-16T06:49:00Z" || runtimeEventCue["source"] != "controller" || runtimeEventCue["summary"] != "Traffic shift is progressing" || runtimeEventCue["rolloutPhase"] != string(domain.RolloutPhaseCompleted) || runtimeEventCue["currentStep"] != "Complete" || runtimeEventCue["retainedEventCount"] != float64(2) || runtimeEventCue["eventDeploymentCount"] != float64(1) {
		t.Fatalf("expected runtimeEventCue JSON fields in raw overview account detail payload, got %#v", runtimeEventCue)
	}
	cueSync, ok := runtimeEventCue["sync"].(map[string]any)
	if !ok || cueSync["state"] != string(domain.SyncStateSynced) || cueSync["reason"] != "steady" {
		t.Fatalf("expected runtimeEventCue.sync fields in raw overview account detail payload, got %#v", runtimeEventCue["sync"])
	}
	cueHealth, ok := runtimeEventCue["health"].(map[string]any)
	if !ok || cueHealth["overall"] != string(domain.HealthHealthy) || cueHealth["summary"] != "steady" {
		t.Fatalf("expected runtimeEventCue.health fields in raw overview account detail payload, got %#v", runtimeEventCue["health"])
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
			OverviewAccount: func(_ context.Context, _ string) (catalog.AccountOverviewDetail, error) {
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
			OverviewAccount: func(_ context.Context, _ string) (catalog.AccountOverviewDetail, error) {
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
			OverviewAccount: func(_ context.Context, _ string) (catalog.AccountOverviewDetail, error) {
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
		OverviewProjectEnvironment: func(_ context.Context, accountID, appRef, environmentRef string) (catalog.ProjectEnvironmentOverviewDetail, error) {
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
				RuntimeEventCue: &catalog.RuntimeEventCue{
					DeploymentName:       "payments-api-prod",
					Timestamp:            "2026-04-16T06:49:00Z",
					Source:               "controller",
					Summary:              "Traffic shift is progressing",
					Sync:                 domain.Sync{State: domain.SyncStateReconciling, Reason: "warming"},
					Health:               domain.Health{Overall: domain.HealthProgressing, Summary: "candidate warming"},
					RolloutPhase:         domain.RolloutPhaseAccepted,
					CurrentStep:          "ShiftTrafficCanary",
					RetainedEventCount:   3,
					EventDeploymentCount: 1,
				},
				Deployments: []catalog.DeploymentOverview{{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "ap-northeast-2", ExecutionMode: "asgWeightedTargetGroups", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "warming"}}, {Name: "payments-worker-prod", AppRef: "payments-api", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "ap-northeast-2", ExecutionMode: "asgWeightedTargetGroups", Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "steady"}}},
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
	runtimeEventCue, ok := payload["runtimeEventCue"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeEventCue object in raw grouped overview payload, got %#v", payload)
	}
	if runtimeEventCue["deploymentName"] != "payments-api-prod" || runtimeEventCue["timestamp"] != "2026-04-16T06:49:00Z" || runtimeEventCue["source"] != "controller" || runtimeEventCue["summary"] != "Traffic shift is progressing" || runtimeEventCue["rolloutPhase"] != string(domain.RolloutPhaseAccepted) || runtimeEventCue["currentStep"] != "ShiftTrafficCanary" || runtimeEventCue["retainedEventCount"] != float64(3) || runtimeEventCue["eventDeploymentCount"] != float64(1) {
		t.Fatalf("expected runtimeEventCue JSON fields in raw grouped overview payload, got %#v", runtimeEventCue)
	}
	cueSync, ok := runtimeEventCue["sync"].(map[string]any)
	if !ok || cueSync["state"] != string(domain.SyncStateReconciling) || cueSync["reason"] != "warming" {
		t.Fatalf("expected runtimeEventCue.sync fields in raw grouped overview payload, got %#v", runtimeEventCue["sync"])
	}
	cueHealth, ok := runtimeEventCue["health"].(map[string]any)
	if !ok || cueHealth["overall"] != string(domain.HealthProgressing) || cueHealth["summary"] != "candidate warming" {
		t.Fatalf("expected runtimeEventCue.health fields in raw grouped overview payload, got %#v", runtimeEventCue["health"])
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
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}, OverviewProjectEnvironment: func(_ context.Context, _, _, _ string) (catalog.ProjectEnvironmentOverviewDetail, error) {
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
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}, OverviewProjectEnvironment: func(_ context.Context, _, _, _ string) (catalog.ProjectEnvironmentOverviewDetail, error) {
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

func TestRouterOverviewAccountsDeploymentReadAuthorization(t *testing.T) {
	newAuthenticatedRequest := func(path string, authorizedDeployments string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		if authorizedDeployments != "" {
			req.Header.Set(requestactor.AuthorizedDeploymentsHeader, authorizedDeployments)
		}
		return req
	}

	t.Run("collection unauthenticated", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			OverviewAccounts: func(context.Context) ([]catalog.AccountOverview, error) {
				providerCalls++
				return []catalog.AccountOverview{{Name: "prod-core"}}, nil
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator read route\n" {
			t.Fatalf("unexpected 401 response body: %q", body)
		}
		if providerCalls != 0 {
			t.Fatalf("expected auth guard to short-circuit before overview accounts provider, got %d calls", providerCalls)
		}
	})

	t.Run("collection filters visible deployments and recomputes counts", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			OverviewAccounts: func(context.Context) ([]catalog.AccountOverview, error) {
				return []catalog.AccountOverview{
					{
						Name:             "prod-core",
						AWSAccountID:     "123456789012",
						AllowedRegions:   []string{"ap-northeast-2", "us-west-2"},
						DeploymentCount:  3,
						ProjectCount:     2,
						EnvironmentCount: 2,
						SyncedCount:      1,
						ProgressingCount: 1,
						PausedCount:      1,
						RuntimeEventCue: &catalog.RuntimeEventCue{
							DeploymentName:       "hidden-prod",
							Timestamp:            "2026-04-16T07:00:00Z",
							Source:               "controller",
							Summary:              "Hidden deployment event",
							RetainedEventCount:   9,
							EventDeploymentCount: 3,
						},
						Deployments: []catalog.DeploymentOverview{
							{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", Sync: domain.Sync{State: domain.SyncStateSynced}},
							{Name: "worker-prod", AppRef: "worker", EnvironmentRef: "prod-apne2", Sync: domain.Sync{State: domain.SyncStatePaused}, RuntimeSummary: &domain.RuntimeSummary{Timestamp: "2026-04-16T06:10:00Z", Source: "controller", Summary: "Worker paused", Sync: domain.Sync{State: domain.SyncStatePaused}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "steady"}, HistoryCount: 4}},
							{Name: "payments-api-staging", AppRef: "payments-api", EnvironmentRef: "staging-apne2", Sync: domain.Sync{State: domain.SyncStateReconciling}, RuntimeSummary: &domain.RuntimeSummary{Timestamp: "2026-04-16T06:30:00Z", Source: "deployment-controller", Summary: "Staging rollout progressing", Sync: domain.Sync{State: domain.SyncStateReconciling}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "warming"}, RolloutPhase: domain.RolloutPhasePromoting, CurrentStep: "ShiftTrafficLinear", HistoryCount: 1}},
						},
					},
					{
						Name:             "sandbox",
						AWSAccountID:     "999999999999",
						DeploymentCount:  1,
						ProjectCount:     1,
						EnvironmentCount: 1,
						SyncedCount:      1,
						Deployments: []catalog.DeploymentOverview{
							{Name: "shadow-prod", AppRef: "shadow", EnvironmentRef: "prod-apne2", Sync: domain.Sync{State: domain.SyncStateSynced}},
						},
					},
				}, nil
			},
		})
		req := newAuthenticatedRequest("/v1/overview/accounts", "payments-api-staging, worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}

		var accounts []catalog.AccountOverview
		if err := json.Unmarshal(resp.Body.Bytes(), &accounts); err != nil {
			t.Fatalf("decode overview accounts: %v", err)
		}
		if len(accounts) != 1 {
			t.Fatalf("expected one visible account, got %+v", accounts)
		}
		if got := accounts[0]; got.Name != "prod-core" || got.DeploymentCount != 2 || got.ProjectCount != 2 || got.EnvironmentCount != 2 || got.SyncedCount != 0 || got.ProgressingCount != 1 || got.PausedCount != 1 || got.ErrorCount != 0 {
			t.Fatalf("unexpected filtered overview account aggregates: %+v", got)
		}
		if got := accounts[0].Deployments; len(got) != 2 || got[0].Name != "worker-prod" || got[1].Name != "payments-api-staging" {
			t.Fatalf("unexpected filtered overview account deployments: %+v", got)
		}
		if cue := accounts[0].RuntimeEventCue; cue == nil || cue.DeploymentName != "payments-api-staging" || cue.Timestamp != "2026-04-16T06:30:00Z" || cue.Source != "deployment-controller" || cue.Summary != "Staging rollout progressing" || cue.RetainedEventCount != 5 || cue.EventDeploymentCount != 2 {
			t.Fatalf("expected filtered overview account runtime event cue from visible deployments, got %+v", cue)
		}
	})

	t.Run("collection returns empty array when allowlist has no matches", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			OverviewAccounts: func(context.Context) ([]catalog.AccountOverview, error) {
				return []catalog.AccountOverview{{
					Name:         "prod-core",
					AWSAccountID: "123456789012",
					Deployments: []catalog.DeploymentOverview{
						{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"},
					},
				}}, nil
			},
		})
		req := newAuthenticatedRequest("/v1/overview/accounts", "worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if body := strings.TrimSpace(resp.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array, got %q", body)
		}
	})

	t.Run("account detail forbidden when scope has no authorized deployments", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			OverviewAccount: func(_ context.Context, awsAccountID string) (catalog.AccountOverviewDetail, error) {
				providerCalls++
				if awsAccountID != "123456789012" {
					t.Fatalf("unexpected lookup aws account id: %s", awsAccountID)
				}
				return catalog.AccountOverviewDetail{
					Name:         "prod-core",
					AWSAccountID: "123456789012",
					Deployments: []catalog.DeploymentOverview{
						{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", Sync: domain.Sync{State: domain.SyncStateSynced}},
					},
				}, nil
			},
		})
		req := newAuthenticatedRequest("/v1/overview/accounts/123456789012", "worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "forbidden: operator does not have access to this deployment\n" {
			t.Fatalf("unexpected 403 response body: %q", body)
		}
		if providerCalls != 1 {
			t.Fatalf("expected scoped account detail lookup before deny, got %d calls", providerCalls)
		}
	})

	t.Run("account detail filters visible deployments and recomputes counts", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			OverviewAccount: func(_ context.Context, awsAccountID string) (catalog.AccountOverviewDetail, error) {
				if awsAccountID != "123456789012" {
					t.Fatalf("unexpected lookup aws account id: %s", awsAccountID)
				}
				return catalog.AccountOverviewDetail{
					Name:             "prod-core",
					AWSAccountID:     "123456789012",
					AllowedRegions:   []string{"ap-northeast-2", "us-west-2"},
					DeploymentCount:  3,
					ProjectCount:     2,
					EnvironmentCount: 2,
					SyncedCount:      1,
					ProgressingCount: 1,
					PausedCount:      1,
					Deployments: []catalog.DeploymentOverview{
						{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", Sync: domain.Sync{State: domain.SyncStateSynced}},
						{Name: "worker-prod", AppRef: "worker", EnvironmentRef: "prod-apne2", Sync: domain.Sync{State: domain.SyncStatePaused}},
						{Name: "payments-api-staging", AppRef: "payments-api", EnvironmentRef: "staging-apne2", Sync: domain.Sync{State: domain.SyncStateReconciling}},
					},
				}, nil
			},
		})
		req := newAuthenticatedRequest("/v1/overview/accounts/123456789012", "payments-api-staging")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}

		var detail catalog.AccountOverviewDetail
		if err := json.Unmarshal(resp.Body.Bytes(), &detail); err != nil {
			t.Fatalf("decode overview account detail: %v", err)
		}
		if detail.DeploymentCount != 1 || detail.ProjectCount != 1 || detail.EnvironmentCount != 1 || detail.SyncedCount != 0 || detail.ProgressingCount != 1 || detail.PausedCount != 0 || detail.ErrorCount != 0 {
			t.Fatalf("unexpected filtered overview account detail aggregates: %+v", detail)
		}
		if got := detail.Deployments; len(got) != 1 || got[0].Name != "payments-api-staging" {
			t.Fatalf("unexpected filtered overview account detail deployments: %+v", got)
		}
	})

	t.Run("group detail forbidden when scope has no authorized deployments", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			OverviewProjectEnvironment: func(_ context.Context, accountID, appRef, environmentRef string) (catalog.ProjectEnvironmentOverviewDetail, error) {
				providerCalls++
				if accountID != "123456789012" || appRef != "payments-api" || environmentRef != "prod-apne2" {
					t.Fatalf("unexpected grouped lookup: %s %s %s", accountID, appRef, environmentRef)
				}
				return catalog.ProjectEnvironmentOverviewDetail{
					AccountName:    "prod-core",
					AWSAccountID:   "123456789012",
					AppRef:         "payments-api",
					EnvironmentRef: "prod-apne2",
					Deployments: []catalog.DeploymentOverview{
						{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", Sync: domain.Sync{State: domain.SyncStateSynced}},
					},
				}, nil
			},
		})
		req := newAuthenticatedRequest("/v1/overview/accounts/123456789012/projects/payments-api/environments/prod-apne2", "worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "forbidden: operator does not have access to this deployment\n" {
			t.Fatalf("unexpected 403 response body: %q", body)
		}
		if providerCalls != 1 {
			t.Fatalf("expected grouped overview lookup before deny, got %d calls", providerCalls)
		}
	})

	t.Run("group detail filters visible deployments and recomputes counts", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			OverviewProjectEnvironment: func(_ context.Context, accountID, appRef, environmentRef string) (catalog.ProjectEnvironmentOverviewDetail, error) {
				if accountID != "123456789012" || appRef != "payments-api" || environmentRef != "prod-apne2" {
					t.Fatalf("unexpected grouped lookup: %s %s %s", accountID, appRef, environmentRef)
				}
				return catalog.ProjectEnvironmentOverviewDetail{
					AccountName:      "prod-core",
					AWSAccountID:     "123456789012",
					AppRef:           "payments-api",
					EnvironmentRef:   "prod-apne2",
					Region:           "ap-northeast-2",
					DeploymentCount:  3,
					SyncedCount:      1,
					ProgressingCount: 1,
					PausedCount:      1,
					Deployments: []catalog.DeploymentOverview{
						{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", Sync: domain.Sync{State: domain.SyncStateSynced}},
						{Name: "payments-worker-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", Sync: domain.Sync{State: domain.SyncStateReconciling}},
						{Name: "payments-cron-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", Sync: domain.Sync{State: domain.SyncStatePaused}},
					},
				}, nil
			},
		})
		req := newAuthenticatedRequest("/v1/overview/accounts/123456789012/projects/payments-api/environments/prod-apne2", "payments-api-prod, payments-cron-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}

		var detail catalog.ProjectEnvironmentOverviewDetail
		if err := json.Unmarshal(resp.Body.Bytes(), &detail); err != nil {
			t.Fatalf("decode grouped overview detail: %v", err)
		}
		if detail.DeploymentCount != 2 || detail.SyncedCount != 1 || detail.ProgressingCount != 0 || detail.PausedCount != 1 || detail.ErrorCount != 0 {
			t.Fatalf("unexpected filtered grouped overview aggregates: %+v", detail)
		}
		if got := detail.Deployments; len(got) != 2 || got[0].Name != "payments-api-prod" || got[1].Name != "payments-cron-prod" {
			t.Fatalf("unexpected filtered grouped overview deployments: %+v", got)
		}
	})
}

func TestRouterOverviewDeployment(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
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

func TestDeploymentOverviewDetailRoutePreservesDriftedControllerSummariesInRawPayload(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
			if name != "api-prod" {
				return catalog.DeploymentOverviewDetail{}, statusstore.ErrNotFound
			}
			return catalog.DeploymentOverviewDetail{
				Name: "api-prod",
				Sync: domain.Sync{State: domain.SyncStateDrifted, Reason: "Controller detected managed drift in completed deployment"},
				ManagedDrift: []domain.ManagedDriftItem{{
					Field:    "artifact",
					Desired:  "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7",
					Actual:   "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6",
					Severity: domain.ManagedDriftSeverityHigh,
				}},
				RuntimeSummary: &domain.RuntimeSummary{
					Summary: "Deployment has managed drift",
					Sync:    domain.Sync{State: domain.SyncStateDrifted, Reason: "Controller detected managed drift in completed deployment"},
					Health:  domain.Health{Overall: domain.HealthHealthy, Summary: "Deployment is healthy but has managed drift"},
					ManagedDrift: []domain.ManagedDriftItem{{
						Field:    "artifact",
						Desired:  "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7",
						Actual:   "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6",
						Severity: domain.ManagedDriftSeverityHigh,
					}},
					DesiredGeneration:  7,
					ObservedGeneration: 7,
					RolloutPhase:       domain.RolloutPhaseCompleted,
					HistoryCount:       1,
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
	sync, ok := payload["sync"].(map[string]any)
	if !ok || sync["state"] != string(domain.SyncStateDrifted) || sync["reason"] != "Controller detected managed drift in completed deployment" {
		t.Fatalf("expected drifted sync fields in raw payload, got %#v", payload["sync"])
	}
	runtimeSummary, ok := payload["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in raw payload, got %#v", payload["runtimeSummary"])
	}
	if runtimeSummary["summary"] != "Deployment has managed drift" {
		t.Fatalf("expected runtimeSummary.summary to keep drift-aware controller wording, got %#v", runtimeSummary["summary"])
	}
	runtimeSync, ok := runtimeSummary["sync"].(map[string]any)
	if !ok || runtimeSync["state"] != string(domain.SyncStateDrifted) || runtimeSync["reason"] != "Controller detected managed drift in completed deployment" {
		t.Fatalf("expected runtimeSummary.sync drifted fields, got %#v", runtimeSummary["sync"])
	}
	runtimeHealth, ok := runtimeSummary["health"].(map[string]any)
	if !ok || runtimeHealth["overall"] != string(domain.HealthHealthy) || runtimeHealth["summary"] != "Deployment is healthy but has managed drift" {
		t.Fatalf("expected runtimeSummary.health drift-aware fields, got %#v", runtimeSummary["health"])
	}
}

func TestDeploymentOverviewDetailRouteOmitsRuntimeSummarySourceWithoutHistory(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
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
		OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
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
		OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
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
	taskDefinitionImageSource := &domain.ManagedDriftSource{
		Kind:         "resourceInspection",
		Label:        "ecsService payments-api-prod-service",
		Route:        "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=payments-api-prod-service",
		ResourceType: "ecsService",
		ResourceName: "payments-api-prod-service",
	}
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{
				Name: "api-prod",
				ManagedDrift: []domain.ManagedDriftItem{
					{Field: "artifact", Desired: "ghcr.io/devopsartfactory/payments-api:2026-04-16.2", Actual: "ghcr.io/devopsartfactory/payments-api:2026-04-16.1", Severity: domain.ManagedDriftSeverityHigh},
					{Field: "runtimeContext.capacity.desired", Desired: "6", Actual: "4", Severity: domain.ManagedDriftSeverityHigh, ResourceRef: &domain.ManagedDriftResourceRef{ResourceType: "candidateAutoScalingGroup", ResourceName: "payments-api-prod-candidate"}},
					{Field: "runtimeContext.capacity.min", Desired: "4", Actual: "2", Severity: domain.ManagedDriftSeverityMedium},
					{Field: "runtimeContext.capacity.max", Desired: "12", Actual: "8", Severity: domain.ManagedDriftSeverityLow},
					{Field: "runtimeContext.launchTemplate.imageId", Desired: "ami-1234567890abcdef0", Actual: "ami-fffffffffffffffff", Severity: domain.ManagedDriftSeverityHigh},
					{Field: "runtimeContext.launchTemplate.instanceType", Desired: "t3.medium", Actual: "m6i.large", Severity: domain.ManagedDriftSeverityHigh},
					{Field: "runtimeContext.taskDefinition.image", Desired: "ghcr.io/devopsartfactory/payments-api:2026-04-16.2", Actual: "ghcr.io/devopsartfactory/payments-api:2026-04-16.1", Severity: domain.ManagedDriftSeverityHigh, Source: taskDefinitionImageSource},
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
						{Field: "runtimeContext.capacity.desired", Desired: "6", Actual: "4", Severity: domain.ManagedDriftSeverityHigh, ResourceRef: &domain.ManagedDriftResourceRef{ResourceType: "candidateAutoScalingGroup", ResourceName: "payments-api-prod-candidate"}},
						{Field: "runtimeContext.capacity.min", Desired: "4", Actual: "2", Severity: domain.ManagedDriftSeverityMedium},
						{Field: "runtimeContext.capacity.max", Desired: "12", Actual: "8", Severity: domain.ManagedDriftSeverityLow},
						{Field: "runtimeContext.launchTemplate.imageId", Desired: "ami-1234567890abcdef0", Actual: "ami-fffffffffffffffff", Severity: domain.ManagedDriftSeverityHigh},
						{Field: "runtimeContext.launchTemplate.instanceType", Desired: "t3.medium", Actual: "m6i.large", Severity: domain.ManagedDriftSeverityHigh},
						{Field: "runtimeContext.taskDefinition.image", Desired: "ghcr.io/devopsartfactory/payments-api:2026-04-16.2", Actual: "ghcr.io/devopsartfactory/payments-api:2026-04-16.1", Severity: domain.ManagedDriftSeverityHigh, Source: taskDefinitionImageSource},
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

	raw := resp.Body.String()
	for _, fragment := range []string{
		`"field":"runtimeContext.capacity.desired"`,
		`"field":"runtimeContext.capacity.min"`,
		`"field":"runtimeContext.capacity.max"`,
		`"field":"runtimeContext.launchTemplate.imageId"`,
		`"field":"runtimeContext.launchTemplate.instanceType"`,
		`"field":"runtimeContext.taskDefinition.image"`,
	} {
		if !strings.Contains(raw, fragment) {
			t.Fatalf("expected raw overview deployment payload to include %s, got %s", fragment, raw)
		}
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
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
	if !ok || len(managedDrift) != 7 {
		t.Fatalf("expected managedDrift array in overview deployment payload, got %#v", payload["managedDrift"])
	}
	driftItem := requireManagedDriftItemByField(t, managedDrift, "artifact")
	if driftItem["severity"] != string(domain.ManagedDriftSeverityHigh) || driftItem["desired"] != "ghcr.io/devopsartfactory/payments-api:2026-04-16.2" {
		t.Fatalf("expected managedDrift artifact item fields in overview deployment payload, got %#v", driftItem)
	}
	if _, exists := driftItem["source"]; exists {
		t.Fatalf("expected overview drift row without known source to omit source, got %#v", driftItem)
	}
	if taskDefinitionImageDrift := requireManagedDriftItemByField(t, managedDrift, "runtimeContext.taskDefinition.image"); taskDefinitionImageDrift["desired"] != "ghcr.io/devopsartfactory/payments-api:2026-04-16.2" || taskDefinitionImageDrift["actual"] != "ghcr.io/devopsartfactory/payments-api:2026-04-16.1" || taskDefinitionImageDrift["severity"] != string(domain.ManagedDriftSeverityHigh) {
		t.Fatalf("expected managedDrift task-definition image item fields in overview deployment payload, got %#v", taskDefinitionImageDrift)
	} else {
		requireManagedDriftSource(t, taskDefinitionImageDrift, "resourceInspection", "ecsService payments-api-prod-service", "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=payments-api-prod-service", "ecsService", "payments-api-prod-service")
	}
	if launchTemplateImageDrift := requireManagedDriftItemByField(t, managedDrift, "runtimeContext.launchTemplate.imageId"); launchTemplateImageDrift["desired"] != "ami-1234567890abcdef0" || launchTemplateImageDrift["actual"] != "ami-fffffffffffffffff" || launchTemplateImageDrift["severity"] != string(domain.ManagedDriftSeverityHigh) {
		t.Fatalf("expected managedDrift launch-template imageId item fields in overview deployment payload, got %#v", launchTemplateImageDrift)
	}
	if launchTemplateInstanceTypeDrift := requireManagedDriftItemByField(t, managedDrift, "runtimeContext.launchTemplate.instanceType"); launchTemplateInstanceTypeDrift["desired"] != "t3.medium" || launchTemplateInstanceTypeDrift["actual"] != "m6i.large" || launchTemplateInstanceTypeDrift["severity"] != string(domain.ManagedDriftSeverityHigh) {
		t.Fatalf("expected managedDrift launch-template instanceType item fields in overview deployment payload, got %#v", launchTemplateInstanceTypeDrift)
	}
	runtimeManagedDrift, ok := runtimeSummary["managedDrift"].([]any)
	if !ok || len(runtimeManagedDrift) != 7 {
		t.Fatalf("expected runtimeSummary.managedDrift array in overview deployment payload, got %#v", runtimeSummary["managedDrift"])
	}
	if desiredCapacityDrift := requireManagedDriftItemByField(t, runtimeManagedDrift, "runtimeContext.capacity.desired"); desiredCapacityDrift["desired"] != "6" || desiredCapacityDrift["actual"] != "4" || desiredCapacityDrift["severity"] != string(domain.ManagedDriftSeverityHigh) {
		t.Fatalf("expected runtimeSummary desired-capacity drift item fields, got %#v", desiredCapacityDrift)
	}
	if minCapacityDrift := requireManagedDriftItemByField(t, runtimeManagedDrift, "runtimeContext.capacity.min"); minCapacityDrift["desired"] != "4" || minCapacityDrift["actual"] != "2" || minCapacityDrift["severity"] != string(domain.ManagedDriftSeverityMedium) {
		t.Fatalf("expected runtimeSummary min-capacity drift item fields, got %#v", minCapacityDrift)
	}
	if maxCapacityDrift := requireManagedDriftItemByField(t, runtimeManagedDrift, "runtimeContext.capacity.max"); maxCapacityDrift["desired"] != "12" || maxCapacityDrift["actual"] != "8" || maxCapacityDrift["severity"] != string(domain.ManagedDriftSeverityLow) {
		t.Fatalf("expected runtimeSummary max-capacity drift item fields, got %#v", maxCapacityDrift)
	}
	if launchTemplateImageDrift := requireManagedDriftItemByField(t, runtimeManagedDrift, "runtimeContext.launchTemplate.imageId"); launchTemplateImageDrift["desired"] != "ami-1234567890abcdef0" || launchTemplateImageDrift["actual"] != "ami-fffffffffffffffff" || launchTemplateImageDrift["severity"] != string(domain.ManagedDriftSeverityHigh) {
		t.Fatalf("expected runtimeSummary launch-template imageId drift item fields, got %#v", launchTemplateImageDrift)
	}
	if launchTemplateInstanceTypeDrift := requireManagedDriftItemByField(t, runtimeManagedDrift, "runtimeContext.launchTemplate.instanceType"); launchTemplateInstanceTypeDrift["desired"] != "t3.medium" || launchTemplateInstanceTypeDrift["actual"] != "m6i.large" || launchTemplateInstanceTypeDrift["severity"] != string(domain.ManagedDriftSeverityHigh) {
		t.Fatalf("expected runtimeSummary launch-template instanceType drift item fields, got %#v", launchTemplateInstanceTypeDrift)
	}
	if taskDefinitionImageDrift := requireManagedDriftItemByField(t, runtimeManagedDrift, "runtimeContext.taskDefinition.image"); taskDefinitionImageDrift["desired"] != "ghcr.io/devopsartfactory/payments-api:2026-04-16.2" || taskDefinitionImageDrift["actual"] != "ghcr.io/devopsartfactory/payments-api:2026-04-16.1" || taskDefinitionImageDrift["severity"] != string(domain.ManagedDriftSeverityHigh) {
		t.Fatalf("expected runtimeSummary task-definition image drift item fields, got %#v", taskDefinitionImageDrift)
	} else {
		requireManagedDriftSource(t, taskDefinitionImageDrift, "resourceInspection", "ecsService payments-api-prod-service", "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=payments-api-prod-service", "ecsService", "payments-api-prod-service")
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
		OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
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
		OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
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
		OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
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
	t.Run("returns JSON unavailable envelope when resource logs provider is missing", func(t *testing.T) {
		var overviewCalls int
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				overviewCalls++
				if name != "worker-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsTask&resourceName=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Atask%2Fworker-prod%2Fabc123&containerName=worker&instanceId=i-0abc123def4567890&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d with body %q", resp.Code, resp.Body.String())
		}
		if contentType := resp.Header().Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("expected JSON content type, got %q", contentType)
		}
		if overviewCalls != 1 {
			t.Fatalf("expected deployment lookup before missing provider envelope, got %d calls", overviewCalls)
		}

		var got resourcelogs.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode missing provider resource logs payload: %v; body=%q", err, resp.Body.String())
		}
		if got.DeploymentName != "worker-prod" || got.ResourceType != "ecsTask" || got.ResourceName != "arn:aws:ecs:ap-northeast-2:123456789012:task/worker-prod/abc123" {
			t.Fatalf("expected missing provider payload to echo selected resource identifiers, got %+v", got)
		}
		if got.ContainerName != "worker" || got.InstanceID != "i-0abc123def4567890" || got.ClusterARN != "arn:aws:ecs:ap-northeast-2:123456789012:cluster/worker-prod" {
			t.Fatalf("expected missing provider payload to echo accepted selectors, got %+v", got)
		}
		if got.Available || got.UnavailableReason != "overview deployment resource logs unavailable" {
			t.Fatalf("expected unavailable missing provider envelope, got %+v", got)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("decode raw missing provider resource logs payload: %v", err)
		}
		for _, key := range []string{"deploymentName", "resourceType", "resourceName", "containerName", "instanceId", "clusterArn", "available", "unavailableReason"} {
			if _, ok := raw[key]; !ok {
				t.Fatalf("expected raw missing provider payload to include %s, got %#v", key, raw)
			}
		}
	})

	t.Run("enforces deployment read authorization and preserves unavailable payload when authorized", func(t *testing.T) {
		const (
			path                 = "/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service"
			deploymentName       = "worker-prod"
			resourceType         = "ecsService"
			resourceName         = "worker-prod-service"
			unavailableReason    = "resource logs are unavailable because live AWS access is not configured for this local runtime"
			authenticatedID      = "front-door-123"
			authenticatedName    = "Front Door Operator"
			authenticatedEmail   = "frontdoor@example.com"
			forbiddenDeployments = "payments-api-staging"
			allowedDeployments   = "worker-prod, payments-api-staging"
		)

		expected := resourcelogs.Result{
			DeploymentName: deploymentName,
			ResourceType:   resourceType,
			ResourceName:   resourceName,
			LookupContext: resourcelogs.LookupContext{
				Region:                 "ap-northeast-2",
				ControlAccountID:       "123456789012",
				TargetAccountID:        "123456789012",
				CrossAccount:           false,
				ObserverRoleConfigured: false,
			},
			Available: false,
			Entries:   []resourcelogs.Entry{},
			ResultScope: &resourcelogs.ResultScope{
				EffectiveLimit:     resourcelogs.DefaultMaxEntries,
				Limit:              resourcelogs.DefaultMaxEntries,
				ReturnedEntryCount: 0,
				TruncationStatus:   resourcelogs.ScopeTruncationStatusNotApplicable,
			},
			UnavailableReason: unavailableReason,
		}

		var overviewCalls int
		var resourceLogCalls int
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				overviewCalls++
				if name != deploymentName {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(_ context.Context, gotDeploymentName, gotResourceType, gotResourceName string, limit int) (resourcelogs.Result, error) {
				resourceLogCalls++
				if gotDeploymentName != deploymentName || gotResourceType != resourceType || gotResourceName != resourceName {
					t.Fatalf("unexpected resource logs request: deployment=%q type=%q name=%q", gotDeploymentName, gotResourceType, gotResourceName)
				}
				return expected, nil
			},
		})

		newAuthenticatedRequest := func(authorizedDeployments string) *http.Request {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set(requestactor.AuthenticatedHeaderID, authenticatedID)
			req.Header.Set(requestactor.AuthenticatedHeaderName, authenticatedName)
			req.Header.Set(requestactor.AuthenticatedHeaderEmail, authenticatedEmail)
			if authorizedDeployments != "" {
				req.Header.Set(requestactor.AuthorizedDeploymentsHeader, authorizedDeployments)
			}
			return req
		}

		t.Run("unauthenticated", func(t *testing.T) {
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))

			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", resp.Code)
			}
			if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator read route\n" {
				t.Fatalf("unexpected 401 response body: %q", body)
			}
			if overviewCalls != 0 || resourceLogCalls != 0 {
				t.Fatalf("expected auth guard to short-circuit before providers, got overview=%d resourceLogs=%d", overviewCalls, resourceLogCalls)
			}
		})

		t.Run("forbidden", func(t *testing.T) {
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, newAuthenticatedRequest(forbiddenDeployments))

			if resp.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d", resp.Code)
			}
			if body := resp.Body.String(); body != "forbidden: operator does not have access to this deployment\n" {
				t.Fatalf("unexpected 403 response body: %q", body)
			}
			if overviewCalls != 0 || resourceLogCalls != 0 {
				t.Fatalf("expected auth guard to short-circuit before providers, got overview=%d resourceLogs=%d", overviewCalls, resourceLogCalls)
			}
		})

		t.Run("allowed keeps unavailable payload", func(t *testing.T) {
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, newAuthenticatedRequest(allowedDeployments))

			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
			}
			if overviewCalls != 1 || resourceLogCalls != 1 {
				t.Fatalf("expected allowed read to hit both providers once, got overview=%d resourceLogs=%d", overviewCalls, resourceLogCalls)
			}

			var got resourcelogs.Result
			if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode resource logs payload: %v", err)
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("unexpected resource logs payload: got %+v want %+v", got, expected)
			}
			if got.Available {
				t.Fatalf("expected unavailable payload, got %+v", got)
			}
			if got.UnavailableReason != unavailableReason {
				t.Fatalf("expected unavailable reason %q, got %q", unavailableReason, got.UnavailableReason)
			}
		})
	})

	t.Run("returns resource log payload", func(t *testing.T) {
		expected := resourcelogs.Result{
			DeploymentName: "api-prod",
			ResourceType:   "ecsTask",
			ResourceName:   "arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123",
			ContainerName:  "api",
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
				{Timestamp: "2026-04-16T07:10:00Z", Message: "service became healthy", Stream: "ecs/payments-api/service/abc123", LogGroup: "/aws/ecs/payments-api", ContainerName: "api"},
				{Timestamp: "2026-04-16T07:09:00Z", Message: "registered 2/2 targets"},
			},
			ResultScope: &resourcelogs.ResultScope{
				EffectiveLimit:       resourcelogs.DefaultMaxEntries,
				Limit:                resourcelogs.DefaultMaxEntries,
				ReturnedEntryCount:   2,
				ReturnedCount:        2,
				Returned:             2,
				TotalEntryCount:      2,
				Total:                2,
				NewestEntryTimestamp: "2026-04-16T07:10:00Z",
				OldestEntryTimestamp: "2026-04-16T07:09:00Z",
				TruncationStatus:     resourcelogs.ScopeTruncationStatusNotTruncated,
			},
			UnavailableReason: "",
		}

		var gotRequest struct {
			deploymentName string
			resourceType   string
			resourceName   string
			requestID      string
			containerName  string
			limit          int
		}
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
				gotRequest.deploymentName = deploymentName
				gotRequest.resourceType = resourceType
				gotRequest.resourceName = resourceName
				gotRequest.requestID = RequestIDFromContext(ctx)
				gotRequest.containerName = resourcelogs.ContainerNameFromContext(ctx)
				gotRequest.limit = limit
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsTask&resourceName=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Atask%2Fpayments%2Fabc123&containerName=api", nil)
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
			containerName  string
			limit          int
		}{deploymentName: "api-prod", resourceType: "ecsTask", resourceName: "arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123", requestID: responseRequestID, containerName: "api"}) {
			t.Fatalf("unexpected provider request: %+v", gotRequest)
		}

		var got resourcelogs.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode resource logs payload: %v", err)
		}
		expectedWithIDs := expected
		expectedWithIDs.Entries = resourcelogs.AssignEntryIDs(resourcelogs.Request{
			DeploymentName:   "api-prod",
			ResourceType:     "ecsTask",
			ResourceName:     "arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123",
			ControlAccountID: expected.LookupContext.ControlAccountID,
			TargetAccountID:  expected.LookupContext.TargetAccountID,
			Region:           expected.LookupContext.Region,
			ContainerName:    "api",
		}, expected.Source, expected.Entries)
		if !reflect.DeepEqual(got, expectedWithIDs) {
			t.Fatalf("unexpected resource logs payload: got %+v want %+v", got, expectedWithIDs)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw resource logs payload: %v", err)
		}
		for _, key := range []string{"deploymentName", "resourceType", "resourceName", "containerName", "lookupContext", "available", "source", "entries", "resultScope", "unavailableReason"} {
			if _, ok := raw[key]; !ok {
				t.Fatalf("expected %q field in payload, got %#v", key, raw)
			}
		}
		if raw["containerName"] != "api" {
			t.Fatalf("expected containerName to echo ECS task selector, got %#v", raw["containerName"])
		}
		rawEntries, ok := raw["entries"].([]any)
		if !ok || len(rawEntries) != 2 {
			t.Fatalf("expected entries array in payload, got %#v", raw["entries"])
		}
		firstEntry, ok := rawEntries[0].(map[string]any)
		if !ok {
			t.Fatalf("expected first entry object, got %#v", rawEntries[0])
		}
		if firstEntry["containerName"] != "api" {
			t.Fatalf("expected entries[0].containerName to expose resolved ECS task container, got %#v", firstEntry)
		}
		if firstEntry["logGroup"] != "/aws/ecs/payments-api" {
			t.Fatalf("expected entries[0].logGroup to expose resolved CloudWatch log group, got %#v", firstEntry)
		}
		firstEntryID, ok := firstEntry["entryId"].(string)
		if !ok || !strings.HasPrefix(firstEntryID, "rlog_") {
			t.Fatalf("expected backend-authored entries[0].entryId, got %#v", firstEntry)
		}
		secondEntry, ok := rawEntries[1].(map[string]any)
		if !ok {
			t.Fatalf("expected second entry object, got %#v", rawEntries[1])
		}
		secondEntryID, ok := secondEntry["entryId"].(string)
		if !ok || !strings.HasPrefix(secondEntryID, "rlog_") || secondEntryID == firstEntryID {
			t.Fatalf("expected unique backend-authored entries[1].entryId, got first=%q second=%#v", firstEntryID, secondEntry)
		}
		if _, ok := secondEntry["containerName"]; ok {
			t.Fatalf("expected entries[1].containerName to be omitted when unknown, got %#v", secondEntry)
		}
		if _, ok := secondEntry["logGroup"]; ok {
			t.Fatalf("expected entries[1].logGroup to be omitted when unknown, got %#v", secondEntry)
		}
		if _, ok := raw["backingResourceType"]; ok {
			t.Fatalf("expected backingResourceType to be omitted for direct resource logs, got %#v", raw)
		}
		if _, ok := raw["backingResourceName"]; ok {
			t.Fatalf("expected backingResourceName to be omitted for direct resource logs, got %#v", raw)
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
				t.Fatalf("expected lookupContext[%q]=%#v, got %#v", key, want, got)
			}
		}
		if _, ok := raw["emptyReason"]; ok {
			t.Fatalf("expected emptyReason to be omitted for populated recent-log responses, got %#v", raw)
		}
		rawScope, ok := raw["resultScope"].(map[string]any)
		if !ok {
			t.Fatalf("expected resultScope object in resource logs payload, got %#v", raw["resultScope"])
		}
		for key, want := range map[string]any{
			"effectiveLimit":       float64(resourcelogs.DefaultMaxEntries),
			"limit":                float64(resourcelogs.DefaultMaxEntries),
			"returnedEntryCount":   float64(2),
			"returnedCount":        float64(2),
			"returned":             float64(2),
			"totalEntryCount":      float64(2),
			"total":                float64(2),
			"newestEntryTimestamp": "2026-04-16T07:10:00Z",
			"oldestEntryTimestamp": "2026-04-16T07:09:00Z",
			"truncationStatus":     resourcelogs.ScopeTruncationStatusNotTruncated,
			"truncated":            false,
		} {
			if got := rawScope[key]; !reflect.DeepEqual(got, want) {
				t.Fatalf("expected resultScope[%q]=%#v, got %#v in %#v", key, want, got, rawScope)
			}
		}
	})

	t.Run("preserves empty reason for successful empty payload", func(t *testing.T) {
		expected := resourcelogs.Result{
			DeploymentName: "api-prod",
			ResourceType:   "ecsService",
			ResourceName:   "api-prod-service",
			Available:      true,
			Source:         resourcelogs.SourceCloudWatch,
			Entries:        []resourcelogs.Entry{},
			ResultScope: &resourcelogs.ResultScope{
				EffectiveLimit:     resourcelogs.DefaultMaxEntries,
				Limit:              resourcelogs.DefaultMaxEntries,
				ReturnedEntryCount: 0,
				TruncationStatus:   resourcelogs.ScopeTruncationStatusNotTruncated,
			},
			EmptyReason: resourcelogs.EmptyReasonNoRecentEvents,
		}
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(_ context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
				if deploymentName != "api-prod" || resourceType != "ecsService" || resourceName != "api-prod-service" {
					t.Fatalf("unexpected provider request: deployment=%q type=%q name=%q", deploymentName, resourceType, resourceName)
				}
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw empty resource logs payload: %v", err)
		}
		if raw["emptyReason"] != resourcelogs.EmptyReasonNoRecentEvents {
			t.Fatalf("expected emptyReason %q, got %#v", resourcelogs.EmptyReasonNoRecentEvents, raw)
		}
		if raw["unavailableReason"] != "" {
			t.Fatalf("expected unavailableReason to remain empty on available payload, got %#v", raw["unavailableReason"])
		}
	})

	t.Run("passes optional instance selector into provider context and raw payload", func(t *testing.T) {
		expected := resourcelogs.Result{
			DeploymentName: "api-prod",
			ResourceType:   "candidateAutoScalingGroup",
			ResourceName:   "payments-api-prod-candidate",
			InstanceID:     "i-0abc123def4567890",
			Available:      true,
			Source:         resourcelogs.SourceEC2Console,
			Entries: []resourcelogs.Entry{
				{Timestamp: "2026-04-16T07:11:30Z", Message: "selected instance bootstrap output", InstanceID: "i-0abc123def4567890"},
			},
		}

		var gotInstanceID string
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
				gotInstanceID = resourcelogs.InstanceIDFromContext(ctx)
				if deploymentName != "api-prod" || resourceType != "candidateAutoScalingGroup" || resourceName != "payments-api-prod-candidate" {
					t.Fatalf("unexpected provider request: deployment=%q type=%q name=%q", deploymentName, resourceType, resourceName)
				}
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&instanceId=+i-0abc123def4567890+", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if gotInstanceID != "i-0abc123def4567890" {
			t.Fatalf("expected trimmed instanceId in provider context, got %q", gotInstanceID)
		}
		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw selected instance resource logs payload: %v", err)
		}
		if raw["instanceId"] != "i-0abc123def4567890" {
			t.Fatalf("expected instanceId to echo selected EC2 instance, got %#v", raw)
		}
		entries, ok := raw["entries"].([]any)
		if !ok || len(entries) != 1 {
			t.Fatalf("expected one selected-instance entry, got %#v", raw["entries"])
		}
		entry, ok := entries[0].(map[string]any)
		if !ok {
			t.Fatalf("expected selected-instance entry object, got %#v", entries[0])
		}
		if entry["instanceId"] != "i-0abc123def4567890" {
			t.Fatalf("expected entries[].instanceId to identify EC2 console source, got %#v", entry)
		}
		if entry["message"] != "selected instance bootstrap output" {
			t.Fatalf("expected selected-instance entry message to survive, got %#v", entry)
		}
	})

	t.Run("rejects malformed instance selector before provider lookup", func(t *testing.T) {
		var overviewCalls int
		var providerCalls int
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				overviewCalls++
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(_ context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
				providerCalls++
				return resourcelogs.Result{}, nil
			},
		})

		for _, rawInstanceID := range []string{"not-an-instance", "i-", "i-abc/def"} {
			t.Run(rawInstanceID, func(t *testing.T) {
				resp := httptest.NewRecorder()
				router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&instanceId="+url.QueryEscape(rawInstanceID), nil))
				if resp.Code != http.StatusBadRequest {
					t.Fatalf("expected 400 for instanceId=%s, got %d with body %q", rawInstanceID, resp.Code, resp.Body.String())
				}
				if body := resp.Body.String(); body != "invalid instanceId query\n" {
					t.Fatalf("unexpected invalid instanceId response: %q", body)
				}
			})
		}
		if overviewCalls != 0 || providerCalls != 0 {
			t.Fatalf("expected invalid instanceId to short-circuit providers, got overview=%d resourceLogs=%d", overviewCalls, providerCalls)
		}
	})

	t.Run("passes optional entry limit into provider and keeps router fallback truncation", func(t *testing.T) {
		provided := resourcelogs.Result{
			DeploymentName: "api-prod",
			ResourceType:   "ecsService",
			ResourceName:   "payments-api-prod-service",
			ClusterARN:     "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments",
			Available:      true,
			Source:         resourcelogs.SourceCloudWatch,
			Entries: []resourcelogs.Entry{
				{Timestamp: "2026-04-16T07:10:00Z", Message: "newest", Stream: "ecs/payments-api/service/newest"},
				{Timestamp: "2026-04-16T07:09:00Z", Message: "middle", Stream: "ecs/payments-api/service/middle"},
				{Timestamp: "2026-04-16T07:08:00Z", Message: "oldest", Stream: "ecs/payments-api/service/oldest"},
			},
		}

		var providerCalls int
		var gotLimit int
		var gotClusterARN string
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
				providerCalls++
				gotLimit = limit
				gotClusterARN = resourcelogs.ClusterARNFromContext(ctx)
				if deploymentName != "api-prod" || resourceType != "ecsService" || resourceName != "payments-api-prod-service" {
					t.Fatalf("unexpected provider request: deployment=%q type=%q name=%q", deploymentName, resourceType, resourceName)
				}
				return provided, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service&limit=2&clusterArn=+arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments+", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if providerCalls != 1 {
			t.Fatalf("expected provider to be called once, got %d", providerCalls)
		}
		if gotLimit != 2 {
			t.Fatalf("expected provider limit 2, got %d", gotLimit)
		}
		if gotClusterARN != "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments" {
			t.Fatalf("expected provider context cluster ARN, got %q", gotClusterARN)
		}

		var got resourcelogs.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode limited resource logs payload: %v", err)
		}
		if len(got.Entries) != 2 {
			t.Fatalf("expected two entries after limit, got %+v", got.Entries)
		}
		if got.ClusterARN != "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments" {
			t.Fatalf("expected decoded payload to echo selected clusterArn, got %+v", got)
		}
		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw limited resource logs payload: %v", err)
		}
		if raw["clusterArn"] != "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments" {
			t.Fatalf("expected raw payload to echo selected clusterArn, got %#v", raw)
		}
		if got.ResultScope == nil {
			t.Fatalf("expected resultScope in limited resource logs payload")
		}
		if got.ResultScope.RequestedLimit == nil || *got.ResultScope.RequestedLimit != 2 {
			t.Fatalf("expected requestedLimit 2, got %+v", got.ResultScope)
		}
		if got.ResultScope.EffectiveLimit != 2 || got.ResultScope.Limit != 2 || got.ResultScope.ReturnedEntryCount != 2 || got.ResultScope.Returned != 2 || got.ResultScope.TotalEntryCount != 3 || got.ResultScope.Total != 3 || got.ResultScope.NewestEntryTimestamp != "2026-04-16T07:10:00Z" || got.ResultScope.OldestEntryTimestamp != "2026-04-16T07:09:00Z" || got.ResultScope.TruncationStatus != resourcelogs.ScopeTruncationStatusTruncated {
			t.Fatalf("unexpected limited resultScope: %+v", got.ResultScope)
		}
		if got.ResultScope.ReturnedCount != 2 || !got.ResultScope.Truncated {
			t.Fatalf("expected additive resultScope returnedCount/truncated fields, got %+v", got.ResultScope)
		}
		if got.Entries[0].Message != "newest" || got.Entries[1].Message != "middle" {
			t.Fatalf("expected limit to preserve provider entry order, got %+v", got.Entries)
		}
		if len(provided.Entries) != 3 {
			t.Fatalf("expected provider result slice to remain unchanged, got %+v", provided.Entries)
		}
	})

	t.Run("marks exact-cap resource log windows as unknown total", func(t *testing.T) {
		provided := resourcelogs.Result{
			DeploymentName: "api-prod",
			ResourceType:   "ecsService",
			ResourceName:   "payments-api-prod-service",
			Available:      true,
			Source:         resourcelogs.SourceCloudWatch,
			Entries: []resourcelogs.Entry{
				{Timestamp: "2026-04-16T07:10:00Z", Message: "newest", Stream: "ecs/payments-api/service/newest"},
				{Timestamp: "2026-04-16T07:09:00Z", Message: "middle", Stream: "ecs/payments-api/service/middle"},
			},
		}

		var gotLimit int
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(_ context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
				gotLimit = limit
				if deploymentName != "api-prod" || resourceType != "ecsService" || resourceName != "payments-api-prod-service" {
					t.Fatalf("unexpected provider request: deployment=%q type=%q name=%q", deploymentName, resourceType, resourceName)
				}
				return provided, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service&limit=2", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if gotLimit != 2 {
			t.Fatalf("expected provider limit 2, got %d", gotLimit)
		}
		var got resourcelogs.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode exact-cap resource logs payload: %v", err)
		}
		if got.ResultScope == nil {
			t.Fatalf("expected resultScope in exact-cap resource logs payload")
		}
		if got.ResultScope.RequestedLimit == nil || *got.ResultScope.RequestedLimit != 2 {
			t.Fatalf("expected requestedLimit 2, got %+v", got.ResultScope)
		}
		if got.ResultScope.EffectiveLimit != 2 || got.ResultScope.Limit != 2 || got.ResultScope.ReturnedEntryCount != 2 || got.ResultScope.ReturnedCount != 2 || got.ResultScope.Returned != 2 || got.ResultScope.TotalEntryCount != 2 || got.ResultScope.Total != 2 || got.ResultScope.TruncationStatus != resourcelogs.ScopeTruncationStatusUnknown || got.ResultScope.Truncated {
			t.Fatalf("expected exact-cap scope to report unknown total without truncation, got %+v", got.ResultScope)
		}
		if got.ResultScope.NewestEntryTimestamp != "2026-04-16T07:10:00Z" || got.ResultScope.OldestEntryTimestamp != "2026-04-16T07:09:00Z" {
			t.Fatalf("expected exact-cap timestamp window from returned entries, got %+v", got.ResultScope)
		}
	})

	t.Run("passes optional ECS task cluster selector into resource log provider", func(t *testing.T) {
		var gotClusterARN string
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
				gotClusterARN = resourcelogs.ClusterARNFromContext(ctx)
				if deploymentName != "api-prod" || resourceType != "ecsTask" || resourceName != "arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123" {
					t.Fatalf("unexpected ECS task log request: deployment=%q type=%q name=%q", deploymentName, resourceType, resourceName)
				}
				return resourcelogs.Result{DeploymentName: deploymentName, ResourceType: resourceType, ResourceName: resourceName, ClusterARN: gotClusterARN, Available: true, Source: resourcelogs.SourceCloudWatch, Entries: []resourcelogs.Entry{}}, nil
			},
		})

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsTask&resourceName="+url.QueryEscape("arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123")+"&clusterArn="+url.QueryEscape(" arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments "), nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if gotClusterARN != "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments" {
			t.Fatalf("expected provider context cluster ARN, got %q", gotClusterARN)
		}
		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw ECS task logs payload: %v", err)
		}
		if raw["clusterArn"] != "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments" {
			t.Fatalf("expected raw ECS task payload to echo selected clusterArn, got %#v", raw)
		}
	})

	t.Run("rejects invalid limit before provider lookup", func(t *testing.T) {
		var overviewCalls int
		var providerCalls int
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				overviewCalls++
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(_ context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
				providerCalls++
				return resourcelogs.Result{}, nil
			},
		})

		for _, rawLimit := range []string{"0", "-1", "many"} {
			t.Run(rawLimit, func(t *testing.T) {
				resp := httptest.NewRecorder()
				router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service&limit="+rawLimit, nil))
				if resp.Code != http.StatusBadRequest {
					t.Fatalf("expected 400 for limit=%s, got %d with body %q", rawLimit, resp.Code, resp.Body.String())
				}
				if body := resp.Body.String(); body != "invalid limit query\n" {
					t.Fatalf("unexpected invalid limit response: %q", body)
				}
			})
		}
		if overviewCalls != 0 || providerCalls != 0 {
			t.Fatalf("expected invalid limit to short-circuit providers, got overview=%d resourceLogs=%d", overviewCalls, providerCalls)
		}
	})

	t.Run("rejects invalid ECS cluster selector before resource log provider lookup", func(t *testing.T) {
		var overviewCalls int
		var providerCalls int
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) {
				overviewCalls++
				return catalog.DeploymentOverviewDetail{}, nil
			},
			OverviewDeploymentResourceLogs: func(context.Context, string, string, string, int) (resourcelogs.Result, error) {
				providerCalls++
				return resourcelogs.Result{}, nil
			},
		})

		for _, resourceType := range []string{"ecsService", "ecsTask"} {
			for _, rawClusterARN := range []string{"not-an-arn", "arn:aws:sqs:ap-northeast-2:123456789012:queue/name", "arn:aws:ecs:ap-northeast-2:12345678901x:cluster/payments", "arn:aws:ecs:ap-northeast-2:123456789012:service/cluster/name"} {
				t.Run(resourceType+"/"+rawClusterARN, func(t *testing.T) {
					resourceName := "api-prod-service"
					if resourceType == "ecsTask" {
						resourceName = "arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123"
					}
					resp := httptest.NewRecorder()
					router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType="+resourceType+"&resourceName="+url.QueryEscape(resourceName)+"&clusterArn="+url.QueryEscape(rawClusterARN), nil))
					if resp.Code != http.StatusBadRequest {
						t.Fatalf("expected 400, got %d with body %q", resp.Code, resp.Body.String())
					}
					if body := resp.Body.String(); body != "invalid clusterArn query\n" {
						t.Fatalf("unexpected invalid clusterArn response: %q", body)
					}
				})
			}
		}
		if overviewCalls != 0 || providerCalls != 0 {
			t.Fatalf("expected invalid clusterArn to short-circuit providers, got overview=%d resourceLogs=%d", overviewCalls, providerCalls)
		}
	})

	t.Run("returns codeDeploy deployment-group resource log payload", func(t *testing.T) {
		expected := resourcelogs.Result{
			DeploymentName:      "payments-api-staging",
			ResourceType:        "codeDeployDeploymentGroup",
			ResourceName:        "payments-api-staging-codedeploy-group",
			ResourceDescription: "CodeDeploy deployment group coordinating ECS task-set traffic shifting",
			LookupContext: resourcelogs.LookupContext{
				Region:                 "ap-northeast-2",
				ControlAccountID:       "123456789012",
				TargetAccountID:        "123456789012",
				CrossAccount:           false,
				ObserverRoleConfigured: true,
			},
			Available: true,
			Source:    resourcelogs.SourceCodeDeploy,
			Entries: []resourcelogs.Entry{{
				Timestamp: "2026-04-18T09:18:00Z",
				Message:   "Status: Succeeded — deployment completed for the latest linear rollout",
				Stream:    "d-ABC123",
			}},
		}

		var gotRequest struct {
			deploymentName string
			resourceType   string
			resourceName   string
		}
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "payments-api-staging" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
				_ = RequestIDFromContext(ctx)
				gotRequest.deploymentName = deploymentName
				gotRequest.resourceType = resourceType
				gotRequest.resourceName = resourceName
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if !reflect.DeepEqual(gotRequest, struct {
			deploymentName string
			resourceType   string
			resourceName   string
		}{deploymentName: "payments-api-staging", resourceType: "codeDeployDeploymentGroup", resourceName: "payments-api-staging-codedeploy-group"}) {
			t.Fatalf("unexpected provider request: %+v", gotRequest)
		}
		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw CodeDeploy resource logs payload: %v", err)
		}
		if got := raw["source"]; got != resourcelogs.SourceCodeDeploy {
			t.Fatalf("expected source %q, got %#v", resourcelogs.SourceCodeDeploy, got)
		}
		if got := raw["resourceDescription"]; got != "CodeDeploy deployment group coordinating ECS task-set traffic shifting" {
			t.Fatalf("expected resourceDescription to be serialized, got %#v", got)
		}
		entries, ok := raw["entries"].([]any)
		if !ok || len(entries) != 1 {
			t.Fatalf("expected one CodeDeploy resource log entry, got %#v", raw["entries"])
		}
		entry, ok := entries[0].(map[string]any)
		if !ok {
			t.Fatalf("expected CodeDeploy entry object, got %#v", entries[0])
		}
		if got := entry["stream"]; got != "d-ABC123" {
			t.Fatalf("expected deployment id stream, got %#v", got)
		}
	})

	t.Run("returns bootstrap-source log payloads without changing the route contract", func(t *testing.T) {
		expected := resourcelogs.Result{
			DeploymentName:      "api-prod",
			ResourceType:        "userDataBootstrapSource",
			ResourceName:        "file:userdata/api-prod.sh",
			BackingResourceType: "candidateAutoScalingGroup",
			BackingResourceName: "api-prod-candidate",
			LookupContext: resourcelogs.LookupContext{
				Region:                 "ap-northeast-2",
				ControlAccountID:       "123456789012",
				TargetAccountID:        "123456789012",
				CrossAccount:           false,
				ObserverRoleConfigured: true,
			},
			Available: true,
			Source:    resourcelogs.SourceEC2Console,
			Entries: []resourcelogs.Entry{{
				Timestamp: "2026-04-16T07:11:30Z",
				Message:   "bootstrap start",
			}, {
				Timestamp: "2026-04-16T07:11:45Z",
				Message:   "bootstrap complete",
			}},
			ResultScope: &resourcelogs.ResultScope{
				EffectiveLimit:       resourcelogs.DefaultMaxEntries,
				Limit:                resourcelogs.DefaultMaxEntries,
				ReturnedEntryCount:   2,
				ReturnedCount:        2,
				Returned:             2,
				TotalEntryCount:      2,
				Total:                2,
				NewestEntryTimestamp: "2026-04-16T07:11:45Z",
				OldestEntryTimestamp: "2026-04-16T07:11:30Z",
				TruncationStatus:     resourcelogs.ScopeTruncationStatusNotTruncated,
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
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
				gotRequest.deploymentName = deploymentName
				gotRequest.resourceType = resourceType
				gotRequest.resourceName = resourceName
				gotRequest.requestID = RequestIDFromContext(ctx)
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=file:userdata/api-prod.sh", nil)
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
		}{deploymentName: "api-prod", resourceType: "userDataBootstrapSource", resourceName: "file:userdata/api-prod.sh", requestID: responseRequestID}) {
			t.Fatalf("unexpected provider request: %+v", gotRequest)
		}

		var got resourcelogs.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode bootstrap-source resource logs payload: %v", err)
		}
		expectedWithIDs := expected
		expectedWithIDs.Entries = resourcelogs.AssignEntryIDs(resourcelogs.Request{
			DeploymentName:      "api-prod",
			ResourceType:        "userDataBootstrapSource",
			ResourceName:        "file:userdata/api-prod.sh",
			ResourceDescription: expected.ResourceDescription,
			BackingResourceType: expected.BackingResourceType,
			BackingResourceName: expected.BackingResourceName,
			ControlAccountID:    expected.LookupContext.ControlAccountID,
			TargetAccountID:     expected.LookupContext.TargetAccountID,
			Region:              expected.LookupContext.Region,
		}, expected.Source, expected.Entries)
		if !reflect.DeepEqual(got, expectedWithIDs) {
			t.Fatalf("unexpected bootstrap-source resource logs payload: got %+v want %+v", got, expectedWithIDs)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw bootstrap-source resource logs payload: %v", err)
		}
		if raw["resourceType"] != "userDataBootstrapSource" || raw["source"] != string(resourcelogs.SourceEC2Console) {
			t.Fatalf("expected bootstrap-source route to preserve resource type/source, got %#v", raw)
		}
		if raw["backingResourceType"] != "candidateAutoScalingGroup" || raw["backingResourceName"] != "api-prod-candidate" {
			t.Fatalf("expected bootstrap-source route to surface backing resource identity, got %#v", raw)
		}
		entries, ok := raw["entries"].([]any)
		if !ok || len(entries) != 2 {
			t.Fatalf("expected bootstrap-source entries array, got %#v", raw["entries"])
		}
		first, ok := entries[0].(map[string]any)
		if !ok || first["timestamp"] != "2026-04-16T07:11:30Z" || first["message"] != "bootstrap start" {
			t.Fatalf("expected first bootstrap-source entry shape, got %#v", entries[0])
		}
		firstEntryID, ok := first["entryId"].(string)
		if !ok || !strings.HasPrefix(firstEntryID, "rlog_") {
			t.Fatalf("expected first bootstrap-source entry ID, got %#v", first)
		}
	})

	t.Run("honors inbound request-id header", func(t *testing.T) {
		const inboundRequestID = "operator-correlation-123"

		var providerRequestID string
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
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
				OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
					t.Fatalf("overview deployment lookup should not run for invalid query")
					return catalog.DeploymentOverviewDetail{}, nil
				},
				OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
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
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
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
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "missing" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{}, statusstore.ErrNotFound
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
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
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
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
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
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
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{}, errors.New(providerErr)
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
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
	t.Run("enforces deployment read authorization and preserves unavailable payload when authorized", func(t *testing.T) {
		const (
			path                 = "/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn:aws:ecs:ap-northeast-2:123456789012:cluster/worker"
			deploymentName       = "worker-prod"
			resourceType         = "ecsService"
			resourceName         = "worker-prod-service"
			unavailableReason    = "resource inspection is unavailable because live AWS access is not configured for this local runtime"
			authenticatedID      = "front-door-123"
			authenticatedName    = "Front Door Operator"
			authenticatedEmail   = "frontdoor@example.com"
			forbiddenDeployments = "payments-api-staging"
			allowedDeployments   = "worker-prod, payments-api-staging"
		)

		expected := resourceinspection.Result{
			DeploymentName:      deploymentName,
			ResourceType:        resourceType,
			ResourceName:        resourceName,
			ResourceDescription: "ECS service Tenet updates with native rolling deployment",
			ClusterARN:          "arn:aws:ecs:ap-northeast-2:123456789012:cluster/worker",
			LookupContext: resourceinspection.LookupContext{
				Region:                 "ap-northeast-2",
				ControlAccountID:       "123456789012",
				TargetAccountID:        "123456789012",
				CrossAccount:           false,
				ObserverRoleConfigured: false,
			},
			Available:         false,
			UnavailableReason: unavailableReason,
		}

		var overviewCalls int
		var inspectionCalls int
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				overviewCalls++
				if name != deploymentName {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(_ context.Context, gotDeploymentName, gotResourceType, gotResourceName string, _ int) (resourceinspection.Result, error) {
				inspectionCalls++
				if gotDeploymentName != deploymentName || gotResourceType != resourceType || gotResourceName != resourceName {
					t.Fatalf("unexpected resource inspection request: deployment=%q type=%q name=%q", gotDeploymentName, gotResourceType, gotResourceName)
				}
				return expected, nil
			},
		})

		newAuthenticatedRequest := func(authorizedDeployments string) *http.Request {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set(requestactor.AuthenticatedHeaderID, authenticatedID)
			req.Header.Set(requestactor.AuthenticatedHeaderName, authenticatedName)
			req.Header.Set(requestactor.AuthenticatedHeaderEmail, authenticatedEmail)
			if authorizedDeployments != "" {
				req.Header.Set(requestactor.AuthorizedDeploymentsHeader, authorizedDeployments)
			}
			return req
		}

		t.Run("unauthenticated", func(t *testing.T) {
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))

			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", resp.Code)
			}
			if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator read route\n" {
				t.Fatalf("unexpected 401 response body: %q", body)
			}
			if overviewCalls != 0 || inspectionCalls != 0 {
				t.Fatalf("expected auth guard to short-circuit before providers, got overview=%d inspection=%d", overviewCalls, inspectionCalls)
			}
		})

		t.Run("forbidden", func(t *testing.T) {
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, newAuthenticatedRequest(forbiddenDeployments))

			if resp.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d", resp.Code)
			}
			if body := resp.Body.String(); body != "forbidden: operator does not have access to this deployment\n" {
				t.Fatalf("unexpected 403 response body: %q", body)
			}
			if overviewCalls != 0 || inspectionCalls != 0 {
				t.Fatalf("expected auth guard to short-circuit before providers, got overview=%d inspection=%d", overviewCalls, inspectionCalls)
			}
		})

		t.Run("allowed keeps unavailable payload", func(t *testing.T) {
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, newAuthenticatedRequest(allowedDeployments))

			if resp.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
			}
			if overviewCalls != 1 || inspectionCalls != 1 {
				t.Fatalf("expected allowed read to hit both providers once, got overview=%d inspection=%d", overviewCalls, inspectionCalls)
			}

			var got resourceinspection.Result
			if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode resource inspection payload: %v", err)
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("unexpected resource inspection payload: got %+v want %+v", got, expected)
			}
			if got.Available {
				t.Fatalf("expected unavailable payload, got %+v", got)
			}
			if got.UnavailableReason != unavailableReason {
				t.Fatalf("expected unavailable reason %q, got %q", unavailableReason, got.UnavailableReason)
			}
			var raw map[string]any
			if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
				t.Fatalf("decode raw resource inspection payload: %v", err)
			}
			if raw["clusterArn"] != "arn:aws:ecs:ap-northeast-2:123456789012:cluster/worker" {
				t.Fatalf("expected unavailable raw payload to echo selected clusterArn, got %#v", raw)
			}
			if raw["resourceDescription"] != "ECS service Tenet updates with native rolling deployment" {
				t.Fatalf("expected unavailable raw payload to echo selected resourceDescription, got %#v", raw)
			}
		})
	})

	t.Run("rejects invalid bounded inspection limit before provider lookup", func(t *testing.T) {
		for _, resourceType := range []string{"ecsService", "candidateAutoScalingGroup", "serviceAutoScalingGroup"} {
			t.Run(resourceType, func(t *testing.T) {
				var overviewCalls int
				var inspectionCalls int
				router := NewRouter(Options{
					BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
					OverviewDeployment: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) {
						overviewCalls++
						return catalog.DeploymentOverviewDetail{Name: "api-prod"}, nil
					},
					OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
						inspectionCalls++
						return resourceinspection.Result{}, nil
					},
				})

				resp := httptest.NewRecorder()
				router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType="+resourceType+"&resourceName=api-prod-resource&limit=0", nil))
				if resp.Code != http.StatusBadRequest {
					t.Fatalf("expected 400 for invalid %s inspection limit, got %d with body %q", resourceType, resp.Code, resp.Body.String())
				}
				if body := resp.Body.String(); body != "invalid limit query\n" {
					t.Fatalf("unexpected invalid limit response: %q", body)
				}
				if overviewCalls != 0 || inspectionCalls != 0 {
					t.Fatalf("expected invalid limit to short-circuit providers, got overview=%d inspection=%d", overviewCalls, inspectionCalls)
				}
			})
		}
	})

	t.Run("passes explicit ASG inspection limit to provider", func(t *testing.T) {
		var gotLimit int
		requestedLimit := 4
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{Name: "api-prod"}, nil
			},
			OverviewDeploymentResourceInspection: func(_ context.Context, name, resourceType, resourceName string, limit int) (resourceinspection.Result, error) {
				gotLimit = limit
				return resourceinspection.Result{
					DeploymentName: name,
					ResourceType:   resourceType,
					ResourceName:   resourceName,
					Available:      true,
					Source:         resourceinspection.SourceAutoScalingGroup,
					AutoScalingGroup: &resourceinspection.AutoScalingInspection{
						AutoScalingGroupName: resourceName,
						InstancesScope: &resourceinspection.AutoScalingInstanceScope{
							Available:             true,
							RequestedLimit:        &requestedLimit,
							EffectiveLimit:        4,
							ReturnedInstanceCount: 2,
							TruncationStatus:      "not_truncated",
							Summary:               "Returned 2 Auto Scaling instance(s) within the effective limit 4.",
						},
					},
					ResultScope: &resourceinspection.ResultScope{
						SampleType:        "autoScalingInstances",
						RequestedLimit:    &requestedLimit,
						EffectiveLimit:    4,
						ReturnedItemCount: 2,
						TruncationStatus:  "not_truncated",
					},
				}, nil
			},
		})

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=api-prod-candidate&limit=4", nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected ASG resource inspection to accept positive limit, got %d with body %q", resp.Code, resp.Body.String())
		}
		if gotLimit != 4 {
			t.Fatalf("expected ASG resource inspection limit 4, got %d", gotLimit)
		}
		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("decode ASG resource inspection payload: %v", err)
		}
		autoScalingGroup, ok := raw["autoScalingGroup"].(map[string]any)
		if !ok {
			t.Fatalf("expected autoScalingGroup object in raw payload, got %#v", raw["autoScalingGroup"])
		}
		instancesScope, ok := autoScalingGroup["instancesScope"].(map[string]any)
		if !ok {
			t.Fatalf("expected instancesScope object in raw payload, got %#v", autoScalingGroup["instancesScope"])
		}
		for key, want := range map[string]any{
			"available":             true,
			"effectiveLimit":        float64(4),
			"requestedLimit":        float64(4),
			"returnedInstanceCount": float64(2),
			"truncationStatus":      "not_truncated",
			"summary":               "Returned 2 Auto Scaling instance(s) within the effective limit 4.",
		} {
			if !reflect.DeepEqual(instancesScope[key], want) {
				t.Fatalf("expected instancesScope[%q]=%#v, got %#v in %#v", key, want, instancesScope[key], instancesScope)
			}
		}
		resultScope, ok := raw["resultScope"].(map[string]any)
		if !ok {
			t.Fatalf("expected top-level resultScope object in raw payload, got %#v", raw["resultScope"])
		}
		for key, want := range map[string]any{
			"sampleType":        "autoScalingInstances",
			"effectiveLimit":    float64(4),
			"requestedLimit":    float64(4),
			"returnedItemCount": float64(2),
			"truncationStatus":  "not_truncated",
		} {
			if !reflect.DeepEqual(resultScope[key], want) {
				t.Fatalf("expected resultScope[%q]=%#v, got %#v in %#v", key, want, resultScope[key], resultScope)
			}
		}
	})

	t.Run("encodes ASG scaling activities in raw JSON payload", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{Name: "api-prod"}, nil
			},
			OverviewDeploymentResourceInspection: func(_ context.Context, name, resourceType, resourceName string, _ int) (resourceinspection.Result, error) {
				progress := int32(60)
				scalingActivities := []resourceinspection.AutoScalingActivitySummary{{
					ActivityID:    "activity-123",
					StatusCode:    "InProgress",
					Progress:      &progress,
					StartTime:     "2026-05-16T06:20:00Z",
					Cause:         "desired capacity changed",
					Description:   "Launching EC2 instance: i-0abc123def4567890.",
					StatusMessage: "Waiting for instance warmup.",
				}}
				return resourceinspection.Result{
					DeploymentName: name,
					ResourceType:   resourceType,
					ResourceName:   resourceName,
					Available:      true,
					Source:         resourceinspection.SourceAutoScalingGroup,
					AutoScalingGroup: &resourceinspection.AutoScalingInspection{
						AutoScalingGroupName: resourceName,
						Instances:            []resourceinspection.AutoScalingGroupInstanceSummary{},
						ScalingActivities:    &scalingActivities,
					},
				}, nil
			},
		})

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=api-prod-candidate", nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected ASG resource inspection to return 200, got %d with body %q", resp.Code, resp.Body.String())
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("decode ASG resource inspection payload: %v", err)
		}
		autoScalingGroup, ok := raw["autoScalingGroup"].(map[string]any)
		if !ok {
			t.Fatalf("expected autoScalingGroup object in raw payload, got %#v", raw["autoScalingGroup"])
		}
		activities, ok := autoScalingGroup["scalingActivities"].([]any)
		if !ok || len(activities) != 1 {
			t.Fatalf("expected one scaling activity in raw payload, got %#v", autoScalingGroup["scalingActivities"])
		}
		activity, ok := activities[0].(map[string]any)
		if !ok {
			t.Fatalf("expected scaling activity object, got %#v", activities[0])
		}
		for key, want := range map[string]any{
			"activityId":    "activity-123",
			"statusCode":    "InProgress",
			"progress":      float64(60),
			"startTime":     "2026-05-16T06:20:00Z",
			"cause":         "desired capacity changed",
			"description":   "Launching EC2 instance: i-0abc123def4567890.",
			"statusMessage": "Waiting for instance warmup.",
		} {
			if !reflect.DeepEqual(activity[key], want) {
				t.Fatalf("expected scaling activity %q=%#v, got %#v in %#v", key, want, activity[key], activity)
			}
		}
	})

	t.Run("passes selected ASG inspection instanceId to provider context", func(t *testing.T) {
		var gotInstanceID string
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{Name: "api-prod"}, nil
			},
			OverviewDeploymentResourceInspection: func(ctx context.Context, name, resourceType, resourceName string, limit int) (resourceinspection.Result, error) {
				gotInstanceID = resourceinspection.InstanceIDFromContext(ctx)
				return resourceinspection.Result{
					DeploymentName:    name,
					ResourceType:      resourceType,
					ResourceName:      resourceName,
					InstanceID:        gotInstanceID,
					Available:         false,
					UnavailableReason: resourceaws.UnavailableReasonResourceNotFound,
				}, nil
			},
		})

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=api-prod-candidate&instanceId=+i-0abc123def4567890+", nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected selected ASG resource inspection to pass, got %d with body %q", resp.Code, resp.Body.String())
		}
		if gotInstanceID != "i-0abc123def4567890" {
			t.Fatalf("expected trimmed instanceId in provider context, got %q", gotInstanceID)
		}
		var got map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode selected ASG inspection payload: %v", err)
		}
		if got["instanceId"] != "i-0abc123def4567890" || got["unavailableReason"] != resourceaws.UnavailableReasonResourceNotFound {
			t.Fatalf("expected selected instanceId and unavailable reason in raw payload, got %v", got)
		}
	})

	t.Run("returns selected ASG inspection EC2 detail raw fields", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{Name: "api-prod"}, nil
			},
			OverviewDeploymentResourceInspection: func(ctx context.Context, name, resourceType, resourceName string, limit int) (resourceinspection.Result, error) {
				instanceID := resourceinspection.InstanceIDFromContext(ctx)
				return resourceinspection.Result{
					DeploymentName: name,
					ResourceType:   resourceType,
					ResourceName:   resourceName,
					InstanceID:     instanceID,
					Available:      true,
					Source:         resourceinspection.SourceAutoScalingGroup,
					AutoScalingGroup: &resourceinspection.AutoScalingInspection{
						AutoScalingGroupName: resourceName,
						Instances: []resourceinspection.AutoScalingGroupInstanceSummary{{
							InstanceID:           instanceID,
							AvailabilityZone:     "ap-northeast-2a",
							LifecycleState:       "InService",
							HealthStatus:         "Healthy",
							ProtectedFromScaleIn: true,
							EC2Instance: &resourceinspection.EC2InstanceDetail{
								InstanceState:    "running",
								InstanceType:     "m6i.large",
								ImageID:          "ami-abc123",
								PrivateIPAddress: "10.0.1.24",
								PublicIPAddress:  "203.0.113.24",
								SubnetID:         "subnet-abc123",
								VPCID:            "vpc-abc123",
								LaunchTime:       "2026-05-05T07:30:00Z",
							},
							EC2InstanceStatus: &resourceinspection.EC2InstanceStatusDetail{
								InstanceState:  "running",
								InstanceStatus: "ok",
								SystemStatus:   "impaired",
								Events: []resourceinspection.EC2InstanceStatusEvent{{
									Code:            "system-reboot",
									Description:     "scheduled maintenance",
									InstanceEventID: "event-123",
									NotBefore:       "2026-05-06T01:00:00Z",
								}},
							},
						}},
					},
				}, nil
			},
		})

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=api-prod-candidate&instanceId=i-0abc123def4567890", nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected selected ASG resource inspection EC2 detail to pass, got %d with body %q", resp.Code, resp.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode selected ASG EC2 detail payload: %v", err)
		}
		autoScalingGroup, ok := got["autoScalingGroup"].(map[string]any)
		if !ok {
			t.Fatalf("expected raw autoScalingGroup object, got %#v", got)
		}
		instances, ok := autoScalingGroup["instances"].([]any)
		if !ok || len(instances) != 1 {
			t.Fatalf("expected one selected ASG instance in raw payload, got %#v", autoScalingGroup["instances"])
		}
		instance, ok := instances[0].(map[string]any)
		if !ok {
			t.Fatalf("expected selected ASG instance object, got %#v", instances[0])
		}
		ec2Instance, ok := instance["ec2Instance"].(map[string]any)
		if !ok {
			t.Fatalf("expected selected ASG instance ec2Instance object, got %#v", instance)
		}
		for key, want := range map[string]string{
			"instanceState":    "running",
			"instanceType":     "m6i.large",
			"imageId":          "ami-abc123",
			"privateIpAddress": "10.0.1.24",
			"publicIpAddress":  "203.0.113.24",
			"subnetId":         "subnet-abc123",
			"vpcId":            "vpc-abc123",
			"launchTime":       "2026-05-05T07:30:00Z",
		} {
			if ec2Instance[key] != want {
				t.Fatalf("expected ec2Instance.%s=%q, got %#v", key, want, ec2Instance)
			}
		}
		ec2Status, ok := instance["ec2InstanceStatus"].(map[string]any)
		if !ok {
			t.Fatalf("expected selected ASG instance ec2InstanceStatus object, got %#v", instance)
		}
		for key, want := range map[string]string{
			"instanceState":  "running",
			"instanceStatus": "ok",
			"systemStatus":   "impaired",
		} {
			if ec2Status[key] != want {
				t.Fatalf("expected ec2InstanceStatus.%s=%q, got %#v", key, want, ec2Status)
			}
		}
		events, ok := ec2Status["events"].([]any)
		if !ok || len(events) != 1 {
			t.Fatalf("expected one EC2 status event in raw payload, got %#v", ec2Status["events"])
		}
		event, ok := events[0].(map[string]any)
		if !ok || event["code"] != "system-reboot" || event["description"] != "scheduled maintenance" || event["instanceEventId"] != "event-123" || event["notBefore"] != "2026-05-06T01:00:00Z" {
			t.Fatalf("unexpected EC2 status event payload: %#v", events[0])
		}
	})

	t.Run("rejects invalid ASG inspection instanceId before provider lookup", func(t *testing.T) {
		for _, rawInstanceID := range []string{"not-an-instance", "i-", "i-abc/def"} {
			t.Run(rawInstanceID, func(t *testing.T) {
				var overviewCalls int
				var inspectionCalls int
				router := NewRouter(Options{
					BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
					OverviewDeployment: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) {
						overviewCalls++
						return catalog.DeploymentOverviewDetail{Name: "api-prod"}, nil
					},
					OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
						inspectionCalls++
						return resourceinspection.Result{}, nil
					},
				})

				resp := httptest.NewRecorder()
				router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=api-prod-candidate&instanceId="+url.QueryEscape(rawInstanceID), nil))
				if resp.Code != http.StatusBadRequest {
					t.Fatalf("expected 400 for instanceId=%s, got %d with body %q", rawInstanceID, resp.Code, resp.Body.String())
				}
				if body := resp.Body.String(); body != "invalid instanceId query\n" {
					t.Fatalf("unexpected invalid instanceId response: %q", body)
				}
				if overviewCalls != 0 || inspectionCalls != 0 {
					t.Fatalf("expected invalid instanceId to short-circuit providers, got overview=%d inspection=%d", overviewCalls, inspectionCalls)
				}
			})
		}
	})

	t.Run("ignores limit query for unbounded inspection resources", func(t *testing.T) {
		for _, resourceType := range []string{"trafficShiftPlan", "userDataBootstrapSource"} {
			t.Run(resourceType, func(t *testing.T) {
				var gotLimit int
				router := NewRouter(Options{
					BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
					OverviewDeployment: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) {
						return catalog.DeploymentOverviewDetail{Name: "api-prod"}, nil
					},
					OverviewDeploymentResourceInspection: func(_ context.Context, name, gotResourceType, resourceName string, limit int) (resourceinspection.Result, error) {
						gotLimit = limit
						return resourceinspection.Result{DeploymentName: name, ResourceType: gotResourceType, ResourceName: resourceName, Available: true}, nil
					},
				})

				resp := httptest.NewRecorder()
				router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType="+resourceType+"&resourceName=api-prod-routing&limit=0", nil))
				if resp.Code != http.StatusOK {
					t.Fatalf("expected %s resource inspection to ignore unsupported limit query, got %d with body %q", resourceType, resp.Code, resp.Body.String())
				}
				if gotLimit != 0 {
					t.Fatalf("expected %s resource inspection limit to stay zero, got %d", resourceType, gotLimit)
				}
			})
		}
	})

	t.Run("returns resource inspection payload", func(t *testing.T) {
		requestedLimit := 3
		expected := resourceinspection.Result{
			DeploymentName: "api-prod",
			ResourceType:   "ecsService",
			ResourceName:   "payments-api-prod-service",
			ClusterARN:     "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments",
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
				ClusterARN:                    "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments",
				ServiceARN:                    "arn:aws:ecs:ap-northeast-2:123456789012:service/payments/payments-api-prod-service",
				ServiceName:                   "payments-api-prod-service",
				Status:                        "ACTIVE",
				DesiredCount:                  2,
				RunningCount:                  2,
				PendingCount:                  0,
				MinimumHealthyPercent:         aws.Int32(100),
				MaximumPercent:                aws.Int32(200),
				HealthCheckGracePeriodSeconds: aws.Int32(45),
				DeploymentControllerType:      "ECS",
				DeploymentCircuitBreaker: &resourceinspection.ECSDeploymentCircuitBreaker{
					Enabled:  true,
					Rollback: true,
				},
				DeploymentAlarmConfiguration: &resourceinspection.ECSDeploymentAlarmConfiguration{
					Enabled:    true,
					Rollback:   false,
					AlarmNames: []string{"payments-api-5xx", "payments-api-latency"},
				},
				TaskDefinition: "payments-api:42",
				Deployments: []resourceinspection.ECSDeploymentSummary{{
					ID:                 "ecs-svc/123",
					Status:             "PRIMARY",
					RolloutState:       "COMPLETED",
					RolloutStateReason: "service reached steady state",
					TaskDefinition:     "payments-api:42",
					DesiredCount:       2,
					RunningCount:       2,
					PendingCount:       0,
					CreatedAt:          "2026-04-18T00:10:00Z",
					UpdatedAt:          "2026-04-18T00:11:00Z",
					Primary:            aws.Bool(true),
					Active:             aws.Bool(true),
				}},
				Events: []resourceinspection.ECSServiceEvent{{
					EventID:   "ecs_evt_test_steady_state",
					CreatedAt: "2026-04-18T00:12:00Z",
					Message:   "service reached steady state",
				}},
				EventsScope: &resourceinspection.ECSServiceEventScope{
					EffectiveLimit:     10,
					ReturnedEventCount: 1,
					TruncationStatus:   "not_truncated",
					NewestEventAt:      "2026-04-18T00:12:00Z",
					OldestEventAt:      "2026-04-18T00:12:00Z",
					Summary:            "Showing 1 ECS service events retained in this inspection response.",
				},
				Tasks: []resourceinspection.ECSTaskSummary{{
					TaskARN:              "arn:aws:ecs:ap-northeast-2:123456789012:task/payments/task-1",
					TaskID:               "task-1",
					ContainerInstanceARN: "arn:aws:ecs:ap-northeast-2:123456789012:container-instance/payments/ci-task-1",
					EC2InstanceID:        "i-0abc123def4567890",
					EC2Instance: &resourceinspection.EC2InstanceDetail{
						InstanceState:    "running",
						InstanceType:     "m6i.large",
						ImageID:          "ami-abc123",
						PrivateIPAddress: "10.0.4.12",
						SubnetID:         "subnet-abc123",
						VPCID:            "vpc-abc123",
						LaunchTime:       "2026-05-05T07:30:00Z",
					},
					CapacityProviderName: "payments-capacity",
					AvailabilityZone:     "ap-northeast-2a",
					Containers: []resourceinspection.ECSTaskContainerInfo{{
						Name:       "api",
						LastStatus: "RUNNING",
						NetworkBindings: []resourceinspection.ECSTaskContainerNetworkBinding{{
							BindIP:             "0.0.0.0",
							ContainerPort:      aws.Int32(8080),
							HostPort:           aws.Int32(32768),
							Protocol:           "tcp",
							ContainerPortRange: "8080-8081",
							HostPortRange:      "32768-32769",
						}},
					}},
				}},
				TasksScope: &resourceinspection.ECSTaskSummaryScope{
					Available:         true,
					RequestedLimit:    &requestedLimit,
					EffectiveLimit:    3,
					ReturnedTaskCount: 1,
					TruncationStatus:  "not_truncated",
				},
			},
		}

		var gotRequest struct {
			deploymentName string
			resourceType   string
			resourceName   string
			limit          int
			clusterARN     string
			requestID      string
		}
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourceinspection.Result, error) {
				gotRequest.deploymentName = deploymentName
				gotRequest.resourceType = resourceType
				gotRequest.resourceName = resourceName
				gotRequest.limit = limit
				gotRequest.clusterARN = resourceinspection.ClusterARNFromContext(ctx)
				gotRequest.requestID = RequestIDFromContext(ctx)
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=payments-api-prod-service&limit=3&clusterArn=arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments", nil)
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
			limit          int
			clusterARN     string
			requestID      string
		}{deploymentName: "api-prod", resourceType: "ecsService", resourceName: "payments-api-prod-service", limit: 3, clusterARN: "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments", requestID: responseRequestID}) {
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
		for _, key := range []string{"deploymentName", "resourceType", "resourceName", "clusterArn", "lookupContext", "available", "source", "ecsService", "unavailableReason"} {
			if _, ok := raw[key]; !ok {
				t.Fatalf("expected %q field in payload, got %#v", key, raw)
			}
		}
		if raw["clusterArn"] != "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments" {
			t.Fatalf("expected raw payload to echo selected clusterArn, got %#v", raw)
		}
		ecsService, ok := raw["ecsService"].(map[string]any)
		if !ok {
			t.Fatalf("expected ecsService object in payload, got %#v", raw["ecsService"])
		}
		for _, key := range []string{"minimumHealthyPercent", "maximumPercent", "healthCheckGracePeriodSeconds", "deploymentControllerType", "deploymentCircuitBreaker", "deploymentAlarmConfiguration", "events", "eventsScope", "tasks", "tasksScope"} {
			if _, ok := ecsService[key]; !ok {
				t.Fatalf("expected ECS service inspection field %q in payload, got %#v", key, ecsService)
			}
		}
		if ecsService["deploymentControllerType"] != "ECS" {
			t.Fatalf("expected ECS service deploymentControllerType in raw payload, got %#v", ecsService)
		}
		deployments, ok := ecsService["deployments"].([]any)
		if !ok || len(deployments) != 1 {
			t.Fatalf("expected ECS service deployments array in raw payload, got %#v", ecsService["deployments"])
		}
		deployment, ok := deployments[0].(map[string]any)
		if !ok {
			t.Fatalf("expected ECS service deployment object in raw payload, got %#v", deployments[0])
		}
		for key, want := range map[string]any{
			"id":                 "ecs-svc/123",
			"status":             "PRIMARY",
			"rolloutState":       "COMPLETED",
			"rolloutStateReason": "service reached steady state",
			"taskDefinition":     "payments-api:42",
			"desiredCount":       float64(2),
			"runningCount":       float64(2),
			"pendingCount":       float64(0),
			"createdAt":          "2026-04-18T00:10:00Z",
			"updatedAt":          "2026-04-18T00:11:00Z",
			"primary":            true,
			"active":             true,
		} {
			if !reflect.DeepEqual(deployment[key], want) {
				t.Fatalf("expected deployment.%s=%#v, got %#v in %#v", key, want, deployment[key], deployment)
			}
		}
		circuitBreaker, ok := ecsService["deploymentCircuitBreaker"].(map[string]any)
		if !ok {
			t.Fatalf("expected deploymentCircuitBreaker object in ECS service payload, got %#v", ecsService["deploymentCircuitBreaker"])
		}
		for key, want := range map[string]any{"enabled": true, "rollback": true} {
			if circuitBreaker[key] != want {
				t.Fatalf("expected deploymentCircuitBreaker.%s=%#v, got %#v in %#v", key, want, circuitBreaker[key], circuitBreaker)
			}
		}
		alarmConfig, ok := ecsService["deploymentAlarmConfiguration"].(map[string]any)
		if !ok {
			t.Fatalf("expected deploymentAlarmConfiguration object in ECS service payload, got %#v", ecsService["deploymentAlarmConfiguration"])
		}
		for key, want := range map[string]any{"enabled": true, "rollback": false} {
			if alarmConfig[key] != want {
				t.Fatalf("expected deploymentAlarmConfiguration.%s=%#v, got %#v in %#v", key, want, alarmConfig[key], alarmConfig)
			}
		}
		alarmNames, ok := alarmConfig["alarmNames"].([]any)
		if !ok || !reflect.DeepEqual(alarmNames, []any{"payments-api-5xx", "payments-api-latency"}) {
			t.Fatalf("expected deploymentAlarmConfiguration.alarmNames in raw payload, got %#v", alarmConfig["alarmNames"])
		}
		eventsScope, ok := ecsService["eventsScope"].(map[string]any)
		if !ok {
			t.Fatalf("expected eventsScope object in ECS service payload, got %#v", ecsService["eventsScope"])
		}
		for key, want := range map[string]any{
			"effectiveLimit":     float64(10),
			"returnedEventCount": float64(1),
			"truncationStatus":   "not_truncated",
			"truncated":          false,
			"newestEventAt":      "2026-04-18T00:12:00Z",
			"oldestEventAt":      "2026-04-18T00:12:00Z",
			"summary":            "Showing 1 ECS service events retained in this inspection response.",
		} {
			if !reflect.DeepEqual(eventsScope[key], want) {
				t.Fatalf("expected eventsScope[%q]=%#v, got %#v in %#v", key, want, eventsScope[key], eventsScope)
			}
		}
		events, ok := ecsService["events"].([]any)
		if !ok || len(events) != 1 {
			t.Fatalf("expected ECS service events array in raw payload, got %#v", ecsService["events"])
		}
		event, ok := events[0].(map[string]any)
		if !ok {
			t.Fatalf("expected ECS service event object in raw payload, got %#v", events[0])
		}
		for key, want := range map[string]any{
			"eventId":   "ecs_evt_test_steady_state",
			"createdAt": "2026-04-18T00:12:00Z",
			"message":   "service reached steady state",
		} {
			if !reflect.DeepEqual(event[key], want) {
				t.Fatalf("expected event.%s=%#v, got %#v in %#v", key, want, event[key], event)
			}
		}
		tasksScope, ok := ecsService["tasksScope"].(map[string]any)
		if !ok {
			t.Fatalf("expected tasksScope object in ECS service payload, got %#v", ecsService["tasksScope"])
		}
		if tasksScope["available"] != true || tasksScope["requestedLimit"] != float64(3) || tasksScope["effectiveLimit"] != float64(3) || tasksScope["returnedTaskCount"] != float64(1) || tasksScope["truncationStatus"] != "not_truncated" {
			t.Fatalf("unexpected tasksScope payload: %#v", tasksScope)
		}
		tasks, ok := ecsService["tasks"].([]any)
		if !ok || len(tasks) != 1 {
			t.Fatalf("expected one task sample in ECS service payload, got %#v", ecsService["tasks"])
		}
		task, ok := tasks[0].(map[string]any)
		if !ok {
			t.Fatalf("expected task sample object in ECS service payload, got %#v", tasks[0])
		}
		for key, want := range map[string]any{
			"containerInstanceArn": "arn:aws:ecs:ap-northeast-2:123456789012:container-instance/payments/ci-task-1",
			"ec2InstanceId":        "i-0abc123def4567890",
			"capacityProviderName": "payments-capacity",
			"availabilityZone":     "ap-northeast-2a",
		} {
			if task[key] != want {
				t.Fatalf("expected task sample %s=%#v, got %#v", key, want, task)
			}
		}
		containers, ok := task["containers"].([]any)
		if !ok || len(containers) != 1 {
			t.Fatalf("expected task sample containers in raw payload, got %#v", task["containers"])
		}
		container, ok := containers[0].(map[string]any)
		if !ok {
			t.Fatalf("expected task sample container object, got %#v", containers[0])
		}
		networkBindings, ok := container["networkBindings"].([]any)
		if !ok || len(networkBindings) != 1 {
			t.Fatalf("expected container networkBindings array in raw payload, got %#v", container["networkBindings"])
		}
		networkBinding, ok := networkBindings[0].(map[string]any)
		if !ok {
			t.Fatalf("expected networkBindings item object, got %#v", networkBindings[0])
		}
		for key, want := range map[string]any{
			"bindIp":             "0.0.0.0",
			"containerPort":      float64(8080),
			"hostPort":           float64(32768),
			"protocol":           "tcp",
			"containerPortRange": "8080-8081",
			"hostPortRange":      "32768-32769",
		} {
			if networkBinding[key] != want {
				t.Fatalf("expected networkBinding.%s=%#v, got %#v in %#v", key, want, networkBinding[key], networkBinding)
			}
		}
		ec2Instance, ok := task["ec2Instance"].(map[string]any)
		if !ok {
			t.Fatalf("expected optional ECS service task ec2Instance host detail object, got %#v", task["ec2Instance"])
		}
		for key, want := range map[string]any{
			"instanceState":    "running",
			"instanceType":     "m6i.large",
			"imageId":          "ami-abc123",
			"privateIpAddress": "10.0.4.12",
			"subnetId":         "subnet-abc123",
			"vpcId":            "vpc-abc123",
			"launchTime":       "2026-05-05T07:30:00Z",
		} {
			if ec2Instance[key] != want {
				t.Fatalf("expected task ec2Instance %s=%#v, got %#v", key, want, ec2Instance)
			}
		}
	})

	t.Run("passes optional ECS task cluster selector into resource inspection provider", func(t *testing.T) {
		var gotClusterARN string
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{Name: "api-prod"}, nil
			},
			OverviewDeploymentResourceInspection: func(ctx context.Context, deploymentName, resourceType, resourceName string, limit int) (resourceinspection.Result, error) {
				gotClusterARN = resourceinspection.ClusterARNFromContext(ctx)
				if deploymentName != "api-prod" || resourceType != "ecsTask" || resourceName != "arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123" {
					t.Fatalf("unexpected ECS task inspection request: deployment=%q type=%q name=%q", deploymentName, resourceType, resourceName)
				}
				return resourceinspection.Result{DeploymentName: deploymentName, ResourceType: resourceType, ResourceName: resourceName, ClusterARN: gotClusterARN, Available: true, Source: resourceinspection.SourceECSService, ECSTask: &resourceinspection.ECSTaskInspection{ECSTaskSummary: resourceinspection.ECSTaskSummary{TaskARN: resourceName, TaskID: "abc123", ContainerInstanceARN: "arn:aws:ecs:ap-northeast-2:123456789012:container-instance/payments/ci-abc123", CapacityProviderName: "payments-ec2", AvailabilityZone: "ap-northeast-2b", Containers: []resourceinspection.ECSTaskContainerInfo{{Name: "api", ImageDigest: "sha256:api-digest", RuntimeID: "runtime-api-123", LastStatus: "STOPPED", ExitCode: aws.Int32(1), Reason: "Error", NetworkBindings: []resourceinspection.ECSTaskContainerNetworkBinding{{BindIP: "127.0.0.1", ContainerPort: aws.Int32(8080), HostPort: aws.Int32(8080), Protocol: "tcp"}}}}}}}, nil
			},
		})

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsTask&resourceName="+url.QueryEscape("arn:aws:ecs:ap-northeast-2:123456789012:task/payments/abc123")+"&clusterArn="+url.QueryEscape(" arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments "), nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if gotClusterARN != "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments" {
			t.Fatalf("expected provider context cluster ARN, got %q", gotClusterARN)
		}
		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw ECS task inspection payload: %v", err)
		}
		if raw["clusterArn"] != "arn:aws:ecs:ap-northeast-2:123456789012:cluster/payments" {
			t.Fatalf("expected raw ECS task inspection payload to echo selected clusterArn, got %#v", raw)
		}
		ecsTask, ok := raw["ecsTask"].(map[string]any)
		if !ok {
			t.Fatalf("expected ecsTask object in raw ECS task inspection payload, got %#v", raw)
		}
		for key, want := range map[string]any{
			"containerInstanceArn": "arn:aws:ecs:ap-northeast-2:123456789012:container-instance/payments/ci-abc123",
			"capacityProviderName": "payments-ec2",
			"availabilityZone":     "ap-northeast-2b",
		} {
			if ecsTask[key] != want {
				t.Fatalf("expected ECS task inspection %s=%#v, got %#v", key, want, ecsTask)
			}
		}
		containers, ok := ecsTask["containers"].([]any)
		if !ok || len(containers) != 1 {
			t.Fatalf("expected ECS task containers in raw payload, got %#v", ecsTask["containers"])
		}
		container, ok := containers[0].(map[string]any)
		if !ok {
			t.Fatalf("expected ECS task container object, got %#v", containers[0])
		}
		for key, want := range map[string]any{
			"imageDigest": "sha256:api-digest",
			"runtimeId":   "runtime-api-123",
		} {
			if container[key] != want {
				t.Fatalf("expected ECS task container %s=%#v, got %#v in %#v", key, want, container[key], container)
			}
		}
		networkBindings, ok := container["networkBindings"].([]any)
		if !ok || len(networkBindings) != 1 {
			t.Fatalf("expected ECS task container networkBindings in raw payload, got %#v", container["networkBindings"])
		}
		networkBinding, ok := networkBindings[0].(map[string]any)
		if !ok {
			t.Fatalf("expected ECS task networkBinding object, got %#v", networkBindings[0])
		}
		for key, want := range map[string]any{
			"bindIp":        "127.0.0.1",
			"containerPort": float64(8080),
			"hostPort":      float64(8080),
			"protocol":      "tcp",
		} {
			if networkBinding[key] != want {
				t.Fatalf("expected ECS task networkBinding.%s=%#v, got %#v in %#v", key, want, networkBinding[key], networkBinding)
			}
		}
	})

	t.Run("rejects invalid ECS cluster selector before resource inspection provider lookup", func(t *testing.T) {
		var overviewCalls int
		var providerCalls int
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) {
				overviewCalls++
				return catalog.DeploymentOverviewDetail{}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
				providerCalls++
				return resourceinspection.Result{}, nil
			},
		})

		for _, rawClusterARN := range []string{
			"arn:aws:ecs:ap-northeast-2:12345678901x:cluster/payments",
			"arn:aws:ecs:ap-northeast-2:123456789012:service/cluster/name",
		} {
			t.Run(rawClusterARN, func(t *testing.T) {
				resp := httptest.NewRecorder()
				router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=api-prod-service&clusterArn="+url.QueryEscape(rawClusterARN), nil))
				if resp.Code != http.StatusBadRequest {
					t.Fatalf("expected 400, got %d with body %q", resp.Code, resp.Body.String())
				}
				if body := resp.Body.String(); body != "invalid clusterArn query\n" {
					t.Fatalf("unexpected invalid clusterArn response: %q", body)
				}
			})
		}
		if overviewCalls != 0 || providerCalls != 0 {
			t.Fatalf("expected invalid clusterArn to short-circuit providers, got overview=%d resourceInspection=%d", overviewCalls, providerCalls)
		}
	})

	t.Run("returns ECS service success-empty task sample scope", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{Name: "api-prod"}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
				return resourceinspection.Result{
					DeploymentName: "api-prod",
					ResourceType:   "ecsService",
					ResourceName:   "api-prod-service",
					Available:      true,
					Source:         resourceinspection.SourceECSService,
					ECSService: &resourceinspection.ECSServiceInspection{
						ServiceName: "api-prod-service",
						Deployments: []resourceinspection.ECSDeploymentSummary{},
						Events:      []resourceinspection.ECSServiceEvent{},
						TasksScope:  &resourceinspection.ECSTaskSummaryScope{Available: true, EffectiveLimit: 10, ReturnedTaskCount: 0, TruncationStatus: "not_truncated"},
					},
				}, nil
			},
		})

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=api-prod-service", nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw resource inspection payload: %v", err)
		}
		ecsService := raw["ecsService"].(map[string]any)
		tasksScope := ecsService["tasksScope"].(map[string]any)
		if tasksScope["available"] != true || tasksScope["returnedTaskCount"] != float64(0) || tasksScope["truncationStatus"] != "not_truncated" {
			t.Fatalf("expected successful empty task-scope payload, got %#v", tasksScope)
		}
		if _, ok := tasksScope["unavailableReason"]; ok {
			t.Fatalf("expected successful empty task scope to omit unavailableReason, got %#v", tasksScope)
		}
	})

	t.Run("returns ECS service sample-unavailable task scope", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{Name: "api-prod"}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
				return resourceinspection.Result{
					DeploymentName: "api-prod",
					ResourceType:   "ecsService",
					ResourceName:   "api-prod-service",
					Available:      true,
					Source:         resourceinspection.SourceECSService,
					ECSService: &resourceinspection.ECSServiceInspection{
						ServiceName: "api-prod-service",
						Deployments: []resourceinspection.ECSDeploymentSummary{},
						Events:      []resourceinspection.ECSServiceEvent{},
						TasksScope: &resourceinspection.ECSTaskSummaryScope{
							Available:         false,
							EffectiveLimit:    10,
							ReturnedTaskCount: 0,
							TruncationStatus:  "not_applicable",
							UnavailableReason: "aws_unavailable",
							Summary:           "ECS task sampling is unavailable because AWS access for the task bridge could not be completed.",
						},
					},
				}, nil
			},
		})

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=api-prod-service", nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw resource inspection payload: %v", err)
		}
		ecsService := raw["ecsService"].(map[string]any)
		tasksScope := ecsService["tasksScope"].(map[string]any)
		if tasksScope["available"] != false || tasksScope["unavailableReason"] != "aws_unavailable" || tasksScope["truncationStatus"] != "not_applicable" || tasksScope["summary"] == "" {
			t.Fatalf("expected explicit sample-unavailable task-scope payload, got %#v", tasksScope)
		}
	})

	t.Run("returns CodeDeploy application summary payload", func(t *testing.T) {
		expected := resourceinspection.Result{
			DeploymentName: "payments-api-staging",
			ResourceType:   "codeDeployApplication",
			ResourceName:   "payments-api-staging-codedeploy-app",
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
				Kind:           "codeDeployApplication",
				DisplayName:    "CodeDeploy application",
				InspectionMode: "summary_only",
				Summary:        "CodeDeploy application coordinating ECS traffic shifting (style: linear; qualifiers: min healthy 80% and warmup 45s)",
			},
		}

		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "payments-api-staging" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=codeDeployApplication&resourceName=payments-api-staging-codedeploy-app", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw CodeDeploy application inspection payload: %v", err)
		}
		trafficRoutingResource, ok := raw["trafficRoutingResource"].(map[string]any)
		if !ok {
			t.Fatalf("expected trafficRoutingResource object in payload, got %#v", raw["trafficRoutingResource"])
		}
		for key, want := range map[string]any{
			"kind":           "codeDeployApplication",
			"displayName":    "CodeDeploy application",
			"inspectionMode": "summary_only",
			"summary":        "CodeDeploy application coordinating ECS traffic shifting (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		} {
			if got := trafficRoutingResource[key]; !reflect.DeepEqual(got, want) {
				t.Fatalf("expected trafficRoutingResource[%q]=%v, got %#v", key, want, got)
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
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
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

	t.Run("returns weighted target group listener-rule live payload", func(t *testing.T) {
		expected := resourceinspection.Result{
			DeploymentName: "api-prod",
			ResourceType:   "productionListenerRule",
			ResourceName:   "arn:aws:elasticloadbalancing:ap-northeast-2:210987654321:listener-rule/app/payments/50dc6c495c0c9188/abcdef0123456789/1234567890abcdef",
			LookupContext: resourceinspection.LookupContext{
				Region:                 "ap-northeast-2",
				ControlAccountID:       "123456789012",
				TargetAccountID:        "210987654321",
				CrossAccount:           true,
				ObserverRoleConfigured: false,
			},
			Available: true,
			Source:    resourceinspection.SourceTrafficRoutingAWS,
			TrafficRoutingResource: &resourceinspection.TrafficRoutingInspection{
				Kind:           "productionListenerRule",
				DisplayName:    "Production listener rule",
				InspectionMode: "live",
				Summary:        "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout",
				ListenerRule: &resourceinspection.ListenerRuleInspection{
					RuleARN:   "arn:aws:elasticloadbalancing:ap-northeast-2:210987654321:listener-rule/app/payments/50dc6c495c0c9188/abcdef0123456789/1234567890abcdef",
					Priority:  "10",
					IsDefault: false,
					Conditions: []resourceinspection.ListenerRuleConditionInspection{
						{Field: "host-header", Values: []string{"payments.example.com"}},
						{Field: "path-pattern", Values: []string{"/api/*"}},
					},
					Actions: []resourceinspection.ListenerActionInspection{
						{
							Type:  "forward",
							Order: 1,
							TargetGroups: []resourceinspection.ListenerActionTargetInspection{
								{TargetGroupARN: "arn:aws:elasticloadbalancing:ap-northeast-2:210987654321:targetgroup/payments-active/abc123", Weight: 80},
								{TargetGroupARN: "arn:aws:elasticloadbalancing:ap-northeast-2:210987654321:targetgroup/payments-candidate/def456", Weight: 20},
							},
						},
					},
				},
			},
		}

		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=productionListenerRule&resourceName="+url.QueryEscape("arn:aws:elasticloadbalancing:ap-northeast-2:210987654321:listener-rule/app/payments/50dc6c495c0c9188/abcdef0123456789/1234567890abcdef"), nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw weighted target group inspection payload: %v", err)
		}
		trafficRoutingResource, ok := raw["trafficRoutingResource"].(map[string]any)
		if !ok {
			t.Fatalf("expected trafficRoutingResource object in payload, got %#v", raw["trafficRoutingResource"])
		}
		for key, want := range map[string]any{
			"kind":           "productionListenerRule",
			"displayName":    "Production listener rule",
			"inspectionMode": "live",
			"summary":        "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout",
		} {
			if got := trafficRoutingResource[key]; !reflect.DeepEqual(got, want) {
				t.Fatalf("expected trafficRoutingResource[%q]=%v, got %#v", key, want, got)
			}
		}
		listenerRule, ok := trafficRoutingResource["listenerRule"].(map[string]any)
		if !ok {
			t.Fatalf("expected listenerRule object in payload, got %#v", trafficRoutingResource["listenerRule"])
		}
		for key, want := range map[string]any{
			"ruleArn":   "arn:aws:elasticloadbalancing:ap-northeast-2:210987654321:listener-rule/app/payments/50dc6c495c0c9188/abcdef0123456789/1234567890abcdef",
			"priority":  "10",
			"isDefault": false,
		} {
			if got := listenerRule[key]; !reflect.DeepEqual(got, want) {
				t.Fatalf("expected listenerRule[%q]=%v, got %#v", key, want, got)
			}
		}
		conditions, ok := listenerRule["conditions"].([]any)
		if !ok || len(conditions) != 2 {
			t.Fatalf("expected two listenerRule conditions, got %#v", listenerRule["conditions"])
		}
		actions, ok := listenerRule["actions"].([]any)
		if !ok || len(actions) != 1 {
			t.Fatalf("expected one listenerRule action, got %#v", listenerRule["actions"])
		}
		action, ok := actions[0].(map[string]any)
		if !ok || action["type"] != "forward" {
			t.Fatalf("expected forward listenerRule action, got %#v", actions[0])
		}
		targets, ok := action["targetGroups"].([]any)
		if !ok || len(targets) != 2 {
			t.Fatalf("expected weighted listenerRule target groups, got %#v", action["targetGroups"])
		}
	})

	t.Run("returns CodeDeploy task-set target-group summary payload", func(t *testing.T) {
		expected := resourceinspection.Result{
			DeploymentName: "payments-api-staging",
			ResourceType:   "blueTaskSetTargetGroup",
			ResourceName:   "payments-api-staging-blue",
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
				Kind:           "blueTaskSetTargetGroup",
				DisplayName:    "Blue task set target group",
				InspectionMode: "summary_only",
				Summary:        "Blue task set target group CodeDeploy treats as the live ECS traffic destination (style: linear; qualifiers: min healthy 80% and warmup 45s)",
			},
		}

		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "payments-api-staging" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=blueTaskSetTargetGroup&resourceName=payments-api-staging-blue", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw CodeDeploy task-set target-group inspection payload: %v", err)
		}
		trafficRoutingResource, ok := raw["trafficRoutingResource"].(map[string]any)
		if !ok {
			t.Fatalf("expected trafficRoutingResource object in payload, got %#v", raw["trafficRoutingResource"])
		}
		for key, want := range map[string]any{
			"kind":           "blueTaskSetTargetGroup",
			"displayName":    "Blue task set target group",
			"inspectionMode": "summary_only",
			"summary":        "Blue task set target group CodeDeploy treats as the live ECS traffic destination (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		} {
			if got := trafficRoutingResource[key]; !reflect.DeepEqual(got, want) {
				t.Fatalf("expected trafficRoutingResource[%q]=%v, got %#v", key, want, got)
			}
		}
	})

	t.Run("returns CodeDeploy task-set target-group live payload", func(t *testing.T) {
		expected := resourceinspection.Result{
			DeploymentName: "payments-api-staging",
			ResourceType:   "blueTaskSetTargetGroup",
			ResourceName:   "payments-api-staging-blue",
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
				Kind:           "blueTaskSetTargetGroup",
				DisplayName:    "Blue task set target group",
				InspectionMode: "live",
				Summary:        "Blue task set target group CodeDeploy treats as the live ECS traffic destination",
				TargetGroup: &resourceinspection.TargetGroupInspection{
					TargetGroupName:            "payments-api-staging-blue",
					TargetGroupARN:             "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-staging-blue/abc123",
					Protocol:                   "HTTP",
					Port:                       8080,
					TargetType:                 "ip",
					LoadBalancerARNs:           []string{"arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:loadbalancer/app/payments/abc123"},
					HealthCheckProtocol:        "HTTP",
					HealthCheckPort:            "traffic-port",
					HealthCheckPath:            "/healthz",
					HealthCheckIntervalSeconds: 30,
					HealthCheckTimeoutSeconds:  5,
					HealthyThresholdCount:      2,
					UnhealthyThresholdCount:    3,
					TargetHealthCounts:         map[string]int{"healthy": 1, "unhealthy": 1},
					Targets: []resourceinspection.TargetHealthInspection{
						{TargetID: "10.0.1.10", Port: 8080, State: "healthy"},
						{TargetID: "10.0.2.10", Port: 8080, State: "unhealthy", Reason: "Target.ResponseCodeMismatch", Description: "Health checks failed"},
					},
				},
			},
		}

		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "payments-api-staging" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=blueTaskSetTargetGroup&resourceName=payments-api-staging-blue", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw CodeDeploy task-set target-group live payload: %v", err)
		}
		trafficRoutingResource, ok := raw["trafficRoutingResource"].(map[string]any)
		if !ok {
			t.Fatalf("expected trafficRoutingResource object in payload, got %#v", raw["trafficRoutingResource"])
		}
		if got := trafficRoutingResource["inspectionMode"]; got != "live" {
			t.Fatalf("expected live task-set target-group inspection mode, got %#v", got)
		}
		targetGroup, ok := trafficRoutingResource["targetGroup"].(map[string]any)
		if !ok {
			t.Fatalf("expected targetGroup object in payload, got %#v", trafficRoutingResource["targetGroup"])
		}
		for key, want := range map[string]any{
			"targetGroupName":            "payments-api-staging-blue",
			"targetGroupArn":             "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-staging-blue/abc123",
			"protocol":                   "HTTP",
			"port":                       float64(8080),
			"targetType":                 "ip",
			"healthCheckProtocol":        "HTTP",
			"healthCheckPort":            "traffic-port",
			"healthCheckPath":            "/healthz",
			"healthCheckIntervalSeconds": float64(30),
			"healthCheckTimeoutSeconds":  float64(5),
			"healthyThresholdCount":      float64(2),
			"unhealthyThresholdCount":    float64(3),
		} {
			if got := targetGroup[key]; !reflect.DeepEqual(got, want) {
				t.Fatalf("expected targetGroup[%q]=%v, got %#v", key, want, got)
			}
		}
		healthCounts, ok := targetGroup["targetHealthCounts"].(map[string]any)
		if !ok || healthCounts["healthy"] != float64(1) || healthCounts["unhealthy"] != float64(1) {
			t.Fatalf("unexpected targetHealthCounts: %#v", targetGroup["targetHealthCounts"])
		}
	})

	t.Run("returns weighted target group live payload", func(t *testing.T) {
		expected := resourceinspection.Result{
			DeploymentName: "api-prod",
			ResourceType:   "activeTargetGroup",
			ResourceName:   "api-prod-active",
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
				Kind:           "activeTargetGroup",
				DisplayName:    "Active target group",
				InspectionMode: "live",
				Summary:        "Currently active target group receiving production traffic",
				TargetGroup: &resourceinspection.TargetGroupInspection{
					TargetGroupName:            "api-prod-active",
					TargetGroupARN:             "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/api-prod-active/abc123",
					Protocol:                   "HTTP",
					Port:                       8080,
					TargetType:                 "instance",
					LoadBalancerARNs:           []string{"arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:loadbalancer/app/api/abc123"},
					HealthCheckProtocol:        "HTTP",
					HealthCheckPort:            "traffic-port",
					HealthCheckPath:            "/healthz",
					HealthCheckIntervalSeconds: 30,
					HealthCheckTimeoutSeconds:  5,
					HealthyThresholdCount:      2,
					UnhealthyThresholdCount:    3,
					TargetHealthCounts:         map[string]int{"healthy": 3},
					Targets: []resourceinspection.TargetHealthInspection{
						{TargetID: "i-abc123", Port: 8080, State: "healthy"},
						{TargetID: "i-def456", Port: 8080, State: "healthy"},
						{TargetID: "i-ghi789", Port: 8080, State: "healthy"},
					},
				},
			},
		}

		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=activeTargetGroup&resourceName=api-prod-active", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw weighted target-group live payload: %v", err)
		}
		trafficRoutingResource, ok := raw["trafficRoutingResource"].(map[string]any)
		if !ok {
			t.Fatalf("expected trafficRoutingResource object in payload, got %#v", raw["trafficRoutingResource"])
		}
		for key, want := range map[string]any{
			"kind":           "activeTargetGroup",
			"displayName":    "Active target group",
			"inspectionMode": "live",
			"summary":        "Currently active target group receiving production traffic",
		} {
			if got := trafficRoutingResource[key]; !reflect.DeepEqual(got, want) {
				t.Fatalf("expected trafficRoutingResource[%q]=%v, got %#v", key, want, got)
			}
		}
		targetGroup, ok := trafficRoutingResource["targetGroup"].(map[string]any)
		if !ok {
			t.Fatalf("expected weighted targetGroup object in payload, got %#v", trafficRoutingResource["targetGroup"])
		}
		for key, want := range map[string]any{
			"targetGroupName":     "api-prod-active",
			"targetGroupArn":      "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/api-prod-active/abc123",
			"protocol":            "HTTP",
			"port":                float64(8080),
			"targetType":          "instance",
			"healthCheckProtocol": "HTTP",
			"healthCheckPath":     "/healthz",
		} {
			if got := targetGroup[key]; !reflect.DeepEqual(got, want) {
				t.Fatalf("expected targetGroup[%q]=%v, got %#v", key, want, got)
			}
		}
		healthCounts, ok := targetGroup["targetHealthCounts"].(map[string]any)
		if !ok || healthCounts["healthy"] != float64(3) {
			t.Fatalf("unexpected targetHealthCounts: %#v", targetGroup["targetHealthCounts"])
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
					ID:           "lt-abc123",
					Name:         "api-prod-candidate",
					Version:      "$Default",
					ImageID:      "ami-abc123",
					InstanceType: "m6i.large",
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
			ResultScope: &resourceinspection.ResultScope{
				SampleType:        "autoScalingInstances",
				EffectiveLimit:    2,
				ReturnedItemCount: 1,
				ReturnedCount:     1,
				TruncationStatus:  "truncated",
				Truncated:         true,
			},
		}

		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
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
		if got := launchTemplate["imageId"]; got != "ami-abc123" {
			t.Fatalf("expected launchTemplate imageId, got %#v", got)
		}
		if got := launchTemplate["instanceType"]; got != "m6i.large" {
			t.Fatalf("expected launchTemplate instanceType, got %#v", got)
		}
		resultScope, ok := raw["resultScope"].(map[string]any)
		if !ok {
			t.Fatalf("expected resultScope object in payload, got %#v", raw["resultScope"])
		}
		if got := resultScope["returnedItemCount"]; got != float64(1) {
			t.Fatalf("expected returnedItemCount to remain present, got %#v", got)
		}
		if got := resultScope["returnedCount"]; got != float64(1) {
			t.Fatalf("expected returnedCount alias, got %#v", got)
		}
		if got := resultScope["truncationStatus"]; got != "truncated" {
			t.Fatalf("expected truncationStatus to remain present, got %#v", got)
		}
		if got := resultScope["truncated"]; got != true {
			t.Fatalf("expected truncated alias to mirror truncationStatus, got %#v", got)
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

	t.Run("returns user data bootstrap source inspection payload", func(t *testing.T) {
		expected := resourceinspection.Result{
			DeploymentName: "api-prod",
			ResourceType:   "userDataBootstrapSource",
			ResourceName:   "file:userdata/api-prod.sh",
			LookupContext: resourceinspection.LookupContext{
				Region:                 "ap-northeast-2",
				ControlAccountID:       "123456789012",
				TargetAccountID:        "123456789012",
				CrossAccount:           false,
				ObserverRoleConfigured: true,
			},
			Available: true,
			Source:    resourceinspection.SourceAutoScalingGroup,
			UserDataBootstrapSource: &resourceinspection.UserDataBootstrapSourceInspection{
				DesiredBootstrapSource:  "file:userdata/api-prod.sh",
				ObservedBootstrapSource: "file:userdata/api-prod-legacy.sh",
				BackingResourceType:     "candidateAutoScalingGroup",
				BackingResourceName:     "api-prod-candidate",
				LaunchTemplate: &resourceinspection.AutoScalingLaunchTemplateSummary{
					ID:           "lt-abc123",
					Name:         "api-prod-candidate",
					Version:      "$Default",
					ImageID:      "ami-legacy123",
					InstanceType: "m6i.large",
				},
				SelectedInstance: &resourceinspection.AutoScalingGroupInstanceSummary{
					InstanceID:       "i-0abc123def4567890",
					AvailabilityZone: "ap-northeast-2a",
					LifecycleState:   "InService",
					HealthStatus:     "Healthy",
					EC2InstanceStatus: &resourceinspection.EC2InstanceStatusDetail{
						InstanceState:  "running",
						InstanceStatus: "ok",
						SystemStatus:   "impaired",
					},
				},
			},
		}

		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=userDataBootstrapSource&resourceName=file:userdata/api-prod.sh", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}

		var got resourceinspection.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode bootstrap-source inspection payload: %v", err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("unexpected bootstrap-source inspection payload: %+v", got)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw bootstrap-source inspection payload: %v", err)
		}
		payload, ok := raw["userDataBootstrapSource"].(map[string]any)
		if !ok {
			t.Fatalf("expected userDataBootstrapSource object in payload, got %#v", raw["userDataBootstrapSource"])
		}
		for key, want := range map[string]any{
			"desiredBootstrapSource":  "file:userdata/api-prod.sh",
			"observedBootstrapSource": "file:userdata/api-prod-legacy.sh",
			"backingResourceType":     "candidateAutoScalingGroup",
			"backingResourceName":     "api-prod-candidate",
		} {
			if got := payload[key]; !reflect.DeepEqual(got, want) {
				t.Fatalf("expected userDataBootstrapSource[%q]=%v, got %#v", key, want, got)
			}
		}
		launchTemplate, ok := payload["launchTemplate"].(map[string]any)
		if !ok {
			t.Fatalf("expected launchTemplate object in payload, got %#v", payload["launchTemplate"])
		}
		if got := launchTemplate["name"]; got != "api-prod-candidate" {
			t.Fatalf("expected launchTemplate name, got %#v", got)
		}
		if got := launchTemplate["imageId"]; got != "ami-legacy123" {
			t.Fatalf("expected launchTemplate imageId, got %#v", got)
		}
		if got := launchTemplate["instanceType"]; got != "m6i.large" {
			t.Fatalf("expected launchTemplate instanceType, got %#v", got)
		}
		selectedInstance, ok := payload["selectedInstance"].(map[string]any)
		if !ok {
			t.Fatalf("expected selectedInstance object in bootstrap-source payload, got %#v", payload["selectedInstance"])
		}
		if selectedInstance["instanceId"] != "i-0abc123def4567890" || selectedInstance["healthStatus"] != "Healthy" {
			t.Fatalf("unexpected selectedInstance ASG fields: %#v", selectedInstance)
		}
		selectedStatus, ok := selectedInstance["ec2InstanceStatus"].(map[string]any)
		if !ok {
			t.Fatalf("expected selected bootstrap-source instance EC2 status object, got %#v", selectedInstance)
		}
		for key, want := range map[string]string{
			"instanceState":  "running",
			"instanceStatus": "ok",
			"systemStatus":   "impaired",
		} {
			if selectedStatus[key] != want {
				t.Fatalf("expected selectedInstance.ec2InstanceStatus.%s=%q, got %#v", key, want, selectedStatus)
			}
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
					ApplicationName:      "api-prod-codedeploy-app",
					DeploymentGroupName:  "api-prod-codedeploy-group",
					DeploymentConfigName: "CodeDeployDefault.ECSLinear10PercentEvery1Minutes",
					ServiceRoleARN:       "arn:aws:iam::123456789012:role/TenetCodeDeployRole",
					DeploymentType:       "BLUE_GREEN",
					DeploymentOption:     "WITH_TRAFFIC_CONTROL",
					LatestDeployment: &resourceinspection.CodeDeployDeploymentSummary{
						DeploymentID:  "d-NEWEST",
						Status:        "Succeeded",
						CreateTime:    "2026-04-18T09:15:00Z",
						EndTime:       "2026-04-18T09:18:00Z",
						Source:        "last_attempted",
						StatusMessage: "Deployment completed after shifting traffic.",
						RollbackInfo: &resourceinspection.CodeDeployRollbackInfo{
							RollbackDeploymentID:           "d-ROLLBACK",
							RollbackTriggeringDeploymentID: "d-TRIGGER",
							RollbackMessage:                "Automatic rollback was configured.",
						},
						ErrorInformation: &resourceinspection.CodeDeployErrorInformation{
							Code:    "HEALTH_CONSTRAINTS",
							Message: "Target health was below the deployment threshold.",
						},
						AlarmConfiguration: &resourceinspection.CodeDeployAlarmConfiguration{
							Enabled:                boolPtr(true),
							IgnorePollAlarmFailure: boolPtr(false),
							Alarms:                 []resourceinspection.CodeDeployAlarm{{Name: "payments-api-5xx"}, {Name: "payments-api-latency"}},
						},
						DeploymentOverview: &resourceinspection.CodeDeployDeploymentOverview{
							Pending:    0,
							InProgress: 1,
							Succeeded:  4,
							Failed:     0,
							Skipped:    0,
							Ready:      2,
						},
					},
					ECSServices:           []resourceinspection.CodeDeployECSServiceBinding{{ClusterName: "payments-prod", ServiceName: "api-prod-service"}},
					TargetGroups:          []string{"api-prod-blue", "api-prod-green"},
					ProductionListenerARN: "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:listener/app/payments/abc123/prod456",
					TestListenerARN:       "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:listener/app/payments/abc123/test789",
				},
			},
		}

		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
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
		deploymentGroup := trafficRoutingResource["codeDeployDeploymentGroup"].(map[string]any)
		latestDeployment, ok := deploymentGroup["latestDeployment"].(map[string]any)
		if !ok {
			t.Fatalf("expected latestDeployment object in payload, got %#v", deploymentGroup["latestDeployment"])
		}
		if got := latestDeployment["deploymentId"]; got != "d-NEWEST" {
			t.Fatalf("expected latest deployment id, got %#v", got)
		}
		if got := latestDeployment["status"]; got != "Succeeded" {
			t.Fatalf("expected latest deployment status, got %#v", got)
		}
		if got := latestDeployment["createTime"]; got != "2026-04-18T09:15:00Z" {
			t.Fatalf("expected latest deployment createTime, got %#v", got)
		}
		if got := latestDeployment["endTime"]; got != "2026-04-18T09:18:00Z" {
			t.Fatalf("expected latest deployment endTime, got %#v", got)
		}
		if got := latestDeployment["source"]; got != "last_attempted" {
			t.Fatalf("expected latest deployment source, got %#v", got)
		}
		if got := latestDeployment["statusMessage"]; got != "Deployment completed after shifting traffic." {
			t.Fatalf("expected latest deployment statusMessage, got %#v", got)
		}
		rollbackInfo, ok := latestDeployment["rollbackInfo"].(map[string]any)
		if !ok || rollbackInfo["rollbackDeploymentId"] != "d-ROLLBACK" || rollbackInfo["rollbackTriggeringDeploymentId"] != "d-TRIGGER" || rollbackInfo["rollbackMessage"] != "Automatic rollback was configured." {
			t.Fatalf("expected rollbackInfo diagnostics, got %#v", latestDeployment["rollbackInfo"])
		}
		errorInformation, ok := latestDeployment["errorInformation"].(map[string]any)
		if !ok || errorInformation["code"] != "HEALTH_CONSTRAINTS" || errorInformation["message"] != "Target health was below the deployment threshold." {
			t.Fatalf("expected errorInformation diagnostics, got %#v", latestDeployment["errorInformation"])
		}
		alarmConfiguration, ok := latestDeployment["alarmConfiguration"].(map[string]any)
		if !ok || alarmConfiguration["enabled"] != true || alarmConfiguration["ignorePollAlarmFailure"] != false {
			t.Fatalf("expected alarmConfiguration booleans, got %#v", latestDeployment["alarmConfiguration"])
		}
		alarms, ok := alarmConfiguration["alarms"].([]any)
		if !ok || len(alarms) != 2 {
			t.Fatalf("expected alarmConfiguration alarms, got %#v", alarmConfiguration["alarms"])
		}
		deploymentOverview, ok := latestDeployment["deploymentOverview"].(map[string]any)
		if !ok || deploymentOverview["pending"] != float64(0) || deploymentOverview["inProgress"] != float64(1) || deploymentOverview["succeeded"] != float64(4) || deploymentOverview["failed"] != float64(0) || deploymentOverview["skipped"] != float64(0) || deploymentOverview["ready"] != float64(2) {
			t.Fatalf("expected deploymentOverview diagnostics, got %#v", latestDeployment["deploymentOverview"])
		}
	})

	t.Run("returns live CodeDeploy application payload", func(t *testing.T) {
		expected := resourceinspection.Result{
			DeploymentName: "api-prod",
			ResourceType:   "codeDeployApplication",
			ResourceName:   "api-prod-codedeploy-app",
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
				Kind:           "codeDeployApplication",
				DisplayName:    "CodeDeploy application",
				InspectionMode: "live",
				Summary:        "CodeDeploy application coordinating ECS traffic shifting (style: linear)",
				CodeDeployApplication: &resourceinspection.CodeDeployApplicationInspection{
					ApplicationName:  "api-prod-codedeploy-app",
					ApplicationID:    "a-1234567890",
					ComputePlatform:  "ECS",
					LinkedToGitHub:   true,
					DeploymentGroups: []string{"api-prod-codedeploy-group", "api-prod-codedeploy-group-canary"},
				},
			},
		}

		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=codeDeployApplication&resourceName=api-prod-codedeploy-app", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw live CodeDeploy application inspection payload: %v", err)
		}
		trafficRoutingResource, ok := raw["trafficRoutingResource"].(map[string]any)
		if !ok {
			t.Fatalf("expected trafficRoutingResource object in payload, got %#v", raw["trafficRoutingResource"])
		}
		if got := trafficRoutingResource["inspectionMode"]; got != "live" {
			t.Fatalf("expected live inspectionMode, got %#v", got)
		}
		codeDeployApplication, ok := trafficRoutingResource["codeDeployApplication"].(map[string]any)
		if !ok {
			t.Fatalf("expected codeDeployApplication object in payload, got %#v", trafficRoutingResource["codeDeployApplication"])
		}
		for key, want := range map[string]any{
			"applicationName": "api-prod-codedeploy-app",
			"applicationId":   "a-1234567890",
			"computePlatform": "ECS",
			"linkedToGitHub":  true,
		} {
			if got := codeDeployApplication[key]; !reflect.DeepEqual(got, want) {
				t.Fatalf("expected codeDeployApplication[%q]=%v, got %#v", key, want, got)
			}
		}
		deploymentGroups, ok := codeDeployApplication["deploymentGroups"].([]any)
		if !ok || len(deploymentGroups) != 2 {
			t.Fatalf("expected deploymentGroups array in payload, got %#v", codeDeployApplication["deploymentGroups"])
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
				OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
					t.Fatalf("overview deployment lookup should not run for invalid query")
					return catalog.DeploymentOverviewDetail{}, nil
				},
				OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
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

	t.Run("returns 503 JSON envelope when provider is unavailable after deployment validation", func(t *testing.T) {
		deploymentLookupCalled := false
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				deploymentLookupCalled = true
				if name != "api-prod" {
					t.Fatalf("expected deployment lookup for api-prod, got %q", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d with body %q", resp.Code, resp.Body.String())
		}
		if !deploymentLookupCalled {
			t.Fatalf("expected deployment lookup to run before reporting unavailable provider")
		}
		if got := resp.Header().Get("Content-Type"); got != "application/json" {
			t.Fatalf("expected JSON content type, got %q with body %q", got, resp.Body.String())
		}
		body := bytes.TrimSpace(resp.Body.Bytes())
		var got resourceinspection.Result
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("expected resource inspection JSON envelope, got decode error %v with body %q", err, body)
		}
		if got.DeploymentName != "api-prod" || got.ResourceType != "ecsService" || got.ResourceName != "payments-api-prod-service" {
			t.Fatalf("expected request echo fields in envelope, got %+v", got)
		}
		if got.Available {
			t.Fatalf("expected unavailable envelope, got %+v", got)
		}
		if got.UnavailableReason != "overview deployment resource inspection unavailable" {
			t.Fatalf("expected resource inspection unavailable reason, got %+v", got)
		}
		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("expected raw resource inspection JSON envelope, got decode error %v with body %q", err, body)
		}
		for _, key := range []string{"deploymentName", "resourceType", "resourceName", "lookupContext", "available", "source", "unavailableReason"} {
			if _, ok := raw[key]; !ok {
				t.Fatalf("expected provider-unavailable envelope to include %s in %s", key, body)
			}
		}
	})

	t.Run("propagates deployment not found as 404", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{}, statusstore.ErrNotFound
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
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
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
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
			OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
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
						ID:      "op_plan_create-1",
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
	if record.Operations[0].ID != "op_plan_create-1" || !strings.Contains(resp.Body.String(), `"id":"op_plan_create-1"`) {
		t.Fatalf("expected raw JSON operation id field in plan preview payload, got %+v body=%s", record.Operations, resp.Body.String())
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

func TestRouterCreateDeploymentPlanPreviewPropagatesAliasRequestActor(t *testing.T) {
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
	req.Header.Set(requestactor.HeaderAliasID, "operator-123")
	req.Header.Set(requestactor.HeaderAliasName, "Jane Operator")
	req.Header.Set(requestactor.HeaderAliasEmail, "jane@example.com")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
	}
	var record planpreview.Record
	if err := json.Unmarshal(resp.Body.Bytes(), &record); err != nil {
		t.Fatalf("failed to decode plan preview: %v", err)
	}
	if !record.Actor.Available || record.Actor.ID != "operator-123" || record.Actor.Name != "Jane Operator" || record.Actor.Email != "jane@example.com" {
		t.Fatalf("expected propagated alias request actor, got %+v", record.Actor)
	}
}

func TestRouterCreateDeploymentPlanPreviewProjectsAuthenticatedIdentityIntoActor(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo:                    buildinfo.Info{Version: "dev", Commit: "abc123"},
		TrustAuthenticatedHeaders:    true,
		RequireAuthenticatedIdentity: true,
		CreateDeploymentPlanPreview: func(ctx context.Context, deploymentName string, req planpreview.CreateRequest) (planpreview.CreateResult, error) {
			identity := requestactor.AuthenticatedIdentityFromContext(ctx)
			if !identity.Authenticated || identity.ID != "front-door-123" || identity.Name != "Front Door Operator" || identity.Email != "frontdoor@example.com" {
				t.Fatalf("expected propagated authenticated identity, got %+v", identity)
			}
			return planpreview.CreateResult{
				Created: true,
				Record: planpreview.Record{
					ID:             "plan_1234abcd",
					DeploymentName: deploymentName,
					Status:         planpreview.StatusReady,
					CreatedAt:      "2026-04-17T08:00:00Z",
					Actor:          requestactor.FromContext(ctx),
					Source:         planpreview.SourceContext{DeploymentName: deploymentName, Route: "/v1/overview/deployments/api-prod", Fingerprint: req.IfMatch},
				},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{"ifMatch":"plan_input"}`))
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
	req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
	}
	var record planpreview.Record
	if err := json.Unmarshal(resp.Body.Bytes(), &record); err != nil {
		t.Fatalf("failed to decode plan preview: %v", err)
	}
	if !record.Actor.Available || record.Actor.ID != "front-door-123" || record.Actor.Name != "Front Door Operator" || record.Actor.Email != "frontdoor@example.com" {
		t.Fatalf("expected projected actor from authenticated identity, got %+v", record.Actor)
	}
}

func TestRouterCreateDeploymentPlanPreviewAcceptsAliasAuthenticatedIdentityWithoutChangingActorPrecedence(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo:                    buildinfo.Info{Version: "dev", Commit: "abc123"},
		TrustAuthenticatedHeaders:    true,
		RequireAuthenticatedIdentity: true,
		CreateDeploymentPlanPreview: func(ctx context.Context, deploymentName string, req planpreview.CreateRequest) (planpreview.CreateResult, error) {
			identity := requestactor.AuthenticatedIdentityFromContext(ctx)
			if !identity.Authenticated || identity.ID != "front-door-123" || identity.Name != "Front Door Operator" || identity.Email != "frontdoor@example.com" {
				t.Fatalf("expected alias-backed authenticated identity, got %+v", identity)
			}
			return planpreview.CreateResult{
				Created: true,
				Record: planpreview.Record{
					ID:             "plan_1234abcd",
					DeploymentName: deploymentName,
					Status:         planpreview.StatusReady,
					CreatedAt:      "2026-04-17T08:00:00Z",
					Actor:          requestactor.FromContext(ctx),
					Source:         planpreview.SourceContext{DeploymentName: deploymentName, Route: "/v1/overview/deployments/api-prod", Fingerprint: req.IfMatch},
				},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{"ifMatch":"plan_input"}`))
	req.Header.Set(requestactor.HeaderID, "explicit-456")
	req.Header.Set(requestactor.HeaderName, "Alex Explicit")
	req.Header.Set(requestactor.HeaderEmail, "alex.explicit@example.com")
	req.Header.Set(requestactor.HeaderAliasID, "front-door-123")
	req.Header.Set(requestactor.HeaderAliasName, "Front Door Operator")
	req.Header.Set(requestactor.HeaderAliasEmail, "frontdoor@example.com")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
	}
	var record planpreview.Record
	if err := json.Unmarshal(resp.Body.Bytes(), &record); err != nil {
		t.Fatalf("failed to decode plan preview: %v", err)
	}
	if !record.Actor.Available || record.Actor.ID != "explicit-456" || record.Actor.Name != "Alex Explicit" || record.Actor.Email != "alex.explicit@example.com" {
		t.Fatalf("expected explicit local actor to override alias-backed authenticated identity projection, got %+v", record.Actor)
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

	t.Run("unauthenticated when operator auth is enforced", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo:                    buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireAuthenticatedIdentity: true,
			CreateDeploymentPlanPreview: func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
				t.Fatal("create provider should not run without authenticated identity")
				return planpreview.CreateResult{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator write route\n" {
			t.Fatalf("unexpected unauthenticated response body: %q", body)
		}
	})

	t.Run("insufficient trusted role short-circuits create", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                    buildinfo.Info{Version: "dev", Commit: "abc123"},
			TrustAuthenticatedHeaders:    true,
			RequireAuthenticatedIdentity: true,
			RequireOperatorRoleForWrite:  true,
			CreateDeploymentPlanPreview: func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
				providerCalls++
				t.Fatal("create provider should not run without operator-capable role")
				return planpreview.CreateResult{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Viewer")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "viewer@example.com")
		req.Header.Set(requestactor.AuthenticatedRolesHeader, "viewer")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "forbidden: authenticated operator role is required for this operator write route\n" {
			t.Fatalf("unexpected forbidden response body: %q", body)
		}
		if providerCalls != 0 {
			t.Fatalf("expected role guard to short-circuit before create provider, got %d calls", providerCalls)
		}
	})

	t.Run("write role requirement enables trusted role parsing", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                   buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireOperatorRoleForWrite: true,
			CreateDeploymentPlanPreview: func(_ context.Context, deploymentName string, req planpreview.CreateRequest) (planpreview.CreateResult, error) {
				providerCalls++
				return planpreview.CreateResult{Created: true, Record: planpreview.Record{ID: "plan_1234abcd", DeploymentName: deploymentName, Status: planpreview.StatusReady, CreatedAt: "2026-04-17T08:00:00Z", Source: planpreview.SourceContext{DeploymentName: deploymentName, Fingerprint: req.IfMatch}}}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{"ifMatch":"plan_input"}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "operator@example.com")
		req.Header.Set(requestactor.AuthenticatedRolesHeader, "operator")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
		}
		if providerCalls != 1 {
			t.Fatalf("expected create provider to run once, got %d calls", providerCalls)
		}
	})

	t.Run("admin trusted role is operator-capable for create", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                    buildinfo.Info{Version: "dev", Commit: "abc123"},
			TrustAuthenticatedHeaders:    true,
			RequireAuthenticatedIdentity: true,
			RequireOperatorRoleForWrite:  true,
			CreateDeploymentPlanPreview: func(_ context.Context, deploymentName string, req planpreview.CreateRequest) (planpreview.CreateResult, error) {
				providerCalls++
				return planpreview.CreateResult{Created: true, Record: planpreview.Record{ID: "plan_1234abcd", DeploymentName: deploymentName, Status: planpreview.StatusReady, CreatedAt: "2026-04-17T08:00:00Z", Source: planpreview.SourceContext{DeploymentName: deploymentName, Fingerprint: req.IfMatch}}}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{"ifMatch":"plan_input"}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Admin")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "admin@example.com")
		req.Header.Set(requestactor.AuthenticatedRolesHeader, "viewer, admin")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
		}
		if providerCalls != 1 {
			t.Fatalf("expected create provider to run once, got %d calls", providerCalls)
		}
	})

	t.Run("deployment authorization short-circuits unauthorized create", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			CreateDeploymentPlanPreview: func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
				providerCalls++
				t.Fatal("create provider should not run without deployment access")
				return planpreview.CreateResult{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "forbidden: operator does not have access to this deployment\n" {
			t.Fatalf("unexpected forbidden response body: %q", body)
		}
		if providerCalls != 0 {
			t.Fatalf("expected authz guard to short-circuit before create provider, got %d calls", providerCalls)
		}
	})

	t.Run("deployment authorization allows authorized create", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			CreateDeploymentPlanPreview: func(_ context.Context, deploymentName string, req planpreview.CreateRequest) (planpreview.CreateResult, error) {
				providerCalls++
				if deploymentName != "api-prod" {
					t.Fatalf("unexpected deployment name %q", deploymentName)
				}
				return planpreview.CreateResult{Created: true, Record: planpreview.Record{ID: "plan_1234abcd", DeploymentName: deploymentName, Status: planpreview.StatusReady, CreatedAt: "2026-04-17T08:00:00Z", Source: planpreview.SourceContext{DeploymentName: deploymentName, Fingerprint: req.IfMatch}}}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{"ifMatch":"plan_input"}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "api-prod, worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
		}
		if providerCalls != 1 {
			t.Fatalf("expected create provider to run once, got %d calls", providerCalls)
		}
	})

	t.Run("repository policy authorization allows scoped create without role or deployment headers", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			TrustAuthenticatedHeaders:          true,
			RequireOperatorRoleForWrite:        true,
			RequireDeploymentReadAuthorization: true,
			AuthorizeWithRepositoryPolicy:      true,
			RepositoryPolicy: func(_ context.Context, identity requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
				if !identity.Authenticated || identity.ID != "front-door-123" {
					t.Fatalf("expected authenticated identity to reach repository policy evaluation, got %+v", identity)
				}
				if !reflect.DeepEqual(groups.Names, []string{"platform-admins"}) {
					t.Fatalf("expected forwarded groups to reach repository policy evaluation, got %+v", groups.Names)
				}
				return rbacpolicy.Evaluate(rbacpolicy.Policy{
					Version: rbacpolicy.VersionV1,
					Groups: []rbacpolicy.GroupGrant{{
						Name:        "platform-admins",
						Roles:       []string{requestactor.RoleOperator},
						Deployments: []string{"api-prod"},
					}},
				}, identity, groups)
			},
			CreateDeploymentPlanPreview: func(_ context.Context, deploymentName string, req planpreview.CreateRequest) (planpreview.CreateResult, error) {
				providerCalls++
				if deploymentName != "api-prod" {
					t.Fatalf("unexpected deployment name %q", deploymentName)
				}
				return planpreview.CreateResult{Created: true, Record: planpreview.Record{ID: "plan_1234abcd", DeploymentName: deploymentName, Status: planpreview.StatusReady, CreatedAt: "2026-04-17T08:00:00Z", Source: planpreview.SourceContext{DeploymentName: deploymentName, Fingerprint: req.IfMatch}}}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{"ifMatch":"plan_input"}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthenticatedGroupsHeader, "platform-admins")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
		}
		if providerCalls != 1 {
			t.Fatalf("expected create provider to run once, got %d calls", providerCalls)
		}
	})

	t.Run("repository policy remains diagnostic only until authorization mode is enabled", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			TrustAuthenticatedHeaders:          true,
			RequireOperatorRoleForWrite:        true,
			RequireDeploymentReadAuthorization: true,
			RepositoryPolicy: func(_ context.Context, identity requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
				return rbacpolicy.Evaluate(rbacpolicy.Policy{
					Version: rbacpolicy.VersionV1,
					Groups: []rbacpolicy.GroupGrant{{
						Name:        "platform-admins",
						Roles:       []string{requestactor.RoleOperator},
						Deployments: []string{"api-prod"},
					}},
				}, identity, groups)
			},
			CreateDeploymentPlanPreview: func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
				providerCalls++
				t.Fatal("create provider should not run when trusted role headers are still authoritative")
				return planpreview.CreateResult{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{"ifMatch":"plan_input"}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthenticatedGroupsHeader, "platform-admins")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403 from trusted-header role enforcement, got %d with body %q", resp.Code, resp.Body.String())
		}
		if providerCalls != 0 {
			t.Fatalf("expected role guard to short-circuit before create provider, got %d calls", providerCalls)
		}
	})

	t.Run("repository policy authorization denies scoped create without a matching grant", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			TrustAuthenticatedHeaders:          true,
			RequireOperatorRoleForWrite:        true,
			RequireDeploymentReadAuthorization: true,
			AuthorizeWithRepositoryPolicy:      true,
			RepositoryPolicy: func(_ context.Context, identity requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
				return rbacpolicy.Evaluate(rbacpolicy.Policy{
					Version: rbacpolicy.VersionV1,
					Groups: []rbacpolicy.GroupGrant{{
						Name:        "other-team",
						Roles:       []string{requestactor.RoleOperator},
						Deployments: []string{"api-prod"},
					}},
				}, identity, groups)
			},
			CreateDeploymentPlanPreview: func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
				providerCalls++
				t.Fatal("create provider should not run without a matching repository policy grant")
				return planpreview.CreateResult{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{"ifMatch":"plan_input"}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedGroupsHeader, "platform-admins")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403 from repository policy authorization, got %d with body %q", resp.Code, resp.Body.String())
		}
		if providerCalls != 0 {
			t.Fatalf("expected role guard to short-circuit before create provider, got %d calls", providerCalls)
		}
	})

	t.Run("repository policy authorization still requires authenticated identity", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			TrustAuthenticatedHeaders:          true,
			RequireOperatorRoleForWrite:        true,
			RequireDeploymentReadAuthorization: true,
			AuthorizeWithRepositoryPolicy:      true,
			RepositoryPolicy: func(_ context.Context, identity requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
				return rbacpolicy.Evaluate(rbacpolicy.Policy{Version: rbacpolicy.VersionV1}, identity, groups)
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{"ifMatch":"plan_input"}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 from missing trusted identity, got %d with body %q", resp.Code, resp.Body.String())
		}
	})

	t.Run("repository policy authorization allows scoped read from a direct user grant", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			TrustAuthenticatedHeaders:          true,
			RequireOperatorRoleForRead:         true,
			RequireDeploymentReadAuthorization: true,
			AuthorizeWithRepositoryPolicy:      true,
			RepositoryPolicy: func(_ context.Context, identity requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
				return rbacpolicy.Evaluate(rbacpolicy.Policy{
					Version: rbacpolicy.VersionV1,
					Users: []rbacpolicy.Subject{{
						Email:       "operator@example.com",
						Roles:       []string{requestactor.RoleViewer},
						Deployments: []string{"api-prod"},
					}},
				}, identity, groups)
			},
			OverviewDeployment: func(_ context.Context, deploymentName string) (catalog.DeploymentOverviewDetail, error) {
				providerCalls++
				if deploymentName != "api-prod" {
					t.Fatalf("unexpected deployment name %q", deploymentName)
				}
				return catalog.DeploymentOverviewDetail{Name: deploymentName}, nil
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "operator@example.com")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if providerCalls != 1 {
			t.Fatalf("expected overview provider to run once, got %d calls", providerCalls)
		}
	})

	t.Run("project environment authorization allows scoped create", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			CatalogPlacements: func(context.Context) ([]catalog.PlacementView, error) {
				return []catalog.PlacementView{{DeploymentName: "api-prod", AppRef: "payments", EnvironmentRef: "prod"}}, nil
			},
			CreateDeploymentPlanPreview: func(_ context.Context, deploymentName string, req planpreview.CreateRequest) (planpreview.CreateResult, error) {
				providerCalls++
				if deploymentName != "api-prod" {
					t.Fatalf("unexpected deployment name %q", deploymentName)
				}
				return planpreview.CreateResult{Created: true, Record: planpreview.Record{ID: "plan_1234abcd", DeploymentName: deploymentName, Status: planpreview.StatusReady, CreatedAt: "2026-04-17T08:00:00Z", Source: planpreview.SourceContext{DeploymentName: deploymentName, Fingerprint: req.IfMatch}}}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{"ifMatch":"plan_input"}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthorizedProjectsHeader, "payments")
		req.Header.Set(requestactor.AuthorizedEnvironmentsHeader, "prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
		}
		if providerCalls != 1 {
			t.Fatalf("expected create provider to run once, got %d calls", providerCalls)
		}
	})

	t.Run("project environment authorization fails closed without catalog placement", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			CreateDeploymentPlanPreview: func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
				providerCalls++
				t.Fatal("create provider should not run without catalog placement data")
				return planpreview.CreateResult{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/api-prod", strings.NewReader(`{}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthorizedProjectsHeader, "payments")
		req.Header.Set(requestactor.AuthorizedEnvironmentsHeader, "prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d with body %q", resp.Code, resp.Body.String())
		}
		if providerCalls != 0 {
			t.Fatalf("expected authz guard to short-circuit before create provider, got %d calls", providerCalls)
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
						{ID: "op_plan_create-1", Status: applyrequest.OperationStatusPending, Phase: "Provisioning", Target: "api-prod-codedeploy-group (codeDeployDeploymentGroup)", Summary: "Create the CodeDeploy deployment for api-prod."},
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
	if record.Operations[0].ID != "op_plan_create-1" || !strings.Contains(resp.Body.String(), `"id":"op_plan_create-1"`) {
		t.Fatalf("expected raw JSON operation id field in apply request payload, got %+v body=%s", record.Operations, resp.Body.String())
	}
}

func TestRouterGetApplyExecutionPreflight(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		ApplyExecutionPreflight: func(ctx context.Context, applyID string) (applyrequest.ExecutionReadiness, error) {
			if applyID != "apply_1234abcd" {
				return applyrequest.ExecutionReadiness{}, applyrequest.ErrNotFound
			}
			return applyrequest.ExecutionReadiness{
				State:     applyrequest.ExecutionReadinessStateBlocked,
				Summary:   "Local-safe executor handoff metadata is present, but live AWS mutation remains blocked.",
				CheckedAt: "2026-04-18T08:00:00Z",
				Evidence:  []string{"Preflight reused immutable applyId apply_1234abcd and did not contact AWS."},
				DeployerRole: &applyrequest.DeployerRoleReadiness{
					AWSAccountID:   "123456789012",
					Region:         "ap-northeast-2",
					RoleARN:        "arn:aws:iam::123456789012:role/TenetDeployerRole",
					Configured:     true,
					CheckedWithAWS: true,
					Ready:          true,
					Summary:        "Configured deployer role was assumed with a read-only identity probe.",
					Evidence:       []string{"STS GetCallerIdentity succeeded after assuming the configured deployer role."},
				},
				Operations: []applyrequest.OperationExecutionPreflight{
					{OperationID: "op_plan_create-1", Phase: "Provisioning", Target: "api-prod-codedeploy-group (codeDeployDeploymentGroup)", ResourceType: "codeDeployDeploymentGroup", ResourceName: "api-prod-codedeploy-group", RequiredCapability: "codedeploy:CreateDeployment", RequiredRole: "arn:aws:iam::123456789012:role/TenetDeployerRole", InputsSufficient: true, Ready: false, UnavailableReason: "live_aws_mutation_not_implemented", Summary: "Operation has reviewed apply intent, but live AWS mutation is not implemented for this local-safe executor boundary.", CapabilityChecks: []applyrequest.OperationExecutionCapabilityCheck{{Action: "codedeploy:GetDeploymentGroup", ResourceType: "codeDeployDeploymentGroup", ResourceName: "api-prod-codedeploy-group", RequiredRole: "arn:aws:iam::123456789012:role/TenetDeployerRole", State: "blocked", Reason: "live_aws_mutation_not_implemented", Summary: "Future live executors must prove this AWS capability for the ECS/CodeDeploy rollout boundary; this local-safe preflight does not perform live mutation."}, {Action: "codedeploy:CreateDeployment", ResourceType: "codeDeployDeploymentGroup", ResourceName: "api-prod-codedeploy-group", RequiredRole: "arn:aws:iam::123456789012:role/TenetDeployerRole", State: "blocked", Reason: "live_aws_mutation_not_implemented", Summary: "Future live executors must prove this AWS capability for the ECS/CodeDeploy rollout boundary; this local-safe preflight does not perform live mutation."}}, Handoffs: &applyrequest.OperationExecutionPreflightHandoffs{DeploymentDetailRoute: "/v1/overview/deployments/api-prod", PlanApplyWorkspaceRoute: "/?applyId=apply_1234abcd&deployment=api-prod&page=plan&planId=plan_1234abcd", ResourceInspectionRoute: "/v1/overview/deployments/api-prod/resource-inspection?resourceName=api-prod-codedeploy-group&resourceType=codeDeployDeploymentGroup", ResourceLogsRoute: "/v1/overview/deployments/api-prod/resource-logs?resourceName=api-prod-codedeploy-group&resourceType=codeDeployDeploymentGroup"}},
				},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/applies/apply_1234abcd/preflight", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
	}
	var readiness applyrequest.ExecutionReadiness
	if err := json.Unmarshal(resp.Body.Bytes(), &readiness); err != nil {
		t.Fatalf("failed to decode execution preflight: %v", err)
	}
	if readiness.State != applyrequest.ExecutionReadinessStateBlocked || readiness.CheckedAt != "2026-04-18T08:00:00Z" {
		t.Fatalf("unexpected execution preflight payload: %+v", readiness)
	}
	if len(readiness.Operations) != 1 || readiness.Operations[0].RequiredCapability != "codedeploy:CreateDeployment" || readiness.Operations[0].Ready {
		t.Fatalf("expected blocked per-operation preflight row, got %+v", readiness.Operations)
	}
	if len(readiness.Operations[0].CapabilityChecks) != 2 || readiness.Operations[0].CapabilityChecks[0].Action != "codedeploy:GetDeploymentGroup" || readiness.Operations[0].CapabilityChecks[1].State != "blocked" {
		t.Fatalf("expected nested capability checks, got %+v", readiness.Operations[0].CapabilityChecks)
	}
	if readiness.Operations[0].Handoffs == nil || readiness.Operations[0].Handoffs.ResourceInspectionRoute == "" || readiness.Operations[0].Handoffs.ResourceLogsRoute == "" {
		t.Fatalf("expected preflight operation troubleshooting handoffs, got %+v", readiness.Operations[0].Handoffs)
	}
	if readiness.DeployerRole == nil || readiness.DeployerRole.RoleARN != "arn:aws:iam::123456789012:role/TenetDeployerRole" || !readiness.DeployerRole.Ready || !readiness.DeployerRole.CheckedWithAWS {
		t.Fatalf("expected deployer-role readiness evidence, got %+v", readiness.DeployerRole)
	}
	body := resp.Body.String()
	if strings.Contains(body, `"executionReadiness"`) || !strings.Contains(body, `"deployerRole"`) || !strings.Contains(body, `"checkedWithAws":true`) || !strings.Contains(body, `"requiredRole":"arn:aws:iam::123456789012:role/TenetDeployerRole"`) || !strings.Contains(body, `"requiredCapability":"codedeploy:CreateDeployment"`) || !strings.Contains(body, `"capabilityChecks"`) || !strings.Contains(body, `"action":"codedeploy:CreateDeployment"`) || !strings.Contains(body, `"state":"blocked"`) || !strings.Contains(body, `"resourceInspectionRoute"`) || !strings.Contains(body, `"resourceLogsRoute"`) {
		t.Fatalf("expected direct readiness JSON with operation handoffs and without an apply-record wrapper, got %s", resp.Body.String())
	}

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/v1/applies/apply_missing/preflight", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing apply preflight, got %d body=%q", missing.Code, missing.Body.String())
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

func TestRouterCreateApplyRequestPrefersExplicitActorHeadersOverAliases(t *testing.T) {
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
	req.Header.Set(requestactor.HeaderAliasID, "alias-456")
	req.Header.Set(requestactor.HeaderAliasName, "Alias Operator")
	req.Header.Set(requestactor.HeaderAliasEmail, "alias@example.com")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
	}
	var record applyrequest.Record
	if err := json.Unmarshal(resp.Body.Bytes(), &record); err != nil {
		t.Fatalf("failed to decode apply request: %v", err)
	}
	if !record.Actor.Available || record.Actor.ID != "operator-123" || record.Actor.Name != "Jane Operator" || record.Actor.Email != "jane@example.com" {
		t.Fatalf("expected explicit request actor to override aliases, got %+v", record.Actor)
	}
}

func TestRouterCreateApplyRequestProjectsAuthenticatedIdentityIntoActor(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo:                    buildinfo.Info{Version: "dev", Commit: "abc123"},
		TrustAuthenticatedHeaders:    true,
		RequireAuthenticatedIdentity: true,
		CreateApplyRequest: func(ctx context.Context, planID string) (applyrequest.CreateResult, error) {
			identity := requestactor.AuthenticatedIdentityFromContext(ctx)
			if !identity.Authenticated || identity.ID != "front-door-123" || identity.Name != "Front Door Operator" || identity.Email != "frontdoor@example.com" {
				t.Fatalf("expected propagated authenticated identity, got %+v", identity)
			}
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
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
	req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
	}
	var record applyrequest.Record
	if err := json.Unmarshal(resp.Body.Bytes(), &record); err != nil {
		t.Fatalf("failed to decode apply request: %v", err)
	}
	if !record.Actor.Available || record.Actor.ID != "front-door-123" || record.Actor.Name != "Front Door Operator" || record.Actor.Email != "frontdoor@example.com" {
		t.Fatalf("expected projected actor from authenticated identity, got %+v", record.Actor)
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

	t.Run("unauthenticated when operator auth is enforced", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo:                    buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireAuthenticatedIdentity: true,
			CreateApplyRequest: func(context.Context, string) (applyrequest.CreateResult, error) {
				t.Fatal("create provider should not run without authenticated identity")
				return applyrequest.CreateResult{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/plans/plan_1234abcd", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator write route\n" {
			t.Fatalf("unexpected unauthenticated response body: %q", body)
		}
	})

	t.Run("insufficient trusted role short-circuits apply creation", func(t *testing.T) {
		planLookupCalls := 0
		createCalls := 0
		router := NewRouter(Options{
			BuildInfo:                    buildinfo.Info{Version: "dev", Commit: "abc123"},
			TrustAuthenticatedHeaders:    true,
			RequireAuthenticatedIdentity: true,
			RequireOperatorRoleForWrite:  true,
			PlanPreview: func(context.Context, string) (planpreview.Record, error) {
				planLookupCalls++
				t.Fatal("plan lookup should not run without operator-capable role")
				return planpreview.Record{}, nil
			},
			CreateApplyRequest: func(context.Context, string) (applyrequest.CreateResult, error) {
				createCalls++
				t.Fatal("create provider should not run without operator-capable role")
				return applyrequest.CreateResult{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/plans/plan_1234abcd", nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Viewer")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "viewer@example.com")
		req.Header.Set(requestactor.AuthenticatedRolesHeader, "viewer")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "forbidden: authenticated operator role is required for this operator write route\n" {
			t.Fatalf("unexpected forbidden response body: %q", body)
		}
		if planLookupCalls != 0 {
			t.Fatalf("expected role guard to short-circuit before plan lookup, got %d calls", planLookupCalls)
		}
		if createCalls != 0 {
			t.Fatalf("expected role guard to short-circuit before create provider, got %d calls", createCalls)
		}
	})

	t.Run("deployment authorization short-circuits unauthorized create", func(t *testing.T) {
		planLookupCalls := 0
		createCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			PlanPreview: func(context.Context, string) (planpreview.Record, error) {
				planLookupCalls++
				return planpreview.Record{ID: "plan_1234abcd", DeploymentName: "api-prod", Status: planpreview.StatusReady, CreatedAt: "2026-04-18T08:00:00Z"}, nil
			},
			CreateApplyRequest: func(context.Context, string) (applyrequest.CreateResult, error) {
				createCalls++
				t.Fatal("create provider should not run without deployment access")
				return applyrequest.CreateResult{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/plans/plan_1234abcd", nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "forbidden: operator does not have access to this deployment\n" {
			t.Fatalf("unexpected forbidden response body: %q", body)
		}
		if planLookupCalls != 1 {
			t.Fatalf("expected one plan lookup before authz denial, got %d calls", planLookupCalls)
		}
		if createCalls != 0 {
			t.Fatalf("expected authz guard to short-circuit before create provider, got %d calls", createCalls)
		}
	})

	t.Run("deployment authorization allows authorized create", func(t *testing.T) {
		planLookupCalls := 0
		createCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			PlanPreview: func(context.Context, string) (planpreview.Record, error) {
				planLookupCalls++
				return planpreview.Record{ID: "plan_1234abcd", DeploymentName: "api-prod", Status: planpreview.StatusReady, CreatedAt: "2026-04-18T08:00:00Z"}, nil
			},
			CreateApplyRequest: func(_ context.Context, planID string) (applyrequest.CreateResult, error) {
				createCalls++
				return applyrequest.CreateResult{Created: true, Record: applyrequest.Record{ID: "apply_1234abcd", PlanID: planID, DeploymentName: "api-prod", Status: applyrequest.StatusFailed, CreatedAt: "2026-04-18T08:00:00Z"}}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/plans/plan_1234abcd", nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "api-prod, worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
		}
		if planLookupCalls != 1 {
			t.Fatalf("expected one plan lookup, got %d calls", planLookupCalls)
		}
		if createCalls != 1 {
			t.Fatalf("expected create provider to run once, got %d calls", createCalls)
		}
	})

	t.Run("project environment authorization allows scoped create", func(t *testing.T) {
		planLookupCalls := 0
		createCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			CatalogPlacements: func(context.Context) ([]catalog.PlacementView, error) {
				return []catalog.PlacementView{{DeploymentName: "api-prod", AppRef: "payments", EnvironmentRef: "prod"}}, nil
			},
			PlanPreview: func(context.Context, string) (planpreview.Record, error) {
				planLookupCalls++
				return planpreview.Record{ID: "plan_1234abcd", DeploymentName: "api-prod", Status: planpreview.StatusReady, CreatedAt: "2026-04-18T08:00:00Z"}, nil
			},
			CreateApplyRequest: func(_ context.Context, planID string) (applyrequest.CreateResult, error) {
				createCalls++
				return applyrequest.CreateResult{Created: true, Record: applyrequest.Record{ID: "apply_1234abcd", PlanID: planID, DeploymentName: "api-prod", Status: applyrequest.StatusFailed, CreatedAt: "2026-04-18T08:00:00Z"}}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/plans/plan_1234abcd", nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthorizedProjectsHeader, "payments")
		req.Header.Set(requestactor.AuthorizedEnvironmentsHeader, "prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d with body %q", resp.Code, resp.Body.String())
		}
		if planLookupCalls != 1 || createCalls != 1 {
			t.Fatalf("expected one plan lookup and one create call, got %d/%d", planLookupCalls, createCalls)
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

func TestRouterApplyRequestCollection(t *testing.T) {
	records := []applyrequest.Record{
		{
			ID:             "apply_queued_api",
			PlanID:         "plan_queued_api",
			DeploymentName: "api-prod",
			Status:         applyrequest.StatusQueued,
			CreatedAt:      "2026-04-18T08:05:00Z",
			Details:        []string{"queued for local-safe executor handoff"},
			Lifecycle:      applyrequest.Lifecycle{UpdatedAt: "2026-04-18T08:05:00Z", TotalSteps: 1},
			Operations: []applyrequest.Operation{{
				ID:      "op_shift_traffic",
				Status:  applyrequest.OperationStatusQueued,
				Phase:   "Promoting",
				Target:  "api-prod listener",
				Summary: "Shift production traffic.",
			}},
			Source: applyrequest.SourceContext{PlanID: "plan_queued_api", Route: "/v1/plans/plan_queued_api", PlanStatus: "ready"},
		},
		{
			ID:             "apply_queued_worker",
			PlanID:         "plan_queued_worker",
			DeploymentName: "worker-prod",
			Status:         applyrequest.StatusQueued,
			CreatedAt:      "2026-04-18T08:06:00Z",
			Source:         applyrequest.SourceContext{PlanID: "plan_queued_worker", Route: "/v1/plans/plan_queued_worker", PlanStatus: "ready"},
		},
		{
			ID:             "apply_running_api",
			PlanID:         "plan_running_api",
			DeploymentName: "api-prod",
			Status:         applyrequest.StatusRunning,
			CreatedAt:      "2026-04-18T08:07:00Z",
			Details:        []string{"claimed for local-safe executor handoff"},
			Lifecycle:      applyrequest.Lifecycle{UpdatedAt: "2026-04-18T08:07:30Z", StartedAt: "2026-04-18T08:07:00Z", CurrentStep: 1, TotalSteps: 2},
			Claim:          &applyrequest.Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:07:00Z", LastHeartbeatAt: "2026-04-18T08:07:30Z", ExpiresAt: "2026-04-18T08:12:30Z"},
			Operations: []applyrequest.Operation{{
				ID:      "op_shift_traffic",
				Status:  applyrequest.OperationStatusRunning,
				Phase:   "Promoting",
				Target:  "api-prod listener",
				Summary: "Shift production traffic.",
			}},
			Source: applyrequest.SourceContext{PlanID: "plan_running_api", Route: "/v1/plans/plan_running_api", PlanStatus: "ready"},
		},
		{
			ID:             "apply_failed_api",
			PlanID:         "plan_failed_api",
			DeploymentName: "api-prod",
			Status:         applyrequest.StatusFailed,
			CreatedAt:      "2026-04-18T08:00:00Z",
			Error:          "apply execution is disabled",
			Source:         applyrequest.SourceContext{PlanID: "plan_failed_api", Route: "/v1/plans/plan_failed_api", PlanStatus: "ready"},
		},
	}

	t.Run("queued payload excludes terminal records and preserves raw json fields", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			ApplyRequests: func(_ context.Context, query applyrequest.ListQuery) ([]applyrequest.Record, error) {
				if query.Status != applyrequest.StatusQueued {
					t.Fatalf("expected queued list query, got %+v", query)
				}
				return records, nil
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/applies?status=queued", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var got []applyrequest.Record
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode queued applies payload: %v", err)
		}
		if len(got) != 2 || got[0].ID != "apply_queued_api" || got[1].ID != "apply_queued_worker" {
			t.Fatalf("unexpected queued applies payload: %+v", got)
		}
		for _, record := range got {
			if record.Status != applyrequest.StatusQueued {
				t.Fatalf("expected only queued records, got %+v", got)
			}
		}

		var raw []map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw queued applies payload: %v", err)
		}
		for _, key := range []string{"id", "planId", "deploymentName", "status", "createdAt", "details", "lifecycle", "operations", "source"} {
			if _, ok := raw[0][key]; !ok {
				t.Fatalf("expected queued apply payload to include %q, got %#v", key, raw[0])
			}
		}
		operations, ok := raw[0]["operations"].([]any)
		if !ok || len(operations) != 1 {
			t.Fatalf("expected raw operations array, got %#v", raw[0]["operations"])
		}
		firstOperation := operations[0].(map[string]any)
		for _, key := range []string{"id", "status", "phase", "target", "summary"} {
			if _, ok := firstOperation[key]; !ok {
				t.Fatalf("expected queued apply operation to include %q, got %#v", key, firstOperation)
			}
		}
		if _, ok := raw[0]["error"]; ok {
			t.Fatalf("did not expect empty error field on queued apply payload: %#v", raw[0])
		}
	})

	t.Run("active payload includes queued and running records newest first with claim freshness fields", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			ApplyRequests: func(_ context.Context, query applyrequest.ListQuery) ([]applyrequest.Record, error) {
				if query.Status != applyrequest.Status("active") {
					t.Fatalf("expected active list query, got %+v", query)
				}
				return records, nil
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/applies?status=active", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var got []applyrequest.Record
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode active applies payload: %v", err)
		}
		if len(got) != 3 || got[0].ID != "apply_running_api" || got[1].ID != "apply_queued_worker" || got[2].ID != "apply_queued_api" {
			t.Fatalf("unexpected active applies payload: %+v", got)
		}
		for _, record := range got {
			if record.Status != applyrequest.StatusQueued && record.Status != applyrequest.StatusRunning {
				t.Fatalf("expected only queued/running active records, got %+v", got)
			}
		}
		if got[0].Claim == nil || got[0].Claim.LastHeartbeatAt != "2026-04-18T08:07:30Z" || got[0].Claim.ExpiresAt != "2026-04-18T08:12:30Z" {
			t.Fatalf("expected active running record to preserve claim freshness fields, got %+v", got[0].Claim)
		}

		var raw []map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw active applies payload: %v", err)
		}
		for _, key := range []string{"id", "planId", "deploymentName", "status", "createdAt", "details", "lifecycle", "claim", "operations", "source"} {
			if _, ok := raw[0][key]; !ok {
				t.Fatalf("expected active apply payload to include %q, got %#v", key, raw[0])
			}
		}
		claim, ok := raw[0]["claim"].(map[string]any)
		if !ok || claim["workerId"] != "worker-a" || claim["claimedAt"] != "2026-04-18T08:07:00Z" || claim["lastHeartbeatAt"] != "2026-04-18T08:07:30Z" || claim["expiresAt"] != "2026-04-18T08:12:30Z" {
			t.Fatalf("expected raw active apply claim freshness metadata, got %#v", raw[0]["claim"])
		}
	})

	t.Run("stale active claim payload passes active status and stale claim filters", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			ApplyRequests: func(_ context.Context, query applyrequest.ListQuery) ([]applyrequest.Record, error) {
				if query.Status != applyrequest.ListStatusActive || query.Claim != applyrequest.ListClaimStale {
					t.Fatalf("expected active stale-claim list query, got %+v", query)
				}
				return []applyrequest.Record{records[2]}, nil
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/applies?status=active&claim=stale", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var got []applyrequest.Record
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode stale active applies payload: %v", err)
		}
		if len(got) != 1 || got[0].ID != "apply_running_api" || got[0].Claim == nil {
			t.Fatalf("unexpected stale active applies payload: %+v", got)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
		req := httptest.NewRequest(http.MethodGet, "/v1/applies?status=queued", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
	})

	t.Run("provider error", func(t *testing.T) {
		const upstreamErr = "boom: apply list"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			ApplyRequests: func(context.Context, applyrequest.ListQuery) ([]applyrequest.Record, error) {
				return nil, errors.New(upstreamErr)
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/applies?status=queued", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, upstreamErr)
	})

	for _, path := range []string{"/v1/applies", "/v1/applies?status=failed", "/v1/applies?status=running", "/v1/applies?status=active&deployment=api-prod", "/v1/applies?status=queued&workerId=worker-a", "/v1/applies?status=queued&claim=stale", "/v1/applies?status=active&claim=fresh"} {
		t.Run("unsupported query "+path, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				ApplyRequests: func(context.Context, applyrequest.ListQuery) ([]applyrequest.Record, error) {
					t.Fatalf("apply requests provider should not run for unsupported query")
					return nil, nil
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

	t.Run("deployment authorization filters queued records to visible deployments", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			ApplyRequests: func(context.Context, applyrequest.ListQuery) ([]applyrequest.Record, error) {
				return records, nil
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/applies?status=queued", nil)
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "api-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var got []applyrequest.Record
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode queued applies payload: %v", err)
		}
		if len(got) != 1 || got[0].ID != "apply_queued_api" {
			t.Fatalf("expected only authorized queued apply record, got %+v", got)
		}
	})

	t.Run("deployment authorization requires identity before provider", func(t *testing.T) {
		providerCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			ApplyRequests: func(context.Context, applyrequest.ListQuery) ([]applyrequest.Record, error) {
				providerCalls++
				return records, nil
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/applies?status=queued", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.Code)
		}
		if providerCalls != 0 {
			t.Fatalf("expected unauthenticated request to short-circuit before provider, got %d calls", providerCalls)
		}
	})
}

func TestRouterClaimApplyRequest(t *testing.T) {
	t.Run("claim payload", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			ClaimApplyRequest: func(ctx context.Context, applyID string, req applyrequest.ClaimRequest) (applyrequest.Record, error) {
				if applyID != "apply_1234abcd" {
					t.Fatalf("unexpected apply claim for %s", applyID)
				}
				if req.WorkerID != "worker-a" {
					t.Fatalf("unexpected claim request: %+v", req)
				}
				return applyrequest.Record{
					ID:             applyID,
					PlanID:         "plan_1234abcd",
					DeploymentName: "api-prod",
					Status:         applyrequest.StatusRunning,
					CreatedAt:      "2026-04-18T08:05:00Z",
					Details:        []string{"Executor worker-a claimed this queued apply request for local-safe handoff; no live AWS mutation has been attempted by the claim route."},
					Lifecycle:      applyrequest.Lifecycle{UpdatedAt: "2026-04-18T08:10:00Z", StartedAt: "2026-04-18T08:10:00Z", CurrentStep: 1, TotalSteps: 2},
					Claim:          &applyrequest.Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z", LastHeartbeatAt: "2026-04-18T08:10:00Z", ExpiresAt: "2026-04-18T08:15:00Z"},
					Transitions: []applyrequest.Transition{{
						TransitionedAt: "2026-04-18T08:10:00Z",
						PreviousStatus: applyrequest.StatusQueued,
						Status:         applyrequest.StatusRunning,
						Step:           1,
						Details:        []string{"Executor worker-a claimed this queued apply request for local-safe handoff; no live AWS mutation has been attempted by the claim route."},
						Lifecycle:      applyrequest.Lifecycle{UpdatedAt: "2026-04-18T08:10:00Z", StartedAt: "2026-04-18T08:10:00Z", CurrentStep: 1, TotalSteps: 2},
					}},
					Operations: []applyrequest.Operation{
						{Status: applyrequest.OperationStatusRunning, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
						{Status: applyrequest.OperationStatusQueued, Phase: "Finalizing", Target: "api-prod alarms", Summary: "Verify final health."},
					},
					Source: applyrequest.SourceContext{PlanID: "plan_1234abcd", Route: "/v1/plans/plan_1234abcd", PlanStatus: "ready", Fingerprint: "plan_input"},
				}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/claim", strings.NewReader(`{"workerId":"worker-a"}`))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var record applyrequest.Record
		if err := json.Unmarshal(resp.Body.Bytes(), &record); err != nil {
			t.Fatalf("failed to decode apply claim payload: %v", err)
		}
		if record.ID != "apply_1234abcd" || record.Status != applyrequest.StatusRunning || record.Claim == nil || record.Claim.WorkerID != "worker-a" {
			t.Fatalf("unexpected apply claim payload: %+v", record)
		}
		if record.Lifecycle.CurrentStep != 1 || len(record.Operations) != 2 || record.Operations[0].Status != applyrequest.OperationStatusRunning {
			t.Fatalf("expected running lifecycle/operations update, got %+v", record)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw apply claim payload: %v", err)
		}
		for _, key := range []string{"id", "planId", "deploymentName", "status", "createdAt", "details", "lifecycle", "claim", "transitions", "operations", "source"} {
			if _, ok := raw[key]; !ok {
				t.Fatalf("expected apply claim payload to include %q, got %#v", key, raw)
			}
		}
		claim, ok := raw["claim"].(map[string]any)
		if !ok || claim["workerId"] != "worker-a" || claim["claimedAt"] != "2026-04-18T08:10:00Z" || claim["lastHeartbeatAt"] != "2026-04-18T08:10:00Z" || claim["expiresAt"] != "2026-04-18T08:15:00Z" {
			t.Fatalf("expected raw claim freshness metadata, got %#v", raw["claim"])
		}
	})

	t.Run("claim forwards explicit expired claim recovery opt in", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			ClaimApplyRequest: func(ctx context.Context, applyID string, req applyrequest.ClaimRequest) (applyrequest.Record, error) {
				if applyID != "apply_1234abcd" {
					t.Fatalf("unexpected apply claim for %s", applyID)
				}
				if req.WorkerID != "worker-b" || !req.AllowExpiredClaim {
					t.Fatalf("expected explicit expired-claim recovery request, got %+v", req)
				}
				return applyrequest.Record{
					ID:             applyID,
					PlanID:         "plan_1234abcd",
					DeploymentName: "api-prod",
					Status:         applyrequest.StatusRunning,
					CreatedAt:      "2026-04-18T08:05:00Z",
					Details:        []string{"Executor worker-b recovered expired local-safe claim from worker worker-a for local-safe handoff; no live AWS mutation has been attempted by the claim route."},
					Claim:          &applyrequest.Claim{WorkerID: "worker-b", ClaimedAt: "2026-04-18T08:16:00Z", LastHeartbeatAt: "2026-04-18T08:16:00Z", ExpiresAt: "2026-04-18T08:21:00Z"},
					Source:         applyrequest.SourceContext{PlanID: "plan_1234abcd", Route: "/v1/plans/plan_1234abcd", PlanStatus: "ready", Fingerprint: "plan_input"},
				}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/claim", strings.NewReader(`{"workerId":"worker-b","allowExpiredClaim":true}`))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var record applyrequest.Record
		if err := json.Unmarshal(resp.Body.Bytes(), &record); err != nil {
			t.Fatalf("failed to decode apply claim payload: %v", err)
		}
		if record.Claim == nil || record.Claim.WorkerID != "worker-b" {
			t.Fatalf("unexpected recovered apply claim payload: %+v", record)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/claim", strings.NewReader(`{"workerId":"worker-a"}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
	})

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "empty body", body: ""},
		{name: "unknown field", body: `{"workerId":"worker-a","unexpected":true}`},
		{name: "trailing payload", body: `{"workerId":"worker-a"}{"workerId":"worker-b"}`},
	} {
		t.Run("invalid body "+tc.name, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				ClaimApplyRequest: func(context.Context, string, applyrequest.ClaimRequest) (applyrequest.Record, error) {
					t.Fatalf("apply claim provider should not run on invalid body")
					return applyrequest.Record{}, nil
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/claim", strings.NewReader(tc.body))
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", tc.name, resp.Code)
			}
			if body := resp.Body.String(); body != "invalid apply claim request body\n" {
				t.Fatalf("expected invalid body response, got %q", body)
			}
		})
	}

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "missing workerId", body: `{}`},
		{name: "blank workerId", body: `{"workerId":"   "}`},
	} {
		t.Run("invalid claim request "+tc.name, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				ClaimApplyRequest: func(context.Context, string, applyrequest.ClaimRequest) (applyrequest.Record, error) {
					return applyrequest.Record{}, fmt.Errorf("%w: workerId is required", applyrequest.ErrInvalidClaimRequest)
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/claim", strings.NewReader(tc.body))
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d with body %q", tc.name, resp.Code, resp.Body.String())
			}
			if body := resp.Body.String(); !strings.Contains(body, "workerId is required") {
				t.Fatalf("expected workerId validation response, got %q", body)
			}
		})
	}

	t.Run("not found", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			ClaimApplyRequest: func(context.Context, string, applyrequest.ClaimRequest) (applyrequest.Record, error) {
				return applyrequest.Record{}, applyrequest.ErrNotFound
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_missing/claim", strings.NewReader(`{"workerId":"worker-a"}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})

	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "invalid claim state", err: fmt.Errorf("%w: only queued apply requests can be claimed", applyrequest.ErrInvalidState)},
		{name: "already claimed", err: fmt.Errorf("%w: apply_1234abcd has active claim held by worker worker-b; last heartbeat 2026-04-18T08:10:00Z; expires at 2026-04-18T08:15:00Z; retry with the same workerId for idempotent recovery or wait for the lease to become stale", applyrequest.ErrAlreadyClaimed)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				ClaimApplyRequest: func(context.Context, string, applyrequest.ClaimRequest) (applyrequest.Record, error) {
					return applyrequest.Record{}, tc.err
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/claim", strings.NewReader(`{"workerId":"worker-a"}`))
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusConflict {
				t.Fatalf("expected 409, got %d", resp.Code)
			}
		})
	}

	t.Run("deployment authorization allows authorized claim", func(t *testing.T) {
		applyLookupCalls := 0
		claimCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
				applyLookupCalls++
				return applyrequest.Record{ID: "apply_1234abcd", DeploymentName: "api-prod", Status: applyrequest.StatusQueued, CreatedAt: "2026-04-18T08:00:00Z"}, nil
			},
			ClaimApplyRequest: func(_ context.Context, applyID string, req applyrequest.ClaimRequest) (applyrequest.Record, error) {
				claimCalls++
				if applyID != "apply_1234abcd" || req.WorkerID != "worker-a" {
					t.Fatalf("unexpected claim request %q %+v", applyID, req)
				}
				return applyrequest.Record{ID: applyID, DeploymentName: "api-prod", Status: applyrequest.StatusRunning, CreatedAt: "2026-04-18T08:00:00Z", Claim: &applyrequest.Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z"}}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/claim", strings.NewReader(`{"workerId":"worker-a"}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "api-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if applyLookupCalls != 1 || claimCalls != 1 {
			t.Fatalf("expected one apply lookup and one claim, got %d/%d", applyLookupCalls, claimCalls)
		}
	})
}

func TestRouterHeartbeatApplyRequest(t *testing.T) {
	t.Run("heartbeat payload", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			HeartbeatApplyRequest: func(ctx context.Context, applyID string, req applyrequest.HeartbeatRequest) (applyrequest.Record, error) {
				if applyID != "apply_1234abcd" {
					t.Fatalf("unexpected apply heartbeat for %s", applyID)
				}
				if req.WorkerID != "worker-a" {
					t.Fatalf("unexpected heartbeat request: %+v", req)
				}
				return applyrequest.Record{
					ID:             applyID,
					PlanID:         "plan_1234abcd",
					DeploymentName: "api-prod",
					Status:         applyrequest.StatusRunning,
					CreatedAt:      "2026-04-18T08:05:00Z",
					Details:        []string{"Executor worker-a claimed this queued apply request for local-safe handoff; no live AWS mutation has been attempted by the claim route."},
					Lifecycle:      applyrequest.Lifecycle{UpdatedAt: "2026-04-18T08:10:00Z", StartedAt: "2026-04-18T08:10:00Z", CurrentStep: 1, TotalSteps: 2},
					Claim:          &applyrequest.Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z", LastHeartbeatAt: "2026-04-18T08:12:30Z", ExpiresAt: "2026-04-18T08:17:30Z"},
					Transitions: []applyrequest.Transition{{
						TransitionedAt: "2026-04-18T08:10:00Z",
						PreviousStatus: applyrequest.StatusQueued,
						Status:         applyrequest.StatusRunning,
						Step:           1,
					}},
					Operations: []applyrequest.Operation{
						{Status: applyrequest.OperationStatusRunning, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
						{Status: applyrequest.OperationStatusQueued, Phase: "Finalizing", Target: "api-prod alarms", Summary: "Verify final health."},
					},
					Source: applyrequest.SourceContext{PlanID: "plan_1234abcd", Route: "/v1/plans/plan_1234abcd", PlanStatus: "ready", Fingerprint: "plan_input"},
				}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/heartbeat", strings.NewReader(`{"workerId":"worker-a"}`))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var record applyrequest.Record
		if err := json.Unmarshal(resp.Body.Bytes(), &record); err != nil {
			t.Fatalf("failed to decode apply heartbeat payload: %v", err)
		}
		if record.ID != "apply_1234abcd" || record.Status != applyrequest.StatusRunning || record.Claim == nil || record.Claim.WorkerID != "worker-a" {
			t.Fatalf("unexpected apply heartbeat payload: %+v", record)
		}
		if record.Claim.LastHeartbeatAt != "2026-04-18T08:12:30Z" || record.Claim.ExpiresAt != "2026-04-18T08:17:30Z" {
			t.Fatalf("expected refreshed heartbeat fields, got %+v", record.Claim)
		}
		if record.Lifecycle.UpdatedAt != "2026-04-18T08:10:00Z" || record.Lifecycle.CurrentStep != 1 || record.Operations[0].Status != applyrequest.OperationStatusRunning || len(record.Transitions) != 1 {
			t.Fatalf("heartbeat must preserve lifecycle, operations, and transitions, got %+v", record)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/heartbeat", strings.NewReader(`{"workerId":"worker-a"}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
	})

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "empty body", body: ""},
		{name: "unknown field", body: `{"workerId":"worker-a","unexpected":true}`},
		{name: "trailing payload", body: `{"workerId":"worker-a"}{"workerId":"worker-b"}`},
	} {
		t.Run("invalid body "+tc.name, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				HeartbeatApplyRequest: func(context.Context, string, applyrequest.HeartbeatRequest) (applyrequest.Record, error) {
					t.Fatalf("apply heartbeat provider should not run on invalid body")
					return applyrequest.Record{}, nil
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/heartbeat", strings.NewReader(tc.body))
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", tc.name, resp.Code)
			}
			if body := resp.Body.String(); body != "invalid apply heartbeat request body\n" {
				t.Fatalf("expected invalid body response, got %q", body)
			}
		})
	}

	t.Run("invalid heartbeat request", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			HeartbeatApplyRequest: func(context.Context, string, applyrequest.HeartbeatRequest) (applyrequest.Record, error) {
				return applyrequest.Record{}, fmt.Errorf("%w: workerId is required", applyrequest.ErrInvalidClaimRequest)
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/heartbeat", strings.NewReader(`{"workerId":"   "}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d with body %q", resp.Code, resp.Body.String())
		}
		if body := resp.Body.String(); !strings.Contains(body, "workerId is required") {
			t.Fatalf("expected workerId validation response, got %q", body)
		}
	})

	t.Run("not found", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			HeartbeatApplyRequest: func(context.Context, string, applyrequest.HeartbeatRequest) (applyrequest.Record, error) {
				return applyrequest.Record{}, applyrequest.ErrNotFound
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_missing/heartbeat", strings.NewReader(`{"workerId":"worker-a"}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})

	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "wrong worker", err: fmt.Errorf("%w: apply_1234abcd claim is held by worker worker-b; worker worker-a cannot heartbeat it", applyrequest.ErrAlreadyClaimed)},
		{name: "unclaimed", err: fmt.Errorf("%w: apply_1234abcd is not currently claimed", applyrequest.ErrInvalidState)},
		{name: "terminal", err: fmt.Errorf("%w: apply_1234abcd is already terminal with status failed", applyrequest.ErrInvalidState)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				HeartbeatApplyRequest: func(context.Context, string, applyrequest.HeartbeatRequest) (applyrequest.Record, error) {
					return applyrequest.Record{}, tc.err
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/heartbeat", strings.NewReader(`{"workerId":"worker-a"}`))
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusConflict {
				t.Fatalf("expected 409, got %d", resp.Code)
			}
		})
	}

	t.Run("deployment authorization allows authorized heartbeat", func(t *testing.T) {
		applyLookupCalls := 0
		heartbeatCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
				applyLookupCalls++
				return applyrequest.Record{ID: "apply_1234abcd", DeploymentName: "api-prod", Status: applyrequest.StatusRunning, CreatedAt: "2026-04-18T08:00:00Z"}, nil
			},
			HeartbeatApplyRequest: func(_ context.Context, applyID string, req applyrequest.HeartbeatRequest) (applyrequest.Record, error) {
				heartbeatCalls++
				if applyID != "apply_1234abcd" || req.WorkerID != "worker-a" {
					t.Fatalf("unexpected heartbeat request %q %+v", applyID, req)
				}
				return applyrequest.Record{ID: applyID, DeploymentName: "api-prod", Status: applyrequest.StatusRunning, CreatedAt: "2026-04-18T08:00:00Z", Claim: &applyrequest.Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z", LastHeartbeatAt: "2026-04-18T08:12:30Z"}}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/heartbeat", strings.NewReader(`{"workerId":"worker-a"}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "api-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if applyLookupCalls != 1 || heartbeatCalls != 1 {
			t.Fatalf("expected one apply lookup and one heartbeat, got %d/%d", applyLookupCalls, heartbeatCalls)
		}
	})
}

func TestRouterTransitionApplyRequest(t *testing.T) {
	t.Run("claimed apply can transition while preserving claim metadata", func(t *testing.T) {
		store := applyrequest.NewFileStore(t.TempDir() + "/apply-requests.json")
		createdAt := "2026-04-18T08:05:00Z"
		queued := applyrequest.Record{
			ID:             "apply_1234abcd",
			PlanID:         "plan_1234abcd",
			DeploymentName: "api-prod",
			Status:         applyrequest.StatusQueued,
			CreatedAt:      createdAt,
			Details: []string{
				"Tenet persisted this immutable apply request as queued because TENET_AWS_ENABLE_APPLY=true enabled the local-safe executor handoff path.",
				"Tenet will advance this same applyId locally, but live AWS mutation is still not implemented for this backend slice.",
			},
			Lifecycle: applyrequest.Lifecycle{UpdatedAt: createdAt, TotalSteps: 2},
			Operations: []applyrequest.Operation{
				{Status: applyrequest.OperationStatusQueued, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
				{Status: applyrequest.OperationStatusQueued, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
			},
			Source: applyrequest.SourceContext{PlanID: "plan_1234abcd", Route: "/v1/plans/plan_1234abcd", PlanStatus: "ready", Fingerprint: "plan_input"},
		}
		if err := applyrequest.Save(store.Path, []applyrequest.Record{queued}); err != nil {
			t.Fatalf("seed apply request store: %v", err)
		}
		now := time.Date(2026, 4, 18, 8, 10, 0, 0, time.UTC)
		service := applyrequest.Service{
			Store: store,
			Now: func() time.Time {
				return now
			},
		}
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			ClaimApplyRequest: func(ctx context.Context, applyID string, req applyrequest.ClaimRequest) (applyrequest.Record, error) {
				return service.Claim(ctx, applyID, req)
			},
			TransitionApplyRequest: func(ctx context.Context, applyID string, req applyrequest.TransitionRequest) (applyrequest.Record, error) {
				return service.Transition(ctx, applyID, req)
			},
		})

		claimReq := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/claim", strings.NewReader(`{"workerId":"worker-a"}`))
		claimReq.Header.Set("Content-Type", "application/json")
		claimResp := httptest.NewRecorder()
		router.ServeHTTP(claimResp, claimReq)
		if claimResp.Code != http.StatusOK {
			t.Fatalf("expected claim 200, got %d with body %q", claimResp.Code, claimResp.Body.String())
		}
		var claimed applyrequest.Record
		if err := json.Unmarshal(claimResp.Body.Bytes(), &claimed); err != nil {
			t.Fatalf("decode claimed apply response: %v", err)
		}
		if claimed.Status != applyrequest.StatusRunning || claimed.Claim == nil || claimed.Claim.WorkerID != "worker-a" || claimed.Claim.ClaimedAt != "2026-04-18T08:10:00Z" || claimed.Claim.LastHeartbeatAt != "2026-04-18T08:10:00Z" || claimed.Claim.ExpiresAt != "2026-04-18T08:15:00Z" {
			t.Fatalf("expected running claim freshness metadata, got %+v", claimed)
		}

		now = time.Date(2026, 4, 18, 8, 12, 0, 0, time.UTC)
		transitionReq := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/transitions", strings.NewReader(`{"status":"failed","step":2,"error":"manual claimed local-safe transition failed","details":["manual claimed local-safe transition advanced through POST /v1/applies/{applyId}/transitions"]}`))
		transitionReq.Header.Set("Content-Type", "application/json")
		transitionResp := httptest.NewRecorder()
		router.ServeHTTP(transitionResp, transitionReq)
		if transitionResp.Code != http.StatusOK {
			t.Fatalf("expected transition 200, got %d with body %q", transitionResp.Code, transitionResp.Body.String())
		}
		var transitioned applyrequest.Record
		if err := json.Unmarshal(transitionResp.Body.Bytes(), &transitioned); err != nil {
			t.Fatalf("decode transitioned apply response: %v", err)
		}
		if transitioned.ID != queued.ID || transitioned.Status != applyrequest.StatusFailed || transitioned.Error != "manual claimed local-safe transition failed" {
			t.Fatalf("expected failed transition on stable apply id, got %+v", transitioned)
		}
		if transitioned.Claim == nil || *transitioned.Claim != *claimed.Claim {
			t.Fatalf("expected transition to preserve claim metadata, before=%+v after=%+v", claimed.Claim, transitioned.Claim)
		}
		if transitioned.Lifecycle.StartedAt != "2026-04-18T08:10:00Z" || transitioned.Lifecycle.UpdatedAt != "2026-04-18T08:12:00Z" || transitioned.Lifecycle.FinishedAt != "2026-04-18T08:12:00Z" || transitioned.Lifecycle.CurrentStep != 2 || transitioned.Lifecycle.CompletedSteps != 1 || transitioned.Lifecycle.TotalSteps != 2 {
			t.Fatalf("expected transition to preserve and advance lifecycle metadata, got %+v", transitioned.Lifecycle)
		}
		if statuses := []applyrequest.OperationStatus{transitioned.Operations[0].Status, transitioned.Operations[1].Status}; !reflect.DeepEqual(statuses, []applyrequest.OperationStatus{applyrequest.OperationStatusSucceeded, applyrequest.OperationStatusFailed}) {
			t.Fatalf("expected transitioned operation statuses, got %+v", transitioned.Operations)
		}
		if len(transitioned.Transitions) != 2 || transitioned.Transitions[0].Status != applyrequest.StatusRunning || transitioned.Transitions[1].Status != applyrequest.StatusFailed {
			t.Fatalf("expected claim and transition history, got %+v", transitioned.Transitions)
		}

		persisted, err := service.Get(context.Background(), queued.ID)
		if err != nil {
			t.Fatalf("read persisted transitioned apply: %v", err)
		}
		if persisted.Claim == nil || *persisted.Claim != *transitioned.Claim || persisted.Lifecycle != transitioned.Lifecycle {
			t.Fatalf("expected persisted readback to keep claim/lifecycle metadata, got %+v", persisted)
		}
	})

	t.Run("transition payload", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			TransitionApplyRequest: func(ctx context.Context, applyID string, req applyrequest.TransitionRequest) (applyrequest.Record, error) {
				if applyID != "apply_1234abcd" {
					t.Fatalf("unexpected apply transition for %s", applyID)
				}
				expected := applyrequest.TransitionRequest{
					Status:  applyrequest.StatusFailed,
					Step:    2,
					Error:   "promotion failed",
					Details: []string{"executor reported a promotion failure"},
				}
				if !reflect.DeepEqual(req, expected) {
					t.Fatalf("unexpected transition request: %+v", req)
				}
				return applyrequest.Record{
					ID:             applyID,
					PlanID:         "plan_1234abcd",
					DeploymentName: "api-prod",
					Status:         applyrequest.StatusFailed,
					CreatedAt:      "2026-04-18T08:05:00Z",
					Error:          "promotion failed",
					Details:        []string{"executor reported a promotion failure"},
					Lifecycle: applyrequest.Lifecycle{
						UpdatedAt:      "2026-04-18T08:08:00Z",
						StartedAt:      "2026-04-18T08:06:00Z",
						FinishedAt:     "2026-04-18T08:08:00Z",
						CurrentStep:    2,
						CompletedSteps: 1,
						TotalSteps:     3,
					},
					Operations: []applyrequest.Operation{
						{Status: applyrequest.OperationStatusSucceeded, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
						{Status: applyrequest.OperationStatusFailed, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
						{Status: applyrequest.OperationStatusQueued, Phase: "Finalizing", Target: "api-prod alarms", Summary: "Verify final health."},
					},
					Source: applyrequest.SourceContext{PlanID: "plan_1234abcd", Route: "/v1/plans/plan_1234abcd", PlanStatus: "ready", Fingerprint: "plan_input"},
				}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/transitions", strings.NewReader(`{"status":"failed","step":2,"error":"promotion failed","details":["executor reported a promotion failure"]}`))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		var record applyrequest.Record
		if err := json.Unmarshal(resp.Body.Bytes(), &record); err != nil {
			t.Fatalf("failed to decode apply transition payload: %v", err)
		}
		if record.ID != "apply_1234abcd" || record.Status != applyrequest.StatusFailed || record.Error != "promotion failed" {
			t.Fatalf("unexpected apply transition payload: %+v", record)
		}
		if record.Lifecycle.CurrentStep != 2 || record.Lifecycle.CompletedSteps != 1 || len(record.Operations) != 3 {
			t.Fatalf("expected lifecycle/operations update, got %+v", record)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw apply transition payload: %v", err)
		}
		for _, key := range []string{"id", "planId", "deploymentName", "status", "createdAt", "error", "details", "lifecycle", "operations", "source"} {
			if _, ok := raw[key]; !ok {
				t.Fatalf("expected apply transition payload to include %q, got %#v", key, raw)
			}
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/transitions", strings.NewReader(`{"status":"running","step":1}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
	})

	t.Run("unauthenticated when operator auth is enforced", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo:                    buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireAuthenticatedIdentity: true,
			TransitionApplyRequest: func(context.Context, string, applyrequest.TransitionRequest) (applyrequest.Record, error) {
				t.Fatal("apply transition provider should not run without authenticated identity")
				return applyrequest.Record{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/transitions", strings.NewReader(`{"status":"running","step":1}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator write route\n" {
			t.Fatalf("unexpected unauthenticated response body: %q", body)
		}
	})

	t.Run("insufficient trusted role short-circuits transition before apply lookup", func(t *testing.T) {
		applyLookupCalls := 0
		transitionCalls := 0
		router := NewRouter(Options{
			BuildInfo:                    buildinfo.Info{Version: "dev", Commit: "abc123"},
			TrustAuthenticatedHeaders:    true,
			RequireAuthenticatedIdentity: true,
			RequireOperatorRoleForWrite:  true,
			ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
				applyLookupCalls++
				t.Fatal("apply lookup should not run without operator-capable role")
				return applyrequest.Record{}, nil
			},
			TransitionApplyRequest: func(context.Context, string, applyrequest.TransitionRequest) (applyrequest.Record, error) {
				transitionCalls++
				t.Fatal("apply transition provider should not run without operator-capable role")
				return applyrequest.Record{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/transitions", strings.NewReader(`{"status":"running","step":1}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Viewer")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "viewer@example.com")
		req.Header.Set(requestactor.AuthenticatedRolesHeader, "viewer")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "forbidden: authenticated operator role is required for this operator write route\n" {
			t.Fatalf("unexpected forbidden response body: %q", body)
		}
		if applyLookupCalls != 0 {
			t.Fatalf("expected role guard to short-circuit before apply lookup, got %d calls", applyLookupCalls)
		}
		if transitionCalls != 0 {
			t.Fatalf("expected role guard to short-circuit before transition provider, got %d calls", transitionCalls)
		}
	})

	t.Run("deployment authorization short-circuits unauthenticated transition before apply lookup", func(t *testing.T) {
		applyLookupCalls := 0
		transitionCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
				applyLookupCalls++
				t.Fatal("apply lookup should not run without authenticated identity")
				return applyrequest.Record{}, nil
			},
			TransitionApplyRequest: func(context.Context, string, applyrequest.TransitionRequest) (applyrequest.Record, error) {
				transitionCalls++
				t.Fatal("apply transition provider should not run without authenticated identity")
				return applyrequest.Record{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/transitions", strings.NewReader(`{"status":"running","step":1}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "unauthenticated: authenticated identity is required for this operator write route\n" {
			t.Fatalf("unexpected unauthenticated response body: %q", body)
		}
		if applyLookupCalls != 0 {
			t.Fatalf("expected auth guard to short-circuit before apply lookup, got %d calls", applyLookupCalls)
		}
		if transitionCalls != 0 {
			t.Fatalf("expected auth guard to short-circuit before transition provider, got %d calls", transitionCalls)
		}
	})

	t.Run("deployment authorization short-circuits unauthorized transition after apply lookup", func(t *testing.T) {
		applyLookupCalls := 0
		transitionCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
				applyLookupCalls++
				return applyrequest.Record{ID: "apply_1234abcd", DeploymentName: "api-prod", Status: applyrequest.StatusFailed, CreatedAt: "2026-04-18T08:00:00Z"}, nil
			},
			TransitionApplyRequest: func(context.Context, string, applyrequest.TransitionRequest) (applyrequest.Record, error) {
				transitionCalls++
				t.Fatal("apply transition provider should not run without deployment access")
				return applyrequest.Record{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/transitions", strings.NewReader(`{"status":"running","step":1}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "forbidden: operator does not have access to this deployment\n" {
			t.Fatalf("unexpected forbidden response body: %q", body)
		}
		if applyLookupCalls != 1 {
			t.Fatalf("expected one apply lookup before authz denial, got %d calls", applyLookupCalls)
		}
		if transitionCalls != 0 {
			t.Fatalf("expected authz guard to short-circuit before transition provider, got %d calls", transitionCalls)
		}
	})

	t.Run("project environment authorization allows scoped transition", func(t *testing.T) {
		applyLookupCalls := 0
		transitionCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			CatalogPlacements: func(context.Context) ([]catalog.PlacementView, error) {
				return []catalog.PlacementView{{DeploymentName: "api-prod", AppRef: "payments", EnvironmentRef: "prod"}}, nil
			},
			ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
				applyLookupCalls++
				return applyrequest.Record{ID: "apply_1234abcd", DeploymentName: "api-prod", Status: applyrequest.StatusQueued, CreatedAt: "2026-04-18T08:00:00Z"}, nil
			},
			TransitionApplyRequest: func(_ context.Context, applyID string, req applyrequest.TransitionRequest) (applyrequest.Record, error) {
				transitionCalls++
				if applyID != "apply_1234abcd" || req.Status != applyrequest.StatusRunning || req.Step != 1 {
					t.Fatalf("unexpected transition request %q %+v", applyID, req)
				}
				return applyrequest.Record{ID: applyID, DeploymentName: "api-prod", Status: applyrequest.StatusRunning, CreatedAt: "2026-04-18T08:00:00Z"}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/transitions", strings.NewReader(`{"status":"running","step":1}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthorizedProjectsHeader, "payments")
		req.Header.Set(requestactor.AuthorizedEnvironmentsHeader, "prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if applyLookupCalls != 1 || transitionCalls != 1 {
			t.Fatalf("expected one apply lookup and one transition, got %d/%d", applyLookupCalls, transitionCalls)
		}
	})

	t.Run("deployment authorization preserves not found after auth passes", func(t *testing.T) {
		applyLookupCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
				applyLookupCalls++
				return applyrequest.Record{}, applyrequest.ErrNotFound
			},
			TransitionApplyRequest: func(context.Context, string, applyrequest.TransitionRequest) (applyrequest.Record, error) {
				t.Fatal("apply transition provider should not run when apply lookup fails")
				return applyrequest.Record{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_missing/transitions", strings.NewReader(`{"status":"running","step":1}`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "api-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
		if applyLookupCalls != 1 {
			t.Fatalf("expected one apply lookup before not found, got %d calls", applyLookupCalls)
		}
	})

	t.Run("deployment authorization allows authorized transition and preserves invalid body", func(t *testing.T) {
		applyLookupCalls := 0
		router := NewRouter(Options{
			BuildInfo:                          buildinfo.Info{Version: "dev", Commit: "abc123"},
			RequireDeploymentReadAuthorization: true,
			ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
				applyLookupCalls++
				return applyrequest.Record{ID: "apply_1234abcd", DeploymentName: "api-prod", Status: applyrequest.StatusFailed, CreatedAt: "2026-04-18T08:00:00Z"}, nil
			},
			TransitionApplyRequest: func(context.Context, string, applyrequest.TransitionRequest) (applyrequest.Record, error) {
				t.Fatal("apply transition provider should not run on invalid body")
				return applyrequest.Record{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/transitions", strings.NewReader(`{"status":"running","step":1`))
		req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
		req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
		req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
		req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "api-prod, worker-prod")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != "invalid apply transition request body\n" {
			t.Fatalf("expected invalid body response, got %q", body)
		}
		if applyLookupCalls != 1 {
			t.Fatalf("expected one apply lookup before body validation, got %d calls", applyLookupCalls)
		}
	})

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "empty body", body: ""},
		{name: "unknown field", body: `{"status":"running","step":1,"unexpected":true}`},
		{name: "trailing payload", body: `{"status":"running","step":1}{"status":"failed"}`},
	} {
		t.Run("invalid body "+tc.name, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				TransitionApplyRequest: func(context.Context, string, applyrequest.TransitionRequest) (applyrequest.Record, error) {
					t.Fatalf("apply transition provider should not run on invalid body")
					return applyrequest.Record{}, nil
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/transitions", strings.NewReader(tc.body))
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", tc.name, resp.Code)
			}
			if body := resp.Body.String(); body != "invalid apply transition request body\n" {
				t.Fatalf("expected invalid body response, got %q", body)
			}
		})
	}

	t.Run("not found", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			TransitionApplyRequest: func(context.Context, string, applyrequest.TransitionRequest) (applyrequest.Record, error) {
				return applyrequest.Record{}, applyrequest.ErrNotFound
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_missing/transitions", strings.NewReader(`{"status":"running","step":1}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})

	t.Run("invalid transition", func(t *testing.T) {
		const conflictBody = "apply request lifecycle transition is invalid: cannot transition from succeeded to running\n"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			TransitionApplyRequest: func(context.Context, string, applyrequest.TransitionRequest) (applyrequest.Record, error) {
				return applyrequest.Record{}, fmt.Errorf("%w: cannot transition from succeeded to running", applyrequest.ErrInvalidState)
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/transitions", strings.NewReader(`{"status":"running","step":1}`))
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
		const upstreamErr = "boom: apply transition store"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			TransitionApplyRequest: func(context.Context, string, applyrequest.TransitionRequest) (applyrequest.Record, error) {
				return applyrequest.Record{}, errors.New(upstreamErr)
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/applies/apply_1234abcd/transitions", strings.NewReader(`{"status":"running","step":1}`))
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, upstreamErr)
	})

	for _, path := range []string{"/v1/applies/a/b/transitions", "/v1/applies/a/transitions/b"} {
		t.Run("invalid path "+path, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				TransitionApplyRequest: func(context.Context, string, applyrequest.TransitionRequest) (applyrequest.Record, error) {
					t.Fatalf("apply transition provider should not run on invalid path")
					return applyrequest.Record{}, nil
				},
			})
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"status":"running","step":1}`))
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
		CatalogSummary: func(_ context.Context) (catalog.Summary, error) {
			return catalog.Summary{}, nil
		},
		CatalogDeployments: func(_ context.Context) ([]catalog.DeploymentView, error) {
			return []catalog.DeploymentView{}, nil
		},
		CatalogEnvironments: func(_ context.Context) ([]catalog.EnvironmentView, error) {
			return []catalog.EnvironmentView{}, nil
		},
		CatalogAccounts: func(_ context.Context) ([]catalog.AccountView, error) {
			return []catalog.AccountView{}, nil
		},
		CatalogPlacements: func(_ context.Context) ([]catalog.PlacementView, error) {
			return []catalog.PlacementView{}, nil
		},
		CatalogTopology: func(_ context.Context) (catalog.TopologySnapshot, error) {
			return catalog.TopologySnapshot{}, nil
		},
		OverviewAccounts: func(_ context.Context) ([]catalog.AccountOverview, error) {
			return []catalog.AccountOverview{}, nil
		},
		OverviewAccount: func(_ context.Context, _ string) (catalog.AccountOverviewDetail, error) {
			return catalog.AccountOverviewDetail{}, nil
		},
		OverviewApplication: func(_ context.Context, _ string) (catalog.ApplicationOverviewDetail, error) {
			return catalog.ApplicationOverviewDetail{}, nil
		},
		OverviewDeployment: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{}, nil
		},
		OverviewDeploymentResourceLogs: func(context.Context, string, string, string, int) (resourcelogs.Result, error) {
			return resourcelogs.Result{}, nil
		},
		OverviewDeploymentResourceInspection: func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
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
		ApplyRequests: func(context.Context, applyrequest.ListQuery) ([]applyrequest.Record, error) {
			return []applyrequest.Record{{ID: "apply_queued", Status: applyrequest.StatusQueued}}, nil
		},
		ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
			return applyrequest.Record{ID: "apply_1234abcd"}, nil
		},
		TransitionApplyRequest: func(context.Context, string, applyrequest.TransitionRequest) (applyrequest.Record, error) {
			return applyrequest.Record{ID: "apply_1234abcd"}, nil
		},
		DeploymentStatuses: func(_ context.Context) ([]statusstore.Snapshot, error) {
			return []statusstore.Snapshot{}, nil
		},
		DeploymentStatus: func(_ context.Context, name string) (statusstore.Snapshot, error) {
			return statusstore.Snapshot{}, nil
		},
	})

	getPaths := []string{"/", "/healthz", "/v1/info", "/v1/catalog/summary", "/v1/catalog/deployments", "/v1/catalog/environments", "/v1/catalog/accounts", "/v1/catalog/placements", "/v1/catalog/topology", "/v1/overview/applications", "/v1/overview/applications/api-prod", "/v1/overview/accounts", "/v1/overview/accounts/123456789012", "/v1/overview/accounts/123456789012/projects/payments-api/environments/prod", "/v1/overview/deployments/api-prod", "/v1/overview/deployments/api-prod/events", "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", "/v1/overview/deployments/api-prod/resource-inspection?resourceType=ecsService&resourceName=payments-api-prod-service", "/v1/plans/plan_1234abcd", "/v1/applies?status=queued", "/v1/applies/apply_1234abcd", "/v1/status/deployments", "/v1/status/deployments/api-prod"}
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

	req = httptest.NewRequest(http.MethodGet, "/v1/applies/apply_1234abcd/transitions", nil)
	resp = httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for /v1/applies/apply_1234abcd/transitions, got %d", resp.Code)
	}
	if allow := resp.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow header for /v1/applies/apply_1234abcd/transitions, got %q", allow)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/applies/apply_1234abcd/heartbeat", nil)
	resp = httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for /v1/applies/apply_1234abcd/heartbeat, got %d", resp.Code)
	}
	if allow := resp.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow header for /v1/applies/apply_1234abcd/heartbeat, got %q", allow)
	}
}
