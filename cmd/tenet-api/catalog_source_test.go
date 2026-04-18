package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestgithub"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

func TestLoadCatalogSourceFromEnv(t *testing.T) {
	t.Run("defaults to filesystem when TENET_REPO_ROOT is set", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("TENET_REPO_ROOT", root)

		source, err := loadCatalogSourceFromEnv()
		if err != nil {
			t.Fatalf("loadCatalogSourceFromEnv returned error: %v", err)
		}
		if source.kind != apiCatalogSourceFilesystem {
			t.Fatalf("expected filesystem source, got %q", source.kind)
		}
		if source.filesystemRoot != root {
			t.Fatalf("expected filesystem root %q, got %q", root, source.filesystemRoot)
		}
	})

	t.Run("defaults to github when TENET_CATALOG_GITHUB_* is set", func(t *testing.T) {
		t.Setenv("TENET_CATALOG_GITHUB_REPOSITORY", "acme/platform-config")
		t.Setenv("TENET_CATALOG_GITHUB_REF", "main")
		t.Setenv("TENET_CATALOG_GITHUB_PATH", "catalog")
		t.Setenv("TENET_CATALOG_GITHUB_BASE_URL", "https://github.example.test/api/v3")

		source, err := loadCatalogSourceFromEnv()
		if err != nil {
			t.Fatalf("loadCatalogSourceFromEnv returned error: %v", err)
		}
		if source.kind != apiCatalogSourceGitHub {
			t.Fatalf("expected github source, got %q", source.kind)
		}
		if source.githubRequest.Repository != "acme/platform-config" || source.githubRequest.Ref != "main" || source.githubRequest.Path != "catalog" {
			t.Fatalf("unexpected github request: %+v", source.githubRequest)
		}
		if source.githubBaseURL != "https://github.example.test/api/v3" {
			t.Fatalf("unexpected github base URL %q", source.githubBaseURL)
		}
	})

	t.Run("rejects ambiguous implicit source selection", func(t *testing.T) {
		t.Setenv("TENET_REPO_ROOT", t.TempDir())
		t.Setenv("TENET_CATALOG_GITHUB_REPOSITORY", "acme/platform-config")

		if _, err := loadCatalogSourceFromEnv(); err == nil || !strings.Contains(err.Error(), "catalog source is ambiguous") {
			t.Fatalf("expected ambiguous source error, got %v", err)
		}
	})

	t.Run("rejects explicit github source without repository", func(t *testing.T) {
		t.Setenv("TENET_CATALOG_SOURCE", "github")

		if _, err := loadCatalogSourceFromEnv(); err == nil || !strings.Contains(err.Error(), "TENET_CATALOG_GITHUB_REPOSITORY") {
			t.Fatalf("expected missing repository error, got %v", err)
		}
	})
}

func TestValidateCatalogSourceRejectsApplyExecutorForGitHubMode(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	source := newTestGitHubCatalogSource(t, repoRoot)
	if err := validateCatalogSource(source, true); err == nil || !strings.Contains(err.Error(), "GitHub catalog mode is read-only") {
		t.Fatalf("expected GitHub apply validation error, got %v", err)
	}
}

func TestValidateCatalogSourceLoadsGitHubCatalogAtStartup(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	source := newTestGitHubCatalogSource(t, repoRoot)
	if err := validateCatalogSource(source, false); err != nil {
		t.Fatalf("validateCatalogSource returned error: %v", err)
	}
}

func TestAPIOptionsWithGitHubCatalogSourcePreservesOverviewStatusJoin(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	source := newTestGitHubCatalogSource(t, repoRoot)

	store := &stubStatusStore{
		getSnapshot: statusstore.Snapshot{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				ObservedGeneration: 7,
				Sync:               domain.Sync{State: domain.SyncStateOutOfSync, Reason: "PendingPromotion"},
				Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "warming up"},
				DeployedArtifact:   domain.DeploymentArtifact{Image: "payments-api:v1"},
			},
		},
	}

	options := newAPIOptionsWithCatalogSource(buildinfo.Info{Version: "dev", Commit: "abc123"}, source, store)
	if options.OverviewDeployment == nil {
		t.Fatal("expected overview deployment provider")
	}

	detail, err := options.OverviewDeployment(context.Background(), "api-prod")
	if err != nil {
		t.Fatalf("OverviewDeployment returned error: %v", err)
	}
	if store.getCalls != 1 || store.gotGetName != "api-prod" || store.listCalls != 0 {
		t.Fatalf("expected overview detail to reuse named status lookup, got getCalls=%d gotGetName=%q listCalls=%d", store.getCalls, store.gotGetName, store.listCalls)
	}
	if detail.Name != "api-prod" || detail.AccountName != "prod-core" || detail.DeployedArtifact.Image != "payments-api:v1" {
		t.Fatalf("unexpected deployment detail: %+v", detail)
	}
}

