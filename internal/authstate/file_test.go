package authstate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileStoreSaveGetAndList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "states.json")
	store := NewFileStore(path)
	ctx := context.Background()
	created := time.Date(2026, 5, 3, 1, 2, 3, 0, time.UTC)
	setAuthStateNow(t, created)

	states, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list missing file: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("expected missing file to be an empty store, got %#v", states)
	}

	first, err := store.Save(ctx, State{Token: " state-b ", Provider: " GitHub ", ReturnURL: " /after-login ", CreatedAt: created, ExpiresAt: created.Add(5 * time.Minute)})
	if err != nil {
		t.Fatalf("save first state: %v", err)
	}
	if first.Token != "state-b" || first.Provider != "github" || first.ReturnURL != "/after-login" {
		t.Fatalf("state was not normalized: %#v", first)
	}
	if _, err := store.Save(ctx, State{Token: "state-a", Provider: "github", CreatedAt: created, ExpiresAt: created.Add(10 * time.Minute)}); err != nil {
		t.Fatalf("save second state: %v", err)
	}

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list states: %v", err)
	}
	if got := []string{listed[0].Token, listed[1].Token}; got[0] != "state-a" || got[1] != "state-b" {
		t.Fatalf("states were not sorted by token: %#v", got)
	}
	listed[0].Provider = "mutated"
	loaded, err := store.Get(ctx, "state-a")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if loaded.Provider != "github" {
		t.Fatalf("caller mutation leaked into store: %#v", loaded)
	}
}

func TestFileStoreConsumeIsOneTimeAndPersistsEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "states.json")
	store := NewFileStore(path)
	ctx := context.Background()
	created := time.Date(2026, 5, 3, 2, 0, 0, 0, time.UTC)
	consumedAt := created.Add(time.Minute)
	setAuthStateNow(t, consumedAt)

	if _, err := store.Save(ctx, State{Token: "state-1", Provider: "github", CreatedAt: created, ExpiresAt: created.Add(5 * time.Minute)}); err != nil {
		t.Fatalf("save state: %v", err)
	}
	consumed, err := store.Consume(ctx, "state-1")
	if err != nil {
		t.Fatalf("consume state: %v", err)
	}
	if consumed.ConsumedAt == nil || !consumed.ConsumedAt.Equal(consumedAt) {
		t.Fatalf("consume timestamp mismatch: %#v", consumed)
	}
	if _, err := store.Consume(ctx, "state-1"); !errors.Is(err, ErrConsumed) {
		t.Fatalf("expected ErrConsumed on second consume, got %v", err)
	}
	reloaded, err := store.Get(ctx, "state-1")
	if err != nil {
		t.Fatalf("reload consumed state: %v", err)
	}
	if reloaded.ConsumedAt == nil || !reloaded.ConsumedAt.Equal(consumedAt) {
		t.Fatalf("consumed evidence was not persisted: %#v", reloaded)
	}
}

func TestFileStoreConsumeExpiredAndNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "states.json")
	store := NewFileStore(path)
	ctx := context.Background()
	created := time.Date(2026, 5, 3, 3, 0, 0, 0, time.UTC)
	setAuthStateNow(t, created.Add(10*time.Minute))

	if err := Save(path, []State{{Token: "expired", Provider: "github", CreatedAt: created, ExpiresAt: created.Add(5 * time.Minute)}}); err != nil {
		t.Fatalf("save expired state: %v", err)
	}
	if _, err := store.Consume(ctx, "expired"); !errors.Is(err, ErrExpired) {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
	if _, err := store.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFileStoreValidation(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "auth", "states.json"))
	ctx := context.Background()
	created := time.Date(2026, 5, 3, 4, 0, 0, 0, time.UTC)

	cases := []struct {
		name  string
		state State
		want  string
	}{
		{name: "token", state: State{Provider: "github", CreatedAt: created, ExpiresAt: created.Add(time.Minute)}, want: "token is required"},
		{name: "provider", state: State{Token: "state", CreatedAt: created, ExpiresAt: created.Add(time.Minute)}, want: "provider is required"},
		{name: "created", state: State{Token: "state", Provider: "github", ExpiresAt: created.Add(time.Minute)}, want: "createdAt is required"},
		{name: "expires", state: State{Token: "state", Provider: "github", CreatedAt: created}, want: "expiresAt is required"},
		{name: "ordering", state: State{Token: "state", Provider: "github", CreatedAt: created, ExpiresAt: created}, want: "expiresAt must be after createdAt"},
		{name: "return URL", state: State{Token: "state", Provider: "github", ReturnURL: "http://%zz", CreatedAt: created, ExpiresAt: created.Add(time.Minute)}, want: "return URL is invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := store.Save(ctx, tc.state); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q validation error, got %v", tc.want, err)
			}
		})
	}
	if _, err := NewFileStore("").List(ctx); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("expected path validation error, got %v", err)
	}
}

func TestSaveAndLoadUseAuthStatesJSONShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "states.json")
	created := time.Date(2026, 5, 3, 5, 0, 0, 0, time.UTC)
	if err := Save(path, []State{{Token: "state-42", Provider: "github", ReturnURL: "/callback", CreatedAt: created, ExpiresAt: created.Add(time.Minute)}}); err != nil {
		t.Fatalf("save states: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	var raw []map[string]any
	if err := json.Unmarshal(content, &raw); err != nil {
		t.Fatalf("decode state file: %v", err)
	}
	if raw[0]["token"] != "state-42" || raw[0]["provider"] != "github" || raw[0]["returnUrl"] != "/callback" {
		t.Fatalf("unexpected auth state JSON shape: %s", string(content))
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load states: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Token != "state-42" {
		t.Fatalf("loaded states mismatch: %#v", loaded)
	}
}

func setAuthStateNow(t *testing.T, now time.Time) {
	t.Helper()
	previous := authStateNow
	authStateNow = func() time.Time { return now }
	t.Cleanup(func() { authStateNow = previous })
}
