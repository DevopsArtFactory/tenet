package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/auditlog"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/resourceinspection"
	"github.com/DevopsArtFactory/tenet/internal/resourcelogs"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
	"github.com/redis/go-redis/v9"
)

type resourceLogProvider interface {
	Recent(context.Context, resourcelogs.Request) (resourcelogs.Result, error)
}

type resourceInspectionProvider interface {
	Inspect(context.Context, resourceinspection.Request) (resourceinspection.Result, error)
}

var newResourceLogProvider = func() resourceLogProvider {
	return resourcelogs.NewService(resourcelogs.Options{EnableAWS: true})
}

var newResourceInspectionProvider = func() resourceInspectionProvider {
	return resourceinspection.NewService(resourceinspection.Options{EnableAWS: true})
}

func main() {
	version := envOrDefault("TENET_VERSION", "dev")
	commit := envOrDefault("TENET_COMMIT", "unknown")
	addr := envOrDefault("TENET_API_ADDR", ":8080")
	statusBackend := envOrDefault("TENET_STATUS_BACKEND", "file")
	statusPath := os.Getenv("TENET_STATUS_PATH")
	planStorePath := envOrDefault("TENET_PLAN_STORE_PATH", defaultPlanPreviewStorePath())
	applyStorePath := envOrDefault("TENET_APPLY_STORE_PATH", defaultApplyRequestStorePath())
	applyEnabled := awsApplyEnabled()
	webhookConfigured := strings.TrimSpace(os.Getenv("TENET_GITHUB_WEBHOOK_SECRET")) != ""
	redisAddr := os.Getenv("TENET_REDIS_ADDR")
	statusSeedPath := os.Getenv("TENET_STATUS_SEED_PATH")
	catalogSource, err := loadCatalogSourceFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if err := validateCatalogSource(catalogSource, applyEnabled); err != nil {
		log.Fatal(err)
	}
	requestTimeout, err := requestTimeoutFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	info := buildinfo.Current(version, commit)
	catalogSourceKind, repoRoot, githubRepository, githubRef, githubPath, githubBaseURL := catalogSource.logFields()
	log.Printf("starting tenet-api version=%s commit=%s addr=%s catalog_source=%s repo_root=%s github_repository=%s github_ref=%s github_path=%s github_base_url=%s status_backend=%s status_path=%s plan_store_path=%s apply_store_path=%s apply_enabled=%t github_webhook_configured=%t redis_addr=%s request_timeout=%s", info.Version, info.Commit, addr, catalogSourceKind, repoRoot, githubRepository, githubRef, githubPath, githubBaseURL, statusBackend, statusPath, planStorePath, applyStorePath, applyEnabled, webhookConfigured, redisAddr, requestTimeout)

	store := mustOpenStatusStore(statusBackend, statusPath, redisAddr)
	if store != nil {
		if err := statusstore.SeedFromFileIfEmpty(context.Background(), store, statusSeedPath); err != nil {
			log.Fatalf("seed status store: %v", err)
		}
	}
	options := newAPIOptionsWithCatalogSourceAndStores(info, catalogSource, store, planStorePath, applyStorePath)
	options.Logger = logger
	options.RequestTimeout = requestTimeout

	stopApplyExecutor, applyExecutorStarted := startApplyRequestExecutor(logger, repoRoot, applyEnabled, applyStorePath, 0)
	if applyExecutorStarted {
		defer stopApplyExecutor()
	}

	if err := http.ListenAndServe(addr, api.NewRouter(options)); err != nil {
		log.Fatal(err)
	}
}

func newAPIOptions(info buildinfo.Info, repoRoot string, store statusstore.Store) api.Options {
	return newAPIOptionsWithCatalogSourceAndStores(
		info,
		newFilesystemCatalogSource(repoRoot),
		store,
		envOrDefault("TENET_PLAN_STORE_PATH", defaultPlanPreviewStorePath()),
		envOrDefault("TENET_APPLY_STORE_PATH", defaultApplyRequestStorePath()),
	)
}

