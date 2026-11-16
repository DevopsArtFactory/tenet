package authstate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileStorePrunesExpiredStateIncludingConsumed(t *testing.T) {
	now := time.Now().UTC()
	setAuthStateNow(t, now)
	store := NewFileStore(filepath.Join(t.TempDir(), "states.json"))
	old := now.Add(-time.Hour)
	states := []State{
		{Token: "expired", Provider: "google", CreatedAt: old, ExpiresAt: now},
		{Token: "consumed-expired", Provider: "google", CreatedAt: old, ExpiresAt: now, ConsumedAt: &old},
		{Token: "consumed-active", Provider: "google", CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute), ConsumedAt: &now},
		{Token: "active", Provider: "google", CreatedAt: now, ExpiresAt: now.Add(time.Minute)},
	}
	if err := Save(store.Path, states); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), State{Token: "new", Provider: "google", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	got, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("retained %d states, want only 3 unexpired records", len(got))
	}
	if _, err := store.Consume(context.Background(), "consumed-active"); err == nil {
		t.Fatal("reused consumed state")
	}
}

func TestFileStoreLoginRateLimitAndRecovery(t *testing.T) {
	now := time.Now().UTC()
	setAuthStateNow(t, now)
	store := NewFileStore(filepath.Join(t.TempDir(), "states.json"))
	states := make([]State, 120)
	for i := range states {
		states[i] = State{Token: fmt.Sprintf("state-%d", i), Provider: "google", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
	}
	if err := Save(store.Path, states); err != nil {
		t.Fatal(err)
	}
	// Successful callbacks must not let a caller reset the initiation quota.
	if _, err := store.Consume(context.Background(), states[0].Token); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	next := State{Token: "next", Provider: "google", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
	if _, err := store.Save(context.Background(), next); !errors.Is(err, ErrRateLimited) {
		t.Fatal("accepted login beyond per-minute quota")
	}
	after, err := os.ReadFile(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("rate-limited login rewrote state file")
	}
	now = now.Add(time.Minute)
	setAuthStateNow(t, now)
	next.CreatedAt = now
	if _, err := store.Save(context.Background(), next); err != nil {
		t.Fatalf("quota did not recover: %v", err)
	}
}

func TestConcurrentFileStoresEnforceCapacity(t *testing.T) {
	now := time.Now().UTC()
	setAuthStateNow(t, now)
	path := filepath.Join(t.TempDir(), "states.json")
	states := make([]State, 1023)
	for i := range states {
		states[i] = State{Token: fmt.Sprintf("state-%d", i), Provider: "github", CreatedAt: now.Add(-2 * time.Minute), ExpiresAt: now.Add(time.Minute)}
	}
	if err := Save(path, states); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	var accepted atomic.Int32
	for i := range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := NewFileStore(path).Save(context.Background(), State{Token: fmt.Sprintf("new-%d", i), Provider: "github", CreatedAt: now, ExpiresAt: now.Add(time.Minute)})
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrCapacity) {
				t.Errorf("unexpected save error: %v", err)
			}
		}()
	}
	workers.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted %d new states with one free slot", accepted.Load())
	}
	got, err := NewFileStore(path).List(context.Background())
	if err != nil || len(got) != 1024 {
		t.Fatalf("stored=%d err=%v", len(got), err)
	}
	now = now.Add(time.Minute)
	setAuthStateNow(t, now)
	if _, err := NewFileStore(path).Save(context.Background(), State{Token: "after-expiry", Provider: "github", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatalf("capacity did not recover: %v", err)
	}
	got, err = NewFileStore(path).List(context.Background())
	if err != nil || len(got) != 1 {
		t.Fatalf("expired records remain: count=%d err=%v", len(got), err)
	}
}

func TestFileStorePrunesExpiredRecordsWhenAdmissionIsDenied(t *testing.T) {
	now := time.Now().UTC()
	setAuthStateNow(t, now)
	store := NewFileStore(filepath.Join(t.TempDir(), "states.json"))
	states := make([]State, maxLoginsPerMinute)
	for i := range states {
		states[i] = State{Token: fmt.Sprint(i), Provider: "google", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	}
	states = append(states, State{Token: "expired", Provider: "google", CreatedAt: now.Add(-time.Hour), ExpiresAt: now})
	if err := Save(store.Path, states); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(t.Context(), State{Token: "rejected", Provider: "google", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected rate limit: %v", err)
	}
	stored, err := store.List(t.Context())
	if err != nil || len(stored) != maxLoginsPerMinute {
		t.Fatalf("cleanup failed: stored=%d err=%v", len(stored), err)
	}
	if _, err := store.Get(t.Context(), "rejected"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rejected record persisted: %v", err)
	}
}
