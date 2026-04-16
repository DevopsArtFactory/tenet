package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/DevopsArtFactory/tenet/internal/resourcelogs"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
	"github.com/redis/go-redis/v9"
)

type resourceLogProvider interface {
	Recent(context.Context, resourcelogs.Request) (resourcelogs.Result, error)
}

var newResourceLogProvider = func() resourceLogProvider {
	return resourcelogs.NewService(resourcelogs.Options{EnableAWS: true})
}

func main() {
	version := envOrDefault("TENET_VERSION", "dev")
	commit := envOrDefault("TENET_COMMIT", "unknown")
	addr := envOrDefault("TENET_API_ADDR", ":8080")
	repoRoot := os.Getenv("TENET_REPO_ROOT")
	statusBackend := envOrDefault("TENET_STATUS_BACKEND", "file")
	statusPath := os.Getenv("TENET_STATUS_PATH")
	redisAddr := os.Getenv("TENET_REDIS_ADDR")
	statusSeedPath := os.Getenv("TENET_STATUS_SEED_PATH")

	info := buildinfo.Current(version, commit)
	log.Printf("starting tenet-api version=%s commit=%s addr=%s repo_root=%s status_backend=%s status_path=%s redis_addr=%s", info.Version, info.Commit, addr, repoRoot, statusBackend, statusPath, redisAddr)

	store := mustOpenStatusStore(statusBackend, statusPath, redisAddr)
	if store != nil {
		if err := statusstore.SeedFromFileIfEmpty(context.Background(), store, statusSeedPath); err != nil {
			log.Fatalf("seed status store: %v", err)
		}
	}
	options := newAPIOptions(info, repoRoot, store)

	if err := http.ListenAndServe(addr, api.NewRouter(options)); err != nil {
		log.Fatal(err)
	}
}

func newAPIOptions(info buildinfo.Info, repoRoot string, store statusstore.Store) api.Options {
	options := api.Options{BuildInfo: info}
	if repoRoot != "" {
		if _, err := catalog.LoadFromRepository(repoRoot); err != nil {
			log.Fatalf("invalid TENET_REPO_ROOT: %v", err)
		}
		options.CatalogSummary = catalog.RepositorySummaryProvider(repoRoot)
		options.CatalogDeployments = catalog.RepositoryDeploymentProvider(repoRoot)
		options.CatalogEnvironments = catalog.RepositoryEnvironmentProvider(repoRoot)
		options.CatalogAccounts = catalog.RepositoryAccountProvider(repoRoot)
		options.CatalogPlacements = catalog.RepositoryPlacementProvider(repoRoot)
		options.CatalogTopology = catalog.RepositoryTopologyProvider(repoRoot)
		resourceLogService := newResourceLogProvider()
		loadSnapshots := func() ([]statusstore.Snapshot, error) {
			if store == nil {
				return nil, nil
			}
			return store.List(context.Background())
		}
		loadObserverRoleARN := func(targetAccountID string) (string, error) {
			catalogSpec, err := manifestfs.Load(repoRoot)
			if err != nil {
				return "", err
			}
			for _, account := range catalogSpec.Accounts {
				if account.AWSAccountID == targetAccountID {
					return account.ObserverRoleARN, nil
				}
			}
			return "", nil
		}
		options.OverviewApplications = func() ([]catalog.ApplicationOverview, error) {
			compiled, err := catalog.LoadFromRepository(repoRoot)
			if err != nil {
				return nil, err
			}
			snapshots, err := loadSnapshots()
			if err != nil {
				return nil, err
			}
			return catalog.BuildApplicationOverviews(compiled, snapshots)
		}
		options.OverviewAccounts = func() ([]catalog.AccountOverview, error) {
			compiled, err := catalog.LoadFromRepository(repoRoot)
			if err != nil {
				return nil, err
			}
			snapshots, err := loadSnapshots()
			if err != nil {
				return nil, err
			}
			return catalog.BuildAccountOverviews(compiled, snapshots)
		}
		options.OverviewAccount = catalog.RepositoryAccountOverviewDetailProvider(repoRoot, loadSnapshots)
		options.OverviewProjectEnvironment = catalog.RepositoryProjectEnvironmentOverviewDetailProvider(repoRoot, loadSnapshots)
		options.OverviewDeployment = func(name string) (catalog.DeploymentOverviewDetail, error) {
			compiled, err := catalog.LoadFromRepository(repoRoot)
			if err != nil {
				return catalog.DeploymentOverviewDetail{}, err
			}
			if store == nil {
				return catalog.BuildDeploymentOverviewDetail(compiled, nil, name)
			}
			snapshot, err := store.Get(context.Background(), name)
			if err != nil {
				if errors.Is(err, statusstore.ErrNotFound) {
					return catalog.BuildDeploymentOverviewDetail(compiled, nil, name)
				}
				return catalog.DeploymentOverviewDetail{}, err
			}
			return catalog.BuildDeploymentOverviewDetail(compiled, []statusstore.Snapshot{snapshot}, name)
		}
		options.OverviewDeploymentResourceLogs = newOverviewDeploymentResourceLogsProvider(options.OverviewDeployment, loadObserverRoleARN, resourceLogService)
	}
	if store == nil {
		return options
	}
	options.DeploymentStatuses = func() ([]statusstore.Snapshot, error) {
		return store.List(context.Background())
	}
	options.DeploymentStatus = func(name string) (statusstore.Snapshot, error) {
		return store.Get(context.Background(), name)
	}
	return options
}

