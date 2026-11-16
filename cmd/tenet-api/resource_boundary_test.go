package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/rbacpolicy"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/DevopsArtFactory/tenet/internal/resourceaws"
	"github.com/DevopsArtFactory/tenet/internal/resourceinspection"
	"github.com/DevopsArtFactory/tenet/internal/resourcelogs"
)

func TestCrossDeploymentTaskReadsAreDenied(t *testing.T) {
	const task = "arn:aws:ecs:ap-northeast-2:111111111111:task/private-cluster/01234567890123456789012345678901"
	const cluster = "arn:aws:ecs:ap-northeast-2:111111111111:cluster/private-cluster"
	loadDetail := func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
		return catalog.DeploymentOverviewDetail{Name: name, AWSAccountID: "111111111111", Region: "ap-northeast-2", ExecutionMode: "ecsNativeRolling", ECS: &domain.ECSDeploymentIntent{ClusterARN: "arn:aws:ecs:ap-northeast-2:111111111111:cluster/allowed-cluster", ServiceARN: "arn:aws:ecs:ap-northeast-2:111111111111:service/allowed-cluster/allowed-service"}}, nil
	}
	lookup := newOverviewDeploymentResourceLookupProvider(loadDetail, func(context.Context, string) (string, error) { return "arn:aws:iam::111111111111:role/Observer", nil }, func(context.Context, resourceaws.ECSTaskScope) error {
		t.Fatal("foreign task reached AWS authorization")
		return nil
	})
	inspectionCalls := 0
	inspector := resourceinspection.NewService(resourceinspection.Options{InspectECSTask: func(_ context.Context, r resourceinspection.ECSTaskRequest) (resourceinspection.ECSTaskInspection, error) {
		inspectionCalls++
		if r.ResourceName != task || r.ClusterARN != cluster {
			t.Fatalf("unexpected lookup: %+v", r)
		}
		return resourceinspection.ECSTaskInspection{ECSTaskSummary: resourceinspection.ECSTaskSummary{TaskARN: task, ClusterARN: cluster}, TaskDefinition: "private-service-definition"}, nil
	}})
	logs := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true, Entries: []resourcelogs.Entry{{Message: "private-service-log-marker"}}}}
	policy := rbacpolicy.Policy{Version: "v1", Users: []rbacpolicy.Subject{{ID: "github:viewer", Roles: []string{"viewer"}, Deployments: []string{"allowed-deployment"}}}}
	router := api.NewRouter(api.Options{TrustAuthenticatedHeaders: true, RequireAuthenticatedIdentity: true, RequireDeploymentReadAuthorization: true, AuthorizeWithRepositoryPolicy: true,
		RepositoryPolicy: func(_ context.Context, id requestactor.AuthenticatedIdentity, g requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
			return rbacpolicy.Evaluate(policy, id, g)
		},
		OverviewDeployment: loadDetail, OverviewDeploymentResourceInspection: newOverviewDeploymentResourceInspectionProvider(lookup, inspector), OverviewDeploymentResourceLogs: newOverviewDeploymentResourceLogsProvider(lookup, logs),
	})
	for _, deployment := range []string{"private-deployment", "allowed-deployment"} {
		for _, route := range []string{"resource-inspection", "resource-logs"} {
			q := url.Values{"resourceType": {"ecsTask"}, "resourceName": {task}, "clusterArn": {cluster}}
			req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/"+deployment+"/"+route+"?"+q.Encode(), nil)
			req.Header.Set(requestactor.AuthenticatedHeaderID, "github:viewer")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			t.Logf("deployment=%s route=%s status=%d", deployment, route, rec.Code)
			if deployment == "private-deployment" && rec.Code != http.StatusForbidden {
				t.Fatal("control request must be forbidden")
			}
			if deployment == "allowed-deployment" && rec.Code != http.StatusNotFound {
				t.Fatalf("cross-deployment task accepted: %d %s", rec.Code, rec.Body.String())
			}
		}
	}
	if inspectionCalls != 0 || logs.calls != 0 {
		t.Fatalf("calls inspection=%d logs=%d", inspectionCalls, logs.calls)
	}
}

func TestECSTaskLookupAuthorizesBothReaders(t *testing.T) {
	const cluster = "arn:aws:ecs:ap-northeast-2:111111111111:cluster/shared"
	const task = "arn:aws:ecs:ap-northeast-2:111111111111:task/shared/01234567890123456789012345678901"
	for _, tc := range []struct {
		name                                      string
		missingIntent, missingVerifier, wrongHint bool
		authErr                                   error
		wantChecks, wantReads                     int
	}{
		{name: "owned task", wantChecks: 2, wantReads: 2},
		{name: "other service in same cluster", authErr: resourceaws.ErrECSTaskScope, wantChecks: 2},
		{name: "AWS failure", authErr: context.DeadlineExceeded, wantChecks: 2},
		{name: "missing pinned intent", missingIntent: true},
		{name: "missing verifier", missingVerifier: true},
		{name: "forged selector", wrongHint: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checks := 0
			load := func(context.Context, string) (catalog.DeploymentOverviewDetail, error) {
				d := catalog.DeploymentOverviewDetail{Name: "allowed", AWSAccountID: "111111111111", Region: "ap-northeast-2", ExecutionMode: "ecsNativeRolling", ECS: &domain.ECSDeploymentIntent{ClusterARN: cluster, ServiceARN: "arn:aws:ecs:ap-northeast-2:111111111111:service/shared/allowed"}}
				if tc.missingIntent {
					d.ECS = nil
				}
				return d, nil
			}
			var verify func(context.Context, resourceaws.ECSTaskScope) error
			if !tc.missingVerifier {
				verify = func(_ context.Context, s resourceaws.ECSTaskScope) error {
					checks++
					if s.ClusterARN != cluster || s.ServiceARN != "arn:aws:ecs:ap-northeast-2:111111111111:service/shared/allowed" || s.TaskARN != task {
						t.Fatalf("unexpected authorization target: %+v", s)
					}
					return tc.authErr
				}
			}
			lookup := newOverviewDeploymentResourceLookupProvider(load, func(context.Context, string) (string, error) { return "", nil }, verify)
			logs := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
			inspect := &fakeResourceInspectionProvider{result: resourceinspection.Result{Available: true}}
			logCtx, inspectionCtx := context.Background(), context.Background()
			if tc.wrongHint {
				logCtx = resourcelogs.ContextWithClusterARN(logCtx, "arn:aws:ecs:ap-northeast-2:111111111111:cluster/private")
				inspectionCtx = resourceinspection.ContextWithClusterARN(inspectionCtx, "arn:aws:ecs:ap-northeast-2:111111111111:cluster/private")
			}
			_, logErr := newOverviewDeploymentResourceLogsProvider(lookup, logs)(logCtx, "allowed", "ecsTask", task, 10)
			_, inspectionErr := newOverviewDeploymentResourceInspectionProvider(lookup, inspect)(inspectionCtx, "allowed", "ecsTask", task, 10)
			if tc.wantReads > 0 {
				if logErr != nil || inspectionErr != nil {
					t.Fatalf("owned task rejected: %v %v", logErr, inspectionErr)
				}
				if logs.lastRequest.ClusterARN != cluster || inspect.lastRequest.ClusterARN != cluster {
					t.Fatal("trusted cluster did not reach readers")
				}
			} else if logErr == nil || inspectionErr == nil {
				t.Fatal("unverified task admitted")
			}
			if checks != tc.wantChecks || logs.calls+inspect.calls != tc.wantReads {
				t.Fatalf("checks=%d reads=%d", checks, logs.calls+inspect.calls)
			}
		})
	}
}
