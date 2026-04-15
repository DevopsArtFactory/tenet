package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositorySummaryProviderLoadsAndSummarizesRepository(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole"}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2"}`)
	writeJSON(t, filepath.Join(root, "deployments", "payments-api-prod.json"), `{"name":"payments-api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)

	provider := RepositorySummaryProvider(root)
	summary, err := provider()
	if err != nil {
		t.Fatalf("provider returned error: %v", err)
	}
	if summary.AccountCount != 1 || summary.EnvironmentCount != 1 || summary.DeploymentCount != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestRepositorySummaryProviderPropagatesRepositoryErrors(t *testing.T) {
	provider := RepositorySummaryProvider(filepath.Join(t.TempDir(), "missing"))

	_, err := provider()
	if err == nil {
		t.Fatalf("expected provider error")
	}
	if !strings.Contains(err.Error(), "load repository manifests") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepositoryDeploymentProviderLoadsStableDeploymentListing(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole"}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2"}`)
	writeJSON(t, filepath.Join(root, "deployments", "worker-prod.json"), `{"name":"worker-prod","appRef":"payments-worker","environmentRef":"prod-apne2","target":{"type":"ecs"},"strategy":{"type":"rolling"},"traffic":{"port":8080},"capacity":{"desired":1,"min":1,"max":1}}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)

	provider := RepositoryDeploymentProvider(root)
	deployments, err := provider()
	if err != nil {
		t.Fatalf("provider returned error: %v", err)
	}
	if len(deployments) != 2 {
		t.Fatalf("expected two deployments, got %+v", deployments)
	}
	if deployments[0].Name != "api-prod" || deployments[1].Name != "worker-prod" {
		t.Fatalf("expected sorted deployment list, got %+v", deployments)
	}
}

func TestRepositoryEnvironmentProviderLoadsStableEnvironmentListing(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole"}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-usw2.json"), `{"name":"prod-usw2","accountRef":"prod-core","region":"us-west-2","networkRef":"prod-west"}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	if err := os.Mkdir(filepath.Join(root, "deployments"), 0o755); err != nil {
		t.Fatalf("failed to create deployments dir: %v", err)
	}

	provider := RepositoryEnvironmentProvider(root)
	environments, err := provider()
	if err != nil {
		t.Fatalf("provider returned error: %v", err)
	}
	if len(environments) != 2 {
		t.Fatalf("expected two environments, got %+v", environments)
	}
	if environments[0].Name != "prod-apne2" || environments[1].Name != "prod-usw2" {
		t.Fatalf("expected sorted environment list, got %+v", environments)
	}
}
