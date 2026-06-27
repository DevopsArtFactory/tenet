package controllerrefreshaudit

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

// FileStore persists manual controller refresh audit records in a local JSON file.
type FileStore struct {
	Path string
}

// NewFileStore returns a filesystem-backed manual controller refresh audit store.
func NewFileStore(path string) FileStore {
	return FileStore{Path: path}
}

// Load reads persisted manual controller refresh audit records from disk.
func Load(path string) ([]Record, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read controller refresh audit store: %w", err)
	}

	var records []Record
	if err := json.Unmarshal(content, &records); err != nil {
		return nil, fmt.Errorf("decode controller refresh audit store: %w", err)
	}
	for i := range records {
		records[i].Actor = requestactor.Normalize(records[i].Actor)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].ID < records[j].ID
	})
	return records, nil
}

// Save writes the full immutable controller refresh audit set atomically.
func Save(path string, records []Record) error {
	sorted := list(records)
	content, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return fmt.Errorf("encode controller refresh audit store: %w", err)
	}
	content = append(content, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create controller refresh audit store dir: %w", err)
	}
	fileMode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		fileMode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat controller refresh audit store: %w", err)
	}

	tempFile, err := createTempFile(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp controller refresh audit store: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		if tempFile != nil {
			_ = tempFile.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := tempFile.Chmod(fileMode); err != nil {
		return fmt.Errorf("chmod temp controller refresh audit store: %w", err)
	}
	if _, err := tempFile.Write(content); err != nil {
		return fmt.Errorf("write temp controller refresh audit store: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp controller refresh audit store: %w", err)
	}
	tempFile = nil
	if err := renameFile(tempPath, path); err != nil {
		return fmt.Errorf("rename controller refresh audit store: %w", err)
	}
	return nil
}

// List returns all persisted manual controller refresh audit records.
func (s FileStore) List(_ context.Context) ([]Record, error) {
	if s.Path == "" {
		return nil, fmt.Errorf("file controller refresh audit store path is required")
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

// Save persists one immutable manual controller refresh audit record by ID.
func (s FileStore) Save(_ context.Context, record Record) error {
	if s.Path == "" {
		return fmt.Errorf("file controller refresh audit store path is required")
	}
	records, err := Load(s.Path)
	if err != nil {
		var pathErr *os.PathError
		if !(errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist)) {
			return err
		}
	}
	if _, err := get(records, record.ID); err == nil {
		return nil
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	return Save(s.Path, append(records, record))
}