func newAPIOptionsWithPlanStore(info buildinfo.Info, repoRoot string, store statusstore.Store, planStorePath string) api.Options {
	return newAPIOptionsWithCatalogSourceAndStores(info, newFilesystemCatalogSource(repoRoot), store, planStorePath, envOrDefault("TENET_APPLY_STORE_PATH", defaultApplyRequestStorePath()))
}

func newAPIOptionsWithCatalogSource(info buildinfo.Info, source apiCatalogSource, store statusstore.Store) api.Options {
	return newAPIOptionsWithCatalogSourceAndStores(
		info,
		source,
		store,
		envOrDefault("TENET_PLAN_STORE_PATH", defaultPlanPreviewStorePath()),
		envOrDefault("TENET_APPLY_STORE_PATH", defaultApplyRequestStorePath()),
	)
}

func newAPIOptionsWithCatalogSourceAndStores(info buildinfo.Info, source apiCatalogSource, store statusstore.Store, planStorePath string, applyStorePath string) api.Options {
	options := api.Options{
		BuildInfo:                       info,
		GitHubWebhookSecret:             strings.TrimSpace(os.Getenv("TENET_GITHUB_WEBHOOK_SECRET")),
		GitHubWebhookDesiredStateSource: source.webhookDesiredStateSource(),
	}
	if source.configured() {
		loadCatalog := source.loadCatalog
		resourceLogService := newResourceLogProvider()
		resourceInspectionService := newResourceInspectionProvider()
		loadSnapshots := func(ctx context.Context) ([]statusstore.Snapshot, error) {
			if store == nil {
				return nil, nil
			}
			return store.List(ctx)
		}
		loadObserverRoleARN := func(ctx context.Context, targetAccountID string) (string, error) {
			catalogSpec, err := source.loadSpec(ctx)
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
		options.CatalogSummary = func(ctx context.Context) (catalog.Summary, error) {
			compiled, err := loadCatalog(ctx)
			if err != nil {
				return catalog.Summary{}, err
			}
			return catalog.Summarize(compiled), nil
		}
		options.CatalogDeployments = func(ctx context.Context) ([]catalog.DeploymentView, error) {
			compiled, err := loadCatalog(ctx)
			if err != nil {
				return nil, err
			}
			return catalog.ListDeployments(compiled), nil
		}
		options.CatalogEnvironments = func(ctx context.Context) ([]catalog.EnvironmentView, error) {
			compiled, err := loadCatalog(ctx)
			if err != nil {
				return nil, err
			}
			return catalog.ListEnvironments(compiled), nil
		}
		options.CatalogAccounts = func(ctx context.Context) ([]catalog.AccountView, error) {
			compiled, err := loadCatalog(ctx)
			if err != nil {
				return nil, err
			}
			return catalog.ListAccounts(compiled), nil
		}
		options.CatalogPlacements = func(ctx context.Context) ([]catalog.PlacementView, error) {
			compiled, err := loadCatalog(ctx)
			if err != nil {
				return nil, err
			}
			return catalog.ListPlacements(compiled)
		}
		options.CatalogTopology = func(ctx context.Context) (catalog.TopologySnapshot, error) {
			compiled, err := loadCatalog(ctx)
			if err != nil {
				return catalog.TopologySnapshot{}, err
			}
			return catalog.BuildTopology(compiled)
		}
		options.OverviewApplications = func(ctx context.Context) ([]catalog.ApplicationOverview, error) {
			compiled, err := loadCatalog(ctx)
			if err != nil {
				return nil, err
			}
			snapshots, err := loadSnapshots(ctx)
			if err != nil {
				return nil, err
			}
			return catalog.BuildApplicationOverviews(compiled, snapshots)
		}
		options.OverviewApplication = func(ctx context.Context, name string) (catalog.ApplicationOverviewDetail, error) {
			compiled, err := loadCatalog(ctx)
			if err != nil {
				return catalog.ApplicationOverviewDetail{}, err
			}
			if store == nil {
				return catalog.BuildApplicationOverviewDetail(compiled, nil, name)
			}
			snapshot, err := store.Get(ctx, name)
			if err != nil {
				if errors.Is(err, statusstore.ErrNotFound) {
					return catalog.BuildApplicationOverviewDetail(compiled, nil, name)
				}
				return catalog.ApplicationOverviewDetail{}, err
			}
			return catalog.BuildApplicationOverviewDetail(compiled, []statusstore.Snapshot{snapshot}, name)
		}
		options.OverviewAccounts = func(ctx context.Context) ([]catalog.AccountOverview, error) {
			compiled, err := loadCatalog(ctx)
			if err != nil {
				return nil, err
			}
			snapshots, err := loadSnapshots(ctx)
			if err != nil {
				return nil, err
			}
			return catalog.BuildAccountOverviews(compiled, snapshots)
		}
		options.OverviewAccount = func(ctx context.Context, awsAccountID string) (catalog.AccountOverviewDetail, error) {
			compiled, err := loadCatalog(ctx)
			if err != nil {
				return catalog.AccountOverviewDetail{}, err
			}
			currentSnapshots, err := loadSnapshots(ctx)
			if err != nil {
				return catalog.AccountOverviewDetail{}, err
			}
			return catalog.BuildAccountOverviewDetail(compiled, currentSnapshots, awsAccountID)
		}
		options.OverviewProjectEnvironment = func(ctx context.Context, awsAccountID, appRef, environmentRef string) (catalog.ProjectEnvironmentOverviewDetail, error) {
			compiled, err := loadCatalog(ctx)
			if err != nil {
				return catalog.ProjectEnvironmentOverviewDetail{}, err
			}
			currentSnapshots, err := loadSnapshots(ctx)
			if err != nil {
				return catalog.ProjectEnvironmentOverviewDetail{}, err
			}
			return catalog.BuildProjectEnvironmentOverviewDetail(compiled, currentSnapshots, awsAccountID, appRef, environmentRef)
		}
		options.OverviewDeployment = func(ctx context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
			compiled, err := loadCatalog(ctx)
			if err != nil {
				return catalog.DeploymentOverviewDetail{}, err
			}
			if store == nil {
				return catalog.BuildDeploymentOverviewDetail(compiled, nil, name)
			}
			snapshot, err := store.Get(ctx, name)
			if err != nil {
				if errors.Is(err, statusstore.ErrNotFound) {
					return catalog.BuildDeploymentOverviewDetail(compiled, nil, name)
				}
				return catalog.DeploymentOverviewDetail{}, err
			}
			return catalog.BuildDeploymentOverviewDetail(compiled, []statusstore.Snapshot{snapshot}, name)
		}
		options.OverviewDeploymentEvents = func(ctx context.Context, name string) (catalog.DeploymentOverviewEvents, error) {
			compiled, err := loadCatalog(ctx)
			if err != nil {
				return catalog.DeploymentOverviewEvents{}, err
			}
			if store == nil {
				return catalog.BuildDeploymentOverviewEvents(compiled, nil, name)
			}
			snapshot, err := store.Get(ctx, name)
			if err != nil {
				if errors.Is(err, statusstore.ErrNotFound) {
					return catalog.BuildDeploymentOverviewEvents(compiled, nil, name)
				}
				return catalog.DeploymentOverviewEvents{}, err
			}
			return catalog.BuildDeploymentOverviewEvents(compiled, []statusstore.Snapshot{snapshot}, name)
		}
		resourceLookupProvider := newOverviewDeploymentResourceLookupProvider(options.OverviewDeployment, loadObserverRoleARN)
		options.OverviewDeploymentResourceLogs = newOverviewDeploymentResourceLogsProvider(resourceLookupProvider, resourceLogService)
		options.OverviewDeploymentResourceInspection = newOverviewDeploymentResourceInspectionProvider(resourceLookupProvider, resourceInspectionService)
		planService := planpreview.Service{
			LoadDetail: options.OverviewDeployment,
			Store:      planpreview.NewFileStore(planStorePath),
		}
		applyService := applyrequest.Service{
			LoadPlan:           planService.Get,
			CurrentFingerprint: newApplyRequestFingerprintProvider(options.OverviewDeployment),
			Store:              applyrequest.NewFileStore(applyStorePath),
			EnableAWSApply:     awsApplyEnabled(),
		}
		options.CreateDeploymentPlanPreview = planService.Create
		options.PlanPreview = planService.Get
		options.CreateApplyRequest = applyService.Create
		options.ApplyRequest = applyService.Get
		options.TransitionApplyRequest = applyService.Transition
		options.AuditLogs = auditlog.Service{
			ListPlanPreviews:  func() ([]planpreview.Record, error) { return loadPlanPreviewRecords(planStorePath) },
			ListApplyRequests: func() ([]applyrequest.Record, error) { return loadApplyRequestRecords(applyStorePath) },
			ListDeployments:   options.CatalogDeployments,
		}.List
	}
	if store == nil {
		return options
	}
	options.DeploymentStatuses = func(ctx context.Context) ([]statusstore.Snapshot, error) {
		return store.List(ctx)
	}
	options.DeploymentStatus = func(ctx context.Context, name string) (statusstore.Snapshot, error) {
		return store.Get(ctx, name)
	}
	return options
}

func defaultPlanPreviewStorePath() string {
	return filepath.Join(os.TempDir(), "tenet", "plan-previews.json")
}

func defaultApplyRequestStorePath() string {
	return filepath.Join(os.TempDir(), "tenet", "apply-requests.json")
}

func awsApplyEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TENET_AWS_ENABLE_APPLY")), "true")
}

