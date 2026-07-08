package taskmemory

import (
	"context"
	"database/sql"
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

// requireFTS skips a test when the sqlite build lacks FTS5, so the FTS-path
// assertions only run under `go test -tags sqlite_fts5`.
func requireFTS(t *testing.T, store *SQLiteStore) {
	t.Helper()
	if !store.ftsEnabled {
		t.Skip("sqlite build has no FTS5 support; run with -tags sqlite_fts5 to exercise it")
	}
}

// indexedDocCount returns the number of documents actually held in the FTS5
// inverted index, read from the %_docsize shadow table. A plain
// `count(*) FROM task_memory_fts` is unusable here: on an external-content
// table it scans the base table and therefore always equals the base row
// count, so it cannot detect an empty, stale, or duplicated index. The docsize
// table reflects the real index state (0 when empty, one row per indexed doc).
func indexedDocCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM task_memory_fts_docsize`).Scan(&n))
	return n
}

func baseRowCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM task_memories`).Scan(&n))
	return n
}

// TestFTSIndexUsesExternalContent guards PERF-15: the searchable text must be
// stored only once (in task_memories), with the FTS index reading it back via
// external content rather than keeping its own copy.
func TestFTSIndexUsesExternalContent(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "task_memory.db"))
	require.NoError(t, err)
	defer store.Close()
	requireFTS(t, store)

	var ddl string
	require.NoError(t, store.db.QueryRow(`SELECT sql FROM sqlite_master WHERE name = 'task_memory_fts'`).Scan(&ddl))
	assert.Contains(t, ddl, "content='task_memories'", "FTS index must be external-content so searchable text is not stored twice")
}

// TestAddDoesNotDuplicateFTSRows guards BUG-3: re-adding a record with an
// existing deterministic id must not append a second FTS row, so the index
// stays in lockstep with the base table.
func TestAddDoesNotDuplicateFTSRows(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "task_memory.db"))
	require.NoError(t, err)
	defer store.Close()
	requireFTS(t, store)

	ctx := context.Background()
	rec := Record{
		Kind:           KindToolSuccess,
		OS:             "linux",
		ToolName:       "unix",
		ToolInput:      map[string]any{"command": "date"},
		SearchableText: "Kind: tool_success\nTool: unix\nInput: date\nOutput: now\nOS: linux",
	}
	for i := 0; i < 3; i++ {
		require.NoError(t, store.Add(ctx, rec), "re-adding the same record must stay idempotent")
	}

	assert.Equal(t, 1, baseRowCount(t, store.db), "deterministic id should dedupe the base table")
	assert.Equal(t, 1, indexedDocCount(t, store.db), "FTS index must hold exactly one document, in lockstep with the base table")

	// The user-visible symptom of the old duplication was the same record
	// surfacing multiple times in search results; it must now surface once.
	matches, err := store.Search(ctx, Query{Text: "unix date output", OS: "linux", Limit: 10})
	require.NoError(t, err)
	require.Len(t, matches, 1, "a re-added record must appear once, not as duplicates")
	assert.Equal(t, "unix", matches[0].Record.ToolName)
}

// TestMigrationFromLegacyContentfulFTS guards DES-8 plus the on-disk cleanup
// for BUG-3/PERF-15: a v1 database with a duplicated contentful FTS index is
// rebuilt into a deduplicated external-content index on open, and the schema
// version is advanced so the rebuild runs only once.
func TestMigrationFromLegacyContentfulFTS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task_memory.db")
	seedLegacyV1DB(t, path)

	store, err := Open(path)
	require.NoError(t, err)
	defer store.Close()
	requireFTS(t, store)

	ctx := context.Background()
	assert.Equal(t, 1, baseRowCount(t, store.db))
	assert.Equal(t, 1, indexedDocCount(t, store.db), "rebuild must collapse the duplicated legacy index to one document per base record")

	version, err := store.readSchemaVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, dbSchemaVersion, version, "schema version must advance so the rebuild does not repeat")

	// The seeded legacy index carried the record twice; after migration a search
	// must return it exactly once from the rebuilt external-content index.
	matches, err := store.Search(ctx, Query{Text: "database migration", OS: "linux", Limit: 5})
	require.NoError(t, err)
	require.Len(t, matches, 1, "migrated index must surface the record once, not as duplicates")
	assert.Equal(t, "legacy-1", matches[0].Record.ID)
}

// seedLegacyV1DB writes a database in the pre-migration (v1) layout: the base
// tables plus a legacy contentful FTS5 index carrying the BUG-3 duplication
// (the same record indexed twice). It skips the test when the sqlite build
// cannot create an FTS5 table.
func seedLegacyV1DB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	defer db.Close()

	for _, stmt := range []string{
		`CREATE TABLE task_memory_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE task_memories (
			id TEXT PRIMARY KEY, kind TEXT NOT NULL, source_run_id TEXT, source_seq INTEGER,
			source_kind TEXT, os TEXT, arch TEXT, shell TEXT, cwd TEXT, tool_name TEXT,
			tool_input_json TEXT, final_output TEXT, searchable_text TEXT NOT NULL, created_at TEXT NOT NULL
		)`,
	} {
		_, err := db.Exec(stmt)
		require.NoError(t, err)
	}

	if _, err := db.Exec(`CREATE VIRTUAL TABLE task_memory_fts USING fts5(id UNINDEXED, searchable_text, tokenize='unicode61')`); err != nil {
		t.Skip("sqlite build has no FTS5 support; run with -tags sqlite_fts5 to exercise it")
	}

	text := "Kind: tool_success\nUser request: database migration\nTool: unix\nOutput: ok\nOS: linux"
	// Mirror what the production Add path writes: every text column is a
	// non-NULL empty string rather than NULL.
	_, err = db.Exec(`INSERT INTO task_memories(
		id, kind, source_run_id, source_seq, source_kind, os, arch, shell, cwd,
		tool_name, tool_input_json, final_output, searchable_text, created_at
	) VALUES('legacy-1', ?, '', 0, 'task', 'linux', 'amd64', '', '', 'unix', '{}', 'ok', ?, ?)`,
		KindToolSuccess, text, time.Now().UTC().Format(time.RFC3339Nano))
	require.NoError(t, err)

	// BUG-3 in the wild: the same record indexed twice into the contentful table.
	for i := 0; i < 2; i++ {
		_, err = db.Exec(`INSERT INTO task_memory_fts(id, searchable_text) VALUES('legacy-1', ?)`, text)
		require.NoError(t, err)
	}

	_, err = db.Exec(`INSERT INTO task_memory_meta(key, value) VALUES('schema_version', '1')`)
	require.NoError(t, err)
}
