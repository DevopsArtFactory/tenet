package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/ecsexecution"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func ecsExecutionEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("TENET_AWS_ENABLE_ECS_EXECUTION")), "true")
}

func validateECSExecutionConfiguration(source apiCatalogSource) error {
	if !ecsExecutionEnabled() {
		return nil
	}
	if !awsApplyEnabled() || !source.configured() || (source.kind == apiCatalogSourceFilesystem && source.filesystemRoot == "") {
		return fmt.Errorf("live ECS execution requires TENET_AWS_ENABLE_APPLY=true and a configured desired-state repository")
	}
	if source.kind == apiCatalogSourceGitHub && strings.TrimSpace(source.githubRequest.Ref) == "" {
		return fmt.Errorf("live GitHub ECS execution requires an explicit TENET_CATALOG_GITHUB_REF")
	}
	if !trustedAuthenticatedHeadersEnabled() && !authRuntimeDiagnosticFromEnv().TenetManagedLogin.Configured {
		return fmt.Errorf("live ECS execution requires managed login or an explicitly trusted authenticated front door")
	}
	if source.loadPolicy == nil {
		return fmt.Errorf("live ECS execution requires a repository RBAC policy provider")
	}
	policy, err := source.loadPolicy(context.Background())
	if err != nil {
		return fmt.Errorf("live ECS execution requires repository RBAC policy: %w", err)
	}
	if policy.Version == "" {
		return fmt.Errorf("live ECS execution requires a versioned repository RBAC policy")
	}
	return nil
}

func newApplyService(source apiCatalogSource, options api.Options, planStorePath, applyStorePath string) applyrequest.Service {
	plans := planpreview.Service{LoadDetail: options.OverviewDeployment, Store: planpreview.NewFileStore(planStorePath)}
	service := applyrequest.Service{
		LoadPlan: plans.Get, CurrentFingerprint: newApplyRequestFingerprintProvider(options.OverviewDeployment),
		Store: applyrequest.NewFileStore(applyStorePath), EnableAWSApply: awsApplyEnabled(),
		DeployerRoleReadiness: newApplyRequestDeployerRoleReadinessProvider(source, probeDeployerRoleIdentity),
		PermissionSimulation:  newApplyRequestPermissionSimulationProvider(probeApplyPermissionSimulation),
	}
	if ecsExecutionEnabled() {
		service.CreateRetryPlan = func(ctx context.Context, name string) (planpreview.Record, error) {
			result, err := plans.Create(ctx, name, planpreview.CreateRequest{})
			return result.Record, err
		}
		service.RollbackTarget = func(ctx context.Context, parent applyrequest.Record) (*domain.ECSDeploymentIntent, error) {
			role, err := service.DeployerRoleReadiness(ctx, applyrequest.DeployerRoleTarget{DeploymentName: parent.DeploymentName})
			if err != nil {
				return nil, err
			}
			handler := ecsexecution.RollingService{ClientFor: func(ctx context.Context, role applyrequest.DeployerRoleReadiness) (ecsexecution.ServiceClient, error) {
				return ecsexecution.DeployerClient(ctx, role)
			}, CheckCapabilities: checkECSCapabilities}
			return handler.RollbackTarget(ctx, parent, role)
		}
		service.AuthorizeCreate = func(ctx context.Context, record applyrequest.Record) (*applyrequest.ExecutionPrincipal, error) {
			if len(record.Operations) != 3 || record.Operations[0].ECS == nil || record.Operations[0].ECS.DeploymentController != "ECS" || record.Operations[0].ECS.DeploymentStrategy != "ROLLING" {
				return nil, fmt.Errorf("%w: only reviewed native ECS Rolling plans support live execution", applyrequest.ErrPlanNotReady)
			}
			principal := &applyrequest.ExecutionPrincipal{Automatic: applyrequest.AutomaticApprovalFromContext(ctx), Identity: requestactor.AuthenticatedIdentityFromContext(ctx), Groups: requestactor.AuthenticatedGroupsFromContext(ctx)}
			if err := authorizeECSAutomaticApproval(principal); err != nil {
				return nil, err
			}
			if err := authorizeECSPrincipal(ctx, options, principal, record.DeploymentName); err != nil {
				return nil, err
			}
			return principal, nil
		}
	}
	return service
}

