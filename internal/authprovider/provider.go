package authprovider

import "context"

// Token is the normalized result of an OAuth authorization-code exchange.
type Token struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

// Identity is the normalized authenticated user profile returned by an auth provider.
type Identity struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Login    string `json:"login,omitempty"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	URL      string `json:"url,omitempty"`
}

// Provider is the small extension point for future browser/OAuth auth flows.
// Implementations are intentionally not wired to Tenet's trusted-front-door
// request authorization yet; callers must decide how to turn provider identity
// into sessions or trusted forwarded headers.
type Provider interface {
	Name() string
	AuthCodeURL(state string, scopes []string) (string, error)
	Exchange(ctx context.Context, code string) (Token, error)
	User(ctx context.Context, accessToken string) (Identity, error)
}
