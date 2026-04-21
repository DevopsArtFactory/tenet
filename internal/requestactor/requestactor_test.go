package requestactor

import (
	"context"
	"net/http"
	"reflect"
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

func TestAuthenticatedRolesAllowsAnyRole(t *testing.T) {
	roles := AuthenticatedRoles{Names: []string{"viewer", "operator"}}
	if !roles.AllowsAny("admin", "operator") {
		t.Fatalf("expected roles to allow operator-capable access")
	}
	if roles.AllowsAny("admin") {
		t.Fatalf("expected roles to reject missing role")
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