func newApplyRequestFingerprintProvider(loadDetail func(context.Context, string) (catalog.DeploymentOverviewDetail, error)) applyrequest.CurrentFingerprintProvider {
	if loadDetail == nil {
		return nil
	}
	return func(ctx context.Context, deploymentName string) (string, error) {
		detail, err := loadDetail(ctx, deploymentName)
		if err != nil {
			return "", err
		}
		return catalog.DeriveDeploymentSourceFingerprint(detail)
	}
}

func loadPlanPreviewRecords(path string) ([]planpreview.Record, error) {
	records, err := planpreview.Load(path)
	if err == nil {
		return records, nil
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
		return nil, nil
	}
	return nil, err
}

func loadApplyRequestRecords(path string) ([]applyrequest.Record, error) {
	records, err := applyrequest.Load(path)
	if err == nil {
		return records, nil
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
		return nil, nil
	}
	return nil, err
}

func startApplyRequestExecutor(logger *slog.Logger, repoRoot string, applyEnabled bool, applyStorePath string, interval time.Duration) (context.CancelFunc, bool) {
	if !applyEnabled || repoRoot == "" {
		return nil, false
	}

	ctx, cancel := context.WithCancel(context.Background())
	executor := applyrequest.Executor{
		Service: applyrequest.Service{
			Store: applyrequest.NewFileStore(applyStorePath),
		},
		PollInterval: interval,
		Logger:       logger,
	}
	logger.Info("starting local-safe apply executor", "apply_store_path", applyStorePath)
	go func() {
		if err := executor.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Warn("local-safe apply executor stopped", "error", err)
		}
	}()
	return cancel, true
}

