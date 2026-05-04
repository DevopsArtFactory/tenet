package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/authidentity"
	"github.com/DevopsArtFactory/tenet/internal/authprovider"
	"github.com/DevopsArtFactory/tenet/internal/authstate"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

const (
	defaultAuthSessionCookieName     = "tenet_session"
	defaultAuthSessionCookiePath     = "/"
	defaultAuthSessionCookieSameSite = "Lax"
	defaultAuthStateTTL              = 10 * time.Minute
	defaultAuthSessionTTL            = 12 * time.Hour
)

type authSessionConfig struct {
	Provider      authprovider.Provider
	StateStore    authstate.Store
	IdentityStore authidentity.Store
	SessionSecret string
	CookieName    string
	CookieSecure  bool
	StateTTL      time.Duration
	SessionTTL    time.Duration
	Now           func() time.Time
}

func authSessionConfigFromOptions(options Options) authSessionConfig {
	return authSessionConfig{
		Provider:      options.AuthProvider,
		StateStore:    options.AuthStateStore,
		IdentityStore: options.AuthIdentityStore,
		SessionSecret: options.AuthSessionSecret,
		CookieName:    options.AuthSessionCookieName,
		CookieSecure:  options.AuthSessionCookieSecure,
	}
}

func (cfg authSessionConfig) active() bool {
	return cfg.Provider != nil && cfg.StateStore != nil && cfg.IdentityStore != nil && strings.TrimSpace(cfg.SessionSecret) != ""
}

func (cfg authSessionConfig) cookieName() string {
	if name := strings.TrimSpace(cfg.CookieName); name != "" {
		return name
	}
	return defaultAuthSessionCookieName
}

func (cfg authSessionConfig) stateTTL() time.Duration {
	if cfg.StateTTL > 0 {
		return cfg.StateTTL
	}
	return defaultAuthStateTTL
}

func (cfg authSessionConfig) sessionTTL() time.Duration {
	if cfg.SessionTTL > 0 {
		return cfg.SessionTTL
	}
	return defaultAuthSessionTTL
}

func (cfg authSessionConfig) now() time.Time {
	if cfg.Now != nil {
		return cfg.Now().UTC()
	}
	return time.Now().UTC()
}

func (cfg authSessionConfig) secureCookie(r *http.Request) bool {
	return len(cfg.secureCookieSources(r)) > 0
}

func (cfg authSessionConfig) secureCookieSources(r *http.Request) []string {
	sources := []string{}
	if cfg.CookieSecure {
		sources = append(sources, "configured_override")
	}
	if r != nil && r.TLS != nil {
		sources = append(sources, "https_request")
	}
	if r == nil {
		return sources
	}
	if requestHasForwardedProtoHTTPS(r) {
		sources = append(sources, "x_forwarded_proto_https")
	}
	if requestHasForwardedHeaderProtoHTTPS(r) {
		sources = append(sources, "forwarded_proto_https")
	}
	return sources
}

func requestHasForwardedProtoHTTPS(r *http.Request) bool {
	for _, value := range r.Header.Values("X-Forwarded-Proto") {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), "https") {
				return true
			}
		}
	}
	return false
}

func requestHasForwardedHeaderProtoHTTPS(r *http.Request) bool {
	for _, value := range r.Header.Values("Forwarded") {
		for _, forwardedElement := range strings.Split(value, ",") {
			for _, part := range strings.Split(forwardedElement, ";") {
				name, proto, ok := strings.Cut(strings.TrimSpace(part), "=")
				if ok && strings.EqualFold(strings.TrimSpace(name), "proto") && strings.EqualFold(strings.Trim(strings.TrimSpace(proto), "\""), "https") {
					return true
				}
			}
		}
	}
	return false
}

type authSessionClaims struct {
	Provider string    `json:"provider"`
	Subject  string    `json:"subject"`
	Name     string    `json:"name,omitempty"`
	Email    string    `json:"email,omitempty"`
	Expires  time.Time `json:"expiresAt"`
}

