package githubcontents

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/spec"
)

func TestFetchReturnsMetadataAndContent(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/repos/acme/platform-config/contents/deployments/payments-api-prod.yaml" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{
			"name":"payments-api-prod.yaml",
			"path":"deployments/payments-api-prod.yaml",
			"sha":"abc123",
			"size":31,
			"encoding":"base64",
			"content":"`+base64.StdEncoding.EncodeToString([]byte("name: payments-api-prod\n"))+`",
			"html_url":"https://github.example/acme/platform-config/blob/main/deployments/payments-api-prod.yaml",
			"download_url":"https://github.example/acme/platform-config/raw/main/deployments/payments-api-prod.yaml"
		}`), nil
	})
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

func TestListReturnsDirectoryEntries(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/repos/acme/platform-config/contents/catalog/accounts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if got := r.URL.Query().Get("ref"); got != "feature/test" {
			t.Fatalf("unexpected ref query: %q", got)
		}
		return jsonResponse(http.StatusOK, `[
			{"type":"file","name":"prod-core.json","path":"catalog/accounts/prod-core.json","sha":"sha-1","size":123},
			{"type":"dir","name":"nested","path":"catalog/accounts/nested","sha":"sha-2","size":0},
			{"type":"file","name":"sandbox.yaml","path":"catalog/accounts/sandbox.yaml","sha":"sha-3","size":456}
		]`), nil
	})
	entries, err := client.List(context.Background(), Request{
		Repository: "acme/platform-config",
		Ref:        "feature/test",
		Path:       "catalog/accounts",
		Token:      "secret-token",
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("unexpected entry count: %+v", entries)
	}
	if entries[0].Path != "catalog/accounts/nested" || !entries[0].IsDir() {
		t.Fatalf("expected sorted directory entry first, got %+v", entries[0])
	}
	if entries[1].Path != "catalog/accounts/prod-core.json" || entries[1].IsDir() {
		t.Fatalf("unexpected file entry: %+v", entries[1])
	}
}

func TestFetchSendsAuthHeaderAndRefQuery(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if got := r.URL.Query().Get("ref"); got != "feature/test" {
			t.Fatalf("unexpected ref query: %q", got)
		}
		return jsonResponse(http.StatusOK, `{"name":"catalog.json","path":"catalog.json","encoding":"base64","content":"`+base64.StdEncoding.EncodeToString([]byte(`{"name":"ok"}`))+`"}`), nil
	})
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

	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, `unauthorized`), nil
	})
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

	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusNotFound, `not found`), nil
	})
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

func TestListRejectsNotFound(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusNotFound, `not found`), nil
	})
	_, err := client.List(context.Background(), Request{
		Repository: "acme/platform-config",
		Ref:        "main",
		Path:       "catalog/accounts",
	})
	if err == nil {
		t.Fatal("expected not found error")
	}
	assertCode(t, err, ErrorCodeNotFound)
	if !strings.Contains(err.Error(), "directory") {
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

func TestListRejectsInvalidRepositoryOrPath(t *testing.T) {
	t.Parallel()

	cases := []Request{
		{Repository: "acme", Path: "catalog/accounts"},
		{Owner: "acme", Repo: "platform-config", Path: "/catalog/accounts"},
		{Owner: "acme", Repo: "platform-config", Path: "../catalog/accounts"},
	}

	client := Client{}
	for _, req := range cases {
		if _, err := client.List(context.Background(), req); err == nil {
			t.Fatalf("expected invalid input for request %+v", req)
		} else {
			assertCode(t, err, ErrorCodeInvalidInput)
		}
	}
}

func TestFetchRejectsUnsupportedContentEncoding(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"name":"catalog.yaml","path":"catalog.yaml","encoding":"utf-8","content":"plain text"}`), nil
	})
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

	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"name":"catalog.yaml","path":"catalog.yaml","encoding":"base64","content":"%%%not-base64%%%"}`), nil
	})
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

	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusInternalServerError, `boom`), nil
	})
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

func TestListRejectsUnexpectedStatus(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusInternalServerError, `boom`), nil
	})
	_, err := client.List(context.Background(), Request{
		Owner: "acme",
		Repo:  "platform-config",
		Path:  "catalog/accounts",
	})
	if err == nil {
		t.Fatal("expected unexpected status error")
	}
	assertCode(t, err, ErrorCodeUnexpectedStatus)
}

func TestListRejectsFileResponses(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"type":"file","name":"prod-core.json","path":"catalog/accounts/prod-core.json"}`), nil
	})
	_, err := client.List(context.Background(), Request{
		Owner: "acme",
		Repo:  "platform-config",
		Path:  "catalog/accounts",
	})
	if err == nil {
		t.Fatal("expected unexpected content type error")
	}
	assertCode(t, err, ErrorCodeUnexpectedContentType)
	if !strings.Contains(err.Error(), "directory listing") {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestFetchRejectsDirectoryResponses(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `[]`), nil
	})
	_, err := client.Fetch(context.Background(), Request{
		Owner: "acme",
		Repo:  "platform-config",
		Path:  "catalog/accounts",
	})
	if err == nil {
		t.Fatal("expected unexpected content type error")
	}
	assertCode(t, err, ErrorCodeUnexpectedContentType)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func testClient(t *testing.T, fn func(*http.Request) (*http.Response, error)) Client {
	t.Helper()
	return Client{
		BaseURL: "https://github.example",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(fn),
		},
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
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

func TestContentsRequestsPreserveEnterpriseAPIBasePath(t *testing.T) {
	client := testClient(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/api/v3/repos/acme/config/contents/accounts" {
			t.Fatalf("enterprise API prefix lost: %s", request.URL.Path)
		}
		return jsonResponse(http.StatusOK, `[]`), nil
	})
	client.BaseURL = "https://github.example/api/v3"
	if _, err := client.List(context.Background(), Request{Repository: "acme/config", Ref: "main", Path: "accounts"}); err != nil {
		t.Fatal(err)
	}
}
