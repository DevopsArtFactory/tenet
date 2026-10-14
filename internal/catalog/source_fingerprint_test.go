package catalog

import (
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"testing"
)

func TestPinnedECSFingerprintBindsDesiredIntentWithoutRuntimeTimestampChurn(t *testing.T) {
	detail := DeploymentOverviewDetail{Name: "worker", AppRef: "worker", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "ap-northeast-2", Git: &domain.GitSource{Repository: "org/desired", Ref: "main", Revision: "sha-a"}, ECS: &domain.ECSDeploymentIntent{Image: "example/worker:v1", DesiredCount: 2}}
	original, err := DeriveDeploymentSourceFingerprint(detail)
	if err != nil {
		t.Fatal(err)
	}
	detail.RuntimeSummary = &domain.RuntimeSummary{Timestamp: "new observation"}
	detail.Health.Overall = domain.HealthUnhealthy
	same, err := DeriveDeploymentSourceFingerprint(detail)
	if err != nil || same != original {
		t.Fatal("runtime-only refresh invalidated pinned Git approval")
	}
	for _, name := range []string{"commit", "intent", "placement", "source"} {
		t.Run(name, func(t *testing.T) {
			changed := detail
			changed.Git = detail.Git.Clone()
			changed.ECS = detail.ECS.Clone()
			switch name {
			case "commit":
				changed.Git.Revision = "sha-b"
			case "intent":
				changed.ECS.Image = "example/worker:v2"
			case "placement":
				changed.EnvironmentRef = "other"
			case "source":
				changed.Git.Repository = "other/repo"
			}
			hash, err := DeriveDeploymentSourceFingerprint(changed)
			if err != nil || hash == original {
				t.Fatal("changed reviewed inputs preserved approval")
			}
		})
	}
}
