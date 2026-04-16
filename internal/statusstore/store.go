package statusstore

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// Store is a pluggable deployment-status backend.
type Store interface {
	List(context.Context) ([]Snapshot, error)
	Get(context.Context, string) (Snapshot, error)
	Save(context.Context, []Snapshot) error
}

// Writer adds controller-owned per-deployment mutations on top of Store.
type Writer interface {
	Store
	Upsert(context.Context, Snapshot) error
	Delete(context.Context, string) error
}

// BatchWriter applies one reconcile batch without exposing partial store updates.
type BatchWriter interface {
	Store
	Apply(context.Context, []Snapshot, []string) error
}

// FileStore persists snapshots in a local JSON file.
type FileStore struct {
	Path string
}

// NewFileStore returns a filesystem-backed status store.
func NewFileStore(path string) FileStore {
	return FileStore{Path: path}
}

// List loads all snapshots from the JSON file.
func (s FileStore) List(context.Context) ([]Snapshot, error) {
	if s.Path == "" {
		return nil, fmt.Errorf("file status store path is required")
	}
	snapshots, err := Load(s.Path)
	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return snapshots, nil
}

// Get loads one snapshot by deployment name.
func (s FileStore) Get(ctx context.Context, deploymentName string) (Snapshot, error) {
	snapshots, err := s.List(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	return Get(snapshots, deploymentName)
}

// Save writes the full snapshot set to the JSON file.
func (s FileStore) Save(_ context.Context, snapshots []Snapshot) error {
	if s.Path == "" {
		return fmt.Errorf("file status store path is required")
	}
	return Save(s.Path, snapshots)
}

// Upsert writes one snapshot into the JSON file, creating the file if needed.
func (s FileStore) Upsert(ctx context.Context, snapshot Snapshot) error {
	snapshots, err := s.List(ctx)
	if err != nil {
		return err
	}
	return s.Save(ctx, upsertSnapshots(snapshots, snapshot))
}

// Delete removes one snapshot from the JSON file.
func (s FileStore) Delete(ctx context.Context, deploymentName string) error {
	if s.Path == "" {
		return fmt.Errorf("file status store path is required")
	}
	if _, err := os.Stat(s.Path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat status store: %w", err)
	}
	snapshots, err := s.List(ctx)
	if err != nil {
		return err
	}
	return s.Save(ctx, deleteSnapshot(snapshots, deploymentName))
}

// Apply updates and deletes snapshots in one filesystem save.
func (s FileStore) Apply(ctx context.Context, upserts []Snapshot, deletes []string) error {
	snapshots, err := s.List(ctx)
	if err != nil {
		return err
	}
	for _, snapshot := range upserts {
		snapshots = upsertSnapshots(snapshots, snapshot)
	}
	for _, deploymentName := range deletes {
		snapshots = deleteSnapshot(snapshots, deploymentName)
	}
	if len(snapshots) == 0 {
		if s.Path == "" {
			return fmt.Errorf("file status store path is required")
		}
		if _, err := os.Stat(s.Path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("stat status store: %w", err)
		}
	}
	return s.Save(ctx, snapshots)
}

// SeedFromFileIfEmpty loads snapshots from seedPath into the store when the store is empty.
func SeedFromFileIfEmpty(ctx context.Context, store Store, seedPath string) error {
	if seedPath == "" {
		return nil
	}
	snapshots, err := store.List(ctx)
	if err != nil {
		return err
	}
	if len(snapshots) > 0 {
		return nil
	}
	seedSnapshots, err := Load(seedPath)
	if err != nil {
		return err
	}
	return store.Save(ctx, seedSnapshots)
}
