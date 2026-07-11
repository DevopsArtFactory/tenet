package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/auditlog"
	"github.com/DevopsArtFactory/tenet/internal/authidentity"
	"github.com/DevopsArtFactory/tenet/internal/authprovider"
	"github.com/DevopsArtFactory/tenet/internal/authstate"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/controllerrefresh"
	"github.com/DevopsArtFactory/tenet/internal/controllerrefreshaudit"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/rbacpolicy"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/DevopsArtFactory/tenet/internal/resourceaws"
	"github.com/DevopsArtFactory/tenet/internal/resourceinspection"
	"github.com/DevopsArtFactory/tenet/internal/resourcelogs"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
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
	controllerRefreshAuditStorePath := envOrDefault("TENET_CONTROLLER_REFRESH_AUDIT_STORE_PATH", defaultControllerRefreshAuditStorePath())
	applyEnabled := awsApplyEnabled()
	enforceAuthenticatedIdentity := operatorWriteAuthenticationEnforced()
	enforceOperatorRoleAuthorization := operatorWriteRoleAuthorizationEnforced()
	enforceDeploymentReadAuthorization := operatorDeploymentReadAuthorizationEnforced()
	enforceRepositoryPolicyAuthorization := repositoryPolicyAuthorizationEnforced()
	trustAuthenticatedHeaders := trustedAuthenticatedHeadersEnabled() || enforceAuthenticatedIdentity || enforceOperatorRoleAuthorization || enforceDeploymentReadAuthorization || enforceRepositoryPolicyAuthorization
	webhookConfigured := strings.TrimSpace(os.Getenv("TENET_GITHUB_WEBHOOK_SECRET")) != ""
	authRuntime := authRuntimeDiagnosticFromEnv()
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
	log.Printf("starting tenet-api version=%s commit=%s addr=%s catalog_source=%s repo_root=%s github_repository=%s github_ref=%s github_path=%s github_base_url=%s status_backend=%s status_path=%s plan_store_path=%s apply_store_path=%s controller_refresh_audit_store_path=%s apply_enabled=%t trust_authenticated_headers=%t require_authenticated_identity=%t require_operator_role_for_write=%t require_deployment_read_authorization=%t authorize_with_repository_policy=%t github_webhook_configured=%t tenet_managed_login_configured=%t redis_addr=%s request_timeout=%s", info.Version, info.Commit, addr, catalogSourceKind, repoRoot, githubRepository, githubRef, githubPath, githubBaseURL, statusBackend, statusPath, planStorePath, applyStorePath, controllerRefreshAuditStorePath, applyEnabled, trustAuthenticatedHeaders, enforceAuthenticatedIdentity, enforceOperatorRoleAuthorization, enforceDeploymentReadAuthorization, enforceRepositoryPolicyAuthorization, webhookConfigured, authRuntime.TenetManagedLogin.Configured, redisAddr, requestTimeout)

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
		BuildInfo:                          info,
		TrustAuthenticatedHeaders:          trustedAuthenticatedHeadersEnabled() || operatorWriteAuthenticationEnforced() || operatorWriteRoleAuthorizationEnforced() || operatorDeploymentReadAuthorizationEnforced() || repositoryPolicyAuthorizationEnforced(),
		RequireAuthenticatedIdentity:       operatorWriteAuthenticationEnforced() || operatorWriteRoleAuthorizationEnforced(),
		RequireOperatorRoleForRead:         operatorWriteRoleAuthorizationEnforced() && operatorDeploymentReadAuthorizationEnforced(),
		RequireOperatorRoleForWrite:        operatorWriteRoleAuthorizationEnforced(),
		RequireDeploymentReadAuthorization: operatorDeploymentReadAuthorizationEnforced(),
		AuthorizeWithRepositoryPolicy:      repositoryPolicyAuthorizationEnforced(),
		GitHubWebhookSecret:                strings.TrimSpace(os.Getenv("TENET_GITHUB_WEBHOOK_SECRET")),
		AuthRuntime:                        authRuntimeDiagnosticFromEnv(),
		GitHubWebhookDesiredStateSource:    source.webhookDesiredStateSource(),
		GitHubWebhookRefreshPublisher:      newGitHubWebhookRefreshPublisher(store),
	}
	configureManagedLoginRuntime(&options)
	if source.kind == apiCatalogSourceFilesystem {
		root := source.filesystemRoot
		options.RepositoryPolicy = func(_ context.Context, identity requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
			policy, err := rbacpolicy.LoadFromRepository(root)
			if err != nil {
				if errors.Is(err, rbacpolicy.ErrNotConfigured) {
					return rbacpolicy.Evaluate(rbacpolicy.Policy{}, identity, groups)
				}
				return rbacpolicy.Evaluation{Configured: true, Available: false, Source: rbacpolicy.SourceRepository, Summary: "repository RBAC policy evaluation failed", UnavailableReason: err.Error()}, err
			}
			return rbacpolicy.Evaluate(policy, identity, groups)
		}
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
			LoadPlan:              planService.Get,
			CurrentFingerprint:    newApplyRequestFingerprintProvider(options.OverviewDeployment),
			Store:                 applyrequest.NewFileStore(applyStorePath),
			EnableAWSApply:        awsApplyEnabled(),
			DeployerRoleReadiness: newApplyRequestDeployerRoleReadinessProvider(source, probeDeployerRoleIdentity),
		}
		options.CreateDeploymentPlanPreview = planService.Create
		options.PlanPreview = planService.Get
		options.CreateApplyRequest = applyService.Create
		options.ApplyRequests = applyService.List
		options.ApplyRequest = applyService.Get
		options.ApplyExecutionPreflight = applyService.Preflight
		options.ClaimApplyRequest = applyService.Claim
		options.HeartbeatApplyRequest = applyService.Heartbeat
		options.TransitionApplyRequest = applyService.Transition
		controllerRefreshAuditStorePath := envOrDefault("TENET_CONTROLLER_REFRESH_AUDIT_STORE_PATH", defaultControllerRefreshAuditStorePath())
		controllerRefreshAuditStore := controllerrefreshaudit.NewFileStore(controllerRefreshAuditStorePath)
		options.RecordControllerRefreshAudit = controllerRefreshAuditStore.Save
		options.AuditLogs = auditlog.Service{
			ListPlanPreviews:  func() ([]planpreview.Record, error) { return loadPlanPreviewRecords(planStorePath) },
			ListApplyRequests: func() ([]applyrequest.Record, error) { return loadApplyRequestRecords(applyStorePath) },
			ListControllerRefreshAuditRows: func() ([]controllerrefreshaudit.Record, error) {
				return loadControllerRefreshAuditRecords(controllerRefreshAuditStorePath)
			},
			ListDeployments: options.CatalogDeployments,
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

func defaultControllerRefreshAuditStorePath() string {
	return filepath.Join(os.TempDir(), "tenet", "controller-refresh-requests.json")
}

type deployerRoleIdentityProbe func(context.Context, string, string) error

func newApplyRequestDeployerRoleReadinessProvider(source apiCatalogSource, probe deployerRoleIdentityProbe) applyrequest.DeployerRoleReadinessProvider {
	if probe == nil {
		probe = probeDeployerRoleIdentity
	}
	return func(ctx context.Context, target applyrequest.DeployerRoleTarget) (applyrequest.DeployerRoleReadiness, error) {
		deploymentName := strings.TrimSpace(target.DeploymentName)
		readiness := applyrequest.DeployerRoleReadiness{
			Configured:        false,
			Ready:             false,
			UnavailableReason: "missing_deployer_role",
			Summary:           "No deployer role is configured for this target account and region.",
		}
		if deploymentName == "" || source.loadSpec == nil {
			return readiness, nil
		}
		catalogSpec, err := source.loadSpec(ctx)
		if err != nil {
			return applyrequest.DeployerRoleReadiness{}, err
		}
		var environmentRef string
		for _, deployment := range catalogSpec.Deployments {
			if strings.TrimSpace(deployment.Name) == deploymentName {
				environmentRef = deployment.NormalizedEnvironmentRef()
				break
			}
		}
		if environmentRef == "" {
			readiness.Summary = "No desired-state deployment placement was found for this apply request."
			return readiness, nil
		}
		var accountRef, region string
		for _, environment := range catalogSpec.Environments {
			if strings.TrimSpace(environment.Name) == environmentRef {
				accountRef = strings.TrimSpace(environment.AccountRef)
				region = strings.TrimSpace(environment.Region)
				break
			}
		}
		readiness.Region = region
		for _, account := range catalogSpec.Accounts {
			if strings.TrimSpace(account.Name) != accountRef {
				continue
			}
			readiness.AWSAccountID = strings.TrimSpace(account.AWSAccountID)
			readiness.RoleARN = strings.TrimSpace(account.RoleARN)
			break
		}
		if readiness.RoleARN == "" {
			readiness.Evidence = []string{"Desired-state account placement was resolved, but no roleArn is configured for apply execution readiness."}
			return readiness, nil
		}
		readiness.Configured = true
		readiness.CheckedWithAWS = true
		if err := probe(ctx, region, readiness.RoleARN); err != nil {
			readiness.UnavailableReason = "aws_unavailable"
			if reason, ok := resourceaws.UnavailableReasonForError(err); ok && strings.TrimSpace(reason) != "" {
				readiness.UnavailableReason = reason
			}
			readiness.Summary = "AWS credentials or provider access are unavailable in this local-safe environment."
			readiness.Evidence = []string{"Configured deployer role was present, but the read-only identity probe did not complete successfully."}
			return readiness, nil
		}
		readiness.Ready = true
		readiness.UnavailableReason = ""
		readiness.Summary = "Configured deployer role was assumed with a read-only identity probe."
		readiness.Evidence = []string{"STS GetCallerIdentity succeeded after assuming the configured deployer role."}
		return readiness, nil
	}
}

func probeDeployerRoleIdentity(ctx context.Context, region, roleARN string) error {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(strings.TrimSpace(region)))
	if err != nil {
		return err
	}
	cfg.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), strings.TrimSpace(roleARN), func(options *stscreds.AssumeRoleOptions) {
		options.RoleSessionName = "tenet-apply-preflight"
	}))
	_, err = sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	return err
}

func awsApplyEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TENET_AWS_ENABLE_APPLY")), "true")
}

func trustedAuthenticatedHeadersEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TENET_API_TRUST_AUTHENTICATED_HEADERS")), "true")
}

func operatorWriteAuthenticationEnforced() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TENET_API_ENFORCE_AUTHENTICATED_IDENTITY")), "true")
}

func operatorWriteRoleAuthorizationEnforced() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION")), "true")
}

func operatorDeploymentReadAuthorizationEnforced() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION")), "true")
}

func repositoryPolicyAuthorizationEnforced() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY")), "true")
}

func authSessionCookieSecureEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TENET_AUTH_SESSION_COOKIE_SECURE")), "true")
}

func authRuntimeDiagnosticFromEnv() api.AuthRuntimeDiagnostic {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("TENET_AUTH_PROVIDER")))
	settings := map[string]string{
		"TENET_AUTH_PROVIDER":              provider,
		"TENET_GITHUB_OAUTH_CLIENT_ID":     strings.TrimSpace(os.Getenv("TENET_GITHUB_OAUTH_CLIENT_ID")),
		"TENET_GITHUB_OAUTH_CLIENT_SECRET": strings.TrimSpace(os.Getenv("TENET_GITHUB_OAUTH_CLIENT_SECRET")),
		"TENET_GITHUB_OAUTH_REDIRECT_URL":  strings.TrimSpace(os.Getenv("TENET_GITHUB_OAUTH_REDIRECT_URL")),
		"TENET_AUTH_STATE_STORE_PATH":      strings.TrimSpace(os.Getenv("TENET_AUTH_STATE_STORE_PATH")),
		"TENET_AUTH_IDENTITY_STORE_PATH":   strings.TrimSpace(os.Getenv("TENET_AUTH_IDENTITY_STORE_PATH")),
		"TENET_AUTH_SESSION_SECRET":        strings.TrimSpace(os.Getenv("TENET_AUTH_SESSION_SECRET")),
		"TENET_GOOGLE_OIDC_CLIENT_ID":      strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_CLIENT_ID")),
		"TENET_GOOGLE_OIDC_CLIENT_SECRET":  strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_CLIENT_SECRET")),
		"TENET_GOOGLE_OIDC_ISSUER_URL":     strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_ISSUER_URL")),
		"TENET_GOOGLE_OIDC_REDIRECT_URL":   strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_REDIRECT_URL")),
		"TENET_GOOGLE_OIDC_AUTH_URL":       strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_AUTH_URL")),
		"TENET_GOOGLE_OIDC_TOKEN_URL":      strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_TOKEN_URL")),
		"TENET_GOOGLE_OIDC_USERINFO_URL":   strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_USERINFO_URL")),
		"TENET_OKTA_SAML_METADATA_URL":     strings.TrimSpace(os.Getenv("TENET_OKTA_SAML_METADATA_URL")),
		"TENET_OKTA_SAML_ACS_URL":          strings.TrimSpace(os.Getenv("TENET_OKTA_SAML_ACS_URL")),
		"TENET_OKTA_OIDC_ISSUER_URL":       strings.TrimSpace(os.Getenv("TENET_OKTA_OIDC_ISSUER_URL")),
		"TENET_OKTA_OIDC_CLIENT_ID":        strings.TrimSpace(os.Getenv("TENET_OKTA_OIDC_CLIENT_ID")),
		"TENET_OKTA_OIDC_CLIENT_SECRET":    strings.TrimSpace(os.Getenv("TENET_OKTA_OIDC_CLIENT_SECRET")),
		"TENET_OKTA_OIDC_REDIRECT_URL":     strings.TrimSpace(os.Getenv("TENET_OKTA_OIDC_REDIRECT_URL")),
	}
	provider = canonicalAuthProvider(provider)
	requiredSettings := requiredManagedLoginSettings(provider)
	missingSettings := make([]string, 0, len(requiredSettings))
	for _, setting := range requiredSettings {
		if strings.TrimSpace(settings[setting]) == "" {
			missingSettings = append(missingSettings, setting)
		}
	}
	if provider != "" && !managedLoginProviderSupported(provider) {
		for _, setting := range requiredSettings[1:] {
			if !containsString(missingSettings, setting) {
				missingSettings = append(missingSettings, setting)
			}
		}
	}
	configured := managedLoginProviderSupported(provider) && len(missingSettings) == 0
	managedLogin := api.TenetManagedLoginDiagnostic{
		Configured:                       configured,
		Provider:                         provider,
		RequiredSettings:                 requiredSettings,
		MissingSettings:                  missingSettings,
		OAuthStateTTLSeconds:             api.DefaultTenetManagedLoginOAuthStateTTLSeconds(),
		SessionTTLSeconds:                api.DefaultTenetManagedLoginSessionTTLSeconds(),
		SessionExpiryEnforced:            true,
		ProviderAccessTokenStored:        false,
		ProviderAccessTokenReturned:      false,
		ProviderTokenRefreshSupported:    false,
		ProviderTokenRevocationSupported: false,
		LogoutClearsSessionCookie:        configured,
		LogoutRevokesProviderToken:       false,
		StateStoreConfigured:             settings["TENET_AUTH_STATE_STORE_PATH"] != "",
		IdentityStoreConfigured:          settings["TENET_AUTH_IDENTITY_STORE_PATH"] != "",
		SessionSecretConfigured:          settings["TENET_AUTH_SESSION_SECRET"] != "",
	}
	if configured {
		managedLogin.Provider = provider
		managedLogin.LoginRoute, managedLogin.CallbackRoute = managedLoginRoutesForProvider(provider)
		managedLogin.SessionCookieName = "tenet_session"
	}
	enterpriseSSO := enterpriseSSODiagnosticFromSettings(provider, settings)
	return api.AuthRuntimeDiagnostic{TenetManagedLogin: managedLogin, EnterpriseSSO: enterpriseSSO}
}

