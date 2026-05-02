package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/authidentity"
	"github.com/DevopsArtFactory/tenet/internal/authprovider"
	"github.com/DevopsArtFactory/tenet/internal/authstate"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

type fakeAuthProvider struct {
	exchangedCodes []string
	userTokens     []string
	exchangeErr    error
	userErr        error
}

func (p *fakeAuthProvider) Name() string { return authprovider.GitHubProviderName }

func (p *fakeAuthProvider) AuthCodeURL(state string, scopes []string) (string, error) {
	values := url.Values{}
	values.Set("state", state)
	values.Set("scope", strings.Join(scopes, " "))
	return "https://github.example.test/login/oauth/authorize?" + values.Encode(), nil
}

func (p *fakeAuthProvider) Exchange(_ context.Context, code string) (authprovider.Token, error) {
	p.exchangedCodes = append(p.exchangedCodes, code)
	if p.exchangeErr != nil {
		return authprovider.Token{}, p.exchangeErr
	}
	return authprovider.Token{AccessToken: "token-for-" + code, TokenType: "bearer"}, nil
}

func (p *fakeAuthProvider) User(_ context.Context, accessToken string) (authprovider.Identity, error) {
	p.userTokens = append(p.userTokens, accessToken)
	if p.userErr != nil {
		return authprovider.Identity{}, p.userErr
	}
	return authprovider.Identity{
		Provider: authprovider.GitHubProviderName,
		ID:       "12345",
		Login:    "octo",
		Name:     "Octo Operator",
		Email:    "octo@example.com",
		URL:      "https://github.example.test/octo",
	}, nil
}

func managedLoginTestOptions(t *testing.T, provider authprovider.Provider) (Options, string) {
	t.Helper()
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "identities.json")
	return Options{
		AuthRuntime: AuthRuntimeDiagnostic{TenetManagedLogin: TenetManagedLoginDiagnostic{
			Configured:              true,
			Active:                  true,
			Provider:                authprovider.GitHubProviderName,
			RequiredSettings:        RequiredTenetManagedLoginSettings(),
			MissingSettings:         []string{},
			LoginRoute:              "/auth/login/github",
			CallbackRoute:           "/auth/callback/github",
			SessionCookieName:       "tenet_session",
			StateStoreConfigured:    true,
			IdentityStoreConfigured: true,
			SessionSecretConfigured: true,
		}},
		AuthProvider:                 provider,
		AuthStateStore:               authstate.NewFileStore(filepath.Join(dir, "state.json")),
		AuthIdentityStore:            authidentity.NewFileStore(identityPath),
		AuthSessionSecret:            "test-session-secret",
		AuthSessionCookieName:        "tenet_session",
		RequireAuthenticatedIdentity: true,
	}, identityPath
}

func managedLoginCallbackCookie(t *testing.T, router http.Handler, mutateCallback func(*http.Request)) *http.Cookie {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodGet, "/auth/login/github?returnUrl=/", nil)
	loginResp := httptest.NewRecorder()
	router.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusFound {
		t.Fatalf("expected login 302, got %d: %s", loginResp.Code, loginResp.Body.String())
	}
	location, err := url.Parse(loginResp.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse login redirect: %v", err)
	}
	state := location.Query().Get("state")
	if state == "" {
		t.Fatalf("expected login redirect state, got %q", loginResp.Header().Get("Location"))
	}
	callbackURL := "/auth/callback/github?state=" + url.QueryEscape(state) + "&code=code-123"
	callbackReq := httptest.NewRequest(http.MethodGet, callbackURL, nil)
	if mutateCallback != nil {
		mutateCallback(callbackReq)
	}
	callbackResp := httptest.NewRecorder()
	router.ServeHTTP(callbackResp, callbackReq)
	if callbackResp.Code != http.StatusFound {
		t.Fatalf("expected callback 302, got %d: %s", callbackResp.Code, callbackResp.Body.String())
	}
	cookies := callbackResp.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "tenet_session" {
		t.Fatalf("expected one tenet_session cookie, got %#v", cookies)
	}
	return cookies[0]
}

