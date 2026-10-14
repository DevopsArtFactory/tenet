package githubcontents

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestResolveCommitUsesSHAResponseAndPreservesEnterpriseBasePath(t *testing.T) {
	revision := strings.Repeat("a", 40)
	client := testClient(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.EscapedPath() != "/api/v3/repos/acme/config/commits/feature%2Fecs" || request.Header.Get("Accept") != "application/vnd.github.sha" || request.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Fatalf("incorrect revision request: %s", request.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(revision + "\n"))}, nil
	})
	client.BaseURL = "https://github.example.test/api/v3"
	resolved, err := client.ResolveCommit(context.Background(), Request{Repository: "acme/config", Ref: "feature/ecs", Token: "fixture-token"})
	if err != nil || resolved != revision {
		t.Fatalf("resolve commit: %q %v", resolved, err)
	}
}
func TestResolveCommitRejectsMutableOrMalformedRevisionEvidence(t *testing.T) {
	for _, body := range []string{"main", "abc123", `{"sha":"` + strings.Repeat("a", 40) + `"}`, strings.Repeat("a", 41)} {
		client := testClient(t, func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		if _, err := client.ResolveCommit(context.Background(), Request{Repository: "acme/config", Ref: "main"}); err == nil {
			t.Fatalf("accepted revision evidence %q", body)
		}
	}
}
