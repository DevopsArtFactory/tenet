package manifestgithub

import (
	"context"
	"errors"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
)

func TestLoadReadsAccountEnvironmentAndDeploymentFiles(t *testing.T) {
	t.Parallel()

	client := &stubClient{
		entries: map[string][]githubcontents.Entry{
			"catalog/accounts": {
				{Name: "README.md", Path: "catalog/accounts/README.md", Type: "file"},
				{Name: "nested", Path: "catalog/accounts/nested", Type: "dir"},
				{Name: "prod-core.json", Path: "catalog/accounts/prod-core.json", Type: "file"},
			},
			"catalog/environments": {
				{Name: "prod-apne2.json", Path: "catalog/environments/prod-apne2.json", Type: "file"},
			},
			"catalog/deployments": {
				{Name: "payments-api-prod.yaml", Path: "catalog/deployments/payments-api-prod.yaml", Type: "file"},
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
			"catalog/deployments/payments-api-prod.yaml": {
				Path:    "catalog/deployments/payments-api-prod.yaml",
				Content: []byte("name: payments-api-prod\nappRef: payments-api\nenvironmentRef: prod-apne2\ntarget:\n  type: ec2\nstrategy:\n  type: blueGreen\ntraffic:\n  port: 443\ncapacity:\n  desired: 1\n  min: 1\n  max: 1\n"),
			},
		},
	}

	catalog, err := Load(context.Background(), client, githubcontents.Request{
		Repository: "acme/platform-config",
		Ref:        "main",
		Path:       "catalog",
		Token:      "secret-token",
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(catalog.Accounts) != 1 || len(catalog.Environments) != 1 || len(catalog.Deployments) != 1 {
		t.Fatalf("unexpected catalog contents: %+v", catalog)
	}
	if len(client.listRequests) != 3 {
		t.Fatalf("unexpected list request count: %+v", client.listRequests)
	}
	if len(client.fetchRequests) != 3 {
		t.Fatalf("unexpected fetch request count: %+v", client.fetchRequests)
	}
	for _, req := range append(append([]githubcontents.Request{}, client.listRequests...), client.fetchRequests...) {
		if req.Repository != "acme/platform-config" || req.Ref != "main" || req.Token != "secret-token" {
			t.Fatalf("request metadata was not preserved: %+v", req)
		}
	}
}

func TestLoadSupportsRepositoryRoot(t *testing.T) {
	t.Parallel()

	client := &stubClient{
		entries: map[string][]githubcontents.Entry{
			"accounts":     {{Name: "prod-core.json", Path: "accounts/prod-core.json", Type: "file"}},
			"environments": {{Name: "prod-apne2.json", Path: "environments/prod-apne2.json", Type: "file"}},
			"deployments":  {{Name: "payments-api-prod.json", Path: "deployments/payments-api-prod.json", Type: "file"}},
		},
		files: map[string]githubcontents.File{
			"accounts/prod-core.json": {
				Path:    "accounts/prod-core.json",
				Content: []byte(`{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`),
			},
			"environments/prod-apne2.json": {
				Path:    "environments/prod-apne2.json",
				Content: []byte(`{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`),
			},
			"deployments/payments-api-prod.json": {
				Path:    "deployments/payments-api-prod.json",
				Content: []byte(`{"name":"payments-api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`),
			},
		},
	}

	catalog, err := Load(context.Background(), client, githubcontents.Request{Repository: "acme/platform-config"})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(catalog.Accounts) != 1 || len(catalog.Environments) != 1 || len(catalog.Deployments) != 1 {
		t.Fatalf("unexpected catalog contents: %+v", catalog)
	}
}

func TestLoadRejectsMissingRequiredDirectories(t *testing.T) {
	t.Parallel()

	client := &stubClient{
		entries: map[string][]githubcontents.Entry{
			"accounts":     {},
			"environments": {},
		},
		listErr: map[string]error{
			"deployments": &githubcontents.Error{Code: githubcontents.ErrorCodeNotFound, Message: `GitHub desired-config directory "deployments" was not found`},
		},
	}

	if _, err := Load(context.Background(), client, githubcontents.Request{Repository: "acme/platform-config"}); err == nil {
		t.Fatal("expected missing required directory error")
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	client := &stubClient{
		entries: map[string][]githubcontents.Entry{
			"accounts":     {{Name: "broken.json", Path: "accounts/broken.json", Type: "file"}},
			"environments": {},
			"deployments":  {},
		},
		files: map[string]githubcontents.File{
			"accounts/broken.json": {
				Path:    "accounts/broken.json",
				Content: []byte(`{not-json}`),
			},
		},
	}

	if _, err := Load(context.Background(), client, githubcontents.Request{Repository: "acme/platform-config"}); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

type stubClient struct {
	entries       map[string][]githubcontents.Entry
	files         map[string]githubcontents.File
	listErr       map[string]error
	fetchErr      map[string]error
	listRequests  []githubcontents.Request
	fetchRequests []githubcontents.Request
}

func (c *stubClient) List(_ context.Context, req githubcontents.Request) ([]githubcontents.Entry, error) {
	c.listRequests = append(c.listRequests, req)
	if err, ok := c.listErr[req.Path]; ok {
		return nil, err
	}
	entries, ok := c.entries[req.Path]
	if !ok {
		return nil, &githubcontents.Error{Code: githubcontents.ErrorCodeNotFound, Message: "missing"}
	}
	return append([]githubcontents.Entry(nil), entries...), nil
}

func (c *stubClient) Fetch(_ context.Context, req githubcontents.Request) (githubcontents.File, error) {
	c.fetchRequests = append(c.fetchRequests, req)
	if err, ok := c.fetchErr[req.Path]; ok {
		return githubcontents.File{}, err
	}
	file, ok := c.files[req.Path]
	if !ok {
		return githubcontents.File{}, errors.New("missing file")
	}
	return file, nil
}
