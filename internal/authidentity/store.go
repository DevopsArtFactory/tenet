package authidentity

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/authprovider"
)

// ErrNotFound reports that no persisted auth identity exists for the provider
// and subject pair.
var ErrNotFound = errors.New("auth identity not found")

// Identity is Tenet's normalized persisted view of an external auth-provider
// identity. It is intentionally small groundwork for future OAuth/session work;
// it is not wired into trusted-front-door request authorization.
type Identity struct {
	Provider  string    `json:"provider"`
	Subject   string    `json:"subject"`
	Login     string    `json:"login,omitempty"`
	Name      string    `json:"name,omitempty"`
	Email     string    `json:"email,omitempty"`
	URL       string    `json:"url,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Store persists normalized auth-provider identities by provider and subject.
type Store interface {
	List(context.Context) ([]Identity, error)
	Get(context.Context, string, string) (Identity, error)
	Upsert(context.Context, Identity) (Identity, error)
}

// FromProviderIdentity converts an OAuth provider profile into the auth identity
// record shape used by Store implementations.
func FromProviderIdentity(identity authprovider.Identity) Identity {
	return Identity{
		Provider: identity.Provider,
		Subject:  identity.ID,
		Login:    identity.Login,
		Name:     identity.Name,
		Email:    identity.Email,
		URL:      identity.URL,
	}
}

// Key returns the stable provider+subject key used for deterministic ordering
// and future database uniqueness constraints.
func (i Identity) Key() (string, error) {
	provider, subject, err := normalizeKeyParts(i.Provider, i.Subject)
	if err != nil {
		return "", err
	}
	return key(provider, subject), nil
}

func normalizeIdentity(identity Identity) (Identity, error) {
	provider, subject, err := normalizeKeyParts(identity.Provider, identity.Subject)
	if err != nil {
		return Identity{}, err
	}
	identity.Provider = provider
	identity.Subject = subject
	identity.Login = strings.TrimSpace(identity.Login)
	identity.Name = strings.TrimSpace(identity.Name)
	identity.Email = strings.TrimSpace(identity.Email)
	identity.URL = strings.TrimSpace(identity.URL)
	return identity, nil
}

func normalizeKeyParts(provider, subject string) (string, string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return "", "", fmt.Errorf("auth identity provider is required")
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "", "", fmt.Errorf("auth identity subject is required")
	}
	return provider, subject, nil
}

func key(provider, subject string) string {
	return provider + ":" + url.PathEscape(subject)
}

func sortIdentities(identities []Identity) []Identity {
	sorted := append([]Identity(nil), identities...)
	sort.Slice(sorted, func(i, j int) bool {
		left := key(sorted[i].Provider, sorted[i].Subject)
		right := key(sorted[j].Provider, sorted[j].Subject)
		return left < right
	})
	return sorted
}

func findIdentity(identities []Identity, provider, subject string) (Identity, int, error) {
	provider, subject, err := normalizeKeyParts(provider, subject)
	if err != nil {
		return Identity{}, -1, err
	}
	wanted := key(provider, subject)
	for idx, identity := range identities {
		normalized, err := normalizeIdentity(identity)
		if err != nil {
			continue
		}
		if key(normalized.Provider, normalized.Subject) == wanted {
			return normalized, idx, nil
		}
	}
	return Identity{}, -1, fmt.Errorf("%w: %s", ErrNotFound, wanted)
}
