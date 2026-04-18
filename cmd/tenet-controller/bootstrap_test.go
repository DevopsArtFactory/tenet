package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
)

func TestLoadStartupConfigWithoutRepositoryRoot(t *testing.T) {
	config, err := loadStartupConfig(func(key string) string {
		switch key {
		case "TENET_VERSION":
			return "dev"
		case "TENET_COMMIT":
			return "abc123"
		case "TENET_CONTROLLER_WORKERS":
			return "2"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("loadStartupConfig returned error: %v", err)
	}
	if config.WorkerCount != "2" || config.Info.Commit != "abc123" {
		t.Fatalf("unexpected config: %+v", config)
	}
	if config.Summary != nil {
		t.Fatalf("expected nil summary when repo root is unset")
	}
}

func TestLoadStartupConfigLoadsRepositorySummary(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1},"userdata":{"type":"file","path":"userdata/api-prod.sh"}}`)

	config, err := loadStartupConfig(func(key string) string {
		switch key {
		case "TENET_REPO_ROOT":
			return root
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("loadStartupConfig returned error: %v", err)
	}
	if config.Summary == nil || config.Summary.DeploymentCount != 1 {
		t.Fatalf("unexpected summary: %+v", config.Summary)
	}
	if config.CatalogSource.kind != controllerCatalogSourceFilesystem {
		t.Fatalf("expected filesystem catalog source, got %+v", config.CatalogSource)
	}
}

func TestLoadStartupConfigRejectsInvalidRepositoryRoot(t *testing.T) {
	_, err := loadStartupConfig(func(key string) string {
		if key == "TENET_REPO_ROOT" {
			return filepath.Join(t.TempDir(), "missing")
		}
		return ""
	})
	if err == nil {
		t.Fatalf("expected invalid repository root error")
	}
}

func TestLoadStartupConfigLoadsGitHubSummary(t *testing.T) {
	previous := newControllerGitHubCatalogSource
	newControllerGitHubCatalogSource = func(baseURL string, req githubcontents.Request) (controllerCatalogSource, error) {
		if baseURL != "https://github.example.test/api/v3" {
			t.Fatalf("unexpected github base URL %q", baseURL)
		}
		if req.Repository != "acme/platform-config" || req.Ref != "main" || req.Path != "tenet" || req.Token != "secret-token" {
			t.Fatalf("unexpected github request: %+v", req)
		}
		return controllerCatalogSource{
			kind:          controllerCatalogSourceGitHub,
			githubBaseURL: baseURL,
			githubRequest: req,
			loadCatalog: func(_ context.Context) (manifest.Catalog, error) {
				return manifest.Catalog{
					Accounts:     []domain.Account{{Name: "prod-core", AWSAccountID: "123456789012"}},
					Environments: []domain.Environment{{Name: "prod-apne2", AccountRef: "prod-core"}},
					Deployments:  []domain.Deployment{{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}},
				}, nil
			},
		}, nil
	}
	t.Cleanup(func() {
		newControllerGitHubCatalogSource = previous
	})

	config, err := loadStartupConfig(func(key string) string {
		switch key {
		case "TENET_CATALOG_SOURCE":
			return "github"
		case "TENET_CATALOG_GITHUB_REPOSITORY":
			return "acme/platform-config"
		case "TENET_CATALOG_GITHUB_REF":
			return "main"
		case "TENET_CATALOG_GITHUB_PATH":
			return "tenet"
		case "TENET_CATALOG_GITHUB_TOKEN":
			return "secret-token"
		case "TENET_CATALOG_GITHUB_BASE_URL":
			return "https://github.example.test/api/v3"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("loadStartupConfig returned error: %v", err)
	}
	if config.RepoRoot != "" {
		t.Fatalf("expected empty repo root in github mode, got %q", config.RepoRoot)
	}
	if config.CatalogSource.kind != controllerCatalogSourceGitHub {
		t.Fatalf("expected github catalog source, got %+v", config.CatalogSource)
	}
	if config.Summary == nil || config.Summary.DeploymentCount != 1 {
		t.Fatalf("unexpected summary: %+v", config.Summary)
	}
}

func TestLoadStartupConfigRejectsAmbiguousCatalogSource(t *testing.T) {
	_, err := loadStartupConfig(func(key string) string {
		switch key {
		case "TENET_REPO_ROOT":
			return t.TempDir()
		case "TENET_CATALOG_GITHUB_REPOSITORY":
			return "acme/platform-config"
		default:
			return ""
		}
	})
	if err == nil || !strings.Contains(err.Error(), "catalog source is ambiguous") {
		t.Fatalf("expected ambiguous catalog source error, got %v", err)
	}
}

func TestLoadStartupConfigLoadsRuntimeSettings(t *testing.T) {
	config, err := loadStartupConfig(func(key string) string {
		switch key {
		case "TENET_STATUS_PATH":
			return "/tmp/status.json"
		case "TENET_CONTROLLER_REQUEUE_SECONDS":
			return "7"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("loadStartupConfig returned error: %v", err)
	}
	if config.StatusPath != "/tmp/status.json" {
		t.Fatalf("unexpected status path: %q", config.StatusPath)
	}
	if config.RequeueInterval != 7*time.Second {
		t.Fatalf("unexpected requeue interval: %s", config.RequeueInterval)
	}
}

func TestLoadStartupConfigRejectsInvalidRequeueSeconds(t *testing.T) {
	_, err := loadStartupConfig(func(key string) string {
		if key == "TENET_CONTROLLER_REQUEUE_SECONDS" {
			return "bogus"
		}
		return ""
	})
	if err == nil {
		t.Fatalf("expected invalid requeue seconds error")
	}
}

func writeJSON(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
}
