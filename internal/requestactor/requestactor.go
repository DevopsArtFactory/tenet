package requestactor

import (
	"context"
	"net/http"
	"strings"
)

const (
	AuthenticatedIdentitySourceCanonical            = "canonical"
	AuthenticatedIdentitySourceAlias                = "alias"
	AuthenticatedIdentitySourceManagedSessionCookie = "managed_session_cookie"
	AuthenticatedIdentitySourceAbsent               = "absent"

	ActorAttributionSourceExplicitActorHeaders         = "explicit_actor_headers"
	ActorAttributionSourceActorAliasHeaders            = "actor_alias_headers"
	ActorAttributionSourceTrustedAuthenticatedIdentity = "trusted_authenticated_identity"
	ActorAttributionSourceUnavailable                  = "unavailable"

	HeaderID    = "X-Tenet-Actor-Id"
	HeaderName  = "X-Tenet-Actor-Name"
	HeaderEmail = "X-Tenet-Actor-Email"

	HeaderAliasID    = "X-User-ID"
	HeaderAliasName  = "X-User-Name"
	HeaderAliasEmail = "X-User-Email"

	AuthenticatedHeaderID        = "X-Tenet-Authenticated-Id"
	AuthenticatedHeaderName      = "X-Tenet-Authenticated-Name"
	AuthenticatedHeaderEmail     = "X-Tenet-Authenticated-Email"
	AuthenticatedRolesHeader     = "X-Tenet-Authenticated-Roles"
	AuthenticatedGroupsHeader    = "X-Tenet-Authenticated-Groups"
	AuthorizedDeploymentsHeader  = "X-Tenet-Authorized-Deployments"
	AuthorizedProjectsHeader     = "X-Tenet-Authorized-Projects"
	AuthorizedEnvironmentsHeader = "X-Tenet-Authorized-Environments"

	UnavailableReason                      = "actor information is unavailable because no authenticated request context was available for this local request"
	AuthenticatedIdentityUnavailableReason = "authenticated identity is unavailable because no trusted forwarded identity headers were available for this request"

	RoleViewer   = "viewer"
	RoleOperator = "operator"
	RoleAdmin    = "admin"
)

type actorContextKey struct{}
type authenticatedIdentityContextKey struct{}
type authenticatedIdentityProvenanceContextKey struct{}
type authenticatedRolesContextKey struct{}
type authenticatedGroupsContextKey struct{}
type authorizedDeploymentsContextKey struct{}
type authorizedProjectEnvironmentScopesContextKey struct{}

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

// AuthenticatedIdentityProvenance reports which trusted header family supplied each
// authenticated identity field. Source values are "canonical" for
// X-Tenet-Authenticated-* headers, "alias" for X-User-* compatibility headers,
// and "absent" when neither trusted header family supplied that field.
type AuthenticatedIdentityProvenance struct {
	IDSource    string `json:"idSource"`
	NameSource  string `json:"nameSource"`
	EmailSource string `json:"emailSource"`
}

// ActorProvenance reports which request actor header family supplied each
// persisted actor-attribution field. Source values are "canonical" for
// X-Tenet-Actor-* headers, "alias" for X-User-* compatibility headers, and
// "absent" when neither actor header family supplied that field.
type ActorProvenance struct {
	IDSource    string `json:"idSource"`
	NameSource  string `json:"nameSource"`
	EmailSource string `json:"emailSource"`
}

// ActorAttribution explains which request evidence will be used for persisted
// plan/apply/audit actor fields. It is diagnostic-only and never authorizes routes.
type ActorAttribution struct {
	Available  bool     `json:"available"`
	Source     string   `json:"source"`
	Precedence []string `json:"precedence"`
	Summary    string   `json:"summary"`
}

// AuthenticatedRoles captures the front-door-authenticated roles Tenet can trust
// when the API instance is configured to accept forwarded auth headers.
type AuthenticatedRoles struct {
	Names []string `json:"names,omitempty"`
}

