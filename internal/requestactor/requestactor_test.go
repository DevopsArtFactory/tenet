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
