package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
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
	planStorePath := envOrDefault("TENET_PLAN_STORE_PATH", defaultPlanPreviewStorePath())
	redisAddr := os.Getenv("TENET_REDIS_ADDR")
	statusSeedPath := os.Getenv("TENET_STATUS_SEED_PATH")

	info := buildinfo.Current(version, commit)
	log.Printf("starting tenet-api version=%s commit=%s addr=%s repo_root=%s status_backend=%s status_path=%s plan_store_path=%s redis_addr=%s", info.Version, info.Commit, addr, repoRoot, statusBackend, statusPath, planStorePath, redisAddr)

	store := mustOpenStatusStore(statusBackend, statusPath, redisAddr)
	if store != nil {
		if err := statusstore.SeedFromFileIfEmpty(context.Background(), store, statusSeedPath); err != nil {
			log.Fatalf("seed status store: %v", err)
		}
	}
	options := newAPIOptionsWithPlanStore(info, repoRoot, store, planStorePath)

	if err := http.ListenAndServe(addr, api.NewRouter(options)); err != nil {
		log.Fatal(err)
	}
}

func newAPIOptions(info buildinfo.Info, repoRoot string, store statusstore.Store) api.Options {
	return newAPIOptionsWithPlanStore(info, repoRoot, store, envOrDefault("TENET_PLAN_STORE_PATH", defaultPlanPreviewStorePath()))
}

