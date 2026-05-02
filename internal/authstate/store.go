package authstate

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// ErrNotFound reports that no OAuth callback state exists for the token.
var ErrNotFound = errors.New("auth state not found")

// ErrExpired reports that the OAuth callback state exists but is no longer valid.
var ErrExpired = errors.New("auth state expired")

// ErrConsumed reports that the OAuth callback state has already been consumed.
var ErrConsumed = errors.New("auth state already consumed")

// State is Tenet's normalized OAuth callback state record. It is intentionally
// small groundwork for future browser/OAuth flows; it is not a Tenet-managed
// browser session and is not wired into trusted-front-door authorization.
type State struct {
	Token      string     `json:"token"`
	Provider   string     `json:"provider"`
	ReturnURL  string     `json:"returnUrl,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	ConsumedAt *time.Time `json:"consumedAt,omitempty"`
}

// Store persists OAuth callback state tokens and supports one-time consumption.
type Store interface {
	List(context.Context) ([]State, error)
	Get(context.Context, string) (State, error)
	Save(context.Context, State) (State, error)
	Consume(context.Context, string) (State, error)
}

// Expired reports whether the state has passed its expiration boundary.
func (s State) Expired(now time.Time) bool {
	if s.ExpiresAt.IsZero() {
		return false
	}
	return !now.UTC().Before(s.ExpiresAt.UTC())
}

func normalizeState(state State) (State, error) {
	state.Token = strings.TrimSpace(state.Token)
	if state.Token == "" {
		return State{}, fmt.Errorf("auth state token is required")
	}
	state.Provider = strings.ToLower(strings.TrimSpace(state.Provider))
	if state.Provider == "" {
		return State{}, fmt.Errorf("auth state provider is required")
	}
	state.ReturnURL = strings.TrimSpace(state.ReturnURL)
	if state.ReturnURL != "" {
		parsed, err := url.Parse(state.ReturnURL)
		if err != nil {
			return State{}, fmt.Errorf("auth state return URL is invalid: %w", err)
		}
		if parsed.IsAbs() && (parsed.Scheme == "" || parsed.Host == "") {
			return State{}, fmt.Errorf("auth state return URL is invalid")
		}
	}
	if state.CreatedAt.IsZero() {
		return State{}, fmt.Errorf("auth state createdAt is required")
	}
	if state.ExpiresAt.IsZero() {
		return State{}, fmt.Errorf("auth state expiresAt is required")
	}
	if !state.ExpiresAt.After(state.CreatedAt) {
		return State{}, fmt.Errorf("auth state expiresAt must be after createdAt")
	}
	if state.ConsumedAt != nil {
		consumedAt := state.ConsumedAt.UTC()
		state.ConsumedAt = &consumedAt
	}
	state.CreatedAt = state.CreatedAt.UTC()
	state.ExpiresAt = state.ExpiresAt.UTC()
	return state, nil
}

func sortStates(states []State) []State {
	sorted := append([]State(nil), states...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Token < sorted[j].Token
	})
	return sorted
}

func findState(states []State, token string) (State, int, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return State{}, -1, fmt.Errorf("auth state token is required")
	}
	for idx, state := range states {
		normalized, err := normalizeState(state)
		if err != nil {
			continue
		}
		if normalized.Token == token {
			return normalized, idx, nil
		}
	}
	return State{}, -1, fmt.Errorf("%w: %s", ErrNotFound, token)
}
