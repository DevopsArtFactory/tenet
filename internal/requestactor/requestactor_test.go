package requestactor

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestFromHeadersReturnsExplicitUnavailableActorWithoutHeaders(t *testing.T) {
	actor := FromHeaders(http.Header{})
	if actor.Available {
		t.Fatalf("expected unavailable actor, got %+v", actor)
	}
	if actor.Reason != UnavailableReason {
		t.Fatalf("expected unavailable reason, got %+v", actor)
	}
}

func TestFromHeadersReturnsAvailableActorFromExplicitTenetHeaders(t *testing.T) {
	header := http.Header{}
	header.Set(HeaderID, "operator-123")
	header.Set(HeaderName, "Jane Operator")
	header.Set(HeaderEmail, "jane@example.com")

	actor := FromHeaders(header)
	if !actor.Available {
		t.Fatalf("expected available actor, got %+v", actor)
	}
	if actor.ID != "operator-123" || actor.Name != "Jane Operator" || actor.Email != "jane@example.com" {
		t.Fatalf("unexpected available actor details: %+v", actor)
	}
	if actor.Reason != "" {
		t.Fatalf("expected no unavailable reason for available actor, got %+v", actor)
	}
}

func TestFromHeadersReturnsAvailableActorFromAliasHeaders(t *testing.T) {
	header := http.Header{}
	header.Set(HeaderAliasID, "operator-123")
	header.Set(HeaderAliasName, "Jane Operator")
	header.Set(HeaderAliasEmail, "jane@example.com")

	actor := FromHeaders(header)
	if !actor.Available {
		t.Fatalf("expected available actor, got %+v", actor)
	}
	if actor.ID != "operator-123" || actor.Name != "Jane Operator" || actor.Email != "jane@example.com" {
		t.Fatalf("unexpected available actor details: %+v", actor)
	}
}

func TestAuthenticatedIdentityFromHeadersReturnsExplicitUnavailableIdentityWithoutHeaders(t *testing.T) {
	identity := AuthenticatedIdentityFromHeaders(http.Header{})
	if identity.Authenticated {
		t.Fatalf("expected unauthenticated identity, got %+v", identity)
	}
	if identity.Reason != AuthenticatedIdentityUnavailableReason {
		t.Fatalf("expected unavailable reason, got %+v", identity)
	}
}

func TestAuthenticatedIdentityFromHeadersReturnsAuthenticatedIdentity(t *testing.T) {
	header := http.Header{}
	header.Set(AuthenticatedHeaderID, "front-door-123")
	header.Set(AuthenticatedHeaderName, "Front Door Operator")
	header.Set(AuthenticatedHeaderEmail, "frontdoor@example.com")

	identity := AuthenticatedIdentityFromHeaders(header)
	if !identity.Authenticated {
		t.Fatalf("expected authenticated identity, got %+v", identity)
	}
	if identity.ID != "front-door-123" || identity.Name != "Front Door Operator" || identity.Email != "frontdoor@example.com" {
		t.Fatalf("unexpected authenticated identity details: %+v", identity)
	}
	if identity.Reason != "" {
		t.Fatalf("expected no unavailable reason for authenticated identity, got %+v", identity)
	}
}

func TestAuthenticatedIdentityFromHeadersFallsBackToAliasHeaders(t *testing.T) {
	header := http.Header{}
	header.Set(HeaderAliasID, "front-door-123")
	header.Set(HeaderAliasName, "Front Door Operator")
	header.Set(HeaderAliasEmail, "frontdoor@example.com")

	identity := AuthenticatedIdentityFromHeaders(header)
	if !identity.Authenticated {
		t.Fatalf("expected authenticated identity, got %+v", identity)
	}
	if identity.ID != "front-door-123" || identity.Name != "Front Door Operator" || identity.Email != "frontdoor@example.com" {
		t.Fatalf("unexpected alias-backed authenticated identity details: %+v", identity)
	}
}

func TestAuthenticatedIdentityFromHeadersFallsBackPerFieldWhenExplicitTrustedHeadersAreMissing(t *testing.T) {
	header := http.Header{}
	header.Set(AuthenticatedHeaderID, "front-door-123")
	header.Set(HeaderAliasName, "Front Door Operator")
	header.Set(HeaderAliasEmail, "frontdoor@example.com")

	identity := AuthenticatedIdentityFromHeaders(header)
	if !identity.Authenticated {
		t.Fatalf("expected authenticated identity, got %+v", identity)
	}
	if identity.ID != "front-door-123" || identity.Name != "Front Door Operator" || identity.Email != "frontdoor@example.com" {
		t.Fatalf("unexpected mixed trusted/alias authenticated identity details: %+v", identity)
	}
}

