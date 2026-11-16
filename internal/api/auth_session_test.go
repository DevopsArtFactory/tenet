package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/authidentity"
	"github.com/DevopsArtFactory/tenet/internal/authprovider"
	"github.com/DevopsArtFactory/tenet/internal/authstate"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

type fakeAuthProvider struct {
	name           string
	authBaseURL    string
	exchangedCodes []string
	userTokens     []string
	exchangeErr    error
	userErr        error
}

func (p *fakeAuthProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return authprovider.GitHubProviderName
}

func (p *fakeAuthProvider) AuthCodeURL(state string, scopes []string) (string, error) {
	values := url.Values{}
	values.Set("state", state)
	values.Set("scope", strings.Join(scopes, " "))
	baseURL := p.authBaseURL
	if baseURL == "" {
		baseURL = "https://github.example.test/login/oauth/authorize"
	}
	return baseURL + "?" + values.Encode(), nil
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
		Provider: p.Name(),
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
	providerName := authprovider.GitHubProviderName
	loginRoute := "/auth/login/github"
	callbackRoute := "/auth/callback/github"
	requiredSettings := RequiredTenetManagedLoginSettings()
	if provider != nil && provider.Name() == authprovider.GoogleProviderName {
		providerName = authprovider.GoogleProviderName
		loginRoute = "/auth/login/google"
		callbackRoute = "/auth/callback/google"
		requiredSettings = []string{"TENET_AUTH_PROVIDER", "TENET_GOOGLE_OIDC_CLIENT_ID", "TENET_GOOGLE_OIDC_CLIENT_SECRET", "TENET_GOOGLE_OIDC_ISSUER_URL", "TENET_GOOGLE_OIDC_REDIRECT_URL", "TENET_AUTH_STATE_STORE_PATH", "TENET_AUTH_IDENTITY_STORE_PATH", "TENET_AUTH_SESSION_SECRET"}
	} else if provider != nil && provider.Name() == authprovider.OktaOIDCProviderName {
		providerName = authprovider.OktaOIDCProviderName
		loginRoute = "/auth/login/okta"
		callbackRoute = "/auth/callback/okta/oidc"
		requiredSettings = []string{"TENET_AUTH_PROVIDER", "TENET_OKTA_OIDC_ISSUER_URL", "TENET_OKTA_OIDC_CLIENT_ID", "TENET_OKTA_OIDC_CLIENT_SECRET", "TENET_OKTA_OIDC_REDIRECT_URL", "TENET_AUTH_STATE_STORE_PATH", "TENET_AUTH_IDENTITY_STORE_PATH", "TENET_AUTH_SESSION_SECRET"}
	}
	return Options{
		AuthRuntime: AuthRuntimeDiagnostic{TenetManagedLogin: TenetManagedLoginDiagnostic{
			Configured:              true,
			Active:                  true,
			Provider:                providerName,
			RequiredSettings:        requiredSettings,
			MissingSettings:         []string{},
			LoginRoute:              loginRoute,
			CallbackRoute:           callbackRoute,
			LogoutRoute:             "/auth/logout",
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
	for _, cookie := range loginResp.Result().Cookies() {
		callbackReq.AddCookie(cookie)
	}
	if mutateCallback != nil {
		mutateCallback(callbackReq)
	}
	callbackResp := httptest.NewRecorder()
	router.ServeHTTP(callbackResp, callbackReq)
	if callbackResp.Code != http.StatusFound {
		t.Fatalf("expected callback 302, got %d: %s", callbackResp.Code, callbackResp.Body.String())
	}
	cookies := callbackResp.Result().Cookies()
	if len(cookies) != 2 || cookies[0].Name != "tenet_session" {
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
	for _, cookie := range loginResp.Result().Cookies() {
		callbackReq.AddCookie(cookie)
	}
	callbackResp := httptest.NewRecorder()
	router.ServeHTTP(callbackResp, callbackReq)
	if callbackResp.Code != http.StatusFound {
		t.Fatalf("expected callback 302, got %d: %s", callbackResp.Code, callbackResp.Body.String())
	}
	if got := callbackResp.Header().Get("Location"); got != "/v1/operator/context?from=login" {
		t.Fatalf("expected safe in-app redirect, got %q", got)
	}
	cookies := callbackResp.Result().Cookies()
	if len(cookies) != 2 || cookies[0].Name != "tenet_session" || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected one HttpOnly SameSite=Lax tenet_session cookie, got %#v", cookies)
	}
	if len(provider.exchangedCodes) != 1 || provider.exchangedCodes[0] != "code-123" {
		t.Fatalf("expected provider exchange for callback code, got %#v", provider.exchangedCodes)
	}
	if len(provider.userTokens) != 1 || provider.userTokens[0] != "token-for-code-123" {
		t.Fatalf("expected provider token to be used only for profile lookup, got %#v", provider.userTokens)
	}
	identities, err := authidentity.Load(identityPath)
	if err != nil {
		t.Fatalf("load identities: %v", err)
	}
	if len(identities) != 1 || identities[0].Provider != "github" || identities[0].Subject != "12345" {
		t.Fatalf("expected persisted GitHub identity, got %#v", identities)
	}
	identityJSON, err := json.Marshal(identities)
	if err != nil {
		t.Fatalf("marshal persisted identities: %v", err)
	}
	if strings.Contains(string(identityJSON), "token-for-code-123") || strings.Contains(string(identityJSON), "accessToken") {
		t.Fatalf("provider access token leaked into persisted identity JSON: %s", identityJSON)
	}

	reuseReq := httptest.NewRequest(http.MethodGet, "/auth/callback/github?state="+url.QueryEscape(state)+"&code=code-456", nil)
	for _, cookie := range loginResp.Result().Cookies() {
		reuseReq.AddCookie(cookie)
	}
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
		AuthRuntime        AuthRuntimeDiagnostic                        `json:"authRuntime"`
		Identity           requestactor.AuthenticatedIdentity           `json:"identity"`
		IdentityProvenance requestactor.AuthenticatedIdentityProvenance `json:"identityProvenance"`
		Actor              requestactor.Actor                           `json:"actor"`
	}
	if err := json.Unmarshal(contextResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context: %v", err)
	}
	if !body.AuthRuntime.TenetManagedLogin.Active || body.AuthRuntime.TenetManagedLogin.UnavailableReason != "" {
		t.Fatalf("expected active managed login diagnostic, got %#v", body.AuthRuntime.TenetManagedLogin)
	}
	assertManagedLoginLifecycleDiagnostics(t, body.AuthRuntime.TenetManagedLogin, true)
	if strings.Contains(contextResp.Body.String(), "token-for-code-123") || strings.Contains(contextResp.Body.String(), "accessToken") {
		t.Fatalf("operator context leaked provider access token material: %s", contextResp.Body.String())
	}
	if body.AuthRuntime.TenetManagedLogin.SessionCookieSecureOverride || body.AuthRuntime.TenetManagedLogin.SessionCookieSecureForRequest || len(body.AuthRuntime.TenetManagedLogin.SessionCookieSecureSources) != 0 {
		t.Fatalf("expected local HTTP operator context to report non-secure managed-session cookie mode, got %#v", body.AuthRuntime.TenetManagedLogin)
	}
	if !body.Identity.Authenticated || body.Identity.ID != "github:12345" || body.Identity.Name != "Octo Operator" || body.Identity.Email != "octo@example.com" {
		t.Fatalf("expected session-derived authenticated identity, got %#v", body.Identity)
	}
	if body.IdentityProvenance.IDSource != requestactor.AuthenticatedIdentitySourceManagedSessionCookie || body.IdentityProvenance.NameSource != requestactor.AuthenticatedIdentitySourceManagedSessionCookie || body.IdentityProvenance.EmailSource != requestactor.AuthenticatedIdentitySourceManagedSessionCookie {
		t.Fatalf("expected managed-session-cookie identity provenance, got %#v", body.IdentityProvenance)
	}
	if !body.Actor.Available || body.Actor.ID != "github:12345" {
		t.Fatalf("expected actor projection from session identity, got %#v", body.Actor)
	}
}

func TestManagedGoogleOIDCLoginCallbackCreatesSessionAndAuthenticatedContext(t *testing.T) {
	provider := &fakeAuthProvider{name: authprovider.GoogleProviderName, authBaseURL: "https://accounts.example.test/o/oauth2/v2/auth"}
	options, identityPath := managedLoginTestOptions(t, provider)
	router := NewRouter(options)

	loginReq := httptest.NewRequest(http.MethodGet, "/auth/login/google?returnUrl=/v1/operator/context%3Ffrom%3Dgoogle", nil)
	loginResp := httptest.NewRecorder()
	router.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusFound {
		t.Fatalf("expected Google login 302, got %d: %s", loginResp.Code, loginResp.Body.String())
	}
	parsedLocation, err := url.Parse(loginResp.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Google login redirect: %v", err)
	}
	state := parsedLocation.Query().Get("state")
	if state == "" {
		t.Fatalf("expected Google login redirect to carry callback state, got %q", loginResp.Header().Get("Location"))
	}
	scope := parsedLocation.Query().Get("scope")
	for _, want := range []string{"openid", "email", "profile"} {
		if !strings.Contains(scope, want) {
			t.Fatalf("expected Google OIDC scope %q in redirect scope %q", want, scope)
		}
	}

	callbackReq := httptest.NewRequest(http.MethodGet, "/auth/callback/google?state="+url.QueryEscape(state)+"&code=google-code-123", nil)
	for _, cookie := range loginResp.Result().Cookies() {
		callbackReq.AddCookie(cookie)
	}
	callbackResp := httptest.NewRecorder()
	router.ServeHTTP(callbackResp, callbackReq)
	if callbackResp.Code != http.StatusFound {
		t.Fatalf("expected Google callback 302, got %d: %s", callbackResp.Code, callbackResp.Body.String())
	}
	if got := callbackResp.Header().Get("Location"); got != "/v1/operator/context?from=google" {
		t.Fatalf("expected safe in-app Google redirect, got %q", got)
	}
	if len(provider.exchangedCodes) != 1 || provider.exchangedCodes[0] != "google-code-123" {
		t.Fatalf("expected provider exchange for Google callback code, got %#v", provider.exchangedCodes)
	}
	identities, err := authidentity.Load(identityPath)
	if err != nil {
		t.Fatalf("load identities: %v", err)
	}
	if len(identities) != 1 || identities[0].Provider != "google" || identities[0].Subject != "12345" {
		t.Fatalf("expected persisted Google identity, got %#v", identities)
	}

	contextReq := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	contextReq.AddCookie(callbackResp.Result().Cookies()[0])
	contextResp := httptest.NewRecorder()
	router.ServeHTTP(contextResp, contextReq)
	if contextResp.Code != http.StatusOK {
		t.Fatalf("expected operator context 200 after Google callback, got %d: %s", contextResp.Code, contextResp.Body.String())
	}
	var body struct {
		Identity requestactor.AuthenticatedIdentity `json:"identity"`
	}
	if err := json.Unmarshal(contextResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context: %v", err)
	}
	if !body.Identity.Authenticated || body.Identity.ID != "google:12345" || body.Identity.Email != "octo@example.com" {
		t.Fatalf("expected Google session-derived authenticated identity, got %#v", body.Identity)
	}
	if strings.Contains(contextResp.Body.String(), "token-for-google-code-123") || strings.Contains(contextResp.Body.String(), "accessToken") {
		t.Fatalf("operator context leaked Google provider access token material: %s", contextResp.Body.String())
	}
}

func TestManagedOktaOIDCLoginCallbackCreatesSessionAndAuthenticatedContext(t *testing.T) {
	provider := &fakeAuthProvider{name: authprovider.OktaOIDCProviderName, authBaseURL: "https://okta.example.test/oauth2/default/v1/authorize"}
	options, identityPath := managedLoginTestOptions(t, provider)
	router := NewRouter(options)

	loginReq := httptest.NewRequest(http.MethodGet, "/auth/login/okta?returnUrl=/v1/operator/context%3Ffrom%3Dokta", nil)
	loginResp := httptest.NewRecorder()
	router.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusFound {
		t.Fatalf("expected Okta OIDC login 302, got %d: %s", loginResp.Code, loginResp.Body.String())
	}
	parsedLocation, err := url.Parse(loginResp.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Okta OIDC login redirect: %v", err)
	}
	state := parsedLocation.Query().Get("state")
	if state == "" {
		t.Fatalf("expected Okta OIDC login redirect to carry callback state, got %q", loginResp.Header().Get("Location"))
	}
	for _, want := range []string{"openid", "email", "profile"} {
		if !strings.Contains(parsedLocation.Query().Get("scope"), want) {
			t.Fatalf("expected Okta OIDC scope %q in redirect scope %q", want, parsedLocation.Query().Get("scope"))
		}
	}

	callbackReq := httptest.NewRequest(http.MethodGet, "/auth/callback/okta/oidc?state="+url.QueryEscape(state)+"&code=okta-code-123", nil)
	for _, cookie := range loginResp.Result().Cookies() {
		callbackReq.AddCookie(cookie)
	}
	callbackResp := httptest.NewRecorder()
	router.ServeHTTP(callbackResp, callbackReq)
	if callbackResp.Code != http.StatusFound {
		t.Fatalf("expected Okta OIDC callback 302, got %d: %s", callbackResp.Code, callbackResp.Body.String())
	}
	if got := callbackResp.Header().Get("Location"); got != "/v1/operator/context?from=okta" {
		t.Fatalf("expected safe in-app Okta redirect, got %q", got)
	}
	if len(provider.exchangedCodes) != 1 || provider.exchangedCodes[0] != "okta-code-123" {
		t.Fatalf("expected provider exchange for Okta callback code, got %#v", provider.exchangedCodes)
	}
	identities, err := authidentity.Load(identityPath)
	if err != nil {
		t.Fatalf("load identities: %v", err)
	}
	if len(identities) != 1 || identities[0].Provider != "okta_oidc" || identities[0].Subject != "12345" {
		t.Fatalf("expected persisted Okta OIDC identity, got %#v", identities)
	}

	contextReq := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	contextReq.AddCookie(callbackResp.Result().Cookies()[0])
	contextResp := httptest.NewRecorder()
	router.ServeHTTP(contextResp, contextReq)
	if contextResp.Code != http.StatusOK {
		t.Fatalf("expected operator context 200 after Okta OIDC callback, got %d: %s", contextResp.Code, contextResp.Body.String())
	}
	var body struct {
		Identity requestactor.AuthenticatedIdentity `json:"identity"`
	}
	if err := json.Unmarshal(contextResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context: %v", err)
	}
	if !body.Identity.Authenticated || body.Identity.ID != "okta_oidc:12345" || body.Identity.Email != "octo@example.com" {
		t.Fatalf("expected Okta session-derived authenticated identity, got %#v", body.Identity)
	}
	if strings.Contains(contextResp.Body.String(), "token-for-okta-code-123") || strings.Contains(contextResp.Body.String(), "accessToken") {
		t.Fatalf("operator context leaked Okta provider access token material: %s", contextResp.Body.String())
	}
}

func TestManagedLoginOperatorContextPrefersTrustedHeaderIdentityOverSessionCookie(t *testing.T) {
	options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
	options.TrustAuthenticatedHeaders = true
	router := NewRouter(options)
	cookie := managedLoginCallbackCookie(t, router, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	req.AddCookie(cookie)
	req.Header.Set(requestactor.AuthenticatedHeaderID, "front-door-123")
	req.Header.Set(requestactor.AuthenticatedHeaderName, "Front Door Operator")
	req.Header.Set(requestactor.AuthenticatedHeaderEmail, "frontdoor@example.com")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected operator context 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body struct {
		Identity           requestactor.AuthenticatedIdentity           `json:"identity"`
		IdentityProvenance requestactor.AuthenticatedIdentityProvenance `json:"identityProvenance"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context: %v", err)
	}
	if !body.Identity.Authenticated || body.Identity.ID != "front-door-123" || body.Identity.Name != "Front Door Operator" || body.Identity.Email != "frontdoor@example.com" {
		t.Fatalf("expected trusted front-door identity to win over managed session, got %#v", body.Identity)
	}
	if body.IdentityProvenance.IDSource != requestactor.AuthenticatedIdentitySourceCanonical || body.IdentityProvenance.NameSource != requestactor.AuthenticatedIdentitySourceCanonical || body.IdentityProvenance.EmailSource != requestactor.AuthenticatedIdentitySourceCanonical {
		t.Fatalf("expected canonical trusted-header provenance, got %#v", body.IdentityProvenance)
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

func assertManagedLoginLifecycleDiagnostics(t *testing.T, managedLogin TenetManagedLoginDiagnostic, expectLogout bool) {
	t.Helper()
	if managedLogin.OAuthStateTTLSeconds != int64(defaultAuthStateTTL.Seconds()) {
		t.Fatalf("expected default OAuth state TTL diagnostic, got %#v", managedLogin)
	}
	if managedLogin.SessionTTLSeconds != int64(defaultAuthSessionTTL.Seconds()) || !managedLogin.SessionExpiryEnforced {
		t.Fatalf("expected signed session TTL/expiry diagnostics, got %#v", managedLogin)
	}
	if managedLogin.SessionCookiePath != defaultAuthSessionCookiePath || !managedLogin.SessionCookieHTTPOnly || managedLogin.SessionCookieSameSite != defaultAuthSessionCookieSameSite {
		t.Fatalf("expected stable session cookie attribute diagnostics, got %#v", managedLogin)
	}
	if managedLogin.ProviderAccessTokenStored || managedLogin.ProviderAccessTokenReturned || managedLogin.ProviderTokenRefreshSupported || managedLogin.ProviderTokenRevocationSupported {
		t.Fatalf("expected provider-token diagnostics to remain non-secret/non-refreshing/non-revoking, got %#v", managedLogin)
	}
	if managedLogin.LogoutClearsSessionCookie != expectLogout || managedLogin.LogoutRevokesProviderToken {
		t.Fatalf("unexpected logout lifecycle diagnostics: %#v", managedLogin)
	}
}

func TestManagedLoginOperatorContextReportsDisabledLifecycleDefaults(t *testing.T) {
	router := NewRouter(Options{})
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected operator context 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body struct {
		AuthRuntime AuthRuntimeDiagnostic `json:"authRuntime"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode operator context: %v", err)
	}
	managedLogin := body.AuthRuntime.TenetManagedLogin
	if managedLogin.Configured || managedLogin.Active {
		t.Fatalf("expected disabled managed-login runtime, got %#v", managedLogin)
	}
	assertManagedLoginLifecycleDiagnostics(t, managedLogin, false)
}

func TestAuthSessionSecureCookieSourcesDeduplicatesDiagnostics(t *testing.T) {
	t.Run("repeated x-forwarded-proto values report one source", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
		req.Header.Add("X-Forwarded-Proto", "https")
		req.Header.Add("X-Forwarded-Proto", "https, http, HTTPS")

		got := (authSessionConfig{}).secureCookieSources(req)
		if !reflect.DeepEqual(got, []string{"x_forwarded_proto_https"}) {
			t.Fatalf("expected one x-forwarded-proto source, got %#v", got)
		}
	})

	t.Run("repeated forwarded proto values report one source", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
		req.Header.Add("Forwarded", "for=192.0.2.1; proto=https")
		req.Header.Add("Forwarded", "for=192.0.2.2; proto=\"https\", for=192.0.2.3; proto=https")

		got := (authSessionConfig{}).secureCookieSources(req)
		if !reflect.DeepEqual(got, []string{"forwarded_proto_https"}) {
			t.Fatalf("expected one forwarded proto source, got %#v", got)
		}
	})

	t.Run("distinct secure sources keep stable order", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
		req.TLS = &tls.ConnectionState{}
		req.Header.Add("X-Forwarded-Proto", "http, https")
		req.Header.Add("X-Forwarded-Proto", "https")
		req.Header.Add("Forwarded", "for=192.0.2.4; proto=https")
		req.Header.Add("Forwarded", "for=192.0.2.5; proto=\"https\"")

		got := (authSessionConfig{CookieSecure: true}).secureCookieSources(req)
		want := []string{"configured_override", "https_request", "x_forwarded_proto_https", "forwarded_proto_https"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("expected stable de-duplicated source order %#v, got %#v", want, got)
		}
	})
}

