package authprovider

import (
	"strings"
	"testing"
)

func TestNormalizeGoogleOIDCProfile(t *testing.T) {
	identity, err := NormalizeGoogleOIDCProfile(GoogleOIDCProfile{
		Subject:       "google-sub-123",
		Email:         "operator@example.com",
		EmailVerified: true,
		Name:          "Example Operator",
		Profile:       "https://profiles.google.test/operator",
	})
	if err != nil {
		t.Fatalf("NormalizeGoogleOIDCProfile returned error: %v", err)
	}
	if identity.Provider != GoogleOIDCProviderName || identity.ID != "google-sub-123" || identity.Login != "operator@example.com" || identity.Name != "Example Operator" || identity.Email != "operator@example.com" || identity.URL != "https://profiles.google.test/operator" {
		t.Fatalf("unexpected normalized Google OIDC identity: %+v", identity)
	}
}

func TestNormalizeGoogleOIDCProfileRequiresVerifiedEmail(t *testing.T) {
	identity, err := NormalizeGoogleOIDCProfile(GoogleOIDCProfile{
		Subject:       "google-sub-123",
		Email:         "operator@example.com",
		EmailVerified: false,
		Name:          "Example Operator",
	})
	if err != nil {
		t.Fatalf("NormalizeGoogleOIDCProfile returned error: %v", err)
	}
	if identity.Email != "" || identity.Login != "google-sub-123" {
		t.Fatalf("expected unverified email to be omitted and subject login fallback, got %+v", identity)
	}
}

func TestNormalizeGoogleOIDCProfileValidation(t *testing.T) {
	_, err := NormalizeGoogleOIDCProfile(GoogleOIDCProfile{Email: "operator@example.com", EmailVerified: true})
	if err == nil || !strings.Contains(err.Error(), "sub") {
		t.Fatalf("expected missing subject error, got %v", err)
	}
}
