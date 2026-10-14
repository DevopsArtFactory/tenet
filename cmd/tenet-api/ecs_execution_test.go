package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/rbacpolicy"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

func TestLiveECSRequiresExplicitSafeStartupConfiguration(t *testing.T) {
	root := t.TempDir()
	source := newFilesystemCatalogSource(root)
	t.Setenv("TENET_AWS_ENABLE_ECS_EXECUTION", "")
	t.Setenv("TENET_AWS_ENABLE_APPLY", "true")
	t.Setenv("TENET_AUTH_PROVIDER", "")
	t.Setenv("TENET_API_TRUST_AUTHENTICATED_HEADERS", "")
	if err := validateECSExecutionConfiguration(source); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TENET_AWS_ENABLE_ECS_EXECUTION", "true")
	if err := validateECSExecutionConfiguration(source); err == nil {
		t.Fatal("enabled live execution without a trusted login boundary")
	}
	t.Setenv("TENET_API_TRUST_AUTHENTICATED_HEADERS", "true")
	if err := validateECSExecutionConfiguration(source); err == nil {
		t.Fatal("enabled live execution without repository policy")
	}
	if err := os.MkdirAll(filepath.Join(root, "rbac"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rbac", "policy.json"), []byte(`{"version":"v1","users":[{"id":"operator-a","roles":["operator"],"deployments":["worker"]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateECSExecutionConfiguration(source); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TENET_AWS_ENABLE_APPLY", "false")
	if err := validateECSExecutionConfiguration(source); err == nil {
		t.Fatal("live execution ignored disabled apply")
	}
}

func TestECSAuthorizationReloadsPolicyAndIgnoresActorAttribution(t *testing.T) {
	principal := &applyrequest.ExecutionPrincipal{Identity: requestactor.AuthenticatedIdentity{Authenticated: true, ID: "operator-a"}}
	policy := rbacpolicy.Policy{Version: "v1", Users: []rbacpolicy.Subject{{ID: "operator-a", Roles: []string{"operator"}, Deployments: []string{"worker"}}}}
	options := api.Options{RepositoryPolicy: func(_ context.Context, identity requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
		return rbacpolicy.Evaluate(policy, identity, groups)
	}}
	ctx := requestactor.WithContext(context.Background(), requestactor.Actor{Available: true, ID: "operator-a"})
	if err := authorizeECSPrincipal(ctx, options, nil, "worker"); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
		t.Fatal("actor authorized a worker")
	}
	if err := authorizeECSPrincipal(ctx, options, principal, "worker"); err != nil {
		t.Fatal(err)
	}
	if err := authorizeECSPrincipal(ctx, options, principal, "other"); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
		t.Fatal("unscoped deployment was authorized")
	}
	policy.Users[0].Roles = []string{"viewer"}
	if err := authorizeECSPrincipal(ctx, options, principal, "worker"); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
		t.Fatal("revoked operator continued")
	}
	policy.Users[0].Roles = []string{"operator"}
	policy.Users[0].Deployments = nil
	policy.Users[0].Projects = []string{"payments"}
	policy.Users[0].Environments = []string{"production"}
	options.CatalogPlacements = func(context.Context) ([]catalog.PlacementView, error) {
		return []catalog.PlacementView{{DeploymentName: "worker", AppRef: "payments", EnvironmentRef: "production"}}, nil
	}
	if err := authorizeECSPrincipal(ctx, options, principal, "worker"); err != nil {
		t.Fatal(err)
	}
	policy.Users[0].Environments = []string{"staging"}
	if err := authorizeECSPrincipal(ctx, options, principal, "worker"); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
		t.Fatal("project-only match authorized wrong environment")
	}
}

