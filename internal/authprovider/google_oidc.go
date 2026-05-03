package authprovider

import (
	"fmt"
	"strings"
)

const GoogleOIDCProviderName = "google_oidc"

// GoogleOIDCProfile is the local, testable subset of Google OIDC profile/ID-token
// claims Tenet needs to normalize enterprise identity. It intentionally carries
// no raw tokens or provider secrets.
type GoogleOIDCProfile struct {
	Subject       string `json:"sub"`
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified,omitempty"`
	Name          string `json:"name,omitempty"`
	Profile       string `json:"profile,omitempty"`
}

// NormalizeGoogleOIDCProfile converts Google Workspace OIDC identity claims into
// Tenet's provider-neutral identity shape. Google Workspace/Gmail is identity
// only here; this does not model Gmail mailbox access or broader Google API
// authorization.
func NormalizeGoogleOIDCProfile(profile GoogleOIDCProfile) (Identity, error) {
	subject := strings.TrimSpace(profile.Subject)
	if subject == "" {
		return Identity{}, fmt.Errorf("Google OIDC profile sub is required")
	}
	email := strings.TrimSpace(profile.Email)
	if !profile.EmailVerified {
		email = ""
	}
	login := email
	if login == "" {
		login = subject
	}
	return Identity{
		Provider: GoogleOIDCProviderName,
		ID:       subject,
		Login:    login,
		Name:     strings.TrimSpace(profile.Name),
		Email:    email,
		URL:      strings.TrimSpace(profile.Profile),
	}, nil
}
