package registry

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Registry manages run entries in ~/.orbit/runs/.
type Registry struct {
	dir string
}

// New creates a new Registry instance.
// Creates the registry directory if it doesn't exist.
func New(dir string) (*Registry, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create registry directory: %w", err)
	}

	return &Registry{dir: dir}, nil
}

// Register creates or updates a run entry.
// Uses atomic write (temp file + rename).
func (r *Registry) Register(entry *RunEntry) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	filePath := filepath.Join(r.dir, entry.ID+".json")

	// Write to temp file first for atomic operation
	tmpFile, err := os.CreateTemp(r.dir, ".tmp-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on error
	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	// Clear tmpPath so deferred cleanup doesn't try to remove it
	tmpPath = ""

	return nil
}

// Get retrieves a run entry by ID.
// Returns nil, nil if not found.
func (r *Registry) Get(id string) (*RunEntry, error) {
	filePath := filepath.Join(r.dir, id+".json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read entry: %w", err)
	}

	var entry RunEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal entry: %w", err)
	}

	return &entry, nil
}

// List returns all run entries.
// Skips malformed JSON files with a logged warning.
func (r *Registry) List() ([]*RunEntry, error) {
	files, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*RunEntry{}, nil
		}
		return nil, fmt.Errorf("failed to read registry directory: %w", err)
	}

	var entries []*RunEntry
	for _, f := range files {
		if f.IsDir() {
			continue
		}

		// Only process .json files
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(r.dir, f.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			log.Printf("Warning: failed to read registry file %s: %v", f.Name(), err)
			continue
		}

		var entry RunEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			log.Printf("Warning: malformed JSON in registry file %s: %v", f.Name(), err)
			continue
		}

		entries = append(entries, &entry)
	}

	return entries, nil
}

// FindByLogDir finds an entry by its log directory path.
// Returns nil, nil if not found.
func (r *Registry) FindByLogDir(logDir string) (*RunEntry, error) {
	entries, err := r.List()
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.LogDir == logDir {
			return entry, nil
		}
	}

	return nil, nil
}

// UpdateStatus updates the status of a run.
func (r *Registry) UpdateStatus(id string, status RunStatus) error {
	entry, err := r.Get(id)
	if err != nil {
		return err
	}
	if entry == nil {
		return fmt.Errorf("entry not found: %s", id)
	}

	entry.Status = status
	return r.Register(entry)
}

// UpdatePhase updates or adds a phase status.
func (r *Registry) UpdatePhase(id string, phase Phase) error {
	entry, err := r.Get(id)
	if err != nil {
		return err
	}
	if entry == nil {
		return fmt.Errorf("entry not found: %s", id)
	}

	// Find existing phase or add new one
	found := false
	for i, p := range entry.Phases {
		if p.Number == phase.Number {
			entry.Phases[i] = phase
			found = true
			break
		}
	}

	if !found {
		entry.Phases = append(entry.Phases, phase)
	}

	return r.Register(entry)
}
