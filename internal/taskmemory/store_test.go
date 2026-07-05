package taskmemory

import (
	"context"
	"path/filepath"
	"testing"

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
