package authidentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var authIdentityNow = time.Now

// FileStore persists normalized auth identities in a local JSON file. It is a
// development/test-friendly bridge toward a future durable auth_identities
// table, not a runtime auth/session backend.
type FileStore struct {
	Path string
}

// NewFileStore returns a filesystem-backed identity store.
func NewFileStore(path string) FileStore {
	return FileStore{Path: path}
}

// List loads all identities from the JSON file. A missing file is treated as an
// empty store so local bootstrap can create it on the first Upsert.
func (s FileStore) List(context.Context) ([]Identity, error) {
	if s.Path == "" {
		return nil, fmt.Errorf("file auth identity store path is required")
	}
	identities, err := Load(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return identities, nil
}

// Get loads one identity by provider and subject.
func (s FileStore) Get(ctx context.Context, provider, subject string) (Identity, error) {
	identities, err := s.List(ctx)
	if err != nil {
		return Identity{}, err
	}
	identity, _, err := findIdentity(identities, provider, subject)
	return identity, err
}

// Upsert creates or replaces one identity while preserving CreatedAt for an
// existing provider+subject and always refreshing UpdatedAt.
func (s FileStore) Upsert(ctx context.Context, identity Identity) (Identity, error) {
	if s.Path == "" {
		return Identity{}, fmt.Errorf("file auth identity store path is required")
	}
	identity, err := normalizeIdentity(identity)
	if err != nil {
		return Identity{}, err
	}
	identities, err := s.List(ctx)
	if err != nil {
		return Identity{}, err
	}
	now := authIdentityNow().UTC()
	if existing, idx, err := findIdentity(identities, identity.Provider, identity.Subject); err == nil {
		identity.CreatedAt = existing.CreatedAt
		identity.UpdatedAt = now
		identities[idx] = identity
	} else if errors.Is(err, ErrNotFound) {
		identity.CreatedAt = now
		identity.UpdatedAt = now
		identities = append(identities, identity)
	} else {
		return Identity{}, err
	}
	if err := Save(s.Path, identities); err != nil {
		return Identity{}, err
	}
	return identity, nil
}

// Load reads auth identities from a JSON file.
func Load(path string) ([]Identity, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read auth identity store: %w", err)
	}
	var identities []Identity
	if err := json.Unmarshal(content, &identities); err != nil {
		return nil, fmt.Errorf("decode auth identity store: %w", err)
	}
	for idx, identity := range identities {
		normalized, err := normalizeIdentity(identity)
		if err != nil {
			return nil, fmt.Errorf("decode auth identity store identity %d: %w", idx, err)
		}
		identities[idx] = normalized
	}
	return sortIdentities(identities), nil
}

// Save writes auth identities to a JSON file in deterministic key order.
func Save(path string, identities []Identity) error {
	normalized := make([]Identity, 0, len(identities))
	for idx, identity := range identities {
		record, err := normalizeIdentity(identity)
		if err != nil {
			return fmt.Errorf("encode auth identity store identity %d: %w", idx, err)
		}
		normalized = append(normalized, record)
	}
	content, err := json.MarshalIndent(sortIdentities(normalized), "", "  ")
	if err != nil {
		return fmt.Errorf("encode auth identity store: %w", err)
	}
	content = append(content, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create auth identity store dir: %w", err)
	}
	fileMode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		fileMode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat auth identity store: %w", err)
	}
	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp auth identity store: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if err := tempFile.Chmod(fileMode); err != nil {
		tempFile.Close()
		return fmt.Errorf("chmod temp auth identity store: %w", err)
	}
	if _, err := tempFile.Write(content); err != nil {
		tempFile.Close()
		return fmt.Errorf("write temp auth identity store: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp auth identity store: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename auth identity store: %w", err)
	}
	return nil
}