type overviewDeploymentResourceLookupProvider func(context.Context, string, string, string) (resourceLookupRequest, error)

type resourceLookupRequest struct {
	DeploymentName      string
	ResourceType        string
	ResourceName        string
	ResourceDescription string
	ControlAccountID    string
	TargetAccountID     string
	Region              string
	ObserverRoleARN     string
}

func newOverviewDeploymentResourceLookupProvider(
	loadDetail func(context.Context, string) (catalog.DeploymentOverviewDetail, error),
	loadObserverRoleARN func(context.Context, string) (string, error),
) overviewDeploymentResourceLookupProvider {
	return func(ctx context.Context, name, resourceType, resourceName string) (resourceLookupRequest, error) {
		return loadOverviewDeploymentResourceRequest(ctx, loadDetail, loadObserverRoleARN, name, resourceType, resourceName)
	}
}

func loadOverviewDeploymentResourceRequest(
	ctx context.Context,
	loadDetail func(context.Context, string) (catalog.DeploymentOverviewDetail, error),
	loadObserverRoleARN func(context.Context, string) (string, error),
	name string,
	resourceType string,
	resourceName string,
) (resourceLookupRequest, error) {
	detail, err := loadDetail(ctx, name)
	if err != nil {
		return resourceLookupRequest{}, err
	}
	resource, ok := deploymentExecutionResource(detail, resourceType, resourceName)
	if !ok {
		return resourceLookupRequest{}, fmt.Errorf("%w: deployment %s resource %s/%s", statusstore.ErrNotFound, name, resourceType, resourceName)
	}
	observerRoleARN, err := loadObserverRoleARN(ctx, detail.AWSAccountID)
	if err != nil {
		return resourceLookupRequest{}, err
	}
	return resourceLookupRequest{
		DeploymentName:      name,
		ResourceType:        resourceType,
		ResourceName:        resourceName,
		ResourceDescription: resource.Description,
		ControlAccountID:    detail.AWSAccountID,
		TargetAccountID:     detail.AWSAccountID,
		Region:              detail.Region,
		ObserverRoleARN:     observerRoleARN,
	}, nil
}

