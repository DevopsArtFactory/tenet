package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