func newAPIOptionsWithPlanStore(info buildinfo.Info, repoRoot string, store statusstore.Store, planStorePath string) api.Options {
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
		options.OverviewApplication = func(name string) (catalog.ApplicationOverviewDetail, error) {
			compiled, err := catalog.LoadFromRepository(repoRoot)
			if err != nil {
				return catalog.ApplicationOverviewDetail{}, err
			}
			if store == nil {
				return catalog.BuildApplicationOverviewDetail(compiled, nil, name)
			}
			snapshot, err := store.Get(context.Background(), name)
			if err != nil {
				if errors.Is(err, statusstore.ErrNotFound) {
					return catalog.BuildApplicationOverviewDetail(compiled, nil, name)
				}
				return catalog.ApplicationOverviewDetail{}, err
			}
			return catalog.BuildApplicationOverviewDetail(compiled, []statusstore.Snapshot{snapshot}, name)
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
		planService := planpreview.Service{
			LoadDetail: options.OverviewDeployment,
			Store:      planpreview.NewFileStore(planStorePath),
		}
		options.CreateDeploymentPlanPreview = planService.Create
		options.PlanPreview = planService.Get
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

func defaultPlanPreviewStorePath() string {
	return filepath.Join(os.TempDir(), "tenet", "plan-previews.json")
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
			{Resource: "ecsService", Name: detail.Name + "-service", Description: canonicalCodeDeployExecutionDescription(detail, "ECS service Tenet updates during CodeDeploy traffic shifting")},
			{Resource: "codeDeployApplication", Name: detail.Name + "-codedeploy-app", Description: canonicalCodeDeployExecutionDescription(detail, "CodeDeploy application coordinating ECS traffic shifting")},
			{Resource: "codeDeployDeploymentGroup", Name: detail.Name + "-codedeploy-group", Description: canonicalCodeDeployExecutionDescription(detail, "CodeDeploy deployment group Tenet targets for ECS rollout")},
			{Resource: "productionListener", Name: detail.Name + "-prod-listener", Description: canonicalCodeDeployExecutionDescription(detail, "Production listener CodeDeploy shifts during ECS traffic shifting")},
			{Resource: "testListener", Name: detail.Name + "-test-listener", Description: canonicalCodeDeployExecutionDescription(detail, "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion")},
			{Resource: "blueTaskSetTargetGroup", Name: detail.Name + "-blue", Description: canonicalCodeDeployExecutionDescription(detail, "Blue task set target group CodeDeploy treats as the live ECS traffic destination")},
			{Resource: "greenTaskSetTargetGroup", Name: detail.Name + "-green", Description: canonicalCodeDeployExecutionDescription(detail, "Green task set target group CodeDeploy warms before shifting ECS traffic")},
		}
	case "ecsRolling":
		return []domain.RolloutExecutionDetail{
			{Resource: "ecsService", Name: detail.Name + "-service", Description: ecsRollingExecutionDescription(detail.RolloutConfig.Strategy)},
		}
	case "asgWeightedTargetGroups":
		return []domain.RolloutExecutionDetail{
			{Resource: "candidateAutoScalingGroup", Name: detail.Name + "-candidate", Description: canonicalWeightedTargetGroupCandidateExecutionDescription(detail)},
			{Resource: "activeTargetGroup", Name: detail.Name + "-active", Description: "Currently active target group receiving production traffic"},
			{Resource: "candidateTargetGroup", Name: detail.Name + "-candidate", Description: "Candidate target group Tenet will register and warm before traffic shifting"},
			{Resource: "productionListenerRule", Name: detail.Name + "-prod-listener", Description: "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout"},
			{Resource: "trafficShiftPlan", Name: detail.Name + "-weighted-routing", Description: canonicalWeightedTargetGroupTrafficShiftDescription(detail)},
			{Resource: "userDataBootstrapSource", Name: canonicalUserDataBootstrapSource(detail.UserData), Description: "Normalized userdata bootstrap source for candidate EC2 capacity"},
		}
	case "asgInstanceRefresh":
		description := "Service Auto Scaling Group Tenet refreshes in place"
		if batchSize := canonicalInstanceRefreshBatchSize(detail.RolloutConfig); batchSize > 0 {
			description += fmt.Sprintf(" with rolling-update batch size %d", batchSize)
		}
		return []domain.RolloutExecutionDetail{
			{Resource: "serviceAutoScalingGroup", Name: detail.Name + "-service", Description: description},
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

func ecsRollingExecutionDescription(strategy domain.DeploymentRolloutStrategy) string {
	description := "ECS service Tenet updates with native rolling deployment"
	if qualifiers := rolloutStrategyQualifiers(strategy); len(qualifiers) > 0 {
		description += " while respecting " + strings.Join(qualifiers, " and ")
	}
	return description
}

func canonicalWeightedTargetGroupCandidateExecutionDescription(detail catalog.DeploymentOverviewDetail) string {
	capacity := detail.RolloutConfig.Capacity
	description := fmt.Sprintf(
		"Candidate Auto Scaling Group Tenet creates from normalized userdata source %s with desired %d, min %d, and max %d instances",
		canonicalUserDataBootstrapSource(detail.UserData),
		capacity.Desired,
		capacity.Min,
		capacity.Max,
	)
	if warmup := detail.RolloutConfig.Strategy.WarmupSeconds; warmup > 0 {
		description += fmt.Sprintf(" and warmup %ds", warmup)
	}
	return description + " for weighted target group rollout"
}

func canonicalWeightedTargetGroupTrafficShiftDescription(detail catalog.DeploymentOverviewDetail) string {
	var description string
	switch detail.Strategy {
	case "canary":
		description = "Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion"
	case "linear":
		description = "Weighted routing plan Tenet follows to advance production traffic from the active target group to the candidate target group in linear increments"
	default:
		description = "Weighted routing plan Tenet follows to cut over all production traffic from the active target group to the candidate target group in a single blue/green promotion"
	}
	if qualifiers := rolloutStrategyQualifiers(detail.RolloutConfig.Strategy); len(qualifiers) > 0 {
		description += " while respecting " + strings.Join(qualifiers, " and ")
	}
	return description
}

func canonicalCodeDeployExecutionDescription(detail catalog.DeploymentOverviewDetail, base string) string {
	style := detail.Strategy
	if style == "" {
		style = "blueGreen"
	}
	description := fmt.Sprintf("%s (style: %s", base, style)
	if qualifiers := rolloutStrategyQualifiers(detail.RolloutConfig.Strategy); len(qualifiers) > 0 {
		description += "; qualifiers: " + strings.Join(qualifiers, " and ")
	}
	return description + ")"
}

func canonicalInstanceRefreshBatchSize(config domain.DeploymentRolloutConfig) int {
	for _, stack := range config.Stacks {
		if stack.RollingUpdateInstanceCount > 0 {
			return stack.RollingUpdateInstanceCount
		}
	}
	return 0
}

func rolloutStrategyQualifiers(strategy domain.DeploymentRolloutStrategy) []string {
	qualifiers := make([]string, 0, 2)
	if strategy.MinHealthyPercent > 0 {
		qualifiers = append(qualifiers, fmt.Sprintf("min healthy %d%%", strategy.MinHealthyPercent))
	}
	if strategy.WarmupSeconds > 0 {
		qualifiers = append(qualifiers, fmt.Sprintf("warmup %ds", strategy.WarmupSeconds))
	}
	return qualifiers
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