// AuthenticatedGroups captures trusted group claims forwarded by an authenticating front door.
type AuthenticatedGroups struct {
	Names []string `json:"names,omitempty"`
}

// AuthenticatedRolesProvenance reports whether trusted role data came from the
// X-Tenet-Authenticated-Roles header or was absent from the request.
type AuthenticatedRolesProvenance struct {
	Source string `json:"source"`
}

// AuthorizedDeployments captures the trusted front-door deployment allowlist for one request.
type AuthorizedDeployments struct {
	Names []string `json:"names,omitempty"`
}

// AuthorizedProjectEnvironmentScopes captures trusted front-door project/environment visibility
// grants for one request. A deployment matches this scope only when Tenet can prove both its
// project and environment placement from the catalog.
type AuthorizedProjectEnvironmentScopes struct {
	Projects     []string `json:"projects,omitempty"`
	Environments []string `json:"environments,omitempty"`
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

// WithAuthenticatedIdentityProvenanceContext stores diagnostic provenance for the authenticated identity on the context.
func WithAuthenticatedIdentityProvenanceContext(ctx context.Context, provenance AuthenticatedIdentityProvenance) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, authenticatedIdentityProvenanceContextKey{}, NormalizeAuthenticatedIdentityProvenance(provenance))
}

// WithManagedSessionAuthenticatedIdentityContext stores a Tenet-managed session identity and marks its provenance as the session cookie.
func WithManagedSessionAuthenticatedIdentityContext(ctx context.Context, identity AuthenticatedIdentity) context.Context {
	identity = NormalizeAuthenticatedIdentity(identity)
	ctx = WithAuthenticatedIdentityContext(ctx, identity)
	return WithAuthenticatedIdentityProvenanceContext(ctx, AuthenticatedIdentityProvenanceFromManagedSession(identity))
}

// WithAuthenticatedRolesContext stores one normalized authenticated role set on the context.
func WithAuthenticatedRolesContext(ctx context.Context, roles AuthenticatedRoles) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, authenticatedRolesContextKey{}, NormalizeAuthenticatedRoles(roles))
}

// WithAuthenticatedGroupsContext stores one normalized authenticated group set on the context.
func WithAuthenticatedGroupsContext(ctx context.Context, groups AuthenticatedGroups) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, authenticatedGroupsContextKey{}, NormalizeAuthenticatedGroups(groups))
}

// WithAuthorizedDeploymentsContext stores one normalized deployment allowlist on the context.
func WithAuthorizedDeploymentsContext(ctx context.Context, deployments AuthorizedDeployments) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, authorizedDeploymentsContextKey{}, NormalizeAuthorizedDeployments(deployments))
}

// WithAuthorizedProjectEnvironmentScopesContext stores normalized project/environment scopes on the context.
func WithAuthorizedProjectEnvironmentScopesContext(ctx context.Context, scopes AuthorizedProjectEnvironmentScopes) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, authorizedProjectEnvironmentScopesContextKey{}, NormalizeAuthorizedProjectEnvironmentScopes(scopes))
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

// AuthenticatedIdentityProvenanceFromContext returns normalized diagnostic provenance for the context identity.
func AuthenticatedIdentityProvenanceFromContext(ctx context.Context) AuthenticatedIdentityProvenance {
	if ctx == nil {
		return NormalizeAuthenticatedIdentityProvenance(AuthenticatedIdentityProvenance{})
	}
	provenance, _ := ctx.Value(authenticatedIdentityProvenanceContextKey{}).(AuthenticatedIdentityProvenance)
	return NormalizeAuthenticatedIdentityProvenance(provenance)
}

