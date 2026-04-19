package requestactor

import (
	"context"
	"net/http"
	"strings"
)

const (
	HeaderID    = "X-Tenet-Actor-Id"
	HeaderName  = "X-Tenet-Actor-Name"
	HeaderEmail = "X-Tenet-Actor-Email"

	HeaderAliasID    = "X-User-ID"
	HeaderAliasName  = "X-User-Name"
	HeaderAliasEmail = "X-User-Email"

	UnavailableReason = "actor information is unavailable because no authenticated request context was available for this local request"
)

type contextKey struct{}

// Actor captures request-scoped operator identity without inventing missing auth data.
type Actor struct {
	Available bool   `json:"available"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// WithContext stores one normalized request actor on the context.
func WithContext(ctx context.Context, actor Actor) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, Normalize(actor))
}

// FromContext returns the normalized request actor stored on the context.
func FromContext(ctx context.Context) Actor {
	if ctx == nil {
		return Normalize(Actor{})
	}
	actor, _ := ctx.Value(contextKey{}).(Actor)
	return Normalize(actor)
}

// FromHeaders derives explicit local-friendly actor context from Tenet request headers.
func FromHeaders(header http.Header) Actor {
	if header == nil {
		return Normalize(Actor{})
	}
	actor := Actor{
		ID:    headerValue(header, HeaderID, HeaderAliasID),
		Name:  headerValue(header, HeaderName, HeaderAliasName),
		Email: headerValue(header, HeaderEmail, HeaderAliasEmail),
	}
	if actor.ID != "" || actor.Name != "" || actor.Email != "" {
		actor.Available = true
	}
	return Normalize(actor)
}

func headerValue(header http.Header, primary string, aliases ...string) string {
	if value := strings.TrimSpace(header.Get(primary)); value != "" {
		return value
	}
	for _, alias := range aliases {
		if value := strings.TrimSpace(header.Get(alias)); value != "" {
			return value
		}
	}
	return ""
}

// Normalize keeps unavailable actors explicit and strips fallback fields from available actors.
func Normalize(actor Actor) Actor {
	if actor.Available {
		actor.Reason = ""
		return actor
	}
	actor.ID = ""
	actor.Name = ""
	actor.Email = ""
	if actor.Reason == "" {
		actor.Reason = UnavailableReason
	}
	return actor
}