func TestLiveECSAPIStoresTrustedApprovalAndDoesNotAdoptLegacyRequests(t *testing.T) {
	t.Setenv("TENET_AWS_ENABLE_APPLY", "true")
	t.Setenv("TENET_AWS_ENABLE_ECS_EXECUTION", "")
	t.Setenv("TENET_API_TRUST_AUTHENTICATED_HEADERS", "true")
	root := filepath.Join("..", "..", "examples", "ecs-native-repo")
	source := newFilesystemCatalogSource(root)
	dir := t.TempDir()
	plansPath, appliesPath := filepath.Join(dir, "plans.json"), filepath.Join(dir, "applies.json")
	options := newAPIOptionsWithCatalogSourceAndStores(buildinfo.Info{}, source, nil, plansPath, appliesPath)
	createdPlan, err := options.CreateDeploymentPlanPreview(context.Background(), "worker", planpreview.CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := options.CreateApplyRequest(context.Background(), createdPlan.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TENET_AWS_ENABLE_ECS_EXECUTION", "true")
	options = newAPIOptionsWithCatalogSourceAndStores(buildinfo.Info{}, source, nil, plansPath, appliesPath)
	options.RepositoryPolicy = func(_ context.Context, id requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
		return rbacpolicy.Evaluate(rbacpolicy.Policy{Version: "v1", Users: []rbacpolicy.Subject{{ID: "operator-a", Roles: []string{"operator"}, Deployments: []string{"worker"}}}}, id, groups)
	}
	// Rebuild the service with the same policy used by the API and worker.
	service := newApplyService(source, options, plansPath, appliesPath)
	options.CreateApplyRequest = service.Create
	router := api.NewRouter(options)
	contextResponse := httptest.NewRecorder()
	router.ServeHTTP(contextResponse, httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil))
	var runtime struct {
		ECSExecutionEnabled bool `json:"ecsExecutionEnabled"`
	}
	if err := json.Unmarshal(contextResponse.Body.Bytes(), &runtime); err != nil || !runtime.ECSExecutionEnabled {
		t.Fatalf("missing live execution mode: %s", contextResponse.Body.String())
	}
	for _, tc := range []struct {
		name, id string
		status   int
	}{{"actor-only", "", http.StatusUnauthorized}, {"wrong-user", "viewer", http.StatusForbidden}, {"operator", "operator-a", http.StatusCreated}} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/applies/plans/"+createdPlan.Record.ID, nil)
			request.Header.Set(requestactor.HeaderID, "operator-a")
			if tc.id != "" {
				request.Header.Set(requestactor.AuthenticatedHeaderID, tc.id)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Code == http.StatusCreated {
				var record applyrequest.Record
				if err := json.Unmarshal(response.Body.Bytes(), &record); err != nil {
					t.Fatal(err)
				}
				if record.ExecutionReadiness.State != "queued" {
					t.Fatalf("incorrect readiness: %+v", record.ExecutionReadiness)
				}
				if record.Principal == nil || record.Principal.Identity.ID != "operator-a" || record.ID == legacy.Record.ID {
					t.Fatal("trusted approval was lost or legacy request was silently activated")
				}
				executor := newNativeECSExecutor(source, options, plansPath, appliesPath, nil)
				if err := executor.ECS.Authorize(context.Background(), record); err != nil {
					t.Fatal(err)
				}
				record.Principal = nil
				if err := executor.ECS.Authorize(context.Background(), record); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
					t.Fatal("worker accepted record without trusted approval")
				}
			}
		})
	}
}

type fakeECSSimulator struct {
	inputs []*iam.SimulatePrincipalPolicyInput
	mutate func(*iam.SimulatePrincipalPolicyOutput)
}