// AuthenticatedRolesFromContext returns the normalized authenticated roles stored on the context.
func AuthenticatedRolesFromContext(ctx context.Context) AuthenticatedRoles {
	if ctx == nil {
		return NormalizeAuthenticatedRoles(AuthenticatedRoles{})
	}
	roles, _ := ctx.Value(authenticatedRolesContextKey{}).(AuthenticatedRoles)
	return NormalizeAuthenticatedRoles(roles)
}

// AuthenticatedGroupsFromContext returns the normalized authenticated groups stored on the context.
func AuthenticatedGroupsFromContext(ctx context.Context) AuthenticatedGroups {
	if ctx == nil {
		return NormalizeAuthenticatedGroups(AuthenticatedGroups{})
	}
	groups, _ := ctx.Value(authenticatedGroupsContextKey{}).(AuthenticatedGroups)
	return NormalizeAuthenticatedGroups(groups)
}

// AuthorizedDeploymentsFromContext returns the normalized deployment allowlist stored on the context.
func AuthorizedDeploymentsFromContext(ctx context.Context) AuthorizedDeployments {
	if ctx == nil {
		return NormalizeAuthorizedDeployments(AuthorizedDeployments{})
	}
	deployments, _ := ctx.Value(authorizedDeploymentsContextKey{}).(AuthorizedDeployments)
	return NormalizeAuthorizedDeployments(deployments)
}

