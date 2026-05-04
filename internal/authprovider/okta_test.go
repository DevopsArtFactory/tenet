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

func TestNewOktaOIDCProviderDerivesEndpointsFromIssuer(t *testing.T) {
	provider, err := NewOktaOIDCProvider(OktaOIDCConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://tenet.example.com/auth/callback/okta/oidc",
		IssuerURL:    "https://dev-12345.okta.com/oauth2/default/",
	})
	if err != nil {
		t.Fatalf("NewOktaOIDCProvider returned error: %v", err)
	}
	if provider.Name() != OktaOIDCProviderName {
		t.Fatalf("unexpected provider name %q", provider.Name())
	}
	authURL, err := provider.AuthCodeURL("state-123", []string{"openid", "email", "profile"})
	if err != nil {
		t.Fatalf("AuthCodeURL returned error: %v", err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	if parsed.Scheme+"://"+parsed.Host+parsed.Path != "https://dev-12345.okta.com/oauth2/default/v1/authorize" {
		t.Fatalf("expected issuer-derived authorize endpoint, got %s", authURL)
	}
	if parsed.Query().Get("client_id") != "client-id" || parsed.Query().Get("redirect_uri") != "https://tenet.example.com/auth/callback/okta/oidc" || parsed.Query().Get("state") != "state-123" {
		t.Fatalf("unexpected auth URL query: %s", authURL)
	}
	if scope := parsed.Query().Get("scope"); !strings.Contains(scope, "openid") || !strings.Contains(scope, "email") || !strings.Contains(scope, "profile") {
		t.Fatalf("expected OIDC scopes, got %q", scope)
	}
}

func TestOktaOIDCProviderExchangeAndUserNormalizeIdentityWithoutPersistingToken(t *testing.T) {
	var tokenRequests int
	var userinfoRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/default/v1/token":
			tokenRequests++
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST token request, got %s", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			if r.Form.Get("code") != "okta-code" || r.Form.Get("client_secret") != "client-secret" {
				t.Fatalf("unexpected token form: %v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "okta-access-token", "token_type": "Bearer"})
		case "/oauth2/default/v1/userinfo":
			userinfoRequests++
			if got := r.Header.Get("Authorization"); got != "Bearer okta-access-token" {
				t.Fatalf("expected bearer token for transient userinfo lookup, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":                "00u123",
				"preferred_username": "operator@example.com",
				"email":              "operator@example.com",
				"email_verified":     true,
				"name":               "Okta Operator",
				"profile":            "https://okta.example.com/enduser/profile",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewOktaOIDCProvider(OktaOIDCConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://tenet.example.com/auth/callback/okta/oidc",
		IssuerURL:    server.URL + "/oauth2/default",
	})
	if err != nil {
		t.Fatalf("NewOktaOIDCProvider returned error: %v", err)
	}
	token, err := provider.Exchange(context.Background(), "okta-code")
	if err != nil {
		t.Fatalf("Exchange returned error: %v", err)
	}
	if token.AccessToken != "okta-access-token" {
		t.Fatalf("unexpected token: %+v", token)
	}
	identity, err := provider.User(context.Background(), token.AccessToken)
	if err != nil {
		t.Fatalf("User returned error: %v", err)
	}
	if identity.Provider != OktaOIDCProviderName || identity.ID != "00u123" || identity.Login != "operator@example.com" || identity.Name != "Okta Operator" || identity.Email != "operator@example.com" {
		t.Fatalf("unexpected Okta identity: %+v", identity)
	}
	if tokenRequests != 1 || userinfoRequests != 1 {
		t.Fatalf("expected one token and one userinfo request, got token=%d userinfo=%d", tokenRequests, userinfoRequests)
	}
}

func TestOktaOIDCProviderRejectsIncompleteConfig(t *testing.T) {
	if _, err := NewOktaOIDCProvider(OktaOIDCConfig{ClientID: "client-id", ClientSecret: "client-secret", RedirectURL: "https://tenet.example.com/auth/callback/okta/oidc"}); err == nil {
		t.Fatalf("expected missing issuer URL error")
	}
	if _, err := NewOktaOIDCProvider(OktaOIDCConfig{ClientID: "client-id", ClientSecret: "client-secret", RedirectURL: "https://tenet.example.com/auth/callback/okta/oidc", IssuerURL: "ftp://okta.example.com/oauth2/default"}); err == nil {
		t.Fatalf("expected invalid issuer URL error")
	}
}