func handleGitHubLogin(w http.ResponseWriter, r *http.Request, cfg authSessionConfig) {
	handleProviderLogin(w, r, cfg, authprovider.GitHubProviderName, "GitHub OAuth", []string{"read:user", "user:email"})
}

func handleGitHubCallback(w http.ResponseWriter, r *http.Request, cfg authSessionConfig) {
	handleProviderCallback(w, r, cfg, authprovider.GitHubProviderName, "GitHub OAuth")
}

func handleGoogleLogin(w http.ResponseWriter, r *http.Request, cfg authSessionConfig) {
	handleProviderLogin(w, r, cfg, authprovider.GoogleProviderName, "Google Workspace OIDC", []string{"openid", "email", "profile"})
}

func handleGoogleCallback(w http.ResponseWriter, r *http.Request, cfg authSessionConfig) {
	handleProviderCallback(w, r, cfg, authprovider.GoogleProviderName, "Google Workspace OIDC")
}

func handleProviderLogin(w http.ResponseWriter, r *http.Request, cfg authSessionConfig, providerName, providerLabel string, scopes []string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if !cfg.active() || cfg.Provider.Name() != providerName {
		http.Error(w, fmt.Sprintf("tenet-managed %s login is not configured for this API instance", providerLabel), http.StatusServiceUnavailable)
		return
	}
	returnURL := strings.TrimSpace(r.URL.Query().Get("returnUrl"))
	if returnURL == "" {
		returnURL = "/"
	}
	if !safeInAppReturnURL(returnURL) {
		http.Error(w, "returnUrl must be a safe in-app path", http.StatusBadRequest)
		return
	}
	token, err := newAuthStateToken()
	if err != nil {
		respondInternalError(w)
		return
	}
	now := cfg.now()
	if _, err := cfg.StateStore.Save(r.Context(), authstate.State{
		Token:     token,
		Provider:  providerName,
		ReturnURL: returnURL,
		CreatedAt: now,
		ExpiresAt: now.Add(cfg.stateTTL()),
	}); err != nil {
		http.Error(w, "failed to create OAuth callback state", http.StatusServiceUnavailable)
		return
	}
	authURL, err := cfg.Provider.AuthCodeURL(token, scopes)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to build %s authorization URL", providerLabel), http.StatusServiceUnavailable)
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

func handleProviderCallback(w http.ResponseWriter, r *http.Request, cfg authSessionConfig, providerName, providerLabel string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if !cfg.active() || cfg.Provider.Name() != providerName {
		http.Error(w, fmt.Sprintf("tenet-managed %s login is not configured for this API instance", providerLabel), http.StatusServiceUnavailable)
		return
	}
	stateToken := strings.TrimSpace(r.URL.Query().Get("state"))
	if stateToken == "" {
		http.Error(w, "OAuth callback state is required", http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		http.Error(w, "OAuth callback code is required", http.StatusBadRequest)
		return
	}
	state, err := cfg.StateStore.Consume(r.Context(), stateToken)
	if err != nil {
		respondAuthStateConsumeError(w, err)
		return
	}
	if state.Provider != providerName {
		http.Error(w, fmt.Sprintf("OAuth callback provider does not match %s login", providerLabel), http.StatusBadRequest)
		return
	}
	if state.Expired(cfg.now()) {
		http.Error(w, authstate.ErrExpired.Error(), http.StatusBadRequest)
		return
	}
	if !safeInAppReturnURL(state.ReturnURL) {
		http.Error(w, "stored OAuth returnUrl is not a safe in-app path", http.StatusBadRequest)
		return
	}
	token, err := cfg.Provider.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, fmt.Sprintf("%s code exchange failed", providerLabel), http.StatusBadGateway)
		return
	}
	profile, err := cfg.Provider.User(r.Context(), token.AccessToken)
	if err != nil {
		http.Error(w, fmt.Sprintf("%s user lookup failed", providerLabel), http.StatusBadGateway)
		return
	}
	if strings.ToLower(strings.TrimSpace(profile.Provider)) != providerName {
		http.Error(w, fmt.Sprintf("%s user profile returned an unexpected provider", providerLabel), http.StatusBadGateway)
		return
	}
	identity, err := cfg.IdentityStore.Upsert(r.Context(), authidentity.FromProviderIdentity(profile))
	if err != nil {
		http.Error(w, "failed to persist OAuth identity", http.StatusServiceUnavailable)
		return
	}
	claims := authSessionClaims{
		Provider: identity.Provider,
		Subject:  identity.Subject,
		Name:     firstNonEmpty(identity.Name, identity.Login),
		Email:    identity.Email,
		Expires:  cfg.now().Add(cfg.sessionTTL()),
	}
	cookieValue, err := signAuthSessionClaims(claims, cfg.SessionSecret)
	if err != nil {
		respondInternalError(w)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.cookieName(),
		Value:    cookieValue,
		Path:     defaultAuthSessionCookiePath,
		Expires:  claims.Expires,
		MaxAge:   int(claims.Expires.Sub(cfg.now()).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cfg.secureCookie(r),
	})
	http.Redirect(w, r, state.ReturnURL, http.StatusFound)
}

