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

	AuthenticatedHeaderID       = "X-Tenet-Authenticated-Id"
	AuthenticatedHeaderName     = "X-Tenet-Authenticated-Name"
	AuthenticatedHeaderEmail    = "X-Tenet-Authenticated-Email"
	AuthorizedDeploymentsHeader = "X-Tenet-Authorized-Deployments"

	UnavailableReason                      = "actor information is unavailable because no authenticated request context was available for this local request"
	AuthenticatedIdentityUnavailableReason = "authenticated identity is unavailable because no trusted forwarded identity headers were available for this request"
)

type actorContextKey struct{}
type authenticatedIdentityContextKey struct{}
type authorizedDeploymentsContextKey struct{}

// Actor captures request-scoped operator identity without inventing missing auth data.
type Actor struct {
	Available bool   `json:"available"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// AuthenticatedIdentity captures the front-door-authenticated request identity Tenet can trust
// when the API instance is configured to accept forwarded identity headers.
type AuthenticatedIdentity struct {
	Authenticated bool   `json:"authenticated"`
	ID            string `json:"id,omitempty"`
	Name          string `json:"name,omitempty"`
	Email         string `json:"email,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

// AuthorizedDeployments captures the trusted front-door deployment allowlist for one request.
type AuthorizedDeployments struct {
	Names []string `json:"names,omitempty"`
}

// WithContext stores one normalized request actor on the context.
func WithContext(ctx context.Context, actor Actor) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, actorContextKey{}, Normalize(actor))
}

// WithAuthenticatedIdentityContext stores one normalized authenticated identity on the context.
func WithAuthenticatedIdentityContext(ctx context.Context, identity AuthenticatedIdentity) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, authenticatedIdentityContextKey{}, NormalizeAuthenticatedIdentity(identity))
}

// WithAuthorizedDeploymentsContext stores one normalized deployment allowlist on the context.
func WithAuthorizedDeploymentsContext(ctx context.Context, deployments AuthorizedDeployments) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, authorizedDeploymentsContextKey{}, NormalizeAuthorizedDeployments(deployments))
}

// FromContext returns the normalized request actor stored on the context.
func FromContext(ctx context.Context) Actor {
	if ctx == nil {
		return Normalize(Actor{})
	}
	actor, _ := ctx.Value(actorContextKey{}).(Actor)
	actor = Normalize(actor)
	if actor.Available {
		return actor
	}
	identity := AuthenticatedIdentityFromContext(ctx)
	if identity.Authenticated {
		return ProjectAuthenticatedIdentity(identity)
	}
	return actor
}

// AuthenticatedIdentityFromContext returns the normalized authenticated identity stored on the context.
func AuthenticatedIdentityFromContext(ctx context.Context) AuthenticatedIdentity {
	if ctx == nil {
		return NormalizeAuthenticatedIdentity(AuthenticatedIdentity{})
	}
	identity, _ := ctx.Value(authenticatedIdentityContextKey{}).(AuthenticatedIdentity)
	return NormalizeAuthenticatedIdentity(identity)
}

// AuthorizedDeploymentsFromContext returns the normalized deployment allowlist stored on the context.
func AuthorizedDeploymentsFromContext(ctx context.Context) AuthorizedDeployments {
	if ctx == nil {
		return NormalizeAuthorizedDeployments(AuthorizedDeployments{})
	}
	deployments, _ := ctx.Value(authorizedDeploymentsContextKey{}).(AuthorizedDeployments)
	return NormalizeAuthorizedDeployments(deployments)
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

// AuthenticatedIdentityFromHeaders derives one authenticated identity from trusted forwarded
// front-door headers.
func AuthenticatedIdentityFromHeaders(header http.Header) AuthenticatedIdentity {
	if header == nil {
		return NormalizeAuthenticatedIdentity(AuthenticatedIdentity{})
	}
	identity := AuthenticatedIdentity{
		ID:    headerValue(header, AuthenticatedHeaderID),
		Name:  headerValue(header, AuthenticatedHeaderName),
		Email: headerValue(header, AuthenticatedHeaderEmail),
	}
	if identity.ID != "" || identity.Name != "" || identity.Email != "" {
		identity.Authenticated = true
	}
	return NormalizeAuthenticatedIdentity(identity)
}

// AuthorizedDeploymentsFromHeaders derives one normalized deployment allowlist from trusted
// forwarded front-door headers.
func AuthorizedDeploymentsFromHeaders(header http.Header) AuthorizedDeployments {
	if header == nil {
		return NormalizeAuthorizedDeployments(AuthorizedDeployments{})
	}
	names := make([]string, 0)
	for _, raw := range header.Values(AuthorizedDeploymentsHeader) {
		for _, candidate := range strings.Split(raw, ",") {
			if name := strings.TrimSpace(candidate); name != "" {
				names = append(names, name)
			}
		}
	}
	return NormalizeAuthorizedDeployments(AuthorizedDeployments{Names: names})
}

// ProjectAuthenticatedIdentity maps a trusted authenticated identity into persisted actor fields.
func ProjectAuthenticatedIdentity(identity AuthenticatedIdentity) Actor {
	if !NormalizeAuthenticatedIdentity(identity).Authenticated {
		return Normalize(Actor{})
	}
	return Normalize(Actor{
		Available: true,
		ID:        identity.ID,
		Name:      identity.Name,
		Email:     identity.Email,
	})
}

// Allows reports whether the deployment is present in the exact trusted allowlist.
func (deployments AuthorizedDeployments) Allows(deploymentName string) bool {
	for _, authorized := range NormalizeAuthorizedDeployments(deployments).Names {
		if authorized == deploymentName {
			return true
		}
	}
	return false
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

// NormalizeAuthenticatedIdentity keeps unauthenticated identities explicit and strips unused
// fields from authenticated identities.
func NormalizeAuthenticatedIdentity(identity AuthenticatedIdentity) AuthenticatedIdentity {
	if identity.Authenticated {
		identity.Reason = ""
		return identity
	}
	identity.ID = ""
	identity.Name = ""
	identity.Email = ""
	if identity.Reason == "" {
		identity.Reason = AuthenticatedIdentityUnavailableReason
	}
	return identity
}

// NormalizeAuthorizedDeployments trims whitespace, drops empties, and keeps the allowlist stable.
func NormalizeAuthorizedDeployments(deployments AuthorizedDeployments) AuthorizedDeployments {
	if len(deployments.Names) == 0 {
		deployments.Names = nil
		return deployments
	}
	seen := make(map[string]struct{}, len(deployments.Names))
	names := make([]string, 0, len(deployments.Names))
	for _, raw := range deployments.Names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	if len(names) == 0 {
		deployments.Names = nil
		return deployments
	}
	deployments.Names = names
	return deployments
}