func enterpriseSSODiagnosticFromSettings(provider string, settings map[string]string) api.EnterpriseSSODiagnostic {
	provider = canonicalAuthProvider(provider)
	requiredSettings, loginRoute, callbackRoute := enterpriseSSORequiredSettingsAndRoutes(provider)
	if len(requiredSettings) == 0 {
		return api.EnterpriseSSODiagnostic{}
	}
	missingSettings := make([]string, 0, len(requiredSettings))
	for _, setting := range requiredSettings {
		if strings.TrimSpace(settings[setting]) == "" {
			missingSettings = append(missingSettings, setting)
		}
	}
	invalidSettings := enterpriseSSOInvalidSettings(provider, settings)
	configured := len(missingSettings) == 0 && len(invalidSettings) == 0
	diagnostic := api.EnterpriseSSODiagnostic{
		Configured:              configured,
		Active:                  false,
		Provider:                provider,
		RequiredSettings:        requiredSettings,
		MissingSettings:         missingSettings,
		InvalidSettings:         invalidSettings,
		LoginRoute:              loginRoute,
		CallbackRoute:           callbackRoute,
		StateStoreConfigured:    settings["TENET_AUTH_STATE_STORE_PATH"] != "",
		IdentityStoreConfigured: settings["TENET_AUTH_IDENTITY_STORE_PATH"] != "",
		SessionSecretConfigured: settings["TENET_AUTH_SESSION_SECRET"] != "",
		UnavailableReason:       "enterprise SSO provider diagnostics are configured, but provider login/callback runtime is not active for this API instance",
	}
	if configured {
		diagnostic.LogoutRoute = "/auth/logout"
	}
	if len(missingSettings) > 0 {
		diagnostic.UnavailableReason = "enterprise SSO provider configuration is incomplete or missing shared managed-session settings"
	} else if len(invalidSettings) > 0 {
		diagnostic.UnavailableReason = "enterprise SSO provider configuration contains invalid non-secret URL settings"
	}
	return diagnostic
}