func TestAuthenticatedIdentityFromHeadersPrefersExplicitTrustedHeadersOverAliases(t *testing.T) {
	header := http.Header{}
	header.Set(AuthenticatedHeaderID, "front-door-123")
	header.Set(AuthenticatedHeaderName, "Front Door Operator")
	header.Set(AuthenticatedHeaderEmail, "frontdoor@example.com")
	header.Set(HeaderAliasID, "alias-456")
	header.Set(HeaderAliasName, "Alias Operator")
	header.Set(HeaderAliasEmail, "alias@example.com")

	identity := AuthenticatedIdentityFromHeaders(header)
	if identity.ID != "front-door-123" || identity.Name != "Front Door Operator" || identity.Email != "frontdoor@example.com" {
		t.Fatalf("expected explicit trusted headers to win, got %+v", identity)
	}
}

func TestActorProvenanceFromHeadersReportsCanonicalAliasAndAbsentSources(t *testing.T) {
	header := http.Header{}
	header.Set(HeaderID, "operator-123")
	header.Set(HeaderAliasID, "alias-456")
	header.Set(HeaderAliasName, "Alias Operator")

	provenance := ActorProvenanceFromHeaders(header)
	if provenance.IDSource != AuthenticatedIdentitySourceCanonical {
		t.Fatalf("expected canonical id source, got %+v", provenance)
	}
	if provenance.NameSource != AuthenticatedIdentitySourceAlias {
		t.Fatalf("expected alias name source, got %+v", provenance)
	}
	if provenance.EmailSource != AuthenticatedIdentitySourceAbsent {
		t.Fatalf("expected absent email source, got %+v", provenance)
	}
}

func TestSupportedActorHeaderNamesReportCanonicalAndAliasFamilies(t *testing.T) {
	if got, want := SupportedActorHeaderNames(), []string{HeaderID, HeaderName, HeaderEmail}; !reflect.DeepEqual(got, want) {
		t.Fatalf("supported actor headers = %#v, want %#v", got, want)
	}
	if got, want := SupportedActorAliasHeaderNames(), []string{HeaderAliasID, HeaderAliasName, HeaderAliasEmail}; !reflect.DeepEqual(got, want) {
		t.Fatalf("supported actor alias headers = %#v, want %#v", got, want)
	}
	if got, want := SupportedActorAttributionPrecedence(), []string{ActorAttributionSourceExplicitActorHeaders, ActorAttributionSourceActorAliasHeaders, ActorAttributionSourceTrustedAuthenticatedIdentity, ActorAttributionSourceUnavailable}; !reflect.DeepEqual(got, want) {
		t.Fatalf("supported actor attribution precedence = %#v, want %#v", got, want)
	}
}

func TestNormalizeActorProvenanceDefaultsUnknownSourcesToAbsent(t *testing.T) {
	provenance := NormalizeActorProvenance(ActorProvenance{
		IDSource:    " canonical ",
		NameSource:  "surprise",
		EmailSource: "alias",
	})
	if provenance.IDSource != AuthenticatedIdentitySourceCanonical || provenance.NameSource != AuthenticatedIdentitySourceAbsent || provenance.EmailSource != AuthenticatedIdentitySourceAlias {
		t.Fatalf("unexpected normalized actor provenance: %+v", provenance)
	}
}

func TestAttributionReportsActorSourceWithoutAuthorizingRoutes(t *testing.T) {
	for _, tc := range []struct {
		name          string
		actor         Actor
		explicitActor Actor
		identity      AuthenticatedIdentity
		wantAvailable bool
		wantSource    string
		wantSummary   string
	}{
		{
			name:          "explicit actor headers",
			actor:         Actor{Available: true, ID: "operator-123"},
			explicitActor: Actor{Available: true, ID: "operator-123"},
			wantAvailable: true,
			wantSource:    ActorAttributionSourceExplicitActorHeaders,
			wantSummary:   "X-User-* actor aliases only filling missing actor fields",
		},
		{
			name:          "trusted authenticated identity projection",
			actor:         Actor{Available: true, ID: "front-door-123"},
			identity:      AuthenticatedIdentity{Authenticated: true, ID: "front-door-123"},
			wantAvailable: true,
			wantSource:    ActorAttributionSourceTrustedAuthenticatedIdentity,
			wantSummary:   "project from trusted authenticated identity",
		},
		{
			name:          "unavailable",
			wantAvailable: false,
			wantSource:    ActorAttributionSourceUnavailable,
			wantSummary:   "actor attribution does not authorize routes",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Attribution(tc.actor, tc.explicitActor, tc.identity)
			if got.Available != tc.wantAvailable || got.Source != tc.wantSource || !strings.Contains(got.Summary, tc.wantSummary) {
				t.Fatalf("unexpected attribution: %+v", got)
			}
			if want := SupportedActorAttributionPrecedence(); !reflect.DeepEqual(got.Precedence, want) {
				t.Fatalf("unexpected attribution precedence: got %#v, want %#v", got.Precedence, want)
			}
		})
	}
}

