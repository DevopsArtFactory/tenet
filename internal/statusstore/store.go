package statusstore

import (
	"context"
	"fmt"
)

// Store is a pluggable deployment-status backend.
type Store interface {
	List(context.Context) ([]Snapshot, error)
	Get(context.Context, string) (Snapshot, error)
	Save(context.Context, []Snapshot) error
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
	return Load(s.Path)
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
