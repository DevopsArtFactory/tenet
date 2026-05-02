package authstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var authStateNow = time.Now

// FileStore persists OAuth callback states in a local JSON file. It is a
// development/test-friendly bridge toward future browser OAuth callback wiring,
// not a runtime session store.
type FileStore struct {
	Path string
}

// NewFileStore returns a filesystem-backed OAuth state store.
func NewFileStore(path string) FileStore {
	return FileStore{Path: path}
}

// List loads all callback states from disk. A missing file is treated as an
// empty store so local bootstrap can create it on the first Save.
func (s FileStore) List(context.Context) ([]State, error) {
	if s.Path == "" {
		return nil, fmt.Errorf("file auth state store path is required")
	}
	states, err := Load(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return states, nil
}

// Get loads one callback state by token.
func (s FileStore) Get(ctx context.Context, token string) (State, error) {
	states, err := s.List(ctx)
	if err != nil {
		return State{}, err
	}
	state, _, err := findState(states, token)
	return state, err
}

// Save creates or replaces one callback state by token.
func (s FileStore) Save(ctx context.Context, state State) (State, error) {
	if s.Path == "" {
		return State{}, fmt.Errorf("file auth state store path is required")
	}
	state, err := normalizeState(state)
	if err != nil {
		return State{}, err
	}
	states, err := s.List(ctx)
	if err != nil {
		return State{}, err
	}
	if _, idx, err := findState(states, state.Token); err == nil {
		states[idx] = state
	} else if errors.Is(err, ErrNotFound) {
		states = append(states, state)
	} else {
		return State{}, err
	}
	if err := Save(s.Path, states); err != nil {
		return State{}, err
	}
	return state, nil
}

// Consume marks a callback state as used and returns it. Consumed and expired
// states are preserved on disk so operators can diagnose callback failures.
func (s FileStore) Consume(ctx context.Context, token string) (State, error) {
	if s.Path == "" {
		return State{}, fmt.Errorf("file auth state store path is required")
	}
	states, err := s.List(ctx)
	if err != nil {
		return State{}, err
	}
	state, idx, err := findState(states, token)
	if err != nil {
		return State{}, err
	}
	if state.ConsumedAt != nil {
		return State{}, fmt.Errorf("%w: %s", ErrConsumed, state.Token)
	}
	now := authStateNow().UTC()
	if state.Expired(now) {
		return State{}, fmt.Errorf("%w: %s", ErrExpired, state.Token)
	}
	state.ConsumedAt = &now
	states[idx] = state
	if err := Save(s.Path, states); err != nil {
		return State{}, err
	}
	return state, nil
}

// Load reads OAuth callback states from a JSON file.
func Load(path string) ([]State, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read auth state store: %w", err)
	}
	var states []State
	if err := json.Unmarshal(content, &states); err != nil {
		return nil, fmt.Errorf("decode auth state store: %w", err)
	}
	for idx, state := range states {
		normalized, err := normalizeState(state)
		if err != nil {
			return nil, fmt.Errorf("decode auth state store state %d: %w", idx, err)
		}
		states[idx] = normalized
	}
	return sortStates(states), nil
}

// Save writes OAuth callback states to a JSON file in deterministic token order.
func Save(path string, states []State) error {
	normalized := make([]State, 0, len(states))
	for idx, state := range states {
		record, err := normalizeState(state)
		if err != nil {
			return fmt.Errorf("encode auth state store state %d: %w", idx, err)
		}
		normalized = append(normalized, record)
	}
	content, err := json.MarshalIndent(sortStates(normalized), "", "  ")
	if err != nil {
		return fmt.Errorf("encode auth state store: %w", err)
	}
	content = append(content, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create auth state store dir: %w", err)
	}
	fileMode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		fileMode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat auth state store: %w", err)
	}
	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp auth state store: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if err := tempFile.Chmod(fileMode); err != nil {
		tempFile.Close()
		return fmt.Errorf("chmod temp auth state store: %w", err)
	}
	if _, err := tempFile.Write(content); err != nil {
		tempFile.Close()
		return fmt.Errorf("write temp auth state store: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp auth state store: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename auth state store: %w", err)
	}
	return nil
}