func TestManagedLoginOperatorContextReportsSessionCookieSecurityDiagnostics(t *testing.T) {
	t.Run("forwarded https request reports secure source", func(t *testing.T) {
		options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
		router := NewRouter(options)
		req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("expected operator context 200, got %d: %s", resp.Code, resp.Body.String())
		}
		var body struct {
			AuthRuntime AuthRuntimeDiagnostic `json:"authRuntime"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode operator context: %v", err)
		}
		managedLogin := body.AuthRuntime.TenetManagedLogin
		if !managedLogin.SessionCookieSecureForRequest || managedLogin.SessionCookieSecureOverride {
			t.Fatalf("expected forwarded HTTPS secure-cookie diagnostics without override, got %#v", managedLogin)
		}
		if !reflect.DeepEqual(managedLogin.SessionCookieSecureSources, []string{"x_forwarded_proto_https"}) {
			t.Fatalf("expected forwarded HTTPS source evidence, got %#v", managedLogin.SessionCookieSecureSources)
		}
	})

	t.Run("configured override reports explicit secure source", func(t *testing.T) {
		options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
		options.AuthSessionCookieSecure = true
		router := NewRouter(options)
		req := httptest.NewRequest(http.MethodGet, "/v1/operator/context", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("expected operator context 200, got %d: %s", resp.Code, resp.Body.String())
		}
		var body struct {
			AuthRuntime AuthRuntimeDiagnostic `json:"authRuntime"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode operator context: %v", err)
		}
		managedLogin := body.AuthRuntime.TenetManagedLogin
		if !managedLogin.SessionCookieSecureOverride || !managedLogin.SessionCookieSecureForRequest {
			t.Fatalf("expected secure override diagnostics, got %#v", managedLogin)
		}
		if !reflect.DeepEqual(managedLogin.SessionCookieSecureSources, []string{"configured_override"}) {
			t.Fatalf("expected configured override source evidence, got %#v", managedLogin.SessionCookieSecureSources)
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

func TestManagedLoginLogoutClearsSessionCookieAndRedirectsToSafeReturnURL(t *testing.T) {
	options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
	router := NewRouter(options)
	cookie := managedLoginCallbackCookie(t, router, func(req *http.Request) {
		req.Header.Set("X-Forwarded-Proto", "https")
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/logout?returnUrl="+url.QueryEscape("/?page=plan&deployment=payments-api#apply"), nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusFound {
		t.Fatalf("expected logout 302, got %d: %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Location"); got != "/?page=plan&deployment=payments-api#apply" {
		t.Fatalf("expected safe in-app logout redirect, got %q", got)
	}
	cookies := resp.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "tenet_session" {
		t.Fatalf("expected one clearing tenet_session cookie, got %#v", cookies)
	}
	cleared := cookies[0]
	if cleared.Value != "" || cleared.MaxAge >= 0 || !cleared.HttpOnly || cleared.SameSite != http.SameSiteLaxMode || !cleared.Secure || cleared.Path != "/" {
		t.Fatalf("expected secure HttpOnly SameSite=Lax clearing cookie, got %#v", cleared)
	}
}

func TestManagedLoginLogoutDefaultsReturnURLAndRejectsUnsafeReturnURL(t *testing.T) {
	options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
	router := NewRouter(options)

	defaultReq := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
	defaultResp := httptest.NewRecorder()
	router.ServeHTTP(defaultResp, defaultReq)
	if defaultResp.Code != http.StatusFound || defaultResp.Header().Get("Location") != "/" {
		t.Fatalf("expected default logout redirect to /, got %d %q", defaultResp.Code, defaultResp.Header().Get("Location"))
	}

	for _, returnURL := range []string{"https://evil.example/", "//evil.example/path", "settings"} {
		req := httptest.NewRequest(http.MethodGet, "/auth/logout?returnUrl="+url.QueryEscape(returnURL), nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected unsafe logout return URL %q to fail with 400, got %d", returnURL, resp.Code)
		}
	}
}

func TestManagedLoginLogoutRejectsUnsupportedMethods(t *testing.T) {
	options, _ := managedLoginTestOptions(t, &fakeAuthProvider{})
	router := NewRouter(options)

	req := httptest.NewRequest(http.MethodPut, "/auth/logout", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected unsupported logout method to fail with 405, got %d", resp.Code)
	}
	if got := resp.Header().Get("Allow"); got != "GET, POST" {
		t.Fatalf("expected Allow header to list GET, POST, got %q", got)
	}
}

func TestManagedLoginLogoutUnavailableWhenRuntimeInactive(t *testing.T) {
	router := NewRouter(Options{})
	req := httptest.NewRequest(http.MethodGet, "/auth/logout?returnUrl=/", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected inactive managed-login logout route to be unregistered, got %d: %s", resp.Code, resp.Body.String())
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