func handleManagedLoginLogout(w http.ResponseWriter, r *http.Request, cfg authSessionConfig) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !cfg.active() {
		http.NotFound(w, r)
		return
	}
	returnURL := strings.TrimSpace(r.URL.Query().Get("returnUrl"))
	if returnURL == "" {
		returnURL = "/"
	}
	if !safeInAppReturnURL(returnURL) {
		http.Error(w, "returnUrl must be a safe in-app path", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.cookieName(),
		Value:    "",
		Path:     defaultAuthSessionCookiePath,
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cfg.secureCookie(r),
	})
	http.Redirect(w, r, returnURL, http.StatusFound)
}

func respondAuthStateConsumeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, authstate.ErrNotFound):
		http.Error(w, authstate.ErrNotFound.Error(), http.StatusBadRequest)
	case errors.Is(err, authstate.ErrExpired):
		http.Error(w, authstate.ErrExpired.Error(), http.StatusBadRequest)
	case errors.Is(err, authstate.ErrConsumed):
		http.Error(w, authstate.ErrConsumed.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "failed to consume OAuth callback state", http.StatusServiceUnavailable)
	}
}

func safeInAppReturnURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return false
	}
	if parsed.IsAbs() || parsed.Host != "" || strings.HasPrefix(parsed.Path, "//") {
		return false
	}
	return strings.HasPrefix(parsed.Path, "/")
}

func newAuthStateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func signAuthSessionClaims(claims authSessionClaims, secret string) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("auth session secret is required")
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	sig := authSessionSignature(encodedPayload, secret)
	return encodedPayload + "." + sig, nil
}

func parseAuthSessionClaims(value, secret string, now time.Time) (authSessionClaims, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || strings.TrimSpace(secret) == "" {
		return authSessionClaims{}, false
	}
	if !hmac.Equal([]byte(parts[1]), []byte(authSessionSignature(parts[0], secret))) {
		return authSessionClaims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return authSessionClaims{}, false
	}
	var claims authSessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return authSessionClaims{}, false
	}
	if strings.TrimSpace(claims.Provider) == "" || strings.TrimSpace(claims.Subject) == "" || !claims.Expires.After(now.UTC()) {
		return authSessionClaims{}, false
	}
	return claims, true
}

func authSessionSignature(encodedPayload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encodedPayload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func contextWithAuthSession(ctx context.Context, r *http.Request, cfg authSessionConfig) context.Context {
	if !cfg.active() {
		return ctx
	}
	cookie, err := r.Cookie(cfg.cookieName())
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return ctx
	}
	claims, ok := parseAuthSessionClaims(cookie.Value, cfg.SessionSecret, cfg.now())
	if !ok {
		return ctx
	}
	identity := requestactor.AuthenticatedIdentity{
		Authenticated: true,
		ID:            claims.Provider + ":" + claims.Subject,
		Name:          claims.Name,
		Email:         claims.Email,
	}
	return requestactor.WithManagedSessionAuthenticatedIdentityContext(ctx, identity)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