func TestManagedGitHubLoginCallbackCreatesSessionAndAuthenticatedContext(t *testing.T) {
	provider := &fakeAuthProvider{}
	options, identityPath := managedLoginTestOptions(t, provider)
	router := NewRouter(options)

	loginReq := httptest.NewRequest(http.MethodGet, "/auth/login/github?returnUrl=/v1/operator/context%3Ffrom%3Dlogin", nil)
	loginResp := httptest.NewRecorder()
	router.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusFound {
		t.Fatalf("expected login 302, got %d: %s", loginResp.Code, loginResp.Body.String())
	}
	location := loginResp.Header().Get("Location")
	parsedLocation, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse login redirect: %v", err)
	}
	state := parsedLocation.Query().Get("state")
	if state == "" {
		t.Fatalf("expected login redirect to carry callback state, got %q", location)
	}
	if got := parsedLocation.Query().Get("scope"); !strings.Contains(got, "read:user") || !strings.Contains(got, "user:email") {
		t.Fatalf("expected GitHub scopes in redirect, got %q", got)
	}

	callbackReq := httptest.NewRequest(http.MethodGet, "/auth/callback/github?state="+url.QueryEscape(state)+"&code=code-123", nil)
	callbackResp := httptest.NewRecorder()
	router.ServeHTTP(callbackResp, callbackReq)
	if callbackResp.Code != http.StatusFound {
		t.Fatalf("expected callback 302, got %d: %s", callbackResp.Code, callbackResp.Body.String())
	}
	if got := callbackResp.Header().Get("Location"); got != "/v1/operator/context?from=login" {
		t.Fatalf("expected safe in-app redirect, got %q", got)
	}
	cookies := callbackResp.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "tenet_session" || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected one HttpOnly SameSite=Lax tenet_session cookie, got %#v", cookies)
	}
	if len(provider.exchangedCodes) != 1 || provider.exchangedCodes[0] != "code-123" {
		t.Fatalf("expected provider exchange for callback code, got %#v", provider.exchangedCodes)
	}
	identities, err := authidentity.Load(identityPath)
	if err != nil {
		t.Fatalf("load identities: %v", err)
	}
	if len(identities) != 1 || identities[0].Provider != "github" || identities[0].Subject != "12345" {
		t.Fatalf("expected persisted GitHub identity, got %#v", identities)
	}

	reuseReq := httptest.NewRequest(http.MethodGet, "/auth/callback/github?state="+url.QueryEscape(state)+"&code=code-456", nil)
	reuseResp := httptest.NewRecorder()
	router.ServeHTTP(reuseResp, reuseReq)
	if reuseResp.Code != http.StatusBadRequest || !strings.Contains(reuseResp.Body.String(), authstate.ErrConsumed.Error()) {
		t.Fatalf("expected consumed state 400, got %d: %s", reuseResp.Code, reuseResp.Body.String())
	}

	contextReq := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	contextReq.AddCookie(cookies[0])
	contextResp := httptest.NewRecorder()
	router.ServeHTTP(contextResp, contextReq)
	if contextResp.Code != http.StatusOK {
		t.Fatalf("expected operator context 200, got %d: %s", contextResp.Code, contextResp.Body.String())
	}
	var body struct {
		AuthRuntime AuthRuntimeDiagnostic              `json:"authRuntime"`
		Identity    requestactor.AuthenticatedIdentity `json:"identity"`
		Actor       requestactor.Actor                 `json:"actor"`
	}
	if err := json.Unmarshal(contextResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context: %v", err)
	}
	if !body.AuthRuntime.TenetManagedLogin.Active || body.AuthRuntime.TenetManagedLogin.UnavailableReason != "" {
		t.Fatalf("expected active managed login diagnostic, got %#v", body.AuthRuntime.TenetManagedLogin)
	}
	if !body.Identity.Authenticated || body.Identity.ID != "github:12345" || body.Identity.Name != "Octo Operator" || body.Identity.Email != "octo@example.com" {
		t.Fatalf("expected session-derived authenticated identity, got %#v", body.Identity)
	}
	if !body.Actor.Available || body.Actor.ID != "github:12345" {
		t.Fatalf("expected actor projection from session identity, got %#v", body.Actor)
	}
}

func TestManagedGitHubLoginCallbackSecureCookieModes(t *testing.T) {
	t.Run("local http remains usable without secure cookie", func(t *testing.T) {
		options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
		cookie := managedLoginCallbackCookie(t, NewRouter(options), nil)
		if cookie.Secure {
			t.Fatalf("expected local HTTP callback to leave cookie Secure=false, got %#v", cookie)
		}
		if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("expected HttpOnly SameSite=Lax cookie, got %#v", cookie)
		}
	})

	t.Run("direct https request sets secure cookie", func(t *testing.T) {
		options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
		cookie := managedLoginCallbackCookie(t, NewRouter(options), func(req *http.Request) {
			req.URL.Scheme = "https"
			req.TLS = &tls.ConnectionState{}
		})
		if !cookie.Secure {
			t.Fatalf("expected HTTPS callback to set cookie Secure=true, got %#v", cookie)
		}
	})

	t.Run("forwarded https request sets secure cookie", func(t *testing.T) {
		options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
		cookie := managedLoginCallbackCookie(t, NewRouter(options), func(req *http.Request) {
			req.Header.Set("X-Forwarded-Proto", "https")
		})
		if !cookie.Secure {
			t.Fatalf("expected HTTPS front-door callback to set cookie Secure=true, got %#v", cookie)
		}
	})

	t.Run("explicit secure option sets secure cookie behind front door", func(t *testing.T) {
		options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
		options.AuthSessionCookieSecure = true
		cookie := managedLoginCallbackCookie(t, NewRouter(options), nil)
		if !cookie.Secure {
			t.Fatalf("expected forced secure cookie option to set Secure=true, got %#v", cookie)
		}
	})
}

func TestManagedGitHubLoginRejectsUnsafeReturnURL(t *testing.T) {
	options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
	router := NewRouter(options)

	for _, returnURL := range []string{"https://evil.example/", "//evil.example/path", "settings"} {
		req := httptest.NewRequest(http.MethodGet, "/auth/login/github?returnUrl="+url.QueryEscape(returnURL), nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected unsafe return URL %q to fail with 400, got %d", returnURL, resp.Code)
		}
	}
}

func TestManagedGitHubSessionDoesNotOverrideTrustedFrontDoorIdentity(t *testing.T) {
	options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
	options.TrustAuthenticatedHeaders = true
	cookieValue, err := signAuthSessionClaims(authSessionClaims{
		Provider: "github",
		Subject:  "session-subject",
		Name:     "Session Operator",
		Email:    "session@example.com",
		Expires:  time.Now().UTC().Add(time.Hour),
	}, options.AuthSessionSecret)
	if err != nil {
		t.Fatalf("sign session: %v", err)
	}
	router := NewRouter(options)

	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	req.AddCookie(&http.Cookie{Name: "tenet_session", Value: cookieValue})
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-id")
	req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
	req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body struct {
		Identity requestactor.AuthenticatedIdentity `json:"identity"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Identity.ID != "front-door-id" || body.Identity.Email != "frontdoor@example.com" {
		t.Fatalf("expected trusted front-door identity to win over session, got %#v", body.Identity)
	}
}
