package taskmemory

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteStoreAddSearchStatsAndClear(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "task_memory.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	rec := Record{
		Kind:           KindToolSuccess,
		OS:             "linux",
		ToolName:       "unix",
		ToolInput:      map[string]any{"command": "date", "final": true},
		FinalOutput:    "Sat Jun 27 12:00:00 CEST 2026",
		SearchableText: "User request: what time is it\nTool: unix\nCommand: date\nOutput: current local date and time\nOS: linux",
	}
	require.NoError(t, store.Add(ctx, rec))
	require.NoError(t, store.Add(ctx, rec), "deterministic ids should make duplicate inserts idempotent")

	stats, err := store.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Records)

	matches, err := store.Search(ctx, Query{Text: "give me time", OS: "linux", Limit: 5})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, "unix", matches[0].Record.ToolName)
	assert.Equal(t, "date", matches[0].Record.ToolInput["command"])

	require.NoError(t, store.Clear(ctx))
	stats, err = store.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Records)
}

func TestFormatMatchesHandlesEmptyResults(t *testing.T) {
	assert.Equal(t, "No relevant task memories found.", FormatMatches(nil))
}

func TestSearchPrefersActionableToolSuccess(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "task_memory.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	for _, rec := range []Record{
		{
			Kind:           KindTaskRequest,
			OS:             "linux",
			SearchableText: "Kind: task_request\nUser request: what time is it\nOS: linux",
		},
		{
			Kind:           KindTaskCompletion,
			OS:             "linux",
			FinalOutput:    "06:01 AM PDT",
			SearchableText: "Kind: task_completion\nUser request: what time is it\nFinal answer: 06:01 AM PDT\nOS: linux",
		},
		{
			Kind:           KindToolSuccess,
			OS:             "linux",
			ToolName:       "unix",
			ToolInput:      map[string]any{"command": "date '+%I:%M %p %Z'", "timeout": "10s"},
			FinalOutput:    "06:01 AM PDT",
			SearchableText: "Kind: tool_success\nUser request: what time is it\nTool: unix\nInput: {\"command\":\"date '+%I:%M %p %Z'\",\"timeout\":\"10s\"}\nOutput: 06:01 AM PDT\nOS: linux",
		},
	} {
		require.NoError(t, store.Add(ctx, rec))
	}

	matches, err := store.Search(ctx, Query{Text: "what time is it", OS: "linux", Limit: 3})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, KindToolSuccess, matches[0].Record.Kind)
	assert.Equal(t, "unix", matches[0].Record.ToolName)
	assert.Equal(t, "date '+%I:%M %p %Z'", matches[0].Record.ToolInput["command"])
}

func TestRecentReturnsNewestFirst(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "task_memory.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		require.NoError(t, store.Add(ctx, Record{
			Kind:           KindToolSuccess,
			OS:             "linux",
			ToolName:       "unix",
			ToolInput:      map[string]any{"command": fmt.Sprintf("echo %d", i)},
			SearchableText: fmt.Sprintf("Kind: tool_success\nTool: unix\nInput: echo %d", i),
			CreatedAt:      base.Add(time.Duration(i) * time.Minute),
		}))
	}

	tests := []struct {
		name         string
		limit        int
		wantLen      int
		wantFirstCmd string
	}{
		{name: "newest first within limit", limit: 2, wantLen: 2, wantFirstCmd: "echo 2"},
		{name: "limit larger than records", limit: 10, wantLen: 3, wantFirstCmd: "echo 2"},
		{name: "non-positive limit falls back to default", limit: 0, wantLen: 3, wantFirstCmd: "echo 2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			records, err := store.Recent(ctx, tc.limit)
			require.NoError(t, err)
			require.Len(t, records, tc.wantLen)
			assert.Equal(t, tc.wantFirstCmd, records[0].ToolInput["command"])
		})
	}
}

func TestRecentOnEmptyStore(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "task_memory.db"))
	require.NoError(t, err)
	defer store.Close()

	records, err := store.Recent(context.Background(), 5)
	require.NoError(t, err)
	assert.Empty(t, records)
}