// AuthorizedProjectEnvironmentScopesFromContext returns normalized project/environment scopes stored on the context.
func AuthorizedProjectEnvironmentScopesFromContext(ctx context.Context) AuthorizedProjectEnvironmentScopes {
	if ctx == nil {
		return NormalizeAuthorizedProjectEnvironmentScopes(AuthorizedProjectEnvironmentScopes{})
	}
	scopes, _ := ctx.Value(authorizedProjectEnvironmentScopesContextKey{}).(AuthorizedProjectEnvironmentScopes)
	return NormalizeAuthorizedProjectEnvironmentScopes(scopes)
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
// front-door headers. Explicit X-Tenet-Authenticated-* values win, and X-User-* only fills
// fields that are absent on that trusted path.
func AuthenticatedIdentityFromHeaders(header http.Header) AuthenticatedIdentity {
	if header == nil {
		return NormalizeAuthenticatedIdentity(AuthenticatedIdentity{})
	}
	identity := AuthenticatedIdentity{
		ID:    headerValue(header, AuthenticatedHeaderID, HeaderAliasID),
		Name:  headerValue(header, AuthenticatedHeaderName, HeaderAliasName),
		Email: headerValue(header, AuthenticatedHeaderEmail, HeaderAliasEmail),
	}
	if identity.ID != "" || identity.Name != "" || identity.Email != "" {
		identity.Authenticated = true
	}
	return NormalizeAuthenticatedIdentity(identity)
}

// ActorProvenanceFromHeaders reports the source header family used for each
// persisted actor field. Canonical X-Tenet-Actor-* headers win over X-User-*
// compatibility aliases, matching FromHeaders.
func ActorProvenanceFromHeaders(header http.Header) ActorProvenance {
	if header == nil {
		return NormalizeActorProvenance(ActorProvenance{})
	}
	return NormalizeActorProvenance(ActorProvenance{
		IDSource:    headerSource(header, HeaderID, HeaderAliasID),
		NameSource:  headerSource(header, HeaderName, HeaderAliasName),
		EmailSource: headerSource(header, HeaderEmail, HeaderAliasEmail),
	})
}

// Attribution explains whether persisted actor fields came from explicit actor
// headers, trusted authenticated identity projection, or no available evidence.
func Attribution(actor Actor, explicitActor Actor, identity AuthenticatedIdentity) ActorAttribution {
	actor = Normalize(actor)
	explicitActor = Normalize(explicitActor)
	identity = NormalizeAuthenticatedIdentity(identity)
	precedence := SupportedActorAttributionPrecedence()
	if explicitActor.Available {
		return ActorAttribution{
			Available:  true,
			Source:     ActorAttributionSourceExplicitActorHeaders,
			Precedence: precedence,
			Summary:    "persisted plan/apply/audit actor fields will use explicit X-Tenet-Actor-* headers, with X-User-* actor aliases only filling missing actor fields",
		}
	}
	if identity.Authenticated && actor.Available {
		return ActorAttribution{
			Available:  true,
			Source:     ActorAttributionSourceTrustedAuthenticatedIdentity,
			Precedence: precedence,
			Summary:    "persisted plan/apply/audit actor fields will project from trusted authenticated identity because no explicit actor headers were supplied",
		}
	}
	return ActorAttribution{
		Available:  false,
		Source:     ActorAttributionSourceUnavailable,
		Precedence: precedence,
		Summary:    "persisted plan/apply/audit actor fields will remain unavailable until explicit actor headers or trusted authenticated identity are supplied; actor attribution does not authorize routes",
	}
}

// AuthenticatedIdentityProvenanceFromHeaders reports the source header family used
// for each trusted authenticated identity field. Canonical X-Tenet-Authenticated-*
// headers win over X-User-* compatibility aliases, matching AuthenticatedIdentityFromHeaders.
func AuthenticatedIdentityProvenanceFromHeaders(header http.Header) AuthenticatedIdentityProvenance {
	if header == nil {
		return NormalizeAuthenticatedIdentityProvenance(AuthenticatedIdentityProvenance{})
	}
	return NormalizeAuthenticatedIdentityProvenance(AuthenticatedIdentityProvenance{
		IDSource:    headerSource(header, AuthenticatedHeaderID, HeaderAliasID),
		NameSource:  headerSource(header, AuthenticatedHeaderName, HeaderAliasName),
		EmailSource: headerSource(header, AuthenticatedHeaderEmail, HeaderAliasEmail),
	})
}

// AuthenticatedIdentityProvenanceFromManagedSession reports which fields came from a valid Tenet-managed session cookie.
func AuthenticatedIdentityProvenanceFromManagedSession(identity AuthenticatedIdentity) AuthenticatedIdentityProvenance {
	identity = NormalizeAuthenticatedIdentity(identity)
	if !identity.Authenticated {
		return NormalizeAuthenticatedIdentityProvenance(AuthenticatedIdentityProvenance{})
	}
	provenance := AuthenticatedIdentityProvenance{}
	if strings.TrimSpace(identity.ID) != "" {
		provenance.IDSource = AuthenticatedIdentitySourceManagedSessionCookie
	}
	if strings.TrimSpace(identity.Name) != "" {
		provenance.NameSource = AuthenticatedIdentitySourceManagedSessionCookie
	}
	if strings.TrimSpace(identity.Email) != "" {
		provenance.EmailSource = AuthenticatedIdentitySourceManagedSessionCookie
	}
	return NormalizeAuthenticatedIdentityProvenance(provenance)
}

// AuthenticatedRolesFromHeaders derives one normalized trusted role set from forwarded front-door headers.
func AuthenticatedRolesFromHeaders(header http.Header) AuthenticatedRoles {
	if header == nil {
		return NormalizeAuthenticatedRoles(AuthenticatedRoles{})
	}
	names := make([]string, 0)
	for _, raw := range header.Values(AuthenticatedRolesHeader) {
		for _, candidate := range strings.Split(raw, ",") {
			if name := strings.TrimSpace(candidate); name != "" {
				names = append(names, name)
			}
		}
	}
	return NormalizeAuthenticatedRoles(AuthenticatedRoles{Names: names})
}

// AuthenticatedGroupsFromHeaders derives normalized trusted group claims from forwarded front-door headers.
func AuthenticatedGroupsFromHeaders(header http.Header) AuthenticatedGroups {
	if header == nil {
		return NormalizeAuthenticatedGroups(AuthenticatedGroups{})
	}
	return NormalizeAuthenticatedGroups(AuthenticatedGroups{Names: valuesFromCommaHeaders(header, AuthenticatedGroupsHeader)})
}

// AuthenticatedRolesProvenanceFromHeaders reports whether X-Tenet-Authenticated-Roles
// supplied at least one normalized trusted role value.
func AuthenticatedRolesProvenanceFromHeaders(header http.Header) AuthenticatedRolesProvenance {
	if len(AuthenticatedRolesFromHeaders(header).Names) == 0 {
		return NormalizeAuthenticatedRolesProvenance(AuthenticatedRolesProvenance{})
	}
	return AuthenticatedRolesProvenance{Source: AuthenticatedIdentitySourceCanonical}
}

// AuthorizedDeploymentsFromHeaders derives one normalized deployment allowlist from trusted
// forwarded front-door headers.
func AuthorizedDeploymentsFromHeaders(header http.Header) AuthorizedDeployments {
	if header == nil {
		return NormalizeAuthorizedDeployments(AuthorizedDeployments{})
	}
	return NormalizeAuthorizedDeployments(AuthorizedDeployments{Names: valuesFromCommaHeaders(header, AuthorizedDeploymentsHeader)})
}

// AuthorizedProjectEnvironmentScopesFromHeaders derives normalized project/environment scopes from trusted forwarded front-door headers.
func AuthorizedProjectEnvironmentScopesFromHeaders(header http.Header) AuthorizedProjectEnvironmentScopes {
	if header == nil {
		return NormalizeAuthorizedProjectEnvironmentScopes(AuthorizedProjectEnvironmentScopes{})
	}
	return NormalizeAuthorizedProjectEnvironmentScopes(AuthorizedProjectEnvironmentScopes{
		Projects:     valuesFromCommaHeaders(header, AuthorizedProjectsHeader),
		Environments: valuesFromCommaHeaders(header, AuthorizedEnvironmentsHeader),
	})
}

func valuesFromCommaHeaders(header http.Header, name string) []string {
	values := make([]string, 0)
	for _, raw := range header.Values(name) {
		for _, candidate := range strings.Split(raw, ",") {
			if value := strings.TrimSpace(candidate); value != "" {
				values = append(values, value)
			}
		}
	}
	return values
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

// SupportedOperatorRoleNames returns the stable role names that Tenet recognizes for
// trusted front-door operator capabilities.
func SupportedOperatorRoleNames() []string {
	return []string{RoleViewer, RoleOperator, RoleAdmin}
}

// AcceptedOperatorReadRoleNames returns the stable role names that satisfy
// guarded operator read capability checks.
func AcceptedOperatorReadRoleNames() []string {
	return []string{RoleViewer, RoleOperator, RoleAdmin}
}

// AcceptedOperatorWriteRoleNames returns the stable role names that satisfy
// guarded operator write capability checks.
func AcceptedOperatorWriteRoleNames() []string {
	return []string{RoleOperator, RoleAdmin}
}

// SupportedActorHeaderNames returns the canonical explicit actor-attribution
// headers Tenet accepts for persisted plan/apply/audit records.
func SupportedActorHeaderNames() []string {
	return []string{HeaderID, HeaderName, HeaderEmail}
}

// SupportedActorAliasHeaderNames returns the compatible actor-attribution aliases
// Tenet accepts when the matching canonical actor field is absent.
func SupportedActorAliasHeaderNames() []string {
	return []string{HeaderAliasID, HeaderAliasName, HeaderAliasEmail}
}

// SupportedActorAttributionPrecedence returns the stable evidence order Tenet uses
// when choosing actor fields persisted on immutable plan/apply/audit records.
func SupportedActorAttributionPrecedence() []string {
	return []string{ActorAttributionSourceExplicitActorHeaders, ActorAttributionSourceActorAliasHeaders, ActorAttributionSourceTrustedAuthenticatedIdentity, ActorAttributionSourceUnavailable}
}

// SupportedAuthenticatedRoleHeaderNames returns the canonical trusted
// front-door role headers Tenet accepts when authenticated headers are trusted.
// Role forwarding intentionally has no X-User-* compatibility alias.
func SupportedAuthenticatedRoleHeaderNames() []string {
	return []string{AuthenticatedRolesHeader}
}

// SupportedAuthenticatedGroupHeaderNames returns the canonical trusted
// front-door group claim headers Tenet accepts for repository policy diagnostics.
func SupportedAuthenticatedGroupHeaderNames() []string {
	return []string{AuthenticatedGroupsHeader}
}

// SupportedAuthenticatedIdentityHeaderNames returns the canonical trusted
// front-door identity headers Tenet accepts when authenticated headers are trusted.
func SupportedAuthenticatedIdentityHeaderNames() []string {
	return []string{AuthenticatedHeaderID, AuthenticatedHeaderName, AuthenticatedHeaderEmail}
}

// SupportedAuthenticatedIdentityAliasHeaderNames returns the compatibility
// trusted identity aliases Tenet accepts when the matching canonical field is absent.
func SupportedAuthenticatedIdentityAliasHeaderNames() []string {
	return []string{HeaderAliasID, HeaderAliasName, HeaderAliasEmail}
}

// SupportedDeploymentAccessHeaderNames returns the trusted front-door access-scope
// headers Tenet accepts when deployment read authorization is enforced.
func SupportedDeploymentAccessHeaderNames() []string {
	return []string{AuthorizedDeploymentsHeader, AuthorizedProjectsHeader, AuthorizedEnvironmentsHeader}
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

// RecognizedNames returns the normalized forwarded roles that Tenet recognizes as
// supported operator capability roles.
func (roles AuthenticatedRoles) RecognizedNames() []string {
	return roles.partitionSupportedNames(true)
}

// UnrecognizedNames returns the normalized forwarded roles that are not part of
// Tenet's supported operator capability role set.
func (roles AuthenticatedRoles) UnrecognizedNames() []string {
	return roles.partitionSupportedNames(false)
}

func (roles AuthenticatedRoles) partitionSupportedNames(wantSupported bool) []string {
	normalized := NormalizeAuthenticatedRoles(roles)
	if len(normalized.Names) == 0 {
		return []string{}
	}
	supported := make(map[string]struct{}, 3)
	for _, name := range SupportedOperatorRoleNames() {
		supported[name] = struct{}{}
	}
	matches := make([]string, 0, len(normalized.Names))
	for _, name := range normalized.Names {
		_, ok := supported[name]
		if ok == wantSupported {
			matches = append(matches, name)
		}
	}
	return matches
}

// AllowsAny reports whether any requested role matches one trusted forwarded role exactly.
func (roles AuthenticatedRoles) AllowsAny(names ...string) bool {
	normalized := NormalizeAuthenticatedRoles(roles)
	if len(normalized.Names) == 0 || len(names) == 0 {
		return false
	}
	allowed := make(map[string]struct{}, len(normalized.Names))
	for _, name := range normalized.Names {
		allowed[name] = struct{}{}
	}
	for _, name := range names {
		candidate := strings.TrimSpace(name)
		if candidate == "" {
			continue
		}
		if _, ok := allowed[candidate]; ok {
			return true
		}
	}
	return false
}

// AllowsOperatorWrite reports whether the trusted forwarded roles include one operator-capable role.
func (roles AuthenticatedRoles) AllowsOperatorWrite() bool {
	return roles.AllowsAny(RoleOperator, RoleAdmin)
}

// AllowsOperatorRead reports whether the trusted forwarded roles include one read-capable role.
func (roles AuthenticatedRoles) AllowsOperatorRead() bool {
	return roles.AllowsAny(RoleViewer, RoleOperator, RoleAdmin)
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

func headerSource(header http.Header, primary string, aliases ...string) string {
	if value := strings.TrimSpace(header.Get(primary)); value != "" {
		return AuthenticatedIdentitySourceCanonical
	}
	for _, alias := range aliases {
		if value := strings.TrimSpace(header.Get(alias)); value != "" {
			return AuthenticatedIdentitySourceAlias
		}
	}
	return AuthenticatedIdentitySourceAbsent
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

// NormalizeActorProvenance keeps actor-attribution source strings explicit and stable.
func NormalizeActorProvenance(provenance ActorProvenance) ActorProvenance {
	provenance.IDSource = normalizeAuthenticatedIdentitySource(provenance.IDSource)
	provenance.NameSource = normalizeAuthenticatedIdentitySource(provenance.NameSource)
	provenance.EmailSource = normalizeAuthenticatedIdentitySource(provenance.EmailSource)
	return provenance
}

// NormalizeAuthenticatedIdentityProvenance keeps source strings explicit and stable.
func NormalizeAuthenticatedIdentityProvenance(provenance AuthenticatedIdentityProvenance) AuthenticatedIdentityProvenance {
	provenance.IDSource = normalizeAuthenticatedIdentitySource(provenance.IDSource)
	provenance.NameSource = normalizeAuthenticatedIdentitySource(provenance.NameSource)
	provenance.EmailSource = normalizeAuthenticatedIdentitySource(provenance.EmailSource)
	return provenance
}

// NormalizeAuthenticatedRolesProvenance keeps role source strings explicit and stable.
func NormalizeAuthenticatedRolesProvenance(provenance AuthenticatedRolesProvenance) AuthenticatedRolesProvenance {
	provenance.Source = normalizeAuthenticatedIdentitySource(provenance.Source)
	if provenance.Source == AuthenticatedIdentitySourceAlias {
		provenance.Source = AuthenticatedIdentitySourceAbsent
	}
	return provenance
}

func normalizeAuthenticatedIdentitySource(source string) string {
	switch strings.TrimSpace(source) {
	case AuthenticatedIdentitySourceCanonical:
		return AuthenticatedIdentitySourceCanonical
	case AuthenticatedIdentitySourceAlias:
		return AuthenticatedIdentitySourceAlias
	case AuthenticatedIdentitySourceManagedSessionCookie:
		return AuthenticatedIdentitySourceManagedSessionCookie
	default:
		return AuthenticatedIdentitySourceAbsent
	}
}

// NormalizeAuthenticatedRoles trims whitespace, drops empties, and keeps the role set stable.
func NormalizeAuthenticatedRoles(roles AuthenticatedRoles) AuthenticatedRoles {
	if len(roles.Names) == 0 {
		roles.Names = nil
		return roles
	}
	seen := make(map[string]struct{}, len(roles.Names))
	names := make([]string, 0, len(roles.Names))
	for _, raw := range roles.Names {
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
		roles.Names = nil
		return roles
	}
	roles.Names = names
	return roles
}

// NormalizeAuthenticatedGroups trims whitespace, drops empties, and keeps the group set stable.
func NormalizeAuthenticatedGroups(groups AuthenticatedGroups) AuthenticatedGroups {
	groups.Names = normalizeStringSet(groups.Names)
	return groups
}

// NormalizeAuthorizedDeployments trims whitespace, drops empties, and keeps the allowlist stable.
func NormalizeAuthorizedDeployments(deployments AuthorizedDeployments) AuthorizedDeployments {
	deployments.Names = normalizeStringSet(deployments.Names)
	return deployments
}

// NormalizeAuthorizedProjectEnvironmentScopes trims whitespace, drops empties, and keeps scope sets stable.
func NormalizeAuthorizedProjectEnvironmentScopes(scopes AuthorizedProjectEnvironmentScopes) AuthorizedProjectEnvironmentScopes {
	scopes.Projects = normalizeStringSet(scopes.Projects)
	scopes.Environments = normalizeStringSet(scopes.Environments)
	return scopes
}

func normalizeStringSet(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}
