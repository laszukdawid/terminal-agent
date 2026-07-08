package taskmemory

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockingStore is a Store whose Add blocks until released or its context is
// cancelled, so tests can control indexer shutdown timing.
type blockingStore struct {
	mu        sync.Mutex
	addActive bool
	addCalls  int
	release   chan struct{}
}

func (s *blockingStore) Add(ctx context.Context, _ Record) error {
	s.mu.Lock()
	s.addActive = true
	s.addCalls++
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.addActive = false; s.mu.Unlock() }()
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *blockingStore) addRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addActive
}

func (s *blockingStore) Search(context.Context, Query) ([]Match, error) { return nil, nil }
func (s *blockingStore) Recent(context.Context, int) ([]Record, error)  { return nil, nil }
func (s *blockingStore) Stats(context.Context) (Stats, error)           { return Stats{}, nil }
func (s *blockingStore) Clear(context.Context) error                    { return nil }
func (s *blockingStore) Close() error                                   { return nil }

// TestAsyncIndexerCloseJoinsRunLoop guards BUG-15: Close must not return while an
// Add is in flight, even when the drain deadline passes, so the caller can close
// the store afterward without a race.
func TestAsyncIndexerCloseJoinsRunLoop(t *testing.T) {
	store := &blockingStore{release: make(chan struct{})} // Add blocks until ctx cancel
	idx := NewAsyncIndexer(store, Environment{OS: "linux"})
	idx.Add(Record{Kind: KindToolSuccess, SearchableText: "x"})

	// Wait until the run loop is actually inside Add so the deadline path is exercised.
	require.Eventually(t, store.addRunning, time.Second, time.Millisecond, "Add should be in flight")

	done := make(chan struct{})
	go func() { idx.Close(50 * time.Millisecond); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return; run loop was not joined/cancelled")
	}

	// The core guarantee: once Close returns, no Add is running, so closing the
	// store next cannot race an in-flight Add.
	assert.False(t, store.addRunning(), "Close must join the run loop before returning")
}

// TestAsyncIndexerCloseDrainsQueuedRecords verifies the clean-drain path still
// indexes queued records when Add completes promptly.
func TestAsyncIndexerCloseDrainsQueuedRecords(t *testing.T) {
	store := &blockingStore{release: make(chan struct{})}
	close(store.release) // Add returns immediately
	idx := NewAsyncIndexer(store, Environment{OS: "linux"})
	for i := 0; i < 3; i++ {
		idx.Add(Record{Kind: KindToolSuccess, SearchableText: "x"})
	}
	idx.Close(2 * time.Second)
	assert.Equal(t, 3, store.addCalls, "all queued records should be indexed on a clean drain")
	assert.False(t, store.addRunning())
}

func TestStepRecordKinds(t *testing.T) {
	env := Environment{OS: "linux", Arch: "amd64", CWD: "/tmp"}
	tests := []struct {
		name     string
		step     TaskStep
		wantKind string
		wantOK   bool
	}{
		{
			name:     "succeeded tool step indexes as tool_success",
			step:     TaskStep{Status: "succeeded", ToolName: "unix", ToolInput: map[string]any{"command": "date"}, ToolOutput: "now"},
			wantKind: KindToolSuccess,
			wantOK:   true,
		},
		{
			name:     "failed tool step indexes as tool_failure",
			step:     TaskStep{Status: "failed", ToolName: "unix", ToolInput: map[string]any{"command": "docker ps"}, Error: "daemon down"},
			wantKind: KindToolFailure,
			wantOK:   true,
		},
		{
			name:   "final answer step is not indexed; CompletionRecord is the single completion",
			step:   TaskStep{Status: "final_answer", ToolName: "final_answer", FinalAnswer: "noon", Thought: "it is noon"},
			wantOK: false,
		},
		{name: "thought step is not indexed", step: TaskStep{Status: "thought", Thought: "hmm"}, wantOK: false},
		{name: "declined step is not indexed", step: TaskStep{Status: "declined", ToolName: "unix"}, wantOK: false},
		{name: "empty status is not indexed", step: TaskStep{ToolName: "unix"}, wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, ok := StepRecord("run-1", "what time is it", tc.step, env)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantKind, rec.Kind)
			}
		})
	}
}