func TestAuthenticatedIdentityProvenanceFromHeadersReportsCanonicalAliasAndAbsentSources(t *testing.T) {
	header := http.Header{}
	header.Set(AuthenticatedHeaderID, "front-door-123")
	header.Set(HeaderAliasID, "alias-456")
	header.Set(HeaderAliasName, "Alias Operator")

	provenance := AuthenticatedIdentityProvenanceFromHeaders(header)
	if provenance.IDSource != AuthenticatedIdentitySourceCanonical {
		t.Fatalf("expected canonical id source, got %+v", provenance)
	}
	if provenance.NameSource != AuthenticatedIdentitySourceAlias {
		t.Fatalf("expected alias name source, got %+v", provenance)
	}
	if provenance.EmailSource != AuthenticatedIdentitySourceAbsent {
		t.Fatalf("expected absent email source, got %+v", provenance)
	}
}

func TestNormalizeAuthenticatedIdentityProvenanceDefaultsUnknownSourcesToAbsent(t *testing.T) {
	provenance := NormalizeAuthenticatedIdentityProvenance(AuthenticatedIdentityProvenance{
		IDSource:    " canonical ",
		NameSource:  "surprise",
		EmailSource: "alias",
	})
	if provenance.IDSource != AuthenticatedIdentitySourceCanonical || provenance.NameSource != AuthenticatedIdentitySourceAbsent || provenance.EmailSource != AuthenticatedIdentitySourceAlias {
		t.Fatalf("unexpected normalized provenance: %+v", provenance)
	}
}

func TestAuthenticatedRolesFromHeadersReturnsNormalizedRoles(t *testing.T) {
	header := http.Header{}
	header.Add(AuthenticatedRolesHeader, "operator, admin ,,operator")
	header.Add(AuthenticatedRolesHeader, "viewer")

	roles := AuthenticatedRolesFromHeaders(header)
	want := []string{"operator", "admin", "viewer"}
	if !reflect.DeepEqual(roles.Names, want) {
		t.Fatalf("unexpected authenticated roles: got %#v want %#v", roles.Names, want)
	}
}

func TestAuthenticatedRolesProvenanceFromHeadersReportsCanonicalOrAbsent(t *testing.T) {
	header := http.Header{}
	header.Add(AuthenticatedRolesHeader, "operator, admin")
	if got := AuthenticatedRolesProvenanceFromHeaders(header); got.Source != AuthenticatedIdentitySourceCanonical {
		t.Fatalf("expected canonical role source, got %+v", got)
	}

	header = http.Header{}
	header.Add(AuthenticatedRolesHeader, " , ")
	if got := AuthenticatedRolesProvenanceFromHeaders(header); got.Source != AuthenticatedIdentitySourceAbsent {
		t.Fatalf("expected absent role source for empty forwarded header, got %+v", got)
	}
}

func TestSupportedAuthenticatedRoleHeaderNamesReportsCanonicalRoleHeaderOnly(t *testing.T) {
	want := []string{AuthenticatedRolesHeader}
	if got := SupportedAuthenticatedRoleHeaderNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("supported role headers = %#v, want %#v", got, want)
	}
}

func TestNormalizeAuthenticatedRolesProvenanceDefaultsUnsupportedSourcesToAbsent(t *testing.T) {
	for _, source := range []string{"", "alias", "surprise"} {
		if got := NormalizeAuthenticatedRolesProvenance(AuthenticatedRolesProvenance{Source: source}); got.Source != AuthenticatedIdentitySourceAbsent {
			t.Fatalf("expected %q to normalize to absent, got %+v", source, got)
		}
	}
	if got := NormalizeAuthenticatedRolesProvenance(AuthenticatedRolesProvenance{Source: " canonical "}); got.Source != AuthenticatedIdentitySourceCanonical {
		t.Fatalf("expected canonical role source to stay canonical, got %+v", got)
	}
}

