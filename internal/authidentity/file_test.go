package authidentity

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/authprovider"
)

func TestFileStoreUpsertCreatesAndReplacesIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "identities.json")
	store := NewFileStore(path)
	ctx := context.Background()

	first := time.Date(2026, 5, 2, 1, 2, 3, 0, time.UTC)
	second := first.Add(5 * time.Minute)
	setAuthIdentityNow(t, first)
	created, err := store.Upsert(ctx, FromProviderIdentity(authprovider.Identity{
		Provider: " GitHub ",
		ID:       " 12345 ",
		Login:    " octo ",
		Name:     " Octo Cat ",
		Email:    " octo@example.com ",
		URL:      " https://github.com/octo ",
	}))
	if err != nil {
		t.Fatalf("upsert identity: %v", err)
	}
	if created.Provider != "github" || created.Subject != "12345" || created.Login != "octo" {
		t.Fatalf("identity was not normalized: %#v", created)
	}
	if !created.CreatedAt.Equal(first) || !created.UpdatedAt.Equal(first) {
		t.Fatalf("create timestamps mismatch: %#v", created)
	}

	setAuthIdentityNow(t, second)
	updated, err := store.Upsert(ctx, Identity{Provider: "github", Subject: "12345", Login: "octo-renamed"})
	if err != nil {
		t.Fatalf("replace identity: %v", err)
	}
	if updated.Login != "octo-renamed" {
		t.Fatalf("identity was not replaced: %#v", updated)
	}
	if !updated.CreatedAt.Equal(first) || !updated.UpdatedAt.Equal(second) {
		t.Fatalf("update timestamps mismatch: %#v", updated)
	}

	loaded, err := store.Get(ctx, "GitHub", "12345")
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if loaded.Login != "octo-renamed" || !loaded.CreatedAt.Equal(first) || !loaded.UpdatedAt.Equal(second) {
		t.Fatalf("loaded identity mismatch: %#v", loaded)
	}
}

func TestFileStoreListMissingFileAndDefensiveCopy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "identities.json")
	store := NewFileStore(path)
	ctx := context.Background()

	identities, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list missing file: %v", err)
	}
	if len(identities) != 0 {
		t.Fatalf("expected empty missing-file store, got %#v", identities)
	}

	setAuthIdentityNow(t, time.Date(2026, 5, 2, 2, 0, 0, 0, time.UTC))
	if _, err := store.Upsert(ctx, Identity{Provider: "github", Subject: "2", Login: "b"}); err != nil {
		t.Fatalf("upsert second identity: %v", err)
	}
	if _, err := store.Upsert(ctx, Identity{Provider: "github", Subject: "1", Login: "a"}); err != nil {
		t.Fatalf("upsert first identity: %v", err)
	}
	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list identities: %v", err)
	}
	if got := []string{listed[0].Subject, listed[1].Subject}; got[0] != "1" || got[1] != "2" {
		t.Fatalf("identities were not sorted by stable key: %#v", got)
	}
	listed[0].Login = "mutated"
	reloaded, err := store.Get(ctx, "github", "1")
	if err != nil {
		t.Fatalf("get after caller mutation: %v", err)
	}
	if reloaded.Login != "a" {
		t.Fatalf("caller mutation leaked into store: %#v", reloaded)
	}
}

func TestFileStoreValidationAndNotFound(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "auth", "identities.json"))
	ctx := context.Background()

	if _, err := store.Upsert(ctx, Identity{Subject: "123"}); err == nil || !strings.Contains(err.Error(), "provider is required") {
		t.Fatalf("expected provider validation error, got %v", err)
	}
	if _, err := store.Upsert(ctx, Identity{Provider: "github"}); err == nil || !strings.Contains(err.Error(), "subject is required") {
		t.Fatalf("expected subject validation error, got %v", err)
	}
	if _, err := store.Get(ctx, "github", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if _, err := NewFileStore("").List(ctx); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("expected path validation error, got %v", err)
	}
}

func TestSaveAndLoadUseAuthIdentitiesJSONShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "identities.json")
	timestamp := time.Date(2026, 5, 2, 3, 0, 0, 0, time.UTC)
	if err := Save(path, []Identity{{Provider: "github", Subject: "42", Login: "mona", CreatedAt: timestamp, UpdatedAt: timestamp}}); err != nil {
		t.Fatalf("save identities: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read identity file: %v", err)
	}
	var raw []map[string]any
	if err := json.Unmarshal(content, &raw); err != nil {
		t.Fatalf("decode identity file: %v", err)
	}
	if raw[0]["provider"] != "github" || raw[0]["subject"] != "42" || raw[0]["login"] != "mona" {
		t.Fatalf("unexpected auth identity JSON shape: %s", string(content))
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load identities: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Subject != "42" {
		t.Fatalf("loaded identities mismatch: %#v", loaded)
	}
}

func setAuthIdentityNow(t *testing.T, now time.Time) {
	t.Helper()
	previous := authIdentityNow
	authIdentityNow = func() time.Time { return now }
	t.Cleanup(func() { authIdentityNow = previous })
}
