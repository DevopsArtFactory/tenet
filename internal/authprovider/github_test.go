package authprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGitHubProviderAuthCodeURL(t *testing.T) {
	provider, err := NewGitHubProvider(GitHubConfig{
		ClientID:     "client-123",
		ClientSecret: "secret-123",
		RedirectURL:  "https://tenet.example.com/auth/github/callback",
		AuthURL:      "https://github.enterprise.test/login/oauth/authorize?allow_signup=false",
	})
	if err != nil {
		t.Fatalf("NewGitHubProvider returned error: %v", err)
	}

	authURL, err := provider.AuthCodeURL("state-123", []string{"read:user", "user:email", "read:user", " "})
	if err != nil {
		t.Fatalf("AuthCodeURL returned error: %v", err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "github.enterprise.test" || parsed.Path != "/login/oauth/authorize" {
		t.Fatalf("unexpected auth URL target: %s", authURL)
	}
	query := parsed.Query()
	assertQuery(t, query, "allow_signup", "false")
	assertQuery(t, query, "client_id", "client-123")
	assertQuery(t, query, "redirect_uri", "https://tenet.example.com/auth/github/callback")
	assertQuery(t, query, "response_type", "code")
	assertQuery(t, query, "scope", "read:user user:email")
	assertQuery(t, query, "state", "state-123")
}

func TestGitHubProviderExchange(t *testing.T) {
	var sawRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequest = true
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("Accept = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("Content-Type = %q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		assertQuery(t, r.Form, "client_id", "client-id")
		assertQuery(t, r.Form, "client_secret", "client-secret")
		assertQuery(t, r.Form, "code", "code-123")
		assertQuery(t, r.Form, "redirect_uri", "https://tenet.example.com/auth/github/callback")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access-123","token_type":"bearer","scope":"read:user user:email"}`))
	}))
	defer server.Close()
	provider := mustGitHubProvider(t, GitHubConfig{TokenURL: server.URL})

	token, err := provider.Exchange(context.Background(), "code-123")
	if err != nil {
		t.Fatalf("Exchange returned error: %v", err)
	}
	if !sawRequest {
		t.Fatal("token endpoint was not called")
	}
	if token.AccessToken != "access-123" || token.TokenType != "bearer" || token.Scope != "read:user user:email" {
		t.Fatalf("unexpected token: %+v", token)
	}
}

func TestGitHubProviderExchangeReportsProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad_verification_code"}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	provider := mustGitHubProvider(t, GitHubConfig{TokenURL: server.URL})

	_, err := provider.Exchange(context.Background(), "bad-code")
	if err == nil {
		t.Fatal("expected exchange error")
	}
	message := err.Error()
	if !strings.Contains(message, "401 Unauthorized") || !strings.Contains(message, "bad_verification_code") {
		t.Fatalf("error = %q, want status and provider body", message)
	}
}

func TestGitHubProviderUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Fatalf("Accept = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       float64(42),
			"login":    "octocat",
			"name":     "The Octocat",
			"email":    "octocat@example.com",
			"html_url": "https://github.com/octocat",
		})
	}))
	defer server.Close()
	provider := mustGitHubProvider(t, GitHubConfig{UserURL: server.URL})

	identity, err := provider.User(context.Background(), "token-123")
	if err != nil {
		t.Fatalf("User returned error: %v", err)
	}
	if identity.Provider != GitHubProviderName || identity.ID != "42" || identity.Login != "octocat" || identity.Name != "The Octocat" || identity.Email != "octocat@example.com" || identity.URL != "https://github.com/octocat" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestGitHubProviderValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  GitHubConfig
		wantErr string
	}{
		{name: "missing client id", config: GitHubConfig{ClientSecret: "secret", RedirectURL: "https://tenet.example.com/callback"}, wantErr: "missing client id"},
		{name: "missing client secret", config: GitHubConfig{ClientID: "client", RedirectURL: "https://tenet.example.com/callback"}, wantErr: "missing client secret"},
		{name: "missing redirect", config: GitHubConfig{ClientID: "client", ClientSecret: "secret"}, wantErr: "missing redirect URL"},
		{name: "bad endpoint", config: GitHubConfig{ClientID: "client", ClientSecret: "secret", RedirectURL: "https://tenet.example.com/callback", TokenURL: "://bad"}, wantErr: "token URL is invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGitHubProvider(tt.config)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("NewGitHubProvider error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestGitHubProviderRejectsEmptyInputs(t *testing.T) {
	provider := mustGitHubProvider(t, GitHubConfig{})
	if _, err := provider.Exchange(context.Background(), " "); err == nil || !strings.Contains(err.Error(), "non-empty authorization code") {
		t.Fatalf("Exchange empty code error = %v", err)
	}
	if _, err := provider.User(context.Background(), " "); err == nil || !strings.Contains(err.Error(), "non-empty access token") {
		t.Fatalf("User empty token error = %v", err)
	}
}

func mustGitHubProvider(t *testing.T, cfg GitHubConfig) *GitHubProvider {
	t.Helper()
	if cfg.ClientID == "" {
		cfg.ClientID = "client-id"
	}
	if cfg.ClientSecret == "" {
		cfg.ClientSecret = "client-secret"
	}
	if cfg.RedirectURL == "" {
		cfg.RedirectURL = "https://tenet.example.com/auth/github/callback"
	}
	provider, err := NewGitHubProvider(cfg)
	if err != nil {
		t.Fatalf("NewGitHubProvider returned error: %v", err)
	}
	return provider
}

func assertQuery(t *testing.T, values url.Values, key, want string) {
	t.Helper()
	if got := values.Get(key); got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}