func TestCompletionRecordProvenanceAndTextHygiene(t *testing.T) {
	env := Environment{OS: "linux", Arch: "amd64", CWD: "/tmp"}
	tests := []struct {
		name        string
		response    string
		rawTool     string
		rawOutput   string
		prov        CompletionProvenance
		wantSeq     int
		wantInText  []string
		wantOutText []string
	}{
		{
			name:       "final answer completion carries the closing thought and step",
			response:   "It is noon.",
			prov:       CompletionProvenance{Thought: "the clock read 12:00", SourceSeq: 3},
			wantSeq:    3,
			wantInText: []string{"Thought: the clock read 12:00", "Final answer: It is noon."},
		},
		{
			name:        "thought restating the answer is indexed once",
			response:    "It is noon.",
			prov:        CompletionProvenance{Thought: "It is noon.", SourceSeq: 1},
			wantSeq:     1,
			wantInText:  []string{"Final answer: It is noon."},
			wantOutText: []string{"Thought:"},
		},
		{
			name:        "direct output identical to the response is indexed once",
			response:    "file-a\nfile-b",
			rawTool:     "unix",
			rawOutput:   "file-a\nfile-b",
			wantInText:  []string{"Final answer: file-a", "Tool: unix"},
			wantOutText: []string{"Raw output:"},
		},
		{
			name:        "summary fallback without provenance has no empty labels",
			response:    "Summarized outcome.",
			wantInText:  []string{"Final answer: Summarized outcome."},
			wantOutText: []string{"Thought:", "Raw output:", "Tool:", "final_answer"},
		},
		{
			name:       "distinct raw output stays indexed",
			response:   "You have 5 commits.",
			rawTool:    "unix",
			rawOutput:  "447a7cf feat\n25bbb3f chore",
			wantInText: []string{"Final answer: You have 5 commits.", "Raw output: 447a7cf feat", "Tool: unix"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := CompletionRecord("run-1", "the request", tc.response, tc.rawTool, tc.rawOutput, tc.prov, env)
			assert.Equal(t, KindTaskCompletion, rec.Kind)
			assert.Equal(t, tc.wantSeq, rec.SourceSeq)
			assert.Equal(t, tc.rawTool, rec.ToolName)
			for _, want := range tc.wantInText {
				assert.Contains(t, rec.SearchableText, want)
			}
			for _, notWant := range tc.wantOutText {
				assert.NotContains(t, rec.SearchableText, notWant)
			}
		})
	}
}

// A completed run indexes exactly one task_completion record: the final answer
// step is folded into CompletionRecord instead of being indexed on its own, so
// near-duplicate completions can no longer crowd the retrieval slots.
func TestCompletedRunIndexesExactlyOneCompletion(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "task_memory.db"))
	require.NoError(t, err)
	defer store.Close()
	ctx := context.Background()
	env := Environment{OS: "linux", Arch: "amd64", CWD: "/tmp"}

	// The records a completed "what time is it" run emits through the builders.
	require.NoError(t, store.Add(ctx, RequestRecord("run-1", "what time is it", env)))
	step, ok := StepRecord("run-1", "what time is it", TaskStep{
		Iteration: 1, Status: "succeeded", Thought: "run date",
		ToolName: "unix", ToolInput: map[string]any{"command": "date"}, ToolOutput: "12:00 PM",
	}, env)
	require.True(t, ok)
	require.NoError(t, store.Add(ctx, step))
	if _, ok := StepRecord("run-1", "what time is it", TaskStep{
		Iteration: 2, Status: "final_answer", Thought: "the clock read 12:00", ToolName: "final_answer", FinalAnswer: "It is noon.",
	}, env); ok {
		t.Fatal("final answer step must not produce its own record")
	}
	require.NoError(t, store.Add(ctx, CompletionRecord("run-1", "what time is it", "It is noon.", "", "",
		CompletionProvenance{Thought: "the clock read 12:00", SourceSeq: 2}, env)))

	// A second, unrelated completed run keeps the search realistic.
	require.NoError(t, store.Add(ctx, CompletionRecord("run-2", "list files", "three files", "unix", "a b c",
		CompletionProvenance{}, env)))

	matches, err := store.Search(ctx, Query{Text: "what time is it", OS: "linux", Limit: 10})
	require.NoError(t, err)
	completions := 0
	for _, match := range matches {
		if match.Record.Kind == KindTaskCompletion && match.Record.SourceRunID == "run-1" {
			completions++
			assert.Contains(t, match.Record.SearchableText, "Thought: the clock read 12:00")
			assert.Equal(t, 2, match.Record.SourceSeq)
		}
	}
	assert.Equal(t, 1, completions, "run-1 must contribute exactly one task_completion")

	stats, err := store.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 4, stats.Records, "request + tool step + one completion per run")
}