func newOverviewDeploymentResourceLogsProvider(
	loadDetail func(string) (catalog.DeploymentOverviewDetail, error),
	loadObserverRoleARN func(string) (string, error),
	provider resourceLogProvider,
) func(context.Context, string, string, string) (resourcelogs.Result, error) {
	return func(ctx context.Context, name, resourceType, resourceName string) (resourcelogs.Result, error) {
		detail, err := loadDetail(name)
		if err != nil {
			return resourcelogs.Result{}, err
		}
		if !deploymentHasExecutionResource(detail, resourceType, resourceName) {
			return resourcelogs.Result{}, fmt.Errorf("%w: deployment %s resource %s/%s", statusstore.ErrNotFound, name, resourceType, resourceName)
		}
		observerRoleARN, err := loadObserverRoleARN(detail.AWSAccountID)
		if err != nil {
			return resourcelogs.Result{}, err
		}
		return provider.Recent(ctx, resourcelogs.Request{
			DeploymentName:  name,
			ResourceType:    resourceType,
			ResourceName:    resourceName,
			TargetAccountID: detail.AWSAccountID,
			Region:          detail.Region,
			ObserverRoleARN: observerRoleARN,
		})
	}
}

func deploymentHasExecutionResource(detail catalog.DeploymentOverviewDetail, resourceType, resourceName string) bool {
	for _, resource := range rolloutExecutionResources(detail) {
		if resource.Resource == resourceType && resource.Name == resourceName {
			return true
		}
	}
	return false
}

func rolloutExecutionResources(detail catalog.DeploymentOverviewDetail) []domain.RolloutExecutionDetail {
	if detail.Rollout != nil && len(detail.Rollout.Execution) > 0 {
		return append([]domain.RolloutExecutionDetail(nil), detail.Rollout.Execution...)
	}
	return canonicalRolloutExecutionFallback(detail)
}

func canonicalRolloutExecutionFallback(detail catalog.DeploymentOverviewDetail) []domain.RolloutExecutionDetail {
	switch detail.ExecutionMode {
	case "ecsCodeDeploy":
		return []domain.RolloutExecutionDetail{
			{Resource: "ecsService", Name: detail.Name + "-service", Description: "ECS service Tenet updates during deployment"},
			{Resource: "codeDeployApplication", Name: detail.Name + "-codedeploy-app", Description: "CodeDeploy application coordinating ECS traffic shifting"},
			{Resource: "codeDeployDeploymentGroup", Name: detail.Name + "-codedeploy-group", Description: "CodeDeploy deployment group Tenet targets for ECS rollout"},
			{Resource: "productionListener", Name: detail.Name + "-prod-listener", Description: "Production listener CodeDeploy shifts during ECS blue/green rollout"},
			{Resource: "testListener", Name: detail.Name + "-test-listener", Description: "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion"},
			{Resource: "blueTaskSetTargetGroup", Name: detail.Name + "-blue", Description: "Blue task set target group CodeDeploy treats as the live ECS traffic destination"},
			{Resource: "greenTaskSetTargetGroup", Name: detail.Name + "-green", Description: "Green task set target group CodeDeploy warms before shifting ECS traffic"},
		}
	case "asgWeightedTargetGroups":
		return []domain.RolloutExecutionDetail{
			{Resource: "candidateAutoScalingGroup", Name: detail.Name + "-candidate", Description: "Candidate Auto Scaling Group Tenet will create for weighted target group rollout"},
			{Resource: "activeTargetGroup", Name: detail.Name + "-active", Description: "Currently active target group receiving production traffic"},
			{Resource: "candidateTargetGroup", Name: detail.Name + "-candidate", Description: "Candidate target group Tenet will register and warm before traffic shifting"},
			{Resource: "productionListenerRule", Name: detail.Name + "-prod-listener", Description: "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout"},
			{Resource: "trafficShiftPlan", Name: detail.Name + "-weighted-routing", Description: "Weighted routing plan Tenet follows to move production traffic from the active target group to the candidate target group"},
			{Resource: "userDataBootstrapSource", Name: canonicalUserDataBootstrapSource(detail.UserData), Description: "Normalized userdata bootstrap source for candidate EC2 capacity"},
		}
	case "asgInstanceRefresh":
		return []domain.RolloutExecutionDetail{
			{Resource: "serviceAutoScalingGroup", Name: detail.Name + "-service", Description: "Service Auto Scaling Group Tenet refreshes in place"},
			{Resource: "userDataBootstrapSource", Name: canonicalUserDataBootstrapSource(detail.UserData), Description: "Normalized userdata bootstrap source for refreshed EC2 instances"},
		}
	default:
		return nil
	}
}

func canonicalUserDataBootstrapSource(userData domain.DeploymentUserData) string {
	if userData.Type == "" && userData.Path == "" {
		return "unspecified"
	}
	if userData.Path == "" {
		return userData.Type
	}
	if userData.Type == "" {
		return userData.Path
	}
	return userData.Type + ":" + userData.Path
}

func mustOpenStatusStore(backend, path, redisAddr string) statusstore.Store {
	switch backend {
	case "", "file":
		if path == "" {
			return nil
		}
		return statusstore.NewFileStore(path)
	case "redis":
		if redisAddr == "" {
			log.Fatal("TENET_REDIS_ADDR is required when TENET_STATUS_BACKEND=redis")
		}
		client := redis.NewClient(&redis.Options{Addr: redisAddr})
		if err := client.Ping(context.Background()).Err(); err != nil {
			log.Fatalf("connect redis status store: %v", err)
		}
		return statusstore.NewRedisStore(client, os.Getenv("TENET_REDIS_KEY_PREFIX"))
	default:
		log.Fatalf("unsupported TENET_STATUS_BACKEND %q", backend)
		return nil
	}
}

func envOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
