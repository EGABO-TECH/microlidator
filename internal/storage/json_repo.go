// Package storage isolates all file-system I/O behind repository
// abstractions, so the domain/service layers never know whether the ledger
// lives in JSON, CSV, or something else (Repository Pattern).
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"microlidator/internal/domain"
)

// ErrNotFound is returned by Load when the underlying ledger file is missing.
var ErrNotFound = errors.New("ledger file not found. Run 'init' first")

// JSONRepo persists a domain.Ledger as a single JSON file on disk.
type JSONRepo struct {
	Path string
}

// NewJSONRepo creates a repository bound to the given file path.
func NewJSONRepo(path string) *JSONRepo {
	return &JSONRepo{Path: path}
}

// Exists reports whether the ledger file is present on disk.
func (r *JSONRepo) Exists() bool {
	_, err := os.Stat(r.Path)
	return err == nil
}

// Load reads and parses the ledger file.
func (r *JSONRepo) Load() (*domain.Ledger, error) {
	if !r.Exists() {
		return nil, ErrNotFound
	}
	data, err := os.ReadFile(r.Path)
	if err != nil {
		return nil, fmt.Errorf("reading ledger file: %w", err)
	}
	var ledger domain.Ledger
	if err := json.Unmarshal(data, &ledger); err != nil {
		return nil, fmt.Errorf("parsing ledger json: %w", err)
	}
	return &ledger, nil
}

// Save atomically persists the ledger using the Atomic Write Pattern:
// write to a temp file in the same directory, fsync it, then os.Rename
// it over the destination. This guarantees the ledger file is never left
// half-written after a crash or power loss.
func (r *JSONRepo) Save(ledger *domain.Ledger) error {
	dir := filepath.Dir(r.Path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".microlidator-tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	// Ensure the temp file never lingers on any error path.
	defer os.Remove(tmpPath)

	data, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		tmp.Close()
		return fmt.Errorf("marshalling ledger: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, r.Path); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}
