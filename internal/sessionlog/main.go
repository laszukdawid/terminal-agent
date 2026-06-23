// Package sessionlog records a per-run, always-on execution log to a JSONL file.
//
// Each agent invocation (ask/chat/task) creates one Recorder bound to a single file
// under the configured sessions directory. Every step of the run — the request,
// internal thoughts, attempted tools, their results, confirmations, and the final
// response or error — is appended as one JSON object per line.
//
// The package owns id/filename generation and file mechanics only. Translating the
// app's Event and the agent's TaskStep into Record values is the caller's job, which
// keeps this package free of imports from internal/app and internal/agent.
package sessionlog

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

// fileTimestampLayout is a filesystem-safe timestamp used in session filenames.
const fileTimestampLayout = "2006-01-02T15-04-05"

type RecordType string

const (
	RecordMeta         RecordType = "meta"
	RecordRequest      RecordType = "request"
	RecordThought      RecordType = "thought"
	RecordToolCall     RecordType = "tool_call"
	RecordToolResult   RecordType = "tool_result"
	RecordProgress     RecordType = "progress"
	RecordConfirmation RecordType = "confirmation"
	RecordDeclined     RecordType = "declined"
	RecordCompleted    RecordType = "completed"
	RecordFailed       RecordType = "failed"
)

// Meta is the provenance header written as the first line of every session file.
type Meta struct {
	RunID    string `json:"run_id"`
	Kind     string `json:"kind"`
	User     string `json:"user,omitempty"`
	Cwd      string `json:"cwd,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Command  string `json:"command,omitempty"`
	// TaskTimeout records the resolved task-level timeout for task runs
	// (e.g. "15m" or "unlimited"); empty for non-task runs.
	TaskTimeout string `json:"task_timeout,omitempty"`
	// RoutineID records which routine produced the run for routine runs; empty
	// for ad-hoc ask/chat/task runs.
	RoutineID string    `json:"routine_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Record is a single line in a session log file.
type Record struct {
	RunID        string         `json:"run_id"`
	Kind         string         `json:"kind"`
	Type         RecordType     `json:"type"`
	Timestamp    time.Time      `json:"timestamp"`
	Seq          int            `json:"seq"`
	Iteration    int            `json:"iteration,omitempty"`
	Status       string         `json:"status,omitempty"`
	Meta         *Meta          `json:"meta,omitempty"`
	Text         string         `json:"text,omitempty"`
	ToolName     string         `json:"tool_name,omitempty"`
	ToolInput    map[string]any `json:"tool_input,omitempty"`
	ToolResult   string         `json:"tool_result,omitempty"`
	Confirmation string         `json:"confirmation,omitempty"`
	Allowed      *bool          `json:"allowed,omitempty"`
	Error        string         `json:"error,omitempty"`
}

// Recorder appends Records to one run's JSONL file. It is safe for concurrent use.
type Recorder struct {
	runID string
	kind  string
	path  string

	mu  sync.Mutex
	seq int
}

// New generates a run-id, derives the timestamped filename inside dir, writes the meta
// header line, and returns a ready-to-use Recorder. The provided meta's RunID and
// CreatedAt are populated here when empty so the caller need not set them.
//
// New never returns an error: logging must never block a run. If the meta header cannot
// be written it is logged at warn level and a usable Recorder is still returned.
func New(dir string, meta Meta) *Recorder {
	runID := strings.TrimSpace(meta.RunID)
	if runID == "" {
		runID = uuid.NewString()
		meta.RunID = runID
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now()
	}

	r := &Recorder{
		runID: runID,
		kind:  meta.Kind,
		path:  filePath(dir, meta.CreatedAt, meta.Kind, runID),
	}

	r.Write(Record{Type: RecordMeta, Timestamp: meta.CreatedAt, Meta: &meta})
	return r
}

// RunID returns the generated run identifier.
func (r *Recorder) RunID() string {
	return r.runID
}

// Path returns the absolute path of the session file.
func (r *Recorder) Path() string {
	return r.path
}

// Write appends a record to the session file. It stamps RunID, Kind, Timestamp, and a
// monotonic Seq when unset. Errors are swallowed (logged at warn level) so a logging
// failure can never break the run.
func (r *Recorder) Write(rec Record) {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.seq++
	rec.Seq = r.seq
	if rec.RunID == "" {
		rec.RunID = r.runID
	}
	if rec.Kind == "" {
		rec.Kind = r.kind
	}
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now()
	}
	path := r.path
	r.mu.Unlock()

	if err := utils.WriteToJSONLFile(path, rec); err != nil {
		utils.GetLogger().Warn("failed to write session log record",
			zap.String("run_id", r.runID),
			zap.String("path", path),
			zap.Error(err),
		)
	}
}

// filePath builds {dir}/{timestamp}_{kind}_{shortid}.jsonl.
func filePath(dir string, createdAt time.Time, kind, runID string) string {
	shortID := strings.ReplaceAll(runID, "-", "")
	if len(shortID) > 6 {
		shortID = shortID[:6]
	}
	name := createdAt.Format(fileTimestampLayout) + "_" + kind + "_" + shortID + ".jsonl"
	return filepath.Join(dir, name)
}
