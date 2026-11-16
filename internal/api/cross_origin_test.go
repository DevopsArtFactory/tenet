package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/rbacpolicy"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

func TestBrowserWriteOriginProtection(t *testing.T) {
	for _, tc := range []struct {
		name, origin, fetchSite string
		proxy                   bool
		allowed                 bool
	}{
		{name: "sibling origin", origin: "https://untrusted.example.test", fetchSite: "same-site"},
		{name: "cross site", origin: "https://untrusted.test", fetchSite: "cross-site"},
		{name: "origin fallback", origin: "https://untrusted.example.test"},
		{name: "opaque origin", origin: "null"},
		{name: "fetch metadata without origin", fetchSite: "same-site"},
		{name: "different port fallback", origin: "https://tenet.example.test:8443"},
		{name: "trusted proxy still rejects sibling origin", origin: "https://untrusted.example.test", fetchSite: "same-site", proxy: true},
		{name: "same origin", origin: "https://tenet.example.test", fetchSite: "same-origin", allowed: true},
		{name: "same origin fallback", origin: "https://tenet.example.test", allowed: true},
		{name: "non browser session", allowed: true},
		{name: "non browser trusted proxy", proxy: true, allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
			options.TrustAuthenticatedHeaders = tc.proxy
			options.RequireOperatorRoleForWrite = true
			options.RequireDeploymentReadAuthorization = true
			options.AuthorizeWithRepositoryPolicy = true
			policy := rbacpolicy.Policy{Version: "v1", Users: []rbacpolicy.Subject{{ID: "github:12345", Roles: []string{"operator"}, Deployments: []string{"fixture-app"}}}}
			options.RepositoryPolicy = func(_ context.Context, id requestactor.AuthenticatedIdentity, g requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
				return rbacpolicy.Evaluate(policy, id, g)
			}
			planCalls, applyCalls, lookupCalls := 0, 0, 0
			options.CreateDeploymentPlanPreview = func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
				planCalls++
				return planpreview.CreateResult{Created: true}, nil
			}
			options.PlanPreview = func(context.Context, string) (planpreview.Record, error) {
				lookupCalls++
				return planpreview.Record{DeploymentName: "fixture-app"}, nil
			}
			options.CreateApplyRequest = func(context.Context, string) (applyrequest.CreateResult, error) {
				applyCalls++
				return applyrequest.CreateResult{Created: true}, nil
			}
			router := NewRouter(options)
			// Provider callbacks remain valid cross-site GETs with their OAuth state checks.
			cookie := managedLoginCallbackCookie(t, router, func(r *http.Request) {
				r.Header.Set("Sec-Fetch-Site", "cross-site")
				r.Header.Set("Origin", "https://github.com")
			})
			for _, route := range []string{"/v1/plans/deployments/fixture-app", "/v1/applies/plans/known-plan"} {
				req := httptest.NewRequest(http.MethodPost, "https://tenet.example.test"+route, nil)
				req.Header.Set("Origin", tc.origin)
				req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if tc.proxy {
					req.Header.Set(requestactor.AuthenticatedHeaderID, "github:12345")
				} else {
					req.AddCookie(cookie)
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				want := http.StatusForbidden
				if tc.allowed {
					want = http.StatusCreated
				}
				if rec.Code != want {
					t.Errorf("%s: status=%d want=%d body=%s", route, rec.Code, want, rec.Body.String())
				}
				assertResponseRequestID(t, rec)
			}
			if tc.allowed {
				if planCalls != 1 || applyCalls != 1 || lookupCalls == 0 {
					t.Fatalf("legitimate write blocked: plan=%d apply=%d lookup=%d", planCalls, applyCalls, lookupCalls)
				}
			} else if planCalls != 0 || applyCalls != 0 || lookupCalls != 0 {
				t.Fatalf("cross-origin request reached providers: plan=%d apply=%d lookup=%d", planCalls, applyCalls, lookupCalls)
			}
		})
	}
}
