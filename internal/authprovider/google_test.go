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

func TestGoogleProviderAuthCodeURL(t *testing.T) {
	provider, err := NewGoogleProvider(GoogleConfig{
		ClientID:     "google-client",
		ClientSecret: "google-secret",
		RedirectURL:  "https://tenet.example.com/auth/callback/google",
		IssuerURL:    "https://accounts.google.test",
		AuthURL:      "https://accounts.google.test/o/oauth2/v2/auth?prompt=select_account",
	})
	if err != nil {
		t.Fatalf("NewGoogleProvider returned error: %v", err)
	}

	authURL, err := provider.AuthCodeURL("state-123", []string{"openid", "email", "profile", "email", " "})
	if err != nil {
		t.Fatalf("AuthCodeURL returned error: %v", err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "accounts.google.test" || parsed.Path != "/o/oauth2/v2/auth" {
		t.Fatalf("unexpected auth URL target: %s", authURL)
	}
	query := parsed.Query()
	assertQuery(t, query, "prompt", "select_account")
	assertQuery(t, query, "client_id", "google-client")
	assertQuery(t, query, "redirect_uri", "https://tenet.example.com/auth/callback/google")
	assertQuery(t, query, "response_type", "code")
	assertQuery(t, query, "scope", "openid email profile")
	assertQuery(t, query, "state", "state-123")
}

func TestGoogleProviderExchange(t *testing.T) {
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
		assertQuery(t, r.Form, "client_id", "google-client")
		assertQuery(t, r.Form, "client_secret", "google-secret")
		assertQuery(t, r.Form, "code", "code-123")
		assertQuery(t, r.Form, "redirect_uri", "https://tenet.example.com/auth/callback/google")
		assertQuery(t, r.Form, "grant_type", "authorization_code")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"google-access","token_type":"Bearer","scope":"openid email profile"}`))
	}))
	defer server.Close()
	provider := mustGoogleProvider(t, GoogleConfig{TokenURL: server.URL})

	token, err := provider.Exchange(context.Background(), "code-123")
	if err != nil {
		t.Fatalf("Exchange returned error: %v", err)
	}
	if !sawRequest {
		t.Fatal("token endpoint was not called")
	}
	if token.AccessToken != "google-access" || token.TokenType != "Bearer" || token.Scope != "openid email profile" {
		t.Fatalf("unexpected token: %+v", token)
	}
}

func TestGoogleProviderUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer google-access" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("Accept = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":     "google-sub-123",
			"email":   "operator@example.com",
			"name":    "Google Operator",
			"profile": "https://profiles.google.com/operator",
		})
	}))
	defer server.Close()
	provider := mustGoogleProvider(t, GoogleConfig{UserInfoURL: server.URL})

	identity, err := provider.User(context.Background(), "google-access")
	if err != nil {
		t.Fatalf("User returned error: %v", err)
	}
	if identity.Provider != GoogleProviderName || identity.ID != "google-sub-123" || identity.Login != "operator@example.com" || identity.Name != "Google Operator" || identity.Email != "operator@example.com" || identity.URL != "https://profiles.google.com/operator" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestGoogleProviderValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  GoogleConfig
		wantErr string
	}{
		{name: "missing client id", config: GoogleConfig{ClientSecret: "secret", RedirectURL: "https://tenet.example.com/callback", IssuerURL: "https://accounts.google.com"}, wantErr: "missing client id"},
		{name: "missing client secret", config: GoogleConfig{ClientID: "client", RedirectURL: "https://tenet.example.com/callback", IssuerURL: "https://accounts.google.com"}, wantErr: "missing client secret"},
		{name: "missing redirect", config: GoogleConfig{ClientID: "client", ClientSecret: "secret", IssuerURL: "https://accounts.google.com"}, wantErr: "missing redirect URL"},
		{name: "bad issuer", config: GoogleConfig{ClientID: "client", ClientSecret: "secret", RedirectURL: "https://tenet.example.com/callback", IssuerURL: "://bad"}, wantErr: "issuer URL is invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGoogleProvider(tt.config)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("NewGoogleProvider error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestGoogleProviderRejectsEmptyInputs(t *testing.T) {
	provider := mustGoogleProvider(t, GoogleConfig{})
	if _, err := provider.Exchange(context.Background(), " "); err == nil || !strings.Contains(err.Error(), "non-empty authorization code") {
		t.Fatalf("Exchange empty code error = %v", err)
	}
	if _, err := provider.User(context.Background(), " "); err == nil || !strings.Contains(err.Error(), "non-empty access token") {
		t.Fatalf("User empty token error = %v", err)
	}
}

func mustGoogleProvider(t *testing.T, cfg GoogleConfig) *GoogleProvider {
	t.Helper()
	if cfg.ClientID == "" {
		cfg.ClientID = "google-client"
	}
	if cfg.ClientSecret == "" {
		cfg.ClientSecret = "google-secret"
	}
	if cfg.RedirectURL == "" {
		cfg.RedirectURL = "https://tenet.example.com/auth/callback/google"
	}
	if cfg.IssuerURL == "" {
		cfg.IssuerURL = "https://accounts.google.test"
	}
	provider, err := NewGoogleProvider(cfg)
	if err != nil {
		t.Fatalf("NewGoogleProvider returned error: %v", err)
	}
	return provider
}
