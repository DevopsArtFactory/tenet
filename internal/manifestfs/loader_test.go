package manifestfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsAccountEnvironmentAndDeploymentFiles(t *testing.T) {
	root := t.TempDir()
	mustWriteJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	mustWriteJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	mustWriteJSON(t, filepath.Join(root, "deployments", "payments-api-prod.json"), `{"name":"payments-api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"hostname":"api.example.com","port":443},"capacity":{"desired":1,"min":1,"max":1}}`)

	catalog, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(catalog.Accounts) != 1 || len(catalog.Environments) != 1 || len(catalog.Deployments) != 1 {
		t.Fatalf("unexpected catalog contents: %+v", catalog)
	}
}

func TestLoadRejectsMissingRoot(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatalf("expected missing root error")
	}
}

func TestLoadRejectsMissingRequiredDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "accounts"), 0o755); err != nil {
		t.Fatalf("failed to create accounts dir: %v", err)
	}
	if _, err := Load(root); err == nil {
		t.Fatalf("expected missing required directories error")
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	root := t.TempDir()
	mustWriteJSON(t, filepath.Join(root, "accounts", "broken.json"), `{not-json}`)

	if _, err := Load(root); err == nil {
		t.Fatalf("expected invalid JSON error")
	}
}

func TestLoadReadsYAMLDeploymentFiles(t *testing.T) {
	root := t.TempDir()
	mustWriteJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	mustWriteJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	mustWriteManifest(t, filepath.Join(root, "deployments", "payments-api-prod.yaml"), "name: payments-api-prod\nappRef: payments-api\nenvironmentRef: prod-apne2\ntarget:\n  type: ec2\nstrategy:\n  type: blueGreen\ntraffic:\n  hostname: api.example.com\n  port: 443\ncapacity:\n  desired: 1\n  min: 1\n  max: 1\n")

	catalog, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(catalog.Deployments) != 1 || catalog.Deployments[0].Name != "payments-api-prod" {
		t.Fatalf("unexpected YAML deployment contents: %+v", catalog.Deployments)
	}
}

func TestLoadReadsYMLDeploymentFiles(t *testing.T) {
	root := t.TempDir()
	mustWriteJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	mustWriteJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	mustWriteManifest(t, filepath.Join(root, "deployments", "payments-api-prod.yml"), "name: payments-api-prod\nappRef: payments-api\nenvironmentRef: prod-apne2\ntarget:\n  type: ec2\nstrategy:\n  type: blueGreen\ntraffic:\n  port: 443\ncapacity:\n  desired: 1\n  min: 1\n  max: 1\n")

	catalog, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(catalog.Deployments) != 1 || catalog.Deployments[0].Name != "payments-api-prod" {
		t.Fatalf("unexpected YML deployment contents: %+v", catalog.Deployments)
	}
}

func TestLoadRejectsInvalidYAML(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"accounts", "environments", "deployments"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("failed to create required dir %s: %v", dir, err)
		}
	}
	mustWriteManifest(t, filepath.Join(root, "deployments", "broken.yaml"), "name: [unterminated\n")

	if _, err := Load(root); err == nil {
		t.Fatalf("expected invalid YAML error")
	}
}

func TestLoadIgnoresNonManifestFiles(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"accounts", "environments", "deployments"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("failed to create required dir %s: %v", dir, err)
		}
	}
	path := filepath.Join(root, "accounts", "note.txt")
	if err := os.WriteFile(path, []byte("ignore me"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	catalog, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(catalog.Accounts) != 0 || len(catalog.Environments) != 0 || len(catalog.Deployments) != 0 {
		t.Fatalf("expected empty catalog, got %+v", catalog)
	}
}

func mustWriteJSON(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
}

func mustWriteManifest(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
}