func authorizeECSAutomaticApproval(principal *applyrequest.ExecutionPrincipal) error {
	if principal != nil && principal.Automatic && (!ecsAutoSyncEnabled() || !ecsReconcileEnabled() || principal.Identity.ID != strings.TrimSpace(os.Getenv("TENET_ECS_AUTO_SYNC_SUBJECT"))) {
		return applyrequest.ErrExecutionUnauthorized
	}
	return nil
}

func authorizeECSPrincipal(ctx context.Context, options api.Options, principal *applyrequest.ExecutionPrincipal, deployment string) error {
	if principal == nil || !principal.Identity.Authenticated || (principal.Identity.ID == "" && principal.Identity.Email == "") || options.RepositoryPolicy == nil {
		return applyrequest.ErrExecutionUnauthorized
	}

	policy, err := options.RepositoryPolicy(ctx, principal.Identity, principal.Groups)
	if err != nil {
		return fmt.Errorf("reload ECS execution RBAC policy: %w", err)
	}
	if !policy.Configured || !policy.Available || !policy.CanWrite {
		return applyrequest.ErrExecutionUnauthorized
	}
	if policy.DeploymentAccess.Allows(deployment) {
		return nil
	}
	scopes := policy.ProjectEnvironmentScopes
	if options.CatalogPlacements != nil && len(scopes.Projects) > 0 && len(scopes.Environments) > 0 {
		placements, err := options.CatalogPlacements(ctx)
		if err != nil {
			return err
		}
		for _, placement := range placements {
			if placement.DeploymentName == deployment && placement.AppRef != "" && placement.EnvironmentRef != "" && slices.Contains(scopes.Projects, placement.AppRef) && slices.Contains(scopes.Environments, placement.EnvironmentRef) {
				return nil
			}
		}
	}
	return applyrequest.ErrExecutionUnauthorized
}

func newNativeECSExecutor(source apiCatalogSource, options api.Options, planStorePath, applyStorePath string, logger *slog.Logger) applyrequest.Executor {
	service := newApplyService(source, options, planStorePath, applyStorePath)
	handlers := map[string]applyrequest.ECSOperationHandler{
		string(reconcile.StepRegisterECSTaskDefinition): ecsexecution.Registration{ClientFor: func(ctx context.Context, role applyrequest.DeployerRoleReadiness) (ecsexecution.RegistrationClient, error) {
			return ecsexecution.DeployerClient(ctx, role)
		}, CheckCapabilities: checkECSCapabilities},
		string(reconcile.StepUpdateECSService): ecsexecution.RollingService{ClientFor: func(ctx context.Context, role applyrequest.DeployerRoleReadiness) (ecsexecution.ServiceClient, error) {
			return ecsexecution.DeployerClient(ctx, role)
		}, CheckCapabilities: checkECSCapabilities},
		string(reconcile.StepVerifyServiceStability): ecsexecution.RollingObservation{ClientFor: func(ctx context.Context, role applyrequest.DeployerRoleReadiness) (ecsexecution.ObservationClient, error) {
			return ecsexecution.DeployerClient(ctx, role)
		}, CheckCapabilities: checkECSCapabilities},
	}
	return applyrequest.Executor{Service: service, Logger: logger, PollInterval: time.Second, WorkerID: fmt.Sprintf("tenet-api-ecs-%d-%d", os.Getpid(), time.Now().UnixNano()), ECS: &applyrequest.ECSExecution{
		Enabled: ecsExecutionEnabled(), Handlers: handlers,
		AuthorizeMutation: func(_ context.Context, record applyrequest.Record) error {
			return authorizeECSAutomaticApproval(record.Principal)
		},
		Authorize: func(ctx context.Context, record applyrequest.Record) error {
			return authorizeECSPrincipal(ctx, options, record.Principal, record.DeploymentName)
		},
		CurrentIntent: func(ctx context.Context, name string) (*domain.ECSDeploymentIntent, error) {
			compiled, err := source.loadCatalog(ctx)
			if err != nil {
				return nil, err
			}
			// Intent is compiled from Git inputs, independent of our changing runtime observation.
			detail, err := catalog.BuildDeploymentOverviewDetail(compiled, nil, name)
			return detail.ECS.Clone(), err
		},
	}}
}

