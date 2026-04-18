package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
)

type controllerCatalogSourceKind string

const (
	controllerCatalogSourceNone       controllerCatalogSourceKind = ""
	controllerCatalogSourceFilesystem controllerCatalogSourceKind = "filesystem"
	controllerCatalogSourceGitHub     controllerCatalogSourceKind = "github"
)

type controllerCatalogSource struct {
	kind           controllerCatalogSourceKind
	filesystemRoot string
	githubBaseURL  string
	githubRequest  githubcontents.Request
	loadCatalog    func(context.Context) (manifest.Catalog, error)
}

var newControllerGitHubCatalogSource = defaultNewControllerGitHubCatalogSource

func loadControllerCatalogSourceFromEnv(getenv func(string) string) (controllerCatalogSource, error) {
	kindRaw := strings.TrimSpace(getenv("TENET_CATALOG_SOURCE"))
	filesystemRoot := strings.TrimSpace(getenv("TENET_REPO_ROOT"))
	githubBaseURL := strings.TrimSpace(getenv("TENET_CATALOG_GITHUB_BASE_URL"))
	githubRequest := githubcontents.Request{
		Repository: strings.TrimSpace(getenv("TENET_CATALOG_GITHUB_REPOSITORY")),
		Ref:        strings.TrimSpace(getenv("TENET_CATALOG_GITHUB_REF")),
		Path:       strings.TrimSpace(getenv("TENET_CATALOG_GITHUB_PATH")),
		Token:      strings.TrimSpace(getenv("TENET_CATALOG_GITHUB_TOKEN")),
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
			return controllerCatalogSource{}, fmt.Errorf("catalog source is ambiguous: set TENET_CATALOG_SOURCE=filesystem to use TENET_REPO_ROOT or TENET_CATALOG_SOURCE=github to use TENET_CATALOG_GITHUB_*")
		case filesystemRoot != "":
			return newControllerFilesystemCatalogSource(filesystemRoot), nil
		case githubConfigured:
			return newControllerGitHubCatalogSource(githubBaseURL, githubRequest)
		default:
			return controllerCatalogSource{}, nil
		}
	case string(controllerCatalogSourceFilesystem):
		if filesystemRoot == "" {
			return controllerCatalogSource{}, fmt.Errorf("TENET_CATALOG_SOURCE=filesystem requires TENET_REPO_ROOT")
		}
		if githubConfigured {
			return controllerCatalogSource{}, fmt.Errorf("TENET_CATALOG_SOURCE=filesystem cannot be combined with TENET_CATALOG_GITHUB_*")
		}
		return newControllerFilesystemCatalogSource(filesystemRoot), nil
	case string(controllerCatalogSourceGitHub):
		if filesystemRoot != "" {
			return controllerCatalogSource{}, fmt.Errorf("TENET_CATALOG_SOURCE=github cannot be combined with TENET_REPO_ROOT")
		}
		return newControllerGitHubCatalogSource(githubBaseURL, githubRequest)
	default:
		return controllerCatalogSource{}, fmt.Errorf("TENET_CATALOG_SOURCE must be one of filesystem or github")
	}
}

func newControllerFilesystemCatalogSource(root string) controllerCatalogSource {
	root = strings.TrimSpace(root)
	return controllerCatalogSource{
		kind:           controllerCatalogSourceFilesystem,
		filesystemRoot: root,
		loadCatalog: func(_ context.Context) (manifest.Catalog, error) {
			return catalog.LoadFromRepository(root)
		},
	}
}

func defaultNewControllerGitHubCatalogSource(baseURL string, req githubcontents.Request) (controllerCatalogSource, error) {
	req.Repository = strings.TrimSpace(req.Repository)
	req.Ref = strings.TrimSpace(req.Ref)
	req.Path = strings.TrimSpace(req.Path)
	req.Token = strings.TrimSpace(req.Token)
	baseURL = strings.TrimSpace(baseURL)

	if req.Repository == "" {
		return controllerCatalogSource{}, fmt.Errorf("TENET_CATALOG_SOURCE=github requires TENET_CATALOG_GITHUB_REPOSITORY formatted as owner/repo")
	}
	if baseURL != "" {
		if _, err := url.Parse(baseURL); err != nil {
			return controllerCatalogSource{}, fmt.Errorf("TENET_CATALOG_GITHUB_BASE_URL must be a valid URL: %w", err)
		}
	}

	client := githubcontents.Client{BaseURL: baseURL}
	return controllerCatalogSource{
		kind:          controllerCatalogSourceGitHub,
		githubBaseURL: baseURL,
		githubRequest: req,
		loadCatalog: func(ctx context.Context) (manifest.Catalog, error) {
			return catalog.LoadFromGitHub(ctx, client, req)
		},
	}, nil
}

func (s controllerCatalogSource) configured() bool {
	return s.kind != controllerCatalogSourceNone && s.loadCatalog != nil
}

func (s controllerCatalogSource) logFields() (kind string, filesystemRoot string, githubRepository string, githubRef string, githubPath string, githubBaseURL string) {
	return string(s.kind), s.filesystemRoot, s.githubRequest.Repository, s.githubRequest.Ref, s.githubRequest.Path, s.githubBaseURL
}
