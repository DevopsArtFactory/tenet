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

const OktaOIDCProviderName = "okta_oidc"

type OktaOIDCConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	IssuerURL    string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	HTTPClient   *http.Client
}

type OktaOIDCProvider struct {
	clientID     string
	clientSecret string
	redirectURL  string
	issuerURL    string
	authURL      string
	tokenURL     string
	userInfoURL  string
	httpClient   *http.Client
}

func NewOktaOIDCProvider(cfg OktaOIDCConfig) (*OktaOIDCProvider, error) {
	clientID := strings.TrimSpace(cfg.ClientID)
	if clientID == "" {
		return nil, fmt.Errorf("Okta OIDC provider is missing client id")
	}
	clientSecret := strings.TrimSpace(cfg.ClientSecret)
	if clientSecret == "" {
		return nil, fmt.Errorf("Okta OIDC provider is missing client secret")
	}
	redirectURL := strings.TrimSpace(cfg.RedirectURL)
	if redirectURL == "" {
		return nil, fmt.Errorf("Okta OIDC provider is missing redirect URL")
	}
	if _, err := url.ParseRequestURI(redirectURL); err != nil {
		return nil, fmt.Errorf("Okta OIDC provider redirect URL is invalid: %w", err)
	}
	issuerURL, err := validatedOIDCEndpoint("Okta OIDC provider issuer URL", cfg.IssuerURL, "")
	if err != nil {
		return nil, err
	}
	authURL, err := validatedOIDCEndpoint("Okta OIDC provider authorization URL", cfg.AuthURL, issuerDerivedEndpoint(issuerURL, "authorize"))
	if err != nil {
		return nil, err
	}
	tokenURL, err := validatedOIDCEndpoint("Okta OIDC provider token URL", cfg.TokenURL, issuerDerivedEndpoint(issuerURL, "token"))
	if err != nil {
		return nil, err
	}
	userInfoURL, err := validatedOIDCEndpoint("Okta OIDC provider userinfo URL", cfg.UserInfoURL, issuerDerivedEndpoint(issuerURL, "userinfo"))
	if err != nil {
		return nil, err
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &OktaOIDCProvider{
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

func issuerDerivedEndpoint(issuerURL, suffix string) string {
	base := strings.TrimRight(strings.TrimSpace(issuerURL), "/")
	if base == "" {
		return ""
	}
	return base + "/v1/" + suffix
}

func validatedOIDCEndpoint(label, value, fallback string) (string, error) {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		candidate = fallback
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		if err != nil {
			return "", fmt.Errorf("%s is invalid: %w", label, err)
		}
		return "", fmt.Errorf("%s is invalid: expected absolute URL", label)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("%s is invalid: expected http or https URL", label)
	}
	return parsed.String(), nil
}

func (p *OktaOIDCProvider) Name() string { return OktaOIDCProviderName }

func (p *OktaOIDCProvider) AuthCodeURL(state string, scopes []string) (string, error) {
	if p == nil {
		return "", fmt.Errorf("Okta OIDC provider is not configured")
	}
	parsed, err := url.Parse(p.authURL)
	if err != nil {
		return "", fmt.Errorf("Okta OIDC provider authorization URL is invalid: %w", err)
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

func (p *OktaOIDCProvider) Exchange(ctx context.Context, code string) (Token, error) {
	if p == nil {
		return Token{}, fmt.Errorf("Okta OIDC provider is not configured")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return Token{}, fmt.Errorf("Okta OIDC code exchange requires a non-empty authorization code")
	}
	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", p.redirectURL)
	form.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, fmt.Errorf("create Okta OIDC token request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("Okta OIDC token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return Token{}, fmt.Errorf("read Okta OIDC token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Token{}, fmt.Errorf("Okta OIDC token exchange returned %s: %s", resp.Status, operatorBody(body))
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Token{}, fmt.Errorf("decode Okta OIDC token response: %w", err)
	}
	if payload.Error != "" {
		return Token{}, fmt.Errorf("Okta OIDC token exchange returned %s: %s", payload.Error, payload.Description)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return Token{}, fmt.Errorf("Okta OIDC token response did not include access_token")
	}
	return Token{AccessToken: payload.AccessToken, TokenType: payload.TokenType, Scope: payload.Scope}, nil
}

func (p *OktaOIDCProvider) User(ctx context.Context, accessToken string) (Identity, error) {
	if p == nil {
		return Identity{}, fmt.Errorf("Okta OIDC provider is not configured")
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return Identity{}, fmt.Errorf("Okta OIDC userinfo request requires a non-empty access token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.userInfoURL, nil)
	if err != nil {
		return Identity{}, fmt.Errorf("create Okta OIDC userinfo request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("Okta OIDC userinfo request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return Identity{}, fmt.Errorf("read Okta OIDC userinfo response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Identity{}, fmt.Errorf("Okta OIDC userinfo request returned %s: %s", resp.Status, operatorBody(body))
	}
	var payload struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified *bool  `json:"email_verified"`
		Name          string `json:"name"`
		Preferred     string `json:"preferred_username"`
		Profile       string `json:"profile"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Identity{}, fmt.Errorf("decode Okta OIDC userinfo response: %w", err)
	}
	subject := strings.TrimSpace(payload.Subject)
	if subject == "" {
		return Identity{}, fmt.Errorf("Okta OIDC userinfo response did not include sub")
	}
	email := strings.TrimSpace(payload.Email)
	if payload.EmailVerified != nil && !*payload.EmailVerified {
		email = ""
	}
	login := strings.TrimSpace(payload.Preferred)
	if login == "" {
		login = email
	}
	if login == "" {
		login = subject
	}
	return Identity{
		Provider: OktaOIDCProviderName,
		ID:       subject,
		Login:    login,
		Name:     strings.TrimSpace(payload.Name),
		Email:    email,
		URL:      strings.TrimSpace(payload.Profile),
	}, nil
}
