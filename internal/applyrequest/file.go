package applyrequest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

var (
	createTempFile = os.CreateTemp
	renameFile     = os.Rename
	claimLocks     sync.Map
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
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp apply request store: %w", err)
	}
	tempFile = nil
	if err := renameFile(tempPath, path); err != nil {
		return fmt.Errorf("rename apply request store: %w", err)
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
func (s FileStore) Save(_ context.Context, record Record) error {
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
	return Save(s.Path, append(records, record))
}

// Update replaces one persisted apply request by ID.
func (s FileStore) Update(_ context.Context, record Record) error {
	if s.Path == "" {
		return fmt.Errorf("file apply request store path is required")
	}
	records, err := Load(s.Path)
	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	for i, existing := range records {
		if existing.ID == record.ID {
			records[i] = record
			return Save(s.Path, records)
		}
	}
	return fmt.Errorf("%w: %s", ErrNotFound, record.ID)
}

// Claim protects the local file-backed queued-apply lease boundary with one
// in-process lock per store path, then persists the transformed record in the
// same load/check/save critical section.
func (s FileStore) Claim(_ context.Context, id string, _ ClaimRequest, claim func(Record) (Record, error)) (Record, error) {
	if s.Path == "" {
		return Record{}, fmt.Errorf("file apply request store path is required")
	}
	lockValue, _ := claimLocks.LoadOrStore(s.Path, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	records, err := Load(s.Path)
	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
			return Record{}, ErrNotFound
		}
		return Record{}, err
	}
	for i, existing := range records {
		if existing.ID != id {
			continue
		}
		updated, err := claim(existing)
		if err != nil {
			return Record{}, err
		}
		records[i] = updated
		if err := Save(s.Path, records); err != nil {
			return Record{}, err
		}
		return updated, nil
	}
	return Record{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

// Heartbeat protects the local file-backed claim refresh boundary with one
// in-process lock per store path, then persists the transformed record in the
// same load/check/save critical section.
func (s FileStore) Heartbeat(_ context.Context, id string, _ HeartbeatRequest, heartbeat func(Record) (Record, error)) (Record, error) {
	if s.Path == "" {
		return Record{}, fmt.Errorf("file apply request store path is required")
	}
	lockValue, _ := claimLocks.LoadOrStore(s.Path, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	records, err := Load(s.Path)
	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
			return Record{}, ErrNotFound
		}
		return Record{}, err
	}
	for i, existing := range records {
		if existing.ID != id {
			continue
		}
		updated, err := heartbeat(existing)
		if err != nil {
			return Record{}, err
		}
		records[i] = updated
		if err := Save(s.Path, records); err != nil {
			return Record{}, err
		}
		return updated, nil
	}
	return Record{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}
