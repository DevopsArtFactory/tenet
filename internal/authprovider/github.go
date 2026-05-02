package authprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	GitHubProviderName    = "github"
	DefaultGitHubAuthURL  = "https://github.com/login/oauth/authorize"
	DefaultGitHubTokenURL = "https://github.com/login/oauth/access_token"
	DefaultGitHubUserURL  = "https://api.github.com/user"
)

// GitHubConfig configures the GitHub OAuth provider client. AuthURL, TokenURL,
// and UserURL are overrideable for GitHub Enterprise and tests.
type GitHubConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AuthURL      string
	TokenURL     string
	UserURL      string
	HTTPClient   *http.Client
}

// GitHubProvider implements the OAuth authorization-code primitives Tenet needs
// before a browser/session layer exists.
type GitHubProvider struct {
	clientID     string
	clientSecret string
	redirectURL  string
	authURL      string
	tokenURL     string
	userURL      string
	httpClient   *http.Client
}

// NewGitHubProvider validates config and returns a GitHub OAuth provider client.
func NewGitHubProvider(cfg GitHubConfig) (*GitHubProvider, error) {
	clientID := strings.TrimSpace(cfg.ClientID)
	if clientID == "" {
		return nil, fmt.Errorf("GitHub OAuth provider is missing client id")
	}
	clientSecret := strings.TrimSpace(cfg.ClientSecret)
	if clientSecret == "" {
		return nil, fmt.Errorf("GitHub OAuth provider is missing client secret")
	}
	redirectURL := strings.TrimSpace(cfg.RedirectURL)
	if redirectURL == "" {
		return nil, fmt.Errorf("GitHub OAuth provider is missing redirect URL")
	}
	if _, err := url.ParseRequestURI(redirectURL); err != nil {
		return nil, fmt.Errorf("GitHub OAuth provider redirect URL is invalid: %w", err)
	}
	authURL, err := validatedEndpoint("authorization URL", cfg.AuthURL, DefaultGitHubAuthURL)
	if err != nil {
		return nil, err
	}
	tokenURL, err := validatedEndpoint("token URL", cfg.TokenURL, DefaultGitHubTokenURL)
	if err != nil {
		return nil, err
	}
	userURL, err := validatedEndpoint("user URL", cfg.UserURL, DefaultGitHubUserURL)
	if err != nil {
		return nil, err
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &GitHubProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		authURL:      authURL,
		tokenURL:     tokenURL,
		userURL:      userURL,
		httpClient:   httpClient,
	}, nil
}

func validatedEndpoint(label, value, fallback string) (string, error) {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		candidate = fallback
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		if err != nil {
			return "", fmt.Errorf("GitHub OAuth provider %s is invalid: %w", label, err)
		}
		return "", fmt.Errorf("GitHub OAuth provider %s is invalid: expected absolute URL", label)
	}
	return parsed.String(), nil
}

func (p *GitHubProvider) Name() string { return GitHubProviderName }

// AuthCodeURL builds the GitHub authorization URL with deterministic scope
// serialization for stable operator handoffs and tests.
func (p *GitHubProvider) AuthCodeURL(state string, scopes []string) (string, error) {
	if p == nil {
		return "", fmt.Errorf("GitHub OAuth provider is not configured")
	}
	parsed, err := url.Parse(p.authURL)
	if err != nil {
		return "", fmt.Errorf("GitHub OAuth provider authorization URL is invalid: %w", err)
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

func joinScopes(scopes []string) string {
	cleaned := make([]string, 0, len(scopes))
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		cleaned = append(cleaned, scope)
	}
	return strings.Join(cleaned, " ")
}

// Exchange exchanges an OAuth authorization code for an access token.
func (p *GitHubProvider) Exchange(ctx context.Context, code string) (Token, error) {
	if p == nil {
		return Token{}, fmt.Errorf("GitHub OAuth provider is not configured")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return Token{}, fmt.Errorf("GitHub OAuth code exchange requires a non-empty authorization code")
	}
	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", p.redirectURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, fmt.Errorf("create GitHub OAuth token request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("GitHub OAuth token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return Token{}, fmt.Errorf("read GitHub OAuth token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Token{}, fmt.Errorf("GitHub OAuth token exchange returned %s: %s", resp.Status, operatorBody(body))
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Token{}, fmt.Errorf("decode GitHub OAuth token response: %w", err)
	}
	if payload.Error != "" {
		return Token{}, fmt.Errorf("GitHub OAuth token exchange returned %s: %s", payload.Error, payload.Description)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return Token{}, fmt.Errorf("GitHub OAuth token response did not include access_token")
	}
	return Token{AccessToken: payload.AccessToken, TokenType: payload.TokenType, Scope: payload.Scope}, nil
}

// User fetches the GitHub profile for an OAuth access token.
func (p *GitHubProvider) User(ctx context.Context, accessToken string) (Identity, error) {
	if p == nil {
		return Identity{}, fmt.Errorf("GitHub OAuth provider is not configured")
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return Identity{}, fmt.Errorf("GitHub user profile request requires a non-empty access token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.userURL, nil)
	if err != nil {
		return Identity{}, fmt.Errorf("create GitHub user profile request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("GitHub user profile request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return Identity{}, fmt.Errorf("read GitHub user profile response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Identity{}, fmt.Errorf("GitHub user profile request returned %s: %s", resp.Status, operatorBody(body))
	}
	var payload struct {
		ID      int64  `json:"id"`
		Login   string `json:"login"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Identity{}, fmt.Errorf("decode GitHub user profile response: %w", err)
	}
	if payload.ID == 0 && strings.TrimSpace(payload.Login) == "" {
		return Identity{}, fmt.Errorf("GitHub user profile response did not include a usable id or login")
	}
	id := ""
	if payload.ID != 0 {
		id = strconv.FormatInt(payload.ID, 10)
	} else {
		id = payload.Login
	}
	return Identity{
		Provider: GitHubProviderName,
		ID:       id,
		Login:    payload.Login,
		Name:     payload.Name,
		Email:    payload.Email,
		URL:      payload.HTMLURL,
	}, nil
}

func readLimited(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, 1<<20))
}

func operatorBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "empty response body"
	}
	if len(trimmed) > 512 {
		return trimmed[:512] + "…"
	}
	return trimmed
}
