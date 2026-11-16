package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/authidentity"
	"github.com/DevopsArtFactory/tenet/internal/authprovider"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/rbacpolicy"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

func TestOIDCEmailAuthorizationRequiresVerification(t *testing.T) {
	for _, name := range []string{"google", "okta_oidc"} {
		for _, verification := range []string{"false", "missing", "null", "true"} {
			for _, subjectGrant := range []bool{false, true} {
				grant := "email"
				if subjectGrant {
					grant = "subject"
				}
				t.Run(name+"/"+verification+"/"+grant, func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						if r.URL.Path == "/token" {
							_, _ = w.Write([]byte(`{"access_token":"fixture-token","token_type":"Bearer"}`))
							return
						}
						profile := map[string]any{"sub": "fixture-subject", "email": "approved@example.test", "preferred_username": "approved@example.test"}
						if verification != "missing" {
							profile["email_verified"] = verification == "true"
						}
						if verification == "null" {
							profile["email_verified"] = nil
						}
						_ = json.NewEncoder(w).Encode(profile)
					}))
					defer server.Close()
					var provider authprovider.Provider
					var err error
					loginPath, callbackPath := "/auth/login/google", "/auth/callback/google"
					if name == "google" {
						provider, err = authprovider.NewGoogleProvider(authprovider.GoogleConfig{ClientID: "fixture", ClientSecret: "fixture", RedirectURL: "http://tenet.invalid" + callbackPath, AuthURL: server.URL + "/authorize", TokenURL: server.URL + "/token", UserInfoURL: server.URL + "/userinfo"})
					} else {
						loginPath, callbackPath = "/auth/login/okta", "/auth/callback/okta/oidc"
						provider, err = authprovider.NewOktaOIDCProvider(authprovider.OktaOIDCConfig{ClientID: "fixture", ClientSecret: "fixture", RedirectURL: "http://tenet.invalid" + callbackPath, IssuerURL: server.URL, AuthURL: server.URL + "/authorize", TokenURL: server.URL + "/token", UserInfoURL: server.URL + "/userinfo"})
					}
					if err != nil {
						t.Fatal(err)
					}
					options, _ := managedLoginTestOptions(t, provider)
					// A prior login must not preserve an email that is no longer verified.
					previous, err := provider.User(context.Background(), "fixture-token")
					if err != nil {
						t.Fatal(err)
					}
					previous.Email = "approved@example.test"
					persisted := authidentity.FromProviderIdentity(previous)
					if _, err := options.AuthIdentityStore.Upsert(context.Background(), persisted); err != nil {
						t.Fatal(err)
					}
					options.RequireOperatorRoleForWrite = true
					options.RequireDeploymentReadAuthorization = true
					options.AuthorizeWithRepositoryPolicy = true
					user := rbacpolicy.Subject{Email: "approved@example.test", Roles: []string{"operator"}, Deployments: []string{"fixture-app"}}
					if subjectGrant {
						user.Email = ""
						user.ID = name + ":fixture-subject"
					}
					policy := rbacpolicy.Policy{Version: "v1", Users: []rbacpolicy.Subject{user}}
					options.RepositoryPolicy = func(_ context.Context, id requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
						return rbacpolicy.Evaluate(policy, id, groups)
					}
					calls := 0
					options.CreateDeploymentPlanPreview = func(context.Context, string, planpreview.CreateRequest) (planpreview.CreateResult, error) {
						calls++
						return planpreview.CreateResult{Created: true}, nil
					}
					router := NewRouter(options)
					login := httptest.NewRecorder()
					router.ServeHTTP(login, httptest.NewRequest(http.MethodGet, loginPath, nil))
					if login.Code != http.StatusFound {
						t.Fatalf("login: %d %s", login.Code, login.Body.String())
					}
					location, err := url.Parse(login.Header().Get("Location"))
					if err != nil {
						t.Fatal(err)
					}
					request := httptest.NewRequest(http.MethodGet, callbackPath+"?state="+url.QueryEscape(location.Query().Get("state"))+"&code=fixture", nil)
					for _, cookie := range login.Result().Cookies() {
						request.AddCookie(cookie)
					}
					callback := httptest.NewRecorder()
					router.ServeHTTP(callback, request)
					if callback.Code != http.StatusFound {
						t.Fatalf("callback: %d %s", callback.Code, callback.Body.String())
					}
					request = httptest.NewRequest(http.MethodPost, "/v1/plans/deployments/fixture-app", strings.NewReader("{}"))
					for _, cookie := range callback.Result().Cookies() {
						if cookie.MaxAge >= 0 {
							request.AddCookie(cookie)
						}
					}
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					status, wantCalls := http.StatusForbidden, 0
					if verification == "true" || subjectGrant {
						status, wantCalls = http.StatusCreated, 1
					}
					if response.Code != status || calls != wantCalls {
						t.Fatalf("status=%d calls=%d; want status=%d calls=%d: %s", response.Code, calls, status, wantCalls, response.Body.String())
					}
					stored, err := options.AuthIdentityStore.Get(context.Background(), name, "fixture-subject")
					if err != nil {
						t.Fatal(err)
					}
					wantEmail := ""
					if verification == "true" {
						wantEmail = "approved@example.test"
					}
					if stored.Email != wantEmail {
						t.Fatalf("persisted email=%q, want %q", stored.Email, wantEmail)
					}
				})
			}
		}
	}
}

func TestManagedSessionRejectsLegacyEmailClaims(t *testing.T) {
	options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
	// Reproduce the old on-wire format, independently of the current signer.
	payload, err := json.Marshal(map[string]any{"provider": "google", "subject": "fixture", "email": "approved@example.test", "expiresAt": time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	request := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	request.AddCookie(&http.Cookie{Name: defaultAuthSessionCookieName, Value: encoded + "." + authSessionSignature(encoded, options.AuthSessionSecret)})
	cfg := authSessionConfigFromOptions(options)
	ctx := contextWithAuthSession(request.Context(), request, cfg)
	if _, ok := parseAuthSessionClaims(encoded+"."+authSessionSignature(encoded, options.AuthSessionSecret), options.AuthSessionSecret, time.Now()); ok {
		t.Fatal("accepted legacy session claims")
	}
	if ctx != request.Context() {
		t.Fatal("legacy cookie established an authenticated context")
	}
}
