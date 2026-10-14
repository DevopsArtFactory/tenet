package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

type pinnedGitHubFixture struct {
	mutex        sync.Mutex
	head         string
	moveTo       string
	contentCalls int
	refs         []string
	server       *httptest.Server
}

func newPinnedGitHubFixture(t *testing.T) *pinnedGitHubFixture {
	t.Helper()
	a, b := strings.Repeat("a", 40), strings.Repeat("b", 40)
	files := map[string][]byte{}
	for _, name := range []string{"accounts/production.yaml", "environments/prod.yaml", "deployments/worker.yaml"} {
		content, err := os.ReadFile(filepath.Join("..", "..", "examples", "ecs-native-repo", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = content
	}
	files["rbac/policy.yaml"] = []byte("version: v1\nusers:\n  - id: operator\n    roles: [operator]\n    deployments: [worker]\n")
	fixture := &pinnedGitHubFixture{head: a}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.mutex.Lock()
		defer fixture.mutex.Unlock()
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			http.Error(w, "missing fixture token", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/v3/repos/acme/config/commits/main" {
			revision := fixture.head
			if fixture.moveTo != "" {
				fixture.head = fixture.moveTo
				fixture.moveTo = ""
			}
			fmt.Fprint(w, revision)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/api/v3/repos/acme/config/contents/catalog/")
		revision := r.URL.Query().Get("ref")
		if revision != a && revision != b {
			http.Error(w, "contents request did not pin the commit", http.StatusBadRequest)
			return
		}
		fixture.refs = append(fixture.refs, revision)
		fixture.contentCalls++
		if content, ok := files[name]; ok {
			if revision == b && name == "rbac/policy.yaml" {
				content = []byte(strings.ReplaceAll(string(content), "roles: [operator]", "roles: [viewer]"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"type": "file", "name": path.Base(name), "path": "catalog/" + name, "sha": "fixture-blob", "size": len(content), "encoding": "base64", "content": base64.StdEncoding.EncodeToString(content)})
			return
		}
		entries := []map[string]any{}
		for file := range files {
			if path.Dir(file) == name {
				entries = append(entries, map[string]any{"type": "file", "name": path.Base(file), "path": "catalog/" + file, "sha": "fixture-blob"})
			}
		}
		if len(entries) > 0 {
			_ = json.NewEncoder(w).Encode(entries)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}
func (f *pinnedGitHubFixture) request() githubcontents.Request {
	return githubcontents.Request{Repository: "acme/config", Ref: "main", Path: "catalog", Token: "fixture-token"}
}
func (f *pinnedGitHubFixture) setHead(head string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.head = head
}

func TestPinnedGitHubCatalogKeepsOneRevisionAndRechecksPolicyHead(t *testing.T) {
	fixture := newPinnedGitHubFixture(t)
	a, b := strings.Repeat("a", 40), strings.Repeat("b", 40)
	fixture.moveTo = b
	load := newPinnedGitHubCatalogLoader(githubcontents.Client{BaseURL: fixture.server.URL + "/api/v3", HTTPClient: fixture.server.Client()}, fixture.request())
	snapshot, err := load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Catalog.Git.Revision != a || snapshot.Spec.Git.Revision != a || snapshot.Policy.Users[0].Roles[0] != "operator" {
		t.Fatal("snapshot mixed different commit inputs")
	}
	fixture.mutex.Lock()
	for _, ref := range fixture.refs {
		if ref != a {
			t.Fatal("branch advanced during an unpinned contents read")
		}
	}
	beforeCalls := fixture.contentCalls
	fixture.mutex.Unlock()
	originalImage := snapshot.Spec.Deployments[0].Image
	snapshot.Spec.Deployments[0].Image = "poisoned-cache"
	snapshot.Catalog.Deployments[0].ECS.Image = "poisoned-cache"
	snapshot.Policy.Users[0].Roles[0] = "admin"
	fixture.setHead(a)
	again, err := load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fixture.mutex.Lock()
	afterCalls := fixture.contentCalls
	fixture.mutex.Unlock()
	if beforeCalls != afterCalls || again.Spec.Deployments[0].Image != originalImage || again.Catalog.Deployments[0].ECS.Image != originalImage || again.Policy.Users[0].Roles[0] != "operator" {
		t.Fatal("immutable commit cache was refetched or mutated through a caller projection")
	}
	fixture.setHead(b)
	latest, err := load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if latest.Catalog.Git.Revision != b || latest.Policy.Users[0].Roles[0] != "viewer" {
		t.Fatal("branch policy revocation was hidden behind cache")
	}
}

func TestGitHubLiveECSPlansAndApprovalsRetainPinnedRevision(t *testing.T) {
	t.Setenv("TENET_AWS_ENABLE_APPLY", "true")
	t.Setenv("TENET_AWS_ENABLE_ECS_EXECUTION", "true")
	t.Setenv("TENET_API_TRUST_AUTHENTICATED_HEADERS", "true")
	fixture := newPinnedGitHubFixture(t)
	source, err := newGitHubCatalogSource(fixture.server.URL+"/api/v3", fixture.request())
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCatalogSource(source, true); err != nil {
		t.Fatal(err)
	}
	if err := validateECSExecutionConfiguration(source); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	options := newAPIOptionsWithCatalogSourceAndStores(buildinfo.Info{}, source, nil, filepath.Join(dir, "plans.json"), filepath.Join(dir, "applies.json"))
	ctx := requestactor.WithAuthenticatedIdentityContext(context.Background(), requestactor.AuthenticatedIdentity{Authenticated: true, ID: "operator"})
	plan, err := options.CreateDeploymentPlanPreview(ctx, "worker", planpreview.CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	apply, err := options.CreateApplyRequest(ctx, plan.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	application, err := options.OverviewApplication(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	expected := strings.Repeat("a", 40)
	if plan.Record.Source.Git == nil || apply.Record.Source.Git == nil || application.Git == nil || plan.Record.Source.Git.Revision != expected || apply.Record.Source.Git.Revision != expected || application.Git.Revision != expected || application.SourceFingerprint != plan.Record.Source.Fingerprint {
		t.Fatal("pinned source correlation was lost between overview, plan and Apply")
	}
	fixture.setHead(strings.Repeat("b", 40))
	policy, err := options.RepositoryPolicy(ctx, requestactor.AuthenticatedIdentityFromContext(ctx), requestactor.AuthenticatedGroups{})
	if err != nil {
		t.Fatal(err)
	}
	if policy.CanWrite {
		t.Fatal("live GitHub policy provider reused revoked operator grants")
	}
	executor := newNativeECSExecutor(source, options, filepath.Join(dir, "plans.json"), filepath.Join(dir, "applies.json"), nil)
	if err := executor.ECS.Authorize(ctx, apply.Record); err == nil {
		t.Fatal("queued approval bypassed current GitHub RBAC revocation")
	}
}