// checkECSCapabilities is an extra preflight gate; AWS still authorizes each real call.
func checkECSCapabilities(ctx context.Context, role applyrequest.DeployerRoleReadiness, checks []applyrequest.OperationExecutionCapabilityCheck) error {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(role.Region))
	if err != nil {
		return err
	}
	cfg.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), role.RoleARN, func(o *stscreds.AssumeRoleOptions) { o.RoleSessionName = "tenet-ecs-permission-check" }))
	return simulateECSCapabilities(ctx, iam.NewFromConfig(cfg), role, checks)
}

type ecsPolicySimulator interface {
	SimulatePrincipalPolicy(context.Context, *iam.SimulatePrincipalPolicyInput, ...func(*iam.Options)) (*iam.SimulatePrincipalPolicyOutput, error)
}

func simulateECSCapabilities(ctx context.Context, client ecsPolicySimulator, role applyrequest.DeployerRoleReadiness, checks []applyrequest.OperationExecutionCapabilityCheck) error {
	if len(checks) == 0 {
		return fmt.Errorf("concrete ECS capability checks are required")
	}
	for _, check := range checks {
		if check.Action == "" || check.ResourceName == "" || check.RequiredRole != role.RoleARN {
			return fmt.Errorf("invalid concrete ECS capability target")
		}
		resource := check.ResourceName
		if check.Action == "ecs:DescribeTaskDefinition" {
			resource = "*"
		}
		input := &iam.SimulatePrincipalPolicyInput{PolicySourceArn: aws.String(role.RoleARN), ActionNames: []string{check.Action}, ResourceArns: []string{resource}}
		keys := make([]string, 0, len(check.Context))
		for key := range check.Context {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			input.ContextEntries = append(input.ContextEntries, iamtypes.ContextEntry{ContextKeyName: aws.String(key), ContextKeyType: iamtypes.ContextKeyTypeEnumString, ContextKeyValues: []string{check.Context[key]}})
		}
		if check.Action == "iam:PassRole" && check.Context["iam:PassedToService"] == "" {
			input.ContextEntries = append(input.ContextEntries, iamtypes.ContextEntry{ContextKeyName: aws.String("iam:PassedToService"), ContextKeyType: iamtypes.ContextKeyTypeEnumString, ContextKeyValues: []string{"ecs-tasks.amazonaws.com"}})
		}
		if check.Action == "ecs:TagResource" {
			input.ContextEntries = append(input.ContextEntries, iamtypes.ContextEntry{ContextKeyName: aws.String("ecs:CreateAction"), ContextKeyType: iamtypes.ContextKeyTypeEnumString, ContextKeyValues: []string{"RegisterTaskDefinition"}})
		}
		output, err := client.SimulatePrincipalPolicy(ctx, input)
		if err != nil {
			return fmt.Errorf("check %s on %s: %w", check.Action, check.ResourceName, err)
		}
		if output == nil || output.IsTruncated || len(output.EvaluationResults) != 1 {
			return fmt.Errorf("incomplete permission evidence for %s on %s", check.Action, check.ResourceName)
		}
		result := output.EvaluationResults[0]
		if aws.ToString(result.EvalActionName) != check.Action || aws.ToString(result.EvalResourceName) != resource || result.EvalDecision != iamtypes.PolicyEvaluationDecisionTypeAllowed || len(result.MissingContextValues) != 0 {
			return fmt.Errorf("permission not confirmed for %s on %s", check.Action, check.ResourceName)
		}
	}
	return nil
}
