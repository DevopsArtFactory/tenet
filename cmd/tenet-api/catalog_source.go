package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
	"github.com/DevopsArtFactory/tenet/internal/githubwebhook"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/DevopsArtFactory/tenet/internal/manifestgithub"
	"github.com/DevopsArtFactory/tenet/internal/rbacpolicy"
)

type apiCatalogSourceKind string

const (
	apiCatalogSourceNone       apiCatalogSourceKind = ""
	apiCatalogSourceFilesystem apiCatalogSourceKind = "filesystem"
	apiCatalogSourceGitHub     apiCatalogSourceKind = "github"
)

type apiCatalogSource struct {
	loadPolicy     func(context.Context) (rbacpolicy.Policy, error)
	kind           apiCatalogSourceKind
	filesystemRoot string
	githubBaseURL  string
	githubRequest  githubcontents.Request
	loadCatalog    func(context.Context) (manifest.Catalog, error)
	loadSpec       func(context.Context) (manifest.CatalogSpec, error)
}

func loadCatalogSourceFromEnv() (apiCatalogSource, error) {
	kindRaw := strings.TrimSpace(os.Getenv("TENET_CATALOG_SOURCE"))
	filesystemRoot := strings.TrimSpace(os.Getenv("TENET_REPO_ROOT"))
	githubBaseURL := strings.TrimSpace(os.Getenv("TENET_CATALOG_GITHUB_BASE_URL"))
	githubRequest := githubcontents.Request{
		Repository: strings.TrimSpace(os.Getenv("TENET_CATALOG_GITHUB_REPOSITORY")),
		Ref:        strings.TrimSpace(os.Getenv("TENET_CATALOG_GITHUB_REF")),
		Path:       strings.TrimSpace(os.Getenv("TENET_CATALOG_GITHUB_PATH")),
		Token:      strings.TrimSpace(os.Getenv("TENET_CATALOG_GITHUB_TOKEN")),
	}

	githubConfigured := githubRequest.Repository != "" ||
		githubRequest.Ref != "" ||
		githubRequest.Path != "" ||
		githubRequest.Token != "" ||
		githubBaseURL != ""

	switch kindRaw {
	case "":
		switch {
		case filesystemRoot != "" && githubConfigured:
			return apiCatalogSource{}, fmt.Errorf("catalog source is ambiguous: set TENET_CATALOG_SOURCE=filesystem to use TENET_REPO_ROOT or TENET_CATALOG_SOURCE=github to use TENET_CATALOG_GITHUB_*")
		case filesystemRoot != "":
			return newFilesystemCatalogSource(filesystemRoot), nil
		case githubConfigured:
			return newGitHubCatalogSource(githubBaseURL, githubRequest)
		default:
			return apiCatalogSource{}, nil
		}
	case string(apiCatalogSourceFilesystem):
		if filesystemRoot == "" {
			return apiCatalogSource{}, fmt.Errorf("TENET_CATALOG_SOURCE=filesystem requires TENET_REPO_ROOT")
		}
		if githubConfigured {
			return apiCatalogSource{}, fmt.Errorf("TENET_CATALOG_SOURCE=filesystem cannot be combined with TENET_CATALOG_GITHUB_*")
		}
		return newFilesystemCatalogSource(filesystemRoot), nil
	case string(apiCatalogSourceGitHub):
		if filesystemRoot != "" {
			return apiCatalogSource{}, fmt.Errorf("TENET_CATALOG_SOURCE=github cannot be combined with TENET_REPO_ROOT")
		}
		return newGitHubCatalogSource(githubBaseURL, githubRequest)
	default:
		return apiCatalogSource{}, fmt.Errorf("TENET_CATALOG_SOURCE must be one of filesystem or github")
	}
}

func newFilesystemCatalogSource(root string) apiCatalogSource {
	root = strings.TrimSpace(root)
	return apiCatalogSource{
		kind:           apiCatalogSourceFilesystem,
		filesystemRoot: root,
		loadPolicy:     func(context.Context) (rbacpolicy.Policy, error) { return rbacpolicy.LoadFromRepository(root) },
		loadCatalog: func(_ context.Context) (manifest.Catalog, error) {
			return catalog.LoadFromRepository(root)
		},
		loadSpec: func(_ context.Context) (manifest.CatalogSpec, error) {
			return manifestfs.Load(root)
		},
	}
}