func enterpriseSSOInvalidSettings(provider string, settings map[string]string) []string {
	if canonicalAuthProvider(provider) != "okta_saml" {
		return nil
	}
	invalid := []string{}
	if value := strings.TrimSpace(settings["TENET_OKTA_SAML_METADATA_URL"]); value != "" && !isAbsoluteHTTPURL(value) {
		invalid = append(invalid, "TENET_OKTA_SAML_METADATA_URL")
	}
	if value := strings.TrimSpace(settings["TENET_OKTA_SAML_ACS_URL"]); value != "" {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.EscapedPath() != "/auth/callback/okta/saml" {
			invalid = append(invalid, "TENET_OKTA_SAML_ACS_URL")
		}
	}
	return invalid
}

func isAbsoluteHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme != "" && parsed.Host != "" && (parsed.Scheme == "https" || parsed.Scheme == "http")
}

func enterpriseSSORequiredSettingsAndRoutes(provider string) ([]string, string, string) {
	switch provider {
	case "google":
		return []string{
			"TENET_AUTH_PROVIDER",
			"TENET_GOOGLE_OIDC_CLIENT_ID",
			"TENET_GOOGLE_OIDC_CLIENT_SECRET",
			"TENET_GOOGLE_OIDC_ISSUER_URL",
			"TENET_GOOGLE_OIDC_REDIRECT_URL",
			"TENET_AUTH_STATE_STORE_PATH",
			"TENET_AUTH_IDENTITY_STORE_PATH",
			"TENET_AUTH_SESSION_SECRET",
		}, "/auth/login/google", "/auth/callback/google"
	case "okta_saml":
		return []string{
			"TENET_AUTH_PROVIDER",
			"TENET_OKTA_SAML_METADATA_URL",
			"TENET_OKTA_SAML_ACS_URL",
			"TENET_AUTH_STATE_STORE_PATH",
			"TENET_AUTH_IDENTITY_STORE_PATH",
			"TENET_AUTH_SESSION_SECRET",
		}, "/auth/login/okta", "/auth/callback/okta/saml"
	case "okta_oidc":
		return []string{
			"TENET_AUTH_PROVIDER",
			"TENET_OKTA_OIDC_ISSUER_URL",
			"TENET_OKTA_OIDC_CLIENT_ID",
			"TENET_OKTA_OIDC_CLIENT_SECRET",
			"TENET_OKTA_OIDC_REDIRECT_URL",
			"TENET_AUTH_STATE_STORE_PATH",
			"TENET_AUTH_IDENTITY_STORE_PATH",
			"TENET_AUTH_SESSION_SECRET",
		}, "/auth/login/okta", "/auth/callback/okta/oidc"
	default:
		return nil, "", ""
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func configureManagedLoginRuntime(options *api.Options) {
	if options == nil {
		return
	}
	managedLogin := options.AuthRuntime.TenetManagedLogin
	providerName := canonicalAuthProvider(managedLogin.Provider)
	if !managedLogin.Configured || len(managedLogin.MissingSettings) > 0 || !managedLoginProviderSupported(providerName) {
		return
	}
	var provider authprovider.Provider
	var err error
	switch providerName {
	case authprovider.GitHubProviderName:
		provider, err = authprovider.NewGitHubProvider(authprovider.GitHubConfig{
			ClientID:     strings.TrimSpace(os.Getenv("TENET_GITHUB_OAUTH_CLIENT_ID")),
			ClientSecret: strings.TrimSpace(os.Getenv("TENET_GITHUB_OAUTH_CLIENT_SECRET")),
			RedirectURL:  strings.TrimSpace(os.Getenv("TENET_GITHUB_OAUTH_REDIRECT_URL")),
		})
	case authprovider.GoogleProviderName:
		provider, err = authprovider.NewGoogleProvider(authprovider.GoogleConfig{
			ClientID:     strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_CLIENT_ID")),
			ClientSecret: strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_CLIENT_SECRET")),
			RedirectURL:  strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_REDIRECT_URL")),
			IssuerURL:    strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_ISSUER_URL")),
			AuthURL:      strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_AUTH_URL")),
			TokenURL:     strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_TOKEN_URL")),
			UserInfoURL:  strings.TrimSpace(os.Getenv("TENET_GOOGLE_OIDC_USERINFO_URL")),
		})
	case authprovider.OktaOIDCProviderName:
		provider, err = authprovider.NewOktaOIDCProvider(authprovider.OktaOIDCConfig{
			ClientID:     strings.TrimSpace(os.Getenv("TENET_OKTA_OIDC_CLIENT_ID")),
			ClientSecret: strings.TrimSpace(os.Getenv("TENET_OKTA_OIDC_CLIENT_SECRET")),
			RedirectURL:  strings.TrimSpace(os.Getenv("TENET_OKTA_OIDC_REDIRECT_URL")),
			IssuerURL:    strings.TrimSpace(os.Getenv("TENET_OKTA_OIDC_ISSUER_URL")),
		})
	}
	if err != nil {
		options.AuthRuntime.TenetManagedLogin.Active = false
		options.AuthRuntime.TenetManagedLogin.UnavailableReason = "tenet-managed " + providerName + " login is configured but provider initialization failed"
		return
	}
	options.AuthProvider = provider
	options.AuthStateStore = authstate.NewFileStore(strings.TrimSpace(os.Getenv("TENET_AUTH_STATE_STORE_PATH")))
	options.AuthIdentityStore = authidentity.NewFileStore(strings.TrimSpace(os.Getenv("TENET_AUTH_IDENTITY_STORE_PATH")))
	options.AuthSessionSecret = strings.TrimSpace(os.Getenv("TENET_AUTH_SESSION_SECRET"))
	options.AuthSessionCookieName = "tenet_session"
	options.AuthSessionCookieSecure = authSessionCookieSecureEnabled()
	loginRoute, callbackRoute := managedLoginRoutesForProvider(providerName)
	options.AuthRuntime.TenetManagedLogin.Provider = providerName
	options.AuthRuntime.TenetManagedLogin.Active = true
	options.AuthRuntime.TenetManagedLogin.LoginRoute = loginRoute
	options.AuthRuntime.TenetManagedLogin.CallbackRoute = callbackRoute
	options.AuthRuntime.TenetManagedLogin.SessionCookieName = options.AuthSessionCookieName
	options.AuthRuntime.TenetManagedLogin.UnavailableReason = ""
	if providerName == authprovider.GoogleProviderName || providerName == authprovider.OktaOIDCProviderName {
		options.AuthRuntime.EnterpriseSSO.Provider = providerName
		options.AuthRuntime.EnterpriseSSO.Configured = true
		options.AuthRuntime.EnterpriseSSO.Active = true
		options.AuthRuntime.EnterpriseSSO.LoginRoute = loginRoute
		options.AuthRuntime.EnterpriseSSO.CallbackRoute = callbackRoute
		options.AuthRuntime.EnterpriseSSO.LogoutRoute = "/auth/logout"
		options.AuthRuntime.EnterpriseSSO.MissingSettings = []string{}
		options.AuthRuntime.EnterpriseSSO.UnavailableReason = ""
	}
}

func canonicalAuthProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == authprovider.GoogleOIDCProviderAlias {
		return authprovider.GoogleProviderName
	}
	return provider
}

func managedLoginProviderSupported(provider string) bool {
	switch canonicalAuthProvider(provider) {
	case authprovider.GitHubProviderName, authprovider.GoogleProviderName, authprovider.OktaOIDCProviderName:
		return true
	default:
		return false
	}
}

func requiredManagedLoginSettings(provider string) []string {
	switch canonicalAuthProvider(provider) {
	case authprovider.GoogleProviderName:
		settings, _, _ := enterpriseSSORequiredSettingsAndRoutes(authprovider.GoogleProviderName)
		return settings
	case authprovider.OktaOIDCProviderName:
		settings, _, _ := enterpriseSSORequiredSettingsAndRoutes(authprovider.OktaOIDCProviderName)
		return settings
	default:
		return api.RequiredTenetManagedLoginSettings()
	}
}

func managedLoginRoutesForProvider(provider string) (string, string) {
	switch canonicalAuthProvider(provider) {
	case authprovider.GoogleProviderName:
		return "/auth/login/google", "/auth/callback/google"
	case authprovider.OktaOIDCProviderName:
		return "/auth/login/okta", "/auth/callback/okta/oidc"
	default:
		return "/auth/login/github", "/auth/callback/github"
	}
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

func loadControllerRefreshAuditRecords(path string) ([]controllerrefreshaudit.Record, error) {
	records, err := controllerrefreshaudit.Load(path)
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
		WorkerID:     "tenet-api-local-executor",
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
	BackingResourceType string
	BackingResourceName string
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
	backingResourceType, backingResourceName := resourceLookupBackingResource(detail, resourceType)
	return resourceLookupRequest{
		DeploymentName:      name,
		ResourceType:        resourceType,
		ResourceName:        resourceName,
		ResourceDescription: resource.Description,
		BackingResourceType: backingResourceType,
		BackingResourceName: backingResourceName,
		ControlAccountID:    detail.AWSAccountID,
		TargetAccountID:     detail.AWSAccountID,
		Region:              detail.Region,
		ObserverRoleARN:     observerRoleARN,
	}, nil
}

func newOverviewDeploymentResourceLogsProvider(
	loadRequest overviewDeploymentResourceLookupProvider,
	provider resourceLogProvider,
) func(context.Context, string, string, string, int) (resourcelogs.Result, error) {
	return func(ctx context.Context, name, resourceType, resourceName string, limit int) (resourcelogs.Result, error) {
		request, err := loadRequest(ctx, name, resourceType, resourceName)
		if err != nil {
			return resourcelogs.Result{}, err
		}
		return provider.Recent(ctx, resourcelogs.Request{
			DeploymentName:      request.DeploymentName,
			ResourceType:        request.ResourceType,
			ResourceName:        request.ResourceName,
			ResourceDescription: request.ResourceDescription,
			BackingResourceType: request.BackingResourceType,
			BackingResourceName: request.BackingResourceName,
			ControlAccountID:    request.ControlAccountID,
			TargetAccountID:     request.TargetAccountID,
			Region:              request.Region,
			ObserverRoleARN:     request.ObserverRoleARN,
			ContainerName:       resourcelogs.ContainerNameFromContext(ctx),
			InstanceID:          resourcelogs.InstanceIDFromContext(ctx),
			ClusterARN:          resourcelogs.ClusterARNFromContext(ctx),
			Limit:               limit,
		})
	}
}

func newOverviewDeploymentResourceInspectionProvider(
	loadRequest overviewDeploymentResourceLookupProvider,
	provider resourceInspectionProvider,
) func(context.Context, string, string, string, int) (resourceinspection.Result, error) {
	return func(ctx context.Context, name, resourceType, resourceName string, limit int) (resourceinspection.Result, error) {
		request, err := loadRequest(ctx, name, resourceType, resourceName)
		if err != nil {
			return resourceinspection.Result{}, err
		}
		result, err := provider.Inspect(ctx, resourceinspection.Request{
			DeploymentName:           request.DeploymentName,
			ResourceType:             request.ResourceType,
			ResourceName:             request.ResourceName,
			ResourceDescription:      request.ResourceDescription,
			TaskSummaryLimit:         limit,
			AutoScalingInstanceLimit: limit,
			InstanceID:               resourceinspection.InstanceIDFromContext(ctx),
			ClusterARN:               resourceinspection.ClusterARNFromContext(ctx),
			BackingResourceType:      request.BackingResourceType,
			BackingResourceName:      request.BackingResourceName,
			ControlAccountID:         request.ControlAccountID,
			TargetAccountID:          request.TargetAccountID,
			Region:                   request.Region,
			ObserverRoleARN:          request.ObserverRoleARN,
		})
		if err != nil {
			return resourceinspection.Result{}, err
		}
		if result.DeploymentName == "" {
			result.DeploymentName = request.DeploymentName
		}
		if result.ResourceType == "" {
			result.ResourceType = request.ResourceType
		}
		if result.ResourceName == "" {
			result.ResourceName = request.ResourceName
		}
		if result.ResourceDescription == "" {
			result.ResourceDescription = strings.TrimSpace(request.ResourceDescription)
		}
		if result.InstanceID == "" {
			result.InstanceID = resourceinspection.InstanceIDFromContext(ctx)
		}
		if result.ClusterARN == "" {
			result.ClusterARN = resourceinspection.ClusterARNFromContext(ctx)
		}
		if result.BackingResourceType == "" {
			result.BackingResourceType = strings.TrimSpace(request.BackingResourceType)
		}
		if result.BackingResourceName == "" {
			result.BackingResourceName = strings.TrimSpace(request.BackingResourceName)
		}
		return result, nil
	}
}

func deploymentExecutionResource(detail catalog.DeploymentOverviewDetail, resourceType, resourceName string) (domain.RolloutExecutionDetail, bool) {
	for _, resource := range rolloutExecutionResources(detail) {
		if resource.Resource == resourceType && resource.Name == resourceName {
			return resource, true
		}
	}
	if resourceType == "userDataBootstrapSource" {
		return filesystemUserDataBootstrapExecutionResource(detail, resourceName)
	}
	if resourceType == "ecsTask" && strings.HasPrefix(strings.TrimSpace(detail.ExecutionMode), "ecs") {
		return domain.RolloutExecutionDetail{Resource: resourceType, Name: resourceName, Description: "ECS task selected from live service inspection"}, true
	}
	return domain.RolloutExecutionDetail{}, false
}

func filesystemUserDataBootstrapExecutionResource(detail catalog.DeploymentOverviewDetail, resourceName string) (domain.RolloutExecutionDetail, bool) {
	requestedType, requestedPath, ok := splitUserDataBootstrapSource(resourceName)
	if !ok || requestedType != "file" || requestedPath == "" || strings.TrimSpace(detail.UserData.Path) != requestedPath {
		return domain.RolloutExecutionDetail{}, false
	}
	for _, resource := range rolloutExecutionResources(detail) {
		if resource.Resource != "userDataBootstrapSource" {
			continue
		}
		_, resourcePath, ok := splitUserDataBootstrapSource(resource.Name)
		if ok && resourcePath == requestedPath {
			return resource, true
		}
	}
	return domain.RolloutExecutionDetail{}, false
}

func splitUserDataBootstrapSource(resourceName string) (string, string, bool) {
	resourceName = strings.TrimSpace(resourceName)
	if resourceName == "" {
		return "", "", false
	}
	parts := strings.SplitN(resourceName, ":", 2)
	if len(parts) == 1 {
		return "", parts[0], true
	}
	resourceType := strings.TrimSpace(parts[0])
	resourcePath := strings.TrimSpace(parts[1])
	if resourcePath == "" {
		return "", "", false
	}
	return resourceType, resourcePath, true
}

func resourceLookupBackingResource(detail catalog.DeploymentOverviewDetail, resourceType string) (string, string) {
	switch resourceType {
	case "userDataBootstrapSource":
		preferredType := ""
		switch detail.ExecutionMode {
		case "asgWeightedTargetGroups":
			preferredType = "candidateAutoScalingGroup"
		case "asgInstanceRefresh":
			preferredType = "serviceAutoScalingGroup"
		}
		for _, resource := range rolloutExecutionResources(detail) {
			if preferredType != "" && resource.Resource == preferredType {
				return resource.Resource, resource.Name
			}
		}
		for _, resource := range rolloutExecutionResources(detail) {
			switch resource.Resource {
			case "candidateAutoScalingGroup", "serviceAutoScalingGroup":
				return resource.Resource, resource.Name
			}
		}
	case "productionListener", "testListener", "blueTaskSetTargetGroup", "greenTaskSetTargetGroup":
		for _, resource := range rolloutExecutionResources(detail) {
			if resource.Resource == "codeDeployDeploymentGroup" {
				return resource.Resource, resource.Name
			}
		}
	}
	return "", ""
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

type redisClientProvider interface {
	Client() redis.UniversalClient
}

func newGitHubWebhookRefreshPublisher(store statusstore.Store) controllerrefresh.Publisher {
	if store == nil {
		return nil
	}
	provider, ok := store.(redisClientProvider)
	if !ok || provider.Client() == nil {
		return nil
	}
	return controllerrefresh.NewRedisHandoff(provider.Client(), os.Getenv("TENET_REDIS_KEY_PREFIX"))
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
