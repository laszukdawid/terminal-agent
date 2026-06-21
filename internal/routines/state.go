package routines

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"time"
)

// Run outcomes recorded for a routine's most recent execution.
const (
	OutcomeSuccess       = "success"
	OutcomeFailed        = "failed"
	OutcomeTimeout       = "timeout"
	OutcomeTokenExceeded = "token_exceeded"
)

// Run triggers distinguish a manual invocation from a scheduled one.
const (
	TriggerManual    = "manual"
	TriggerScheduled = "scheduled"
)

// DisplayStatus values shown in list views.
const (
	StatusActive   = "active"   // enabled and last run (if any) did not fail
	StatusInactive = "inactive" // disabled
	StatusError    = "error"    // enabled but the last run failed
)

// RunRecord is the persisted status of a routine's most recent run plus the
// daemon-maintained next run time.
type RunRecord struct {
	LastRunAt           time.Time `json:"last_run_at,omitempty"`
	LastStatus          string    `json:"last_status,omitempty"`
	LastDuration        string    `json:"last_duration,omitempty"`
	LastTrigger         string    `json:"last_trigger,omitempty"`
	LastError           string    `json:"last_error,omitempty"`
	LastSessionLog      string    `json:"last_session_log,omitempty"`
	LastResultPath      string    `json:"last_result_path,omitempty"`
	TokensUsed          int       `json:"tokens_used,omitempty"`
	ConsecutiveFailures int       `json:"consecutive_failures,omitempty"`
	NextRunAt           time.Time `json:"next_run_at,omitempty"`
}

// Succeeded reports whether the recorded outcome was a success.
func (r RunRecord) Succeeded() bool {
	return r.LastStatus == OutcomeSuccess
}

type stateFile struct {
	LastSeenAt time.Time            `json:"last_seen_at,omitempty"`
	Runs       map[string]RunRecord `json:"runs,omitempty"`
}

// StateStore persists routine run state to a single JSON file (data dir).
type StateStore struct {
	path string
}

// NewStateStore returns a StateStore backed by the given file path.
func NewStateStore(path string) *StateStore {
	return &StateStore{path: path}
}

// DefaultStateStore returns a StateStore backed by the standard state path.
func DefaultStateStore() *StateStore {
	return NewStateStore(StatePath())
}

func (s *StateStore) lockPath() string {
	return s.path + ".lock"
}

func (s *StateStore) load() (stateFile, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return stateFile{Runs: map[string]RunRecord{}}, nil
	}
	if err != nil {
		return stateFile{}, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return stateFile{Runs: map[string]RunRecord{}}, nil
	}
	var file stateFile
	if err := json.Unmarshal(data, &file); err != nil {
		return stateFile{}, err
	}
	if file.Runs == nil {
		file.Runs = map[string]RunRecord{}
	}
	return file, nil
}

// Get returns the run record for a routine and whether one exists.
func (s *StateStore) Get(id string) (RunRecord, bool, error) {
	file, err := s.load()
	if err != nil {
		return RunRecord{}, false, err
	}
	rec, ok := file.Runs[id]
	return rec, ok, nil
}

// All returns every routine's run record keyed by id.
func (s *StateStore) All() (map[string]RunRecord, error) {
	file, err := s.load()
	if err != nil {
		return nil, err
	}
	return file.Runs, nil
}

// Record stores the outcome of a run. It derives ConsecutiveFailures from the
// prior record and preserves the daemon-maintained NextRunAt when the incoming
// record leaves it zero.
func (s *StateStore) Record(id string, rec RunRecord) error {
	return withLock(s.lockPath(), func() error {
		file, err := s.load()
		if err != nil {
			return err
		}
		prior := file.Runs[id]
		if rec.Succeeded() {
			rec.ConsecutiveFailures = 0
		} else {
			rec.ConsecutiveFailures = prior.ConsecutiveFailures + 1
		}
		if rec.NextRunAt.IsZero() {
			rec.NextRunAt = prior.NextRunAt
		}
		file.Runs[id] = rec
		return writeJSONAtomic(s.path, file)
	})
}

// SetNextRun updates only the NextRunAt field for a routine, preserving its run
// history. Used by the scheduler to publish the upcoming fire time.
func (s *StateStore) SetNextRun(id string, next time.Time) error {
	return withLock(s.lockPath(), func() error {
		file, err := s.load()
		if err != nil {
			return err
		}
		rec := file.Runs[id]
		rec.NextRunAt = next
		file.Runs[id] = rec
		return writeJSONAtomic(s.path, file)
	})
}

// Delete removes a routine's run record.
func (s *StateStore) Delete(id string) error {
	return withLock(s.lockPath(), func() error {
		file, err := s.load()
		if err != nil {
			return err
		}
		if _, ok := file.Runs[id]; !ok {
			return nil
		}
		delete(file.Runs, id)
		return writeJSONAtomic(s.path, file)
	})
}

// SeenNotice describes a run that completed since the user last saw routine
// activity, used for the surface-on-next-launch message.
type SeenNotice struct {
	ID      string
	Record  RunRecord
	Failure bool
}

// PendingSince returns runs whose LastRunAt is after the stored last-seen marker.
func (s *StateStore) PendingSince() ([]SeenNotice, error) {
	file, err := s.load()
	if err != nil {
		return nil, err
	}
	var notices []SeenNotice
	for id, rec := range file.Runs {
		if rec.LastRunAt.After(file.LastSeenAt) {
			notices = append(notices, SeenNotice{ID: id, Record: rec, Failure: !rec.Succeeded()})
		}
	}
	return notices, nil
}

// MarkSeen advances the last-seen marker to now so already-reported runs are not
// shown again.
func (s *StateStore) MarkSeen() error {
	return withLock(s.lockPath(), func() error {
		file, err := s.load()
		if err != nil {
			return err
		}
		file.LastSeenAt = time.Now().UTC()
		return writeJSONAtomic(s.path, file)
	})
}

// ClaimPending atomically returns runs that completed since the last-seen marker
// and advances the marker to the newest LastRunAt it actually reports. Doing both
// under one lock ensures a run that completes concurrently is never skipped: only
// runs included in the returned slice are marked seen, so anything newer remains
// pending for the next call.
func (s *StateStore) ClaimPending() ([]SeenNotice, error) {
	var notices []SeenNotice
	err := withLock(s.lockPath(), func() error {
		file, err := s.load()
		if err != nil {
			return err
		}
		newest := file.LastSeenAt
		for id, rec := range file.Runs {
			if rec.LastRunAt.After(file.LastSeenAt) {
				notices = append(notices, SeenNotice{ID: id, Record: rec, Failure: !rec.Succeeded()})
				if rec.LastRunAt.After(newest) {
					newest = rec.LastRunAt
				}
			}
		}
		if len(notices) == 0 {
			return nil
		}
		file.LastSeenAt = newest
		return writeJSONAtomic(s.path, file)
	})
	if err != nil {
		return nil, err
	}
	return notices, nil
}

// DisplayStatus blends a routine's enabled flag with its last run outcome into
// the active/inactive/error status shown in list views.
func DisplayStatus(enabled bool, rec RunRecord, hasRun bool) string {
	if !enabled {
		return StatusInactive
	}
	if hasRun && !rec.Succeeded() {
		return StatusError
	}
	return StatusActive
}
