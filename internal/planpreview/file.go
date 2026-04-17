package planpreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var (
	createTempFile = os.CreateTemp
	renameFile     = os.Rename
)

// FileStore persists immutable plan previews in a local JSON file.
type FileStore struct {
	Path string
}

// NewFileStore returns a filesystem-backed plan preview store.
func NewFileStore(path string) FileStore {
	return FileStore{Path: path}
}

// Load reads persisted plan previews from disk.
func Load(path string) ([]Record, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plan preview store: %w", err)
	}

	var records []Record
	if err := json.Unmarshal(content, &records); err != nil {
		return nil, fmt.Errorf("decode plan preview store: %w", err)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].ID < records[j].ID
	})
	return records, nil
}

// Save writes the full immutable plan preview set atomically.
func Save(path string, records []Record) error {
	sorted := list(records)
	content, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return fmt.Errorf("encode plan preview store: %w", err)
	}
	content = append(content, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create plan preview store dir: %w", err)
	}
	fileMode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		fileMode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat plan preview store: %w", err)
	}

	tempFile, err := createTempFile(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp plan preview store: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if err := tempFile.Chmod(fileMode); err != nil {
		tempFile.Close()
		return fmt.Errorf("chmod temp plan preview store: %w", err)
	}
	if _, err := tempFile.Write(content); err != nil {
		tempFile.Close()
		return fmt.Errorf("write temp plan preview store: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp plan preview store: %w", err)
	}
	if err := renameFile(tempPath, path); err != nil {
		return fmt.Errorf("rename plan preview store: %w", err)
	}
	return nil
}

// Get loads one plan preview by ID.
func (s FileStore) Get(_ context.Context, id string) (Record, error) {
	if s.Path == "" {
		return Record{}, fmt.Errorf("file plan preview store path is required")
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

// Save persists one immutable plan preview by ID.
func (s FileStore) Save(_ context.Context, record Record) error {
	if s.Path == "" {
		return fmt.Errorf("file plan preview store path is required")
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
