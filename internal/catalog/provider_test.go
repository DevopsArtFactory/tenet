package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
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

func TestRepositoryAccountProviderLoadsStableAccountListing(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2","us-west-2"]}`)
	writeJSON(t, filepath.Join(root, "accounts", "dev-core.json"), `{"name":"dev-core","awsAccountId":"123456789013","roleArn":"arn:aws:iam::123456789013:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	if err := os.Mkdir(filepath.Join(root, "environments"), 0o755); err != nil {
		t.Fatalf("failed to create environments dir: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "deployments"), 0o755); err != nil {
		t.Fatalf("failed to create deployments dir: %v", err)
	}

	provider := RepositoryAccountProvider(root)
	accounts, err := provider()
	if err != nil {
		t.Fatalf("provider returned error: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected two accounts, got %+v", accounts)
	}
	if accounts[0].Name != "dev-core" || accounts[1].Name != "prod-core" {
		t.Fatalf("expected sorted account list, got %+v", accounts)
	}
}

func TestRepositoryPlacementProviderLoadsStablePlacementListing(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "worker-prod.json"), `{"name":"worker-prod","appRef":"payments-worker","environmentRef":"prod-apne2","target":{"type":"ecs"},"strategy":{"type":"rolling"},"traffic":{"port":8080},"capacity":{"desired":1,"min":1,"max":1}}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)

	provider := RepositoryPlacementProvider(root)
	placements, err := provider()
	if err != nil {
		t.Fatalf("provider returned error: %v", err)
	}
	if len(placements) != 2 {
		t.Fatalf("expected two placements, got %+v", placements)
	}
	if placements[0].DeploymentName != "api-prod" || placements[1].DeploymentName != "worker-prod" {
		t.Fatalf("expected sorted placement list, got %+v", placements)
	}
}

func TestRepositoryTopologyProviderLoadsStableTopologySnapshot(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)

	provider := RepositoryTopologyProvider(root)
	topology, err := provider()
	if err != nil {
		t.Fatalf("provider returned error: %v", err)
	}
	if topology.Summary.DeploymentCount != 1 || len(topology.Placements) != 1 {
		t.Fatalf("unexpected topology: %+v", topology)
	}
}

func TestRepositoryAccountOverviewDetailProvider(t *testing.T) {
	t.Run("loads account detail", func(t *testing.T) {
		root := t.TempDir()
		writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2","us-west-2"]}`)
		writeJSON(t, filepath.Join(root, "accounts", "shadow.json"), `{"name":"123456789012","awsAccountId":"210987654321","roleArn":"arn:aws:iam::210987654321:role/TenetDeployerRole","allowedRegions":["eu-west-1"]}`)
		writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2"}`)
		writeJSON(t, filepath.Join(root, "environments", "shadow-euw1.json"), `{"name":"shadow-euw1","accountRef":"123456789012","region":"eu-west-1"}`)
		writeJSON(t, filepath.Join(root, "deployments", "payments-api-prod.json"), `{"name":"payments-api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"canary"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
		writeJSON(t, filepath.Join(root, "deployments", "shadow-api-prod.json"), `{"name":"shadow-api-prod","appRef":"shadow-api","environmentRef":"shadow-euw1","target":{"type":"ecs"},"strategy":{"type":"rolling"},"traffic":{"port":8080},"capacity":{"desired":1,"min":1,"max":1}}`)

		provider := RepositoryAccountOverviewDetailProvider(root, func() ([]statusstore.Snapshot, error) {
			return []statusstore.Snapshot{{
				DeploymentName: "payments-api-prod",
				Status:         domain.DeploymentStatus{Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "stable"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "1/1 healthy"}},
			}}, nil
		})

		detail, err := provider("123456789012")
		if err != nil {
			t.Fatalf("provider returned error: %v", err)
		}
		if detail.Name != "prod-core" || detail.AWSAccountID != "123456789012" || detail.DeploymentCount != 1 || len(detail.Deployments) != 1 {
			t.Fatalf("unexpected detail: %+v", detail)
		}
	})

	t.Run("returns compile error for duplicate aws account ids", func(t *testing.T) {
		root := t.TempDir()
		writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
		writeJSON(t, filepath.Join(root, "accounts", "prod-shadow.json"), `{"name":"prod-shadow","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetShadowRole","allowedRegions":["us-west-2"]}`)
		writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2"}`)
		writeJSON(t, filepath.Join(root, "environments", "prod-usw2.json"), `{"name":"prod-usw2","accountRef":"prod-shadow","region":"us-west-2"}`)
		writeJSON(t, filepath.Join(root, "deployments", "payments-api-prod.json"), `{"name":"payments-api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"canary"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
		writeJSON(t, filepath.Join(root, "deployments", "shadow-api-prod.json"), `{"name":"shadow-api-prod","appRef":"shadow-api","environmentRef":"prod-usw2","target":{"type":"ecs"},"strategy":{"type":"rolling"},"traffic":{"port":8080},"capacity":{"desired":1,"min":1,"max":1}}`)

		provider := RepositoryAccountOverviewDetailProvider(root, func() ([]statusstore.Snapshot, error) {
			return nil, nil
		})

		_, err := provider("123456789012")
		if err == nil || !strings.Contains(err.Error(), "duplicate AWS account ID \"123456789012\"") {
			t.Fatalf("expected duplicate AWS account ID compile error, got %v", err)
		}
		if errors.Is(err, errAmbiguousAccountOverviewDetail) || errors.Is(err, statusstore.ErrNotFound) {
			t.Fatalf("expected compile-time duplicate account ID failure to stay distinct, got %v", err)
		}
	})

	t.Run("propagates snapshot errors", func(t *testing.T) {
		root := t.TempDir()
		writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole"}`)
		if err := os.Mkdir(filepath.Join(root, "environments"), 0o755); err != nil {
			t.Fatalf("failed to create environments dir: %v", err)
		}
		if err := os.Mkdir(filepath.Join(root, "deployments"), 0o755); err != nil {
			t.Fatalf("failed to create deployments dir: %v", err)
		}

		expected := errors.New("snapshots unavailable")
		provider := RepositoryAccountOverviewDetailProvider(root, func() ([]statusstore.Snapshot, error) {
			return nil, expected
		})

		_, err := provider("123456789012")
		if !errors.Is(err, expected) {
			t.Fatalf("expected snapshot error, got %v", err)
		}
	})
}