func newGitHubCatalogSource(baseURL string, req githubcontents.Request) (apiCatalogSource, error) {
	req.Repository = strings.TrimSpace(req.Repository)
	req.Ref = strings.TrimSpace(req.Ref)
	req.Path = strings.TrimSpace(req.Path)
	req.Token = strings.TrimSpace(req.Token)
	baseURL = strings.TrimSpace(baseURL)

	if req.Repository == "" {
		return apiCatalogSource{}, fmt.Errorf("TENET_CATALOG_SOURCE=github requires TENET_CATALOG_GITHUB_REPOSITORY formatted as owner/repo")
	}
	if baseURL != "" {
		if _, err := url.Parse(baseURL); err != nil {
			return apiCatalogSource{}, fmt.Errorf("TENET_CATALOG_GITHUB_BASE_URL must be a valid URL: %w", err)
		}
	}

	client := githubcontents.Client{BaseURL: baseURL}
	pinned := newPinnedGitHubCatalogLoader(client, req)
	return apiCatalogSource{
		kind:          apiCatalogSourceGitHub,
		githubBaseURL: baseURL,
		githubRequest: req,
		loadPolicy: func(ctx context.Context) (rbacpolicy.Policy, error) {
			snapshot, err := pinned(ctx)
			return snapshot.Policy, err
		},
		loadCatalog: func(ctx context.Context) (manifest.Catalog, error) {
			if ecsExecutionEnabled() {
				snapshot, err := pinned(ctx)
				return snapshot.Catalog, err
			}
			return catalog.LoadFromGitHub(ctx, client, req)
		},
		loadSpec: func(ctx context.Context) (manifest.CatalogSpec, error) {
			if ecsExecutionEnabled() {
				snapshot, err := pinned(ctx)
				return snapshot.Spec, err
			}
			return manifestgithub.Load(ctx, client, req)
		},
	}, nil
}

func validateCatalogSource(source apiCatalogSource, applyEnabled bool) error {
	if !source.configured() {
		if applyEnabled {
			return fmt.Errorf("TENET_AWS_ENABLE_APPLY=true requires TENET_REPO_ROOT because the local-safe executor only supports the filesystem catalog source")
		}
		return nil
	}
	if applyEnabled && source.kind != apiCatalogSourceFilesystem && !ecsExecutionEnabled() {
		return fmt.Errorf("TENET_AWS_ENABLE_APPLY=true requires TENET_REPO_ROOT because the local-safe executor only supports the filesystem catalog source; GitHub catalog mode is read-only")
	}
	if _, err := source.loadCatalog(context.Background()); err != nil {
		switch source.kind {
		case apiCatalogSourceFilesystem:
			return fmt.Errorf("validate filesystem catalog source: %w", err)
		case apiCatalogSourceGitHub:
			return fmt.Errorf("validate GitHub catalog source: %w", err)
		default:
			return err
		}
	}
	return nil
}

func (s apiCatalogSource) configured() bool {
	return s.kind != apiCatalogSourceNone && s.loadCatalog != nil
}

func (s apiCatalogSource) logFields() (kind string, filesystemRoot string, githubRepository string, githubRef string, githubPath string, githubBaseURL string) {
	return string(s.kind), s.filesystemRoot, s.githubRequest.Repository, s.githubRequest.Ref, s.githubRequest.Path, s.githubBaseURL
}

func (s apiCatalogSource) webhookDesiredStateSource() githubwebhook.DesiredStateSourceConfig {
	switch s.kind {
	case apiCatalogSourceFilesystem:
		return githubwebhook.DesiredStateSourceConfig{
			Kind: githubwebhook.DesiredStateSourceKindFilesystem,
		}
	case apiCatalogSourceGitHub:
		return githubwebhook.DesiredStateSourceConfig{
			Kind:       githubwebhook.DesiredStateSourceKindGitHub,
			Repository: s.githubRequest.Repository,
			Ref:        s.githubRequest.Ref,
			Path:       s.githubRequest.Path,
			BaseURL:    s.githubBaseURL,
		}
	default:
		return githubwebhook.DesiredStateSourceConfig{}
	}
}
