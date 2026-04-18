package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
)

func TestLoadFromRepositoryReturnsValidatedCatalog(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "payments-api-prod.json"), `{"name":"payments-api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1},"userdata":{"type":"file","path":"userdata/payments-api-prod.sh"}}`)

	catalog, err := LoadFromRepository(root)
	if err != nil {
		t.Fatalf("LoadFromRepository returned error: %v", err)
	}
	if len(catalog.Deployments) != 1 {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}
}

func TestLoadFromRepositoryPropagatesManifestValidationErrors(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole"}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"missing","region":"ap-northeast-2"}`)

	if _, err := LoadFromRepository(root); err == nil {
		t.Fatalf("expected manifest compilation error")
	}
}

func TestLoadFromGitHubReturnsValidatedCatalog(t *testing.T) {
	client := &stubGitHubManifestClient{
		entries: map[string][]githubcontents.Entry{
			"catalog/accounts": {
				{Name: "prod-core.json", Path: "catalog/accounts/prod-core.json", Type: "file"},
			},
			"catalog/environments": {
				{Name: "prod-apne2.json", Path: "catalog/environments/prod-apne2.json", Type: "file"},
			},
			"catalog/deployments": {
				{Name: "payments-api-prod.json", Path: "catalog/deployments/payments-api-prod.json", Type: "file"},
			},
		},
		files: map[string]githubcontents.File{
			"catalog/accounts/prod-core.json": {
				Path:    "catalog/accounts/prod-core.json",
				Content: []byte(`{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`),
			},
			"catalog/environments/prod-apne2.json": {
				Path:    "catalog/environments/prod-apne2.json",
				Content: []byte(`{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`),
			},
			"catalog/deployments/payments-api-prod.json": {
				Path:    "catalog/deployments/payments-api-prod.json",
				Content: []byte(`{"name":"payments-api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1},"userdata":{"type":"file","path":"userdata/payments-api-prod.sh"}}`),
			},
		},
	}

	catalog, err := LoadFromGitHub(context.Background(), client, githubcontents.Request{
		Repository: "acme/platform-config",
		Ref:        "main",
		Path:       "catalog",
	})
	if err != nil {
		t.Fatalf("LoadFromGitHub returned error: %v", err)
	}
	if len(catalog.Deployments) != 1 {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}
}

func TestLoadFromGitHubPropagatesManifestValidationErrors(t *testing.T) {
	client := &stubGitHubManifestClient{
		entries: map[string][]githubcontents.Entry{
			"accounts": {
				{Name: "prod-core.json", Path: "accounts/prod-core.json", Type: "file"},
			},
			"environments": {
				{Name: "prod-apne2.json", Path: "environments/prod-apne2.json", Type: "file"},
			},
			"deployments": {},
		},
		files: map[string]githubcontents.File{
			"accounts/prod-core.json": {
				Path:    "accounts/prod-core.json",
				Content: []byte(`{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole"}`),
			},
			"environments/prod-apne2.json": {
				Path:    "environments/prod-apne2.json",
				Content: []byte(`{"name":"prod-apne2","accountRef":"missing","region":"ap-northeast-2"}`),
			},
		},
	}

	if _, err := LoadFromGitHub(context.Background(), client, githubcontents.Request{Repository: "acme/platform-config"}); err == nil {
		t.Fatalf("expected manifest compilation error")
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

type stubGitHubManifestClient struct {
	entries map[string][]githubcontents.Entry
	files   map[string]githubcontents.File
}

func (c *stubGitHubManifestClient) List(_ context.Context, req githubcontents.Request) ([]githubcontents.Entry, error) {
	return append([]githubcontents.Entry(nil), c.entries[req.Path]...), nil
}

func (c *stubGitHubManifestClient) Fetch(_ context.Context, req githubcontents.Request) (githubcontents.File, error) {
	return c.files[req.Path], nil
}