func (f *fakeECSSimulator) SimulatePrincipalPolicy(_ context.Context, in *iam.SimulatePrincipalPolicyInput, _ ...func(*iam.Options)) (*iam.SimulatePrincipalPolicyOutput, error) {
	f.inputs = append(f.inputs, in)
	out := &iam.SimulatePrincipalPolicyOutput{EvaluationResults: []iamtypes.EvaluationResult{{EvalActionName: aws.String(in.ActionNames[0]), EvalResourceName: aws.String(in.ResourceArns[0]), EvalDecision: iamtypes.PolicyEvaluationDecisionTypeAllowed}}}
	if f.mutate != nil {
		f.mutate(out)
	}
	return out, nil
}
func TestECSCapabilitySimulationUsesConcreteResourcesAndRejectsIncompleteEvidence(t *testing.T) {
	role := applyrequest.DeployerRoleReadiness{RoleARN: "arn:aws:iam::123456789012:role/deployer"}
	checks := []applyrequest.OperationExecutionCapabilityCheck{
		{Context: map[string]string{"ecs:cluster": "arn:aws:ecs:ap-northeast-2:123456789012:cluster/production"}, Action: "ecs:UpdateService", ResourceName: "arn:aws:ecs:ap-northeast-2:123456789012:service/production/worker", RequiredRole: role.RoleARN},
		{Context: map[string]string{"aws:RequestedRegion": "ap-northeast-2"}, Action: "iam:PassRole", ResourceName: "arn:aws:iam::123456789012:role/task", RequiredRole: role.RoleARN},
		{Action: "ecs:TagResource", ResourceName: "arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:*", RequiredRole: role.RoleARN},
		{Action: "ecs:DescribeTaskDefinition", ResourceName: "arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:17", RequiredRole: role.RoleARN},
	}
	fake := &fakeECSSimulator{}
	if err := simulateECSCapabilities(context.Background(), fake, role, checks); err != nil {
		t.Fatal(err)
	}
	if aws.ToString(fake.inputs[0].ContextEntries[0].ContextKeyName) != "ecs:cluster" || fake.inputs[0].ResourceArns[0] != checks[0].ResourceName || fake.inputs[1].ResourceArns[0] != checks[1].ResourceName || len(fake.inputs[1].ContextEntries) != 2 || aws.ToString(fake.inputs[1].ContextEntries[0].ContextKeyName) != "aws:RequestedRegion" || aws.ToString(fake.inputs[1].ContextEntries[1].ContextKeyName) != "iam:PassedToService" || aws.ToString(fake.inputs[2].ContextEntries[0].ContextKeyName) != "ecs:CreateAction" || fake.inputs[3].ResourceArns[0] != "*" {
		t.Fatal("simulation lost concrete resource/action context")
	}
	for _, mutate := range []func(*iam.SimulatePrincipalPolicyOutput){
		func(o *iam.SimulatePrincipalPolicyOutput) {
			o.EvaluationResults[0].EvalDecision = iamtypes.PolicyEvaluationDecisionTypeExplicitDeny
		},
		func(o *iam.SimulatePrincipalPolicyOutput) {
			o.EvaluationResults[0].MissingContextValues = []string{"aws:RequestTag/team"}
		},
		func(o *iam.SimulatePrincipalPolicyOutput) { o.EvaluationResults = nil },
		func(o *iam.SimulatePrincipalPolicyOutput) { o.IsTruncated = true },
		func(o *iam.SimulatePrincipalPolicyOutput) { o.EvaluationResults[0].EvalResourceName = aws.String("*") },
	} {
		if err := simulateECSCapabilities(context.Background(), &fakeECSSimulator{mutate: mutate}, role, checks[:1]); err == nil {
			t.Fatal("unconfirmed simulation was accepted")
		}
	}
}

func TestNativeInfrastructurePassRoleContextDoesNotGetTaskPrincipalAdded(t *testing.T) {
	role := applyrequest.DeployerRoleReadiness{RoleARN: "arn:aws:iam::123456789012:role/deployer"}
	fake := &fakeECSSimulator{}
	checks := []applyrequest.OperationExecutionCapabilityCheck{{Action: "iam:PassRole", ResourceName: "arn:aws:iam::123456789012:role/ecs-infrastructure", RequiredRole: role.RoleARN, Context: map[string]string{"iam:PassedToService": "ecs.amazonaws.com"}}}
	if err := simulateECSCapabilities(context.Background(), fake, role, checks); err != nil {
		t.Fatal(err)
	}
	entries := fake.inputs[0].ContextEntries
	if len(entries) != 1 || aws.ToString(entries[0].ContextKeyName) != "iam:PassedToService" || len(entries[0].ContextKeyValues) != 1 || entries[0].ContextKeyValues[0] != "ecs.amazonaws.com" {
		t.Fatal("native infrastructure PassRole was silently checked as an ECS task role")
	}
}
