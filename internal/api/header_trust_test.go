package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/rbacpolicy"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

func TestAuthenticatedHeadersRequireExplicitTrust(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		trust, session, allowedSession bool
		status                         int
	}{
		{"forged identity without session", false, false, false, http.StatusUnauthorized},
		{"forged grants with unprivileged session", false, true, false, http.StatusForbidden},
		{"authorized managed session", false, true, true, http.StatusCreated},
		{"explicitly trusted proxy", true, false, false, http.StatusCreated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
			options.TrustAuthenticatedHeaders = tc.trust
			options.RequireOperatorRoleForWrite = true
			options.RequireDeploymentReadAuthorization = true
			options.AuthorizeWithRepositoryPolicy = true
			policy := rbacpolicy.Policy{Version: "v1", Users: []rbacpolicy.Subject{{ID: "github:approved", Roles: []string{"operator"}, Deployments: []string{"fixture-app"}}}}
			policy.Groups = []rbacpolicy.GroupGrant{{Name: "administrators", Roles: []string{"admin"}, Deployments: []string{"fixture-app"}}}
			if tc.allowedSession {
				policy.Users = append(policy.Users, rbacpolicy.Subject{ID: "github:12345", Roles: []string{"operator"}, Deployments: []string{"fixture-app"}})
			}
			options.RepositoryPolicy = func(_ context.Context, id requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
				return rbacpolicy.Evaluate(policy, id, groups)
			}
			calls := 0
			options.CreateDeploymentPlanPreview = func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
				calls++
				return planpreview.CreateResult{Created: true}, nil
			}
			router := NewRouter(options)
			req := httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/fixture-app", strings.NewReader("{}"))
			if tc.session {
				req.AddCookie(managedLoginCallbackCookie(t, router, nil))
			}
			req.Header.Set(requestactor.AuthenticatedHeaderID, "github:approved")
			req.Header.Set(requestactor.AuthenticatedRolesHeader, "admin")
			req.Header.Set(requestactor.AuthenticatedGroupsHeader, "administrators")
			req.Header.Set(requestactor.AuthorizedDeploymentsHeader, "*")
			req.Header.Set(requestactor.AuthorizedProjectsHeader, "*")
			req.Header.Set(requestactor.AuthorizedEnvironmentsHeader, "*")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.status, response.Body.String())
			}
			wantCalls := 0
			if tc.status == http.StatusCreated {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("write handler called %d times, want %d", calls, wantCalls)
			}
		})
	}
}