func TestAuthenticatedRolesReportsRecognizedAndUnrecognizedNames(t *testing.T) {
	roles := AuthenticatedRoles{Names: []string{"viewer", "auditor", "operator", "unknown", "admin", "auditor"}}
	if got, want := roles.RecognizedNames(), []string{"viewer", "operator", "admin"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recognized names = %#v, want %#v", got, want)
	}
	if got, want := roles.UnrecognizedNames(), []string{"auditor", "unknown"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unrecognized names = %#v, want %#v", got, want)
	}

	roles = AuthenticatedRoles{}
	if got := roles.RecognizedNames(); got == nil || len(got) != 0 {
		t.Fatalf("expected explicit empty recognized names, got %#v", got)
	}
	if got := roles.UnrecognizedNames(); got == nil || len(got) != 0 {
		t.Fatalf("expected explicit empty unrecognized names, got %#v", got)
	}
}

func TestAuthenticatedRolesAllowsAnyRole(t *testing.T) {
	roles := AuthenticatedRoles{Names: []string{"viewer", "operator"}}
	if !roles.AllowsAny("admin", "operator") {
		t.Fatalf("expected roles to allow operator-capable access")
	}
	if roles.AllowsAny("admin") {
		t.Fatalf("expected roles to reject missing role")
	}
}

