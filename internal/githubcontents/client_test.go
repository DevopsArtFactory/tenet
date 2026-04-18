package githubcontents

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/spec"
)

func TestFetchReturnsMetadataAndContent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/platform-config/contents/deployments/payments-api-prod.yaml" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name":"payments-api-prod.yaml",
			"path":"deployments/payments-api-prod.yaml",
			"sha":"abc123",
			"size":31,
			"encoding":"base64",
			"content":"` + base64.StdEncoding.EncodeToString([]byte("name: payments-api-prod\n")) + `",
			"html_url":"https://github.example/acme/platform-config/blob/main/deployments/payments-api-prod.yaml",
			"download_url":"https://github.example/acme/platform-config/raw/main/deployments/payments-api-prod.yaml"
		}`))
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	file, err := client.Fetch(context.Background(), Request{
		Owner: "acme",
		Repo:  "platform-config",
		Path:  "deployments/payments-api-prod.yaml",
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if file.Owner != "acme" || file.Repo != "platform-config" || file.Path != "deployments/payments-api-prod.yaml" || file.SHA != "abc123" {
		t.Fatalf("unexpected file metadata: %+v", file)
	}
	if string(file.Content) != "name: payments-api-prod\n" {
		t.Fatalf("unexpected file content: %q", string(file.Content))
	}

	var decoded spec.DeploymentSpec
	if err := file.Decode(&decoded); err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if decoded.Name != "payments-api-prod" {
		t.Fatalf("unexpected decoded deployment: %+v", decoded)
	}
}

func TestFetchSendsAuthHeaderAndRefQuery(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if got := r.URL.Query().Get("ref"); got != "feature/test" {
			t.Fatalf("unexpected ref query: %q", got)
		}
		_, _ = w.Write([]byte(`{"name":"catalog.json","path":"catalog.json","encoding":"base64","content":"` + base64.StdEncoding.EncodeToString([]byte(`{"name":"ok"}`)) + `"}`))
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	if _, err := client.Fetch(context.Background(), Request{
		Repository: "acme/platform-config",
		Ref:        "feature/test",
		Path:       "catalog.json",
		Token:      "secret-token",
	}); err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
}

func TestFetchRejectsUnauthorized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := client.Fetch(context.Background(), Request{
		Owner: "acme",
		Repo:  "platform-config",
		Path:  "catalog.yaml",
	})
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	assertCode(t, err, ErrorCodeUnauthorized)
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestFetchRejectsNotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := client.Fetch(context.Background(), Request{
		Repository: "acme/platform-config",
		Ref:        "main",
		Path:       "deployments/missing.yaml",
	})
	if err == nil {
		t.Fatal("expected not found error")
	}
	assertCode(t, err, ErrorCodeNotFound)
	if !strings.Contains(err.Error(), "deployments/missing.yaml") {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestFetchRejectsInvalidRepositoryOrPath(t *testing.T) {
	t.Parallel()

	cases := []Request{
		{Repository: "acme", Path: "catalog.yaml"},
		{Owner: "acme", Repo: "platform-config", Path: "/catalog.yaml"},
		{Owner: "acme", Repo: "platform-config", Path: "../catalog.yaml"},
	}

	client := Client{}
	for _, req := range cases {
		if _, err := client.Fetch(context.Background(), req); err == nil {
			t.Fatalf("expected invalid input for request %+v", req)
		} else {
			assertCode(t, err, ErrorCodeInvalidInput)
		}
	}
}

func TestFetchRejectsUnsupportedContentEncoding(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"catalog.yaml","path":"catalog.yaml","encoding":"utf-8","content":"plain text"}`))
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := client.Fetch(context.Background(), Request{
		Owner: "acme",
		Repo:  "platform-config",
		Path:  "catalog.yaml",
	})
	if err == nil {
		t.Fatal("expected unsupported encoding error")
	}
	assertCode(t, err, ErrorCodeUnsupportedContentEncoding)
}

func TestFetchRejectsInvalidBase64Content(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"catalog.yaml","path":"catalog.yaml","encoding":"base64","content":"%%%not-base64%%%"}`))
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := client.Fetch(context.Background(), Request{
		Owner: "acme",
		Repo:  "platform-config",
		Path:  "catalog.yaml",
	})
	if err == nil {
		t.Fatal("expected invalid base64 error")
	}
	assertCode(t, err, ErrorCodeUnsupportedContentEncoding)
	if !strings.Contains(err.Error(), "invalid base64") {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestDecodeRejectsInvalidYAML(t *testing.T) {
	t.Parallel()

	file := File{
		Path:    "deployments/broken.yaml",
		Content: []byte("name: [unterminated\n"),
	}

	var decoded spec.DeploymentSpec
	err := file.Decode(&decoded)
	if err == nil {
		t.Fatal("expected decode error")
	}
	assertCode(t, err, ErrorCodeDecodeFailure)
	if !strings.Contains(err.Error(), "decode failed") {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestFetchRejectsUnexpectedStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := client.Fetch(context.Background(), Request{
		Owner: "acme",
		Repo:  "platform-config",
		Path:  "catalog.yaml",
	})
	if err == nil {
		t.Fatal("expected unexpected status error")
	}
	assertCode(t, err, ErrorCodeUnexpectedStatus)
}

func assertCode(t *testing.T, err error, want ErrorCode) {
	t.Helper()

	var fetchErr *Error
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected githubcontents.Error, got %T (%v)", err, err)
	}
	if fetchErr.Code != want {
		t.Fatalf("unexpected error code: got %q want %q", fetchErr.Code, want)
	}
}
