package routines

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// ErrNotFound is returned when a routine cannot be resolved by id or name.
var ErrNotFound = errors.New("routine not found")

// ErrExists is returned by Add when a routine with the same id already exists.
var ErrExists = errors.New("routine already exists")

// definitionsFile is the on-disk shape of the routines definitions file.
type definitionsFile struct {
	Routines []Routine `json:"routines"`
}

// Store persists routine definitions to a single JSON file (config dir).
type Store struct {
	path string
}

// NewStore returns a Store backed by the given file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// DefaultStore returns a Store backed by the standard definitions path.
func DefaultStore() *Store {
	return NewStore(DefinitionsPath())
}

func (s *Store) lockPath() string {
	return s.path + ".lock"
}

func (s *Store) load() (definitionsFile, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return definitionsFile{}, nil
	}
	if err != nil {
		return definitionsFile{}, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return definitionsFile{}, nil
	}
	var file definitionsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return definitionsFile{}, fmt.Errorf("parse routines file %s: %w", s.path, err)
	}
	return file, nil
}

// List returns all stored routines.
func (s *Store) List() ([]Routine, error) {
	file, err := s.load()
	if err != nil {
		return nil, err
	}
	return file.Routines, nil
}

// Get resolves a routine by exact id, exact name, or an unambiguous id prefix.
func (s *Store) Get(idOrName string) (Routine, error) {
	file, err := s.load()
	if err != nil {
		return Routine{}, err
	}
	return resolve(file.Routines, idOrName)
}

// Upsert inserts or replaces a routine by id, stamping timestamps. CreatedAt is
// preserved on update.
func (s *Store) Upsert(r Routine) error {
	if err := r.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	return withLock(s.lockPath(), func() error {
		file, err := s.load()
		if err != nil {
			return err
		}
		r.UpdatedAt = now
		if idx := indexByID(file.Routines, r.ID); idx >= 0 {
			if !file.Routines[idx].CreatedAt.IsZero() {
				r.CreatedAt = file.Routines[idx].CreatedAt
			}
			file.Routines[idx] = r
		} else {
			if r.CreatedAt.IsZero() {
				r.CreatedAt = now
			}
			file.Routines = append(file.Routines, r)
		}
		return writeJSONAtomic(s.path, file)
	})
}

// Add inserts a new routine, returning ErrExists if a routine with the same id
// already exists. The existence check and write happen under one lock so a
// concurrent create cannot silently overwrite.
func (s *Store) Add(r Routine) error {
	if err := r.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	return withLock(s.lockPath(), func() error {
		file, err := s.load()
		if err != nil {
			return err
		}
		if indexByID(file.Routines, r.ID) >= 0 {
			return fmt.Errorf("%w: %s", ErrExists, r.ID)
		}
		if r.CreatedAt.IsZero() {
			r.CreatedAt = now
		}
		r.UpdatedAt = now
		file.Routines = append(file.Routines, r)
		return writeJSONAtomic(s.path, file)
	})
}

// Delete removes a routine by id. It reports whether a routine was removed.
func (s *Store) Delete(id string) (bool, error) {
	removed := false
	err := withLock(s.lockPath(), func() error {
		file, err := s.load()
		if err != nil {
			return err
		}
		kept := make([]Routine, 0, len(file.Routines))
		for _, r := range file.Routines {
			if r.ID == id {
				removed = true
				continue
			}
			kept = append(kept, r)
		}
		if !removed {
			return nil
		}
		file.Routines = kept
		return writeJSONAtomic(s.path, file)
	})
	return removed, err
}

// IDTaken reports whether an id already exists. Useful for generating unique ids.
func (s *Store) IDTaken(id string) bool {
	file, err := s.load()
	if err != nil {
		return false
	}
	return indexByID(file.Routines, id) >= 0
}

func indexByID(routines []Routine, id string) int {
	for i, r := range routines {
		if r.ID == id {
			return i
		}
	}
	return -1
}

func resolve(routines []Routine, idOrName string) (Routine, error) {
	query := strings.TrimSpace(idOrName)
	if query == "" {
		return Routine{}, ErrNotFound
	}
	if idx := indexByID(routines, query); idx >= 0 {
		return routines[idx], nil
	}

	var matches []Routine
	for _, r := range routines {
		if r.Name == query || strings.HasPrefix(r.ID, query) {
			matches = append(matches, r)
		}
	}
	switch len(matches) {
	case 0:
		return Routine{}, fmt.Errorf("%w: %s", ErrNotFound, query)
	case 1:
		return matches[0], nil
	default:
		return Routine{}, fmt.Errorf("ambiguous routine reference %q matches %d routines", query, len(matches))
	}
}
