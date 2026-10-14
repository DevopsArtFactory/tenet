package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

func TestECSRecoveryRoutesRequireWriteIdentityRoleAndDeploymentScope(t *testing.T) {
	for _, action := range []string{"rollback", "retry"} {
		for _, tc := range []struct {
			name, id, role, scope string
			status                int
		}{
			{"actor-only", "", "operator", "worker", http.StatusUnauthorized},
			{"viewer", "user", "viewer", "worker", http.StatusForbidden},
			{"wrong-deployment", "user", "operator", "other", http.StatusForbidden},
			{"operator", "user", "operator", "worker", http.StatusCreated},
		} {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				calls := 0
				callback := func(_ context.Context, id string) (applyrequest.CreateResult, error) {
					calls++
					if id != "forward" {
						t.Fatal("lost parent identity")
					}
					return applyrequest.CreateResult{Created: true, Record: applyrequest.Record{ID: "recovery", DeploymentName: "worker", Status: applyrequest.StatusQueued}}, nil
				}
				options := Options{TrustAuthenticatedHeaders: true, RequireAuthenticatedIdentity: true, RequireOperatorRoleForWrite: true, RequireDeploymentReadAuthorization: true, ApplyRequest: func(context.Context, string) (applyrequest.Record, error) {
					return applyrequest.Record{ID: "forward", DeploymentName: "worker", Status: applyrequest.StatusFailed}, nil
				}, RequestApplyRollback: callback, RetryApplyRequest: callback}
				request := httptest.NewRequest(http.MethodPost, "/v1/applies/forward/"+action, strings.NewReader(`{"taskDefinitionArn":"untrusted-caller-target"}`))
				request.Header.Set(requestactor.HeaderID, "operator")
				if tc.id != "" {
					request.Header.Set(requestactor.AuthenticatedHeaderID, tc.id)
				}
				request.Header.Set(requestactor.AuthenticatedRolesHeader, tc.role)
				request.Header.Set(requestactor.AuthorizedDeploymentsHeader, tc.scope)
				response := httptest.NewRecorder()
				NewRouter(options).ServeHTTP(response, request)
				if response.Code != tc.status {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
				expected := 0
				if tc.status == http.StatusCreated {
					expected = 1
					if response.Header().Get("Location") != "/v1/applies/recovery" {
						t.Fatal("missing child execution link")
					}
				}
				if calls != expected {
					t.Fatal("unauthorized recovery reached the execution service")
				}
			})
		}
	}
}

func TestECSRecoveryConflictErrorsAreExplicit(t *testing.T) {
	for _, err := range []error{applyrequest.ErrRecoveryUnavailable, applyrequest.ErrFailedIntentHeld, errors.Join(applyrequest.ErrRecoveryUnavailable, errors.New("intervening deployment"))} {
		response := httptest.NewRecorder()
		respondApplyCreateError(response, err)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), err.Error()) {
			t.Fatalf("recovery conflict was hidden: %d %s", response.Code, response.Body.String())
		}
	}
}