func newOverviewDeploymentResourceLogsProvider(
	loadRequest overviewDeploymentResourceLookupProvider,
	provider resourceLogProvider,
) func(context.Context, string, string, string) (resourcelogs.Result, error) {
	return func(ctx context.Context, name, resourceType, resourceName string) (resourcelogs.Result, error) {
		request, err := loadRequest(ctx, name, resourceType, resourceName)
		if err != nil {
			return resourcelogs.Result{}, err
		}
		return provider.Recent(ctx, resourcelogs.Request{
			DeploymentName:   request.DeploymentName,
			ResourceType:     request.ResourceType,
			ResourceName:     request.ResourceName,
			ControlAccountID: request.ControlAccountID,
			TargetAccountID:  request.TargetAccountID,
			Region:           request.Region,
			ObserverRoleARN:  request.ObserverRoleARN,
		})
	}
}

func newOverviewDeploymentResourceInspectionProvider(
	loadRequest overviewDeploymentResourceLookupProvider,
	provider resourceInspectionProvider,
) func(context.Context, string, string, string) (resourceinspection.Result, error) {
	return func(ctx context.Context, name, resourceType, resourceName string) (resourceinspection.Result, error) {
		request, err := loadRequest(ctx, name, resourceType, resourceName)
		if err != nil {
			return resourceinspection.Result{}, err
		}
		return provider.Inspect(ctx, resourceinspection.Request{
			DeploymentName:      request.DeploymentName,
			ResourceType:        request.ResourceType,
			ResourceName:        request.ResourceName,
			ResourceDescription: request.ResourceDescription,
			ControlAccountID:    request.ControlAccountID,
			TargetAccountID:     request.TargetAccountID,
			Region:              request.Region,
			ObserverRoleARN:     request.ObserverRoleARN,
		})
	}
}

func deploymentExecutionResource(detail catalog.DeploymentOverviewDetail, resourceType, resourceName string) (domain.RolloutExecutionDetail, bool) {
	for _, resource := range rolloutExecutionResources(detail) {
		if resource.Resource == resourceType && resource.Name == resourceName {
			return resource, true
		}
	}
	return domain.RolloutExecutionDetail{}, false
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

func requestTimeoutFromEnv() (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv("TENET_API_REQUEST_TIMEOUT"))
	if value == "" {
		return 30 * time.Second, nil
	}

	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse TENET_API_REQUEST_TIMEOUT: %w", err)
	}
	if timeout < 0 {
		return 0, fmt.Errorf("TENET_API_REQUEST_TIMEOUT must be greater than or equal to 0")
	}
	return timeout, nil
}