func TestAPIOptionsWithGitHubCatalogSourceUsesObserverRoleForResourceInspection(t *testing.T) {
	repoRoot := writeCanaryEC2RepositoryFixture(t)
	const key = "load-spec-trace"
	var gotLoadSpecValue any
	source := newContextAwareGitHubCatalogSource(t, repoRoot, func(ctx context.Context) {
		gotLoadSpecValue = ctx.Value(key)
	})

	provider := &fakeResourceInspectionProvider{}
	restore := stubResourceInspectionProviderFactory(provider)
	defer restore()

	options := newAPIOptionsWithCatalogSource(buildinfo.Info{Version: "dev", Commit: "abc123"}, source, nil)
	if options.OverviewDeploymentResourceInspection == nil {
		t.Fatal("expected resource inspection provider")
	}

	ctx := context.WithValue(context.Background(), key, "trace-value")
	if _, err := options.OverviewDeploymentResourceInspection(ctx, "api-prod", "candidateAutoScalingGroup", "api-prod-candidate"); err != nil {
		t.Fatalf("OverviewDeploymentResourceInspection returned error: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected one inspection call, got %d", provider.calls)
	}
	if gotLoadSpecValue != "trace-value" {
		t.Fatalf("expected request context value to reach GitHub loadSpec, got %#v", gotLoadSpecValue)
	}
	if provider.lastRequest.ObserverRoleARN != "arn:aws:iam::123456789012:role/TenetObserverRole" {
		t.Fatalf("expected observer role from GitHub-backed account spec, got %+v", provider.lastRequest)
	}
	if provider.lastRequest.Region != "ap-northeast-2" || provider.lastRequest.TargetAccountID != "123456789012" {
		t.Fatalf("unexpected inspection request: %+v", provider.lastRequest)
	}
}

func newTestGitHubCatalogSource(t *testing.T, root string) apiCatalogSource {
	t.Helper()

	return newContextAwareGitHubCatalogSource(t, root, nil)
}

func newContextAwareGitHubCatalogSource(t *testing.T, root string, onLoadSpec func(context.Context)) apiCatalogSource {
	t.Helper()

	client := &testGitHubManifestClient{root: root, t: t}
	req := githubcontents.Request{Repository: "acme/platform-config", Ref: "main"}
	return apiCatalogSource{
		kind:          apiCatalogSourceGitHub,
		githubRequest: req,
		loadCatalog: func(_ context.Context) (manifest.Catalog, error) {
			return catalog.LoadFromGitHub(context.Background(), client, req)
		},
		loadSpec: func(ctx context.Context) (manifest.CatalogSpec, error) {
			if onLoadSpec != nil {
				onLoadSpec(ctx)
			}
			return manifestgithub.Load(ctx, client, req)
		},
	}
}

type testGitHubManifestClient struct {
	root string
	t    *testing.T
}

func (c *testGitHubManifestClient) List(_ context.Context, req githubcontents.Request) ([]githubcontents.Entry, error) {
	c.t.Helper()

	dir := filepath.Join(c.root, filepath.FromSlash(req.Path))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	payload := make([]githubcontents.Entry, 0, len(entries))
	for _, entry := range entries {
		entryPath := entry.Name()
		if req.Path != "" {
			entryPath = req.Path + "/" + entry.Name()
		}
		entryType := "file"
		if entry.IsDir() {
			entryType = "dir"
		}
		payload = append(payload, githubcontents.Entry{
			Type: entryType,
			Name: entry.Name(),
			Path: strings.ReplaceAll(entryPath, string(filepath.Separator), "/"),
		})
	}
	return payload, nil
}

func (c *testGitHubManifestClient) Fetch(_ context.Context, req githubcontents.Request) (githubcontents.File, error) {
	c.t.Helper()

	path := filepath.Join(c.root, filepath.FromSlash(req.Path))
	content, err := os.ReadFile(path)
	if err != nil {
		return githubcontents.File{}, err
	}
	return githubcontents.File{
		Name:    filepath.Base(path),
		Path:    strings.ReplaceAll(req.Path, string(filepath.Separator), "/"),
		Content: content,
	}, nil
}
