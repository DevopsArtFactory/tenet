package applyrequest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

var (
	createTempFile = os.CreateTemp
	renameFile     = os.Rename
)

// FileStore persists immutable apply requests in a local JSON file.
type FileStore struct {
	Path string
}

// NewFileStore returns a filesystem-backed apply-request store.
func NewFileStore(path string) FileStore {
	return FileStore{Path: path}
}

// Load reads persisted apply requests from disk.
func Load(path string) ([]Record, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read apply request store: %w", err)
	}

	var records []Record
	if err := json.Unmarshal(content, &records); err != nil {
		return nil, fmt.Errorf("decode apply request store: %w", err)
	}
	for i := range records {
		records[i].Actor = requestactor.Normalize(records[i].Actor)
		for j := range records[i].Transitions {
			records[i].Transitions[j].Actor = requestactor.Normalize(records[i].Transitions[j].Actor)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].ID < records[j].ID
	})
	return records, nil
}

// Save writes the full immutable apply-request set atomically.
func Save(path string, records []Record) error {
	unlock, err := lockStore(context.Background(), path)
	if err != nil {
		return err
	}
	defer unlock()
	return saveUnlocked(path, records)
}

func saveUnlocked(path string, records []Record) error {
	sorted := list(records)
	content, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return fmt.Errorf("encode apply request store: %w", err)
	}
	content = append(content, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create apply request store dir: %w", err)
	}
	fileMode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		fileMode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat apply request store: %w", err)
	}

	tempFile, err := createTempFile(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp apply request store: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		if tempFile != nil {
			_ = tempFile.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := tempFile.Chmod(fileMode); err != nil {
		return fmt.Errorf("chmod temp apply request store: %w", err)
	}
	if _, err := tempFile.Write(content); err != nil {
		return fmt.Errorf("write temp apply request store: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp apply request store: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp apply request store: %w", err)
	}
	tempFile = nil
	if err := renameFile(tempPath, path); err != nil {
		return fmt.Errorf("rename apply request store: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open apply request store directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync apply request store directory: %w", err)
	}
	return nil
}

// Get loads one apply request by ID.
func (s FileStore) Get(_ context.Context, id string) (Record, error) {
	if s.Path == "" {
		return Record{}, fmt.Errorf("file apply request store path is required")
	}
	records, err := Load(s.Path)
	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
			return Record{}, ErrNotFound
		}
		return Record{}, err
	}
	return get(records, id)
}

// List returns all persisted apply requests.
func (s FileStore) List(_ context.Context) ([]Record, error) {
	if s.Path == "" {
		return nil, fmt.Errorf("file apply request store path is required")
	}
	records, err := Load(s.Path)
	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return records, nil
}

// Save persists one immutable apply request by ID.
func (s FileStore) Save(ctx context.Context, record Record) error {
	unlock, err := lockStore(ctx, s.Path)
	if err != nil {
		return err
	}
	defer unlock()
	if s.Path == "" {
		return fmt.Errorf("file apply request store path is required")
	}
	records, err := Load(s.Path)
	if err != nil {
		var pathErr *os.PathError
		if !(errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist)) {
			return err
		}
	}
	for _, existing := range records {
		if existing.ID == record.ID {
			return nil
		}
	}
	return saveUnlocked(s.Path, append(records, record))
}

// Update replaces one persisted apply request by ID.
func (s FileStore) Update(ctx context.Context, record Record) error {
	_, err := s.Mutate(ctx, record.ID, func(Record) (Record, error) { return record, nil })
	return err
}

// Mutate loads, checks and persists one record under a process-shared store lock.
// Callbacks must only transform local state; do not perform network calls here.
func (s FileStore) Mutate(ctx context.Context, id string, change func(Record) (Record, error)) (Record, error) {
	unlock, err := lockStore(ctx, s.Path)
	if err != nil {
		return Record{}, err
	}
	defer unlock()
	records, err := s.List(ctx)
	if err != nil {
		return Record{}, err
	}
	for i, existing := range records {
		if existing.ID != id {
			continue
		}
		updated, err := change(existing)
		if err != nil {
			return Record{}, err
		}
		if updated.ID != existing.ID {
			return Record{}, fmt.Errorf("apply mutation cannot change record identity")
		}
		records[i] = updated
		if err := saveUnlocked(s.Path, records); err != nil {
			return Record{}, err
		}
		return updated, nil
	}
	return Record{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

// Claim atomically persists a validated claim using the same lock as all writers.
func (s FileStore) Claim(ctx context.Context, id string, _ ClaimRequest, claim func(Record) (Record, error)) (Record, error) {
	return s.Mutate(ctx, id, claim)
}

// Heartbeat atomically persists a validated refresh using the same lock as all writers.
func (s FileStore) Heartbeat(ctx context.Context, id string, _ HeartbeatRequest, heartbeat func(Record) (Record, error)) (Record, error) {
	return s.Mutate(ctx, id, heartbeat)
}
