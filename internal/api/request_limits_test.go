package api

import (
	"context"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouterRejectsOversizedBodiesBeforeMutation(t *testing.T) {
	for _, knownLength := range []bool{true, false} {
		called := false
		router := NewRouter(Options{CreateDeploymentPlanPreview: func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
			called = true
			return planpreview.CreateResult{Created: true}, nil
		}})
		req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/app", strings.NewReader("{}"+strings.Repeat(" ", 1<<20)))
		if !knownLength {
			req.ContentLength = -1
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusRequestEntityTooLarge || called {
			t.Fatalf("known length=%v: status=%d mutation=%v", knownLength, response.Code, called)
		}
	}
}

func TestRouterRejectsOversizedChunkedWebhook(t *testing.T) {
	router := NewRouter(Options{GitHubWebhookSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", strings.NewReader(strings.Repeat(" ", (25<<20)+1)))
	req.ContentLength = -1
	req.Header.Set("X-GitHub-Delivery", "oversized-test")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", response.Code, response.Body.String())
	}
}
