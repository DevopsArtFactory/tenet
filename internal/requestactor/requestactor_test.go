package requestactor

import (
	"context"
	"net/http"
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
