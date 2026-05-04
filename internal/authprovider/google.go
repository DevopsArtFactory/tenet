package authprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	GoogleProviderName       = "google"
	GoogleOIDCProviderAlias  = "google_oidc"
	DefaultGoogleIssuerURL   = "https://accounts.google.com"
	DefaultGoogleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	DefaultGoogleTokenURL    = "https://oauth2.googleapis.com/token"
	DefaultGoogleUserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
)

// GoogleConfig configures the Google Workspace OIDC provider client. AuthURL,
// TokenURL, and UserInfoURL are overrideable for tests and private Google-compatible
// endpoints; IssuerURL is retained as an explicit operator diagnostic input.
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	IssuerURL    string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	HTTPClient   *http.Client
}

// GoogleProvider implements the OAuth/OIDC authorization-code primitives Tenet
// needs for Google Workspace SSO. It stores no provider access token; callers use
// the token only long enough to fetch the normalized Google userinfo profile.
type GoogleProvider struct {
	clientID     string
	clientSecret string
	redirectURL  string
	issuerURL    string
	authURL      string
	tokenURL     string
	userInfoURL  string
	httpClient   *http.Client
}

// NewGoogleProvider validates config and returns a Google OIDC provider client.
func NewGoogleProvider(cfg GoogleConfig) (*GoogleProvider, error) {
	clientID := strings.TrimSpace(cfg.ClientID)
	if clientID == "" {
		return nil, fmt.Errorf("Google OIDC provider is missing client id")
	}
	clientSecret := strings.TrimSpace(cfg.ClientSecret)
	if clientSecret == "" {
		return nil, fmt.Errorf("Google OIDC provider is missing client secret")
	}
	redirectURL := strings.TrimSpace(cfg.RedirectURL)
	if redirectURL == "" {
		return nil, fmt.Errorf("Google OIDC provider is missing redirect URL")
	}
	if _, err := url.ParseRequestURI(redirectURL); err != nil {
		return nil, fmt.Errorf("Google OIDC provider redirect URL is invalid: %w", err)
	}
	issuerURL, err := validatedGoogleEndpoint("issuer URL", cfg.IssuerURL, DefaultGoogleIssuerURL)
	if err != nil {
		return nil, err
	}
	authURL, err := validatedGoogleEndpoint("authorization URL", cfg.AuthURL, DefaultGoogleAuthURL)
	if err != nil {
		return nil, err
	}
	tokenURL, err := validatedGoogleEndpoint("token URL", cfg.TokenURL, DefaultGoogleTokenURL)
	if err != nil {
		return nil, err
	}
	userInfoURL, err := validatedGoogleEndpoint("userinfo URL", cfg.UserInfoURL, DefaultGoogleUserInfoURL)
	if err != nil {
		return nil, err
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &GoogleProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		issuerURL:    issuerURL,
		authURL:      authURL,
		tokenURL:     tokenURL,
		userInfoURL:  userInfoURL,
		httpClient:   httpClient,
	}, nil
}

func validatedGoogleEndpoint(label, value, fallback string) (string, error) {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		candidate = fallback
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		if err != nil {
			return "", fmt.Errorf("Google OIDC provider %s is invalid: %w", label, err)
		}
		return "", fmt.Errorf("Google OIDC provider %s is invalid: expected absolute URL", label)
	}
	return parsed.String(), nil
}

func (p *GoogleProvider) Name() string { return GoogleProviderName }

// AuthCodeURL builds the Google authorization URL with deterministic scope
// serialization for stable operator handoffs and tests.
func (p *GoogleProvider) AuthCodeURL(state string, scopes []string) (string, error) {
	if p == nil {
		return "", fmt.Errorf("Google OIDC provider is not configured")
	}
	parsed, err := url.Parse(p.authURL)
	if err != nil {
		return "", fmt.Errorf("Google OIDC provider authorization URL is invalid: %w", err)
	}
	query := parsed.Query()
	query.Set("client_id", p.clientID)
	query.Set("redirect_uri", p.redirectURL)
	query.Set("response_type", "code")
	if trimmed := strings.TrimSpace(state); trimmed != "" {
		query.Set("state", trimmed)
	}
	if joined := joinScopes(scopes); joined != "" {
		query.Set("scope", joined)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// Exchange exchanges a Google OAuth authorization code for an access token.
func (p *GoogleProvider) Exchange(ctx context.Context, code string) (Token, error) {
	if p == nil {
		return Token{}, fmt.Errorf("Google OIDC provider is not configured")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return Token{}, fmt.Errorf("Google OIDC code exchange requires a non-empty authorization code")
	}
	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", p.redirectURL)
	form.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, fmt.Errorf("create Google OIDC token request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("Google OIDC token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return Token{}, fmt.Errorf("read Google OIDC token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Token{}, fmt.Errorf("Google OIDC token exchange returned %s: %s", resp.Status, operatorBody(body))
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Token{}, fmt.Errorf("decode Google OIDC token response: %w", err)
	}
	if payload.Error != "" {
		return Token{}, fmt.Errorf("Google OIDC token exchange returned %s: %s", payload.Error, payload.Description)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return Token{}, fmt.Errorf("Google OIDC token response did not include access_token")
	}
	return Token{AccessToken: payload.AccessToken, TokenType: payload.TokenType, Scope: payload.Scope}, nil
}

// User fetches the Google OIDC userinfo profile for an OAuth access token.
func (p *GoogleProvider) User(ctx context.Context, accessToken string) (Identity, error) {
	if p == nil {
		return Identity{}, fmt.Errorf("Google OIDC provider is not configured")
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return Identity{}, fmt.Errorf("Google OIDC userinfo request requires a non-empty access token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.userInfoURL, nil)
	if err != nil {
		return Identity{}, fmt.Errorf("create Google OIDC userinfo request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("Google OIDC userinfo request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return Identity{}, fmt.Errorf("read Google OIDC userinfo response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Identity{}, fmt.Errorf("Google OIDC userinfo request returned %s: %s", resp.Status, operatorBody(body))
	}
	var payload struct {
		Subject string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Profile string `json:"profile"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Identity{}, fmt.Errorf("decode Google OIDC userinfo response: %w", err)
	}
	if strings.TrimSpace(payload.Subject) == "" {
		return Identity{}, fmt.Errorf("Google OIDC userinfo response did not include sub")
	}
	return Identity{
		Provider: GoogleProviderName,
		ID:       payload.Subject,
		Login:    payload.Email,
		Name:     payload.Name,
		Email:    payload.Email,
		URL:      firstNonEmptyString(payload.Profile, payload.Picture),
	}, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