func TestAuthenticatedRolesAllowsOperatorRead(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roles AuthenticatedRoles
		want  bool
	}{
		{name: "viewer", roles: AuthenticatedRoles{Names: []string{"viewer"}}, want: true},
		{name: "operator", roles: AuthenticatedRoles{Names: []string{"operator"}}, want: true},
		{name: "admin", roles: AuthenticatedRoles{Names: []string{"admin"}}, want: true},
		{name: "missing", roles: AuthenticatedRoles{Names: []string{"auditor"}}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.roles.AllowsOperatorRead(); got != tc.want {
				t.Fatalf("AllowsOperatorRead() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAuthorizedDeploymentsFromHeadersReturnsNormalizedAllowlist(t *testing.T) {
	header := http.Header{}
	header.Add(AuthorizedDeploymentsHeader, "payments-api-prod, payments-api-staging ,,worker-prod")
	header.Add(AuthorizedDeploymentsHeader, "payments-api-prod")

	deployments := AuthorizedDeploymentsFromHeaders(header)
	want := []string{"payments-api-prod", "payments-api-staging", "worker-prod"}
	if !reflect.DeepEqual(deployments.Names, want) {
		t.Fatalf("unexpected deployment allowlist: got %#v want %#v", deployments.Names, want)
	}
}

func TestAuthorizedProjectEnvironmentScopesFromHeadersReturnsNormalizedScopes(t *testing.T) {
	header := http.Header{}
	header.Add(AuthorizedProjectsHeader, "payments, platform ,,payments")
	header.Add(AuthorizedProjectsHeader, "observability")
	header.Add(AuthorizedEnvironmentsHeader, "prod, staging ,,prod")
	header.Add(AuthorizedEnvironmentsHeader, "dev")

	scopes := AuthorizedProjectEnvironmentScopesFromHeaders(header)
	wantProjects := []string{"payments", "platform", "observability"}
	wantEnvironments := []string{"prod", "staging", "dev"}
	if !reflect.DeepEqual(scopes.Projects, wantProjects) {
		t.Fatalf("unexpected project scopes: got %#v want %#v", scopes.Projects, wantProjects)
	}
	if !reflect.DeepEqual(scopes.Environments, wantEnvironments) {
		t.Fatalf("unexpected environment scopes: got %#v want %#v", scopes.Environments, wantEnvironments)
	}
}

func TestAuthorizedDeploymentsAllowsOnlyExactDeploymentMatches(t *testing.T) {
	deployments := AuthorizedDeployments{Names: []string{"payments-api-prod", "worker-prod"}}
	if !deployments.Allows("payments-api-prod") {
		t.Fatalf("expected allowlist to authorize exact deployment")
	}
	if deployments.Allows("payments-api") {
		t.Fatalf("expected allowlist to reject non-exact deployment")
	}
}

func TestFromHeadersPrefersExplicitTenetHeadersOverAliases(t *testing.T) {
	header := http.Header{}
	header.Set(HeaderID, "operator-123")
	header.Set(HeaderName, "Jane Operator")
	header.Set(HeaderEmail, "jane@example.com")
	header.Set(HeaderAliasID, "alias-456")
	header.Set(HeaderAliasName, "Alias Operator")
	header.Set(HeaderAliasEmail, "alias@example.com")

	actor := FromHeaders(header)
	if actor.ID != "operator-123" || actor.Name != "Jane Operator" || actor.Email != "jane@example.com" {
		t.Fatalf("expected explicit Tenet headers to win, got %+v", actor)
	}
}

func TestWithContextAndFromContextRoundTripActor(t *testing.T) {
	ctx := WithContext(context.Background(), Actor{
		Available: true,
		ID:        "operator-123",
		Name:      "Jane Operator",
	})

	actor := FromContext(ctx)
	if !actor.Available || actor.ID != "operator-123" || actor.Name != "Jane Operator" {
		t.Fatalf("unexpected context actor: %+v", actor)
	}
}

func TestFromContextProjectsAuthenticatedIdentityWhenExplicitActorMissing(t *testing.T) {
	ctx := WithContext(context.Background(), Actor{})
	ctx = WithAuthenticatedIdentityContext(ctx, AuthenticatedIdentity{
		Authenticated: true,
		ID:            "front-door-123",
		Name:          "Front Door Operator",
		Email:         "frontdoor@example.com",
	})

	actor := FromContext(ctx)
	if !actor.Available || actor.ID != "front-door-123" || actor.Name != "Front Door Operator" || actor.Email != "frontdoor@example.com" {
		t.Fatalf("expected projected actor from authenticated identity, got %+v", actor)
	}
}

func TestFromContextPrefersExplicitActorOverAuthenticatedIdentity(t *testing.T) {
	ctx := WithContext(context.Background(), Actor{
		Available: true,
		ID:        "operator-123",
		Name:      "Jane Operator",
		Email:     "jane@example.com",
	})
	ctx = WithAuthenticatedIdentityContext(ctx, AuthenticatedIdentity{
		Authenticated: true,
		ID:            "front-door-123",
		Name:          "Front Door Operator",
		Email:         "frontdoor@example.com",
	})

	actor := FromContext(ctx)
	if !actor.Available || actor.ID != "operator-123" || actor.Name != "Jane Operator" || actor.Email != "jane@example.com" {
		t.Fatalf("expected explicit actor to override authenticated identity, got %+v", actor)
	}
}

func TestWithAuthorizedDeploymentsContextRoundTrip(t *testing.T) {
	ctx := WithAuthorizedDeploymentsContext(context.Background(), AuthorizedDeployments{
		Names: []string{"payments-api-prod", " worker-prod ", "payments-api-prod"},
	})

	deployments := AuthorizedDeploymentsFromContext(ctx)
	want := []string{"payments-api-prod", "worker-prod"}
	if !reflect.DeepEqual(deployments.Names, want) {
		t.Fatalf("unexpected context deployment allowlist: got %#v want %#v", deployments.Names, want)
	}
}

func TestWithAuthorizedProjectEnvironmentScopesContextRoundTrip(t *testing.T) {
	ctx := WithAuthorizedProjectEnvironmentScopesContext(context.Background(), AuthorizedProjectEnvironmentScopes{
		Projects:     []string{"payments", " platform ", "payments"},
		Environments: []string{"prod", " staging ", "prod"},
	})

	scopes := AuthorizedProjectEnvironmentScopesFromContext(ctx)
	wantProjects := []string{"payments", "platform"}
	wantEnvironments := []string{"prod", "staging"}
	if !reflect.DeepEqual(scopes.Projects, wantProjects) {
		t.Fatalf("unexpected context project scopes: got %#v want %#v", scopes.Projects, wantProjects)
	}
	if !reflect.DeepEqual(scopes.Environments, wantEnvironments) {
		t.Fatalf("unexpected context environment scopes: got %#v want %#v", scopes.Environments, wantEnvironments)
	}
}

func TestWithAuthenticatedRolesContextRoundTrip(t *testing.T) {
	ctx := WithAuthenticatedRolesContext(context.Background(), AuthenticatedRoles{
		Names: []string{"operator", " admin ", "operator"},
	})

	roles := AuthenticatedRolesFromContext(ctx)
	want := []string{"operator", "admin"}
	if !reflect.DeepEqual(roles.Names, want) {
		t.Fatalf("unexpected context roles: got %#v want %#v", roles.Names, want)
	}
}
