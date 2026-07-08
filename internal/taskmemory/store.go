package taskmemory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/laszukdawid/terminal-agent/internal/utils"
	_ "github.com/mattn/go-sqlite3"
)

const (
	KindTaskRequest    = "task_request"
	KindToolSuccess    = "tool_success"
	KindToolFailure    = "tool_failure"
	KindTaskCompletion = "task_completion"

	defaultSearchLimit = 5
	maxIndexedOutput   = 4000
	maxSnippetOutput   = 600

	// dbSchemaVersion is the current on-disk schema version. It is stored in
	// task_memory_meta and read on open to gate stepwise migrations.
	dbSchemaVersion = 2
	// ftsExternalContentVersion is the schema version that moved the full-text
	// index to an external-content FTS5 table. Databases below it have their FTS
	// index dropped and rebuilt on open (see migrateToExternalContentFTS).
	ftsExternalContentVersion = 2
)

// createTaskMemoryFTS defines the full-text index as an external-content FTS5
// table over task_memories. External content means FTS5 stores only the
// inverted index and reads the document text from task_memories via rowid, so
// the searchable text is never stored twice and the index cannot silently
// diverge from the base table.
const createTaskMemoryFTS = `CREATE VIRTUAL TABLE IF NOT EXISTS task_memory_fts USING fts5(
	searchable_text,
	content='task_memories',
	content_rowid='rowid',
	tokenize='unicode61'
)`

var tokenPattern = regexp.MustCompile(`[\pL\pN_./~$-]+`)

type Store interface {
	Add(context.Context, Record) error
	Search(context.Context, Query) ([]Match, error)
	Recent(context.Context, int) ([]Record, error)
	Stats(context.Context) (Stats, error)
	Clear(context.Context) error
	Close() error
}

type Record struct {
	ID             string
	Kind           string
	SourceRunID    string
	SourceSeq      int
	SourceKind     string
	OS             string
	Arch           string
	Shell          string
	CWD            string
	ToolName       string
	ToolInput      map[string]any
	FinalOutput    string
	SearchableText string
	CreatedAt      time.Time
}

type Query struct {
	Text       string
	OS         string
	Arch       string
	Shell      string
	CWD        string
	Limit      int
	IncludeCWD bool
}

type Match struct {
	Record Record
	Score  float64
}

type Stats struct {
	Records    int
	FTSEnabled bool
}

type Environment struct {
	OS    string
	Arch  string
	Shell string
	CWD   string
}

type SQLiteStore struct {
	db         *sql.DB
	ftsEnabled bool
}

func CurrentEnvironment(cwd string) Environment {
	return Environment{OS: runtime.GOOS, Arch: runtime.GOARCH, Shell: filepath.Base(os.Getenv("SHELL")), CWD: cwd}
}

func Open(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create task memory directory: %w", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open task memory database: %w", err)
	}
	store := &SQLiteStore{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	pragmas := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`PRAGMA foreign_keys=ON`,
	}
	for _, stmt := range pragmas {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("configure task memory database: %w", err)
		}
	}

	// Base tables and indexes are version-independent; the only thing that has
	// changed across schema versions is the shape of the FTS index below.
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS task_memory_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS task_memories (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			source_run_id TEXT,
			source_seq INTEGER,
			source_kind TEXT,
			os TEXT,
			arch TEXT,
			shell TEXT,
			cwd TEXT,
			tool_name TEXT,
			tool_input_json TEXT,
			final_output TEXT,
			searchable_text TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_memories_kind ON task_memories(kind)`,
		`CREATE INDEX IF NOT EXISTS idx_task_memories_os ON task_memories(os)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate task memory database: %w", err)
		}
	}

	s.ftsEnabled = fts5Available(ctx, s.db)

	version, err := s.readSchemaVersion(ctx)
	if err != nil {
		return err
	}

	// The full-text index is an optional accelerator: any failure to build or
	// migrate it degrades to lexical search rather than breaking the store.
	if version < ftsExternalContentVersion {
		// Older databases indexed a contentful FTS5 table that could accumulate
		// duplicate rows and stored the searchable text twice. Rebuild it as an
		// external-content index, which also dedupes any existing drift.
		if err := s.migrateToExternalContentFTS(ctx); err != nil {
			log.Warnw("task memory FTS migration failed; falling back to lexical search", "error", err)
			s.ftsEnabled = false
			return nil
		}
		// Only advance the recorded version once the FTS layout is actually
		// current. Without FTS5 in this build we cannot rebuild the index, so we
		// leave the version behind and let a later FTS5-enabled open finish the
		// migration rather than marking it done prematurely.
		if s.ftsEnabled {
			if err := s.writeSchemaVersion(ctx, dbSchemaVersion); err != nil {
				return err
			}
		}
	} else if s.ftsEnabled {
		// Already migrated: ensure the external-content table exists (it should).
		if _, err := s.db.ExecContext(ctx, createTaskMemoryFTS); err != nil {
			log.Warnw("task memory FTS table unavailable; falling back to lexical search", "error", err)
			s.ftsEnabled = false
		}
	}
	return nil
}

// migrateToExternalContentFTS (re)builds the full-text index as an
// external-content FTS5 table. It drops any pre-existing index (legacy
// contentful layout, possibly carrying duplicate rows) and rebuilds it from
// task_memories, so the index ends up with exactly one entry per base row. It
// is a no-op when FTS5 is unavailable in this build.
func (s *SQLiteStore) migrateToExternalContentFTS(ctx context.Context) error {
	if !s.ftsEnabled {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `DROP TABLE IF EXISTS task_memory_fts`); err != nil {
		return fmt.Errorf("drop legacy task memory search index: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, createTaskMemoryFTS); err != nil {
		return fmt.Errorf("create task memory search index: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO task_memory_fts(task_memory_fts) VALUES('rebuild')`); err != nil {
		return fmt.Errorf("rebuild task memory search index: %w", err)
	}
	return nil
}

// readSchemaVersion returns the stored on-disk schema version, or 0 when the
// database predates versioning or the stored value is unrecognized.
func (s *SQLiteStore) readSchemaVersion(ctx context.Context) (int, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM task_memory_meta WHERE key = 'schema_version'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read task memory schema version: %w", err)
	}
	version, convErr := strconv.Atoi(strings.TrimSpace(raw))
	if convErr != nil {
		return 0, nil
	}
	return version, nil
}

func (s *SQLiteStore) writeSchemaVersion(ctx context.Context, version int) error {
	if _, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO task_memory_meta(key, value) VALUES('schema_version', ?)`, strconv.Itoa(version)); err != nil {
		return fmt.Errorf("write task memory schema version: %w", err)
	}
	return nil
}

// fts5Available reports whether the FTS5 module is compiled into the sqlite
// build. It probes with a temporary virtual table on a single pinned
// connection so the check never persists state or races the connection pool.
func fts5Available(ctx context.Context, db *sql.DB) bool {
	conn, err := db.Conn(ctx)
	if err != nil {
		return false
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `CREATE VIRTUAL TABLE IF NOT EXISTS temp.fts5_probe USING fts5(probe)`); err != nil {
		return false
	}
	_, _ = conn.ExecContext(ctx, `DROP TABLE IF EXISTS temp.fts5_probe`)
	return true
}

func (s *SQLiteStore) Add(ctx context.Context, rec Record) error {
	if strings.TrimSpace(rec.SearchableText) == "" {
		return nil
	}
	rec = normalizeRecord(rec)
	inputJSON, err := json.Marshal(rec.ToolInput)
	if err != nil {
		return fmt.Errorf("encode task memory tool input: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO task_memories(
		id, kind, source_run_id, source_seq, source_kind, os, arch, shell, cwd,
		tool_name, tool_input_json, final_output, searchable_text, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.Kind, rec.SourceRunID, rec.SourceSeq, rec.SourceKind, rec.OS, rec.Arch,
		rec.Shell, rec.CWD, rec.ToolName, string(inputJSON), truncate(rec.FinalOutput, maxIndexedOutput), rec.SearchableText, rec.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if s.ftsEnabled {
		// Index the FTS row only when the base row was actually inserted. The
		// external-content index is keyed by the base row's rowid, so re-adding a
		// record whose deterministic ID already exists (base insert ignored) must
		// not append a second index entry.
		inserted, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if inserted > 0 {
			rowID, err := res.LastInsertId()
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO task_memory_fts(rowid, searchable_text) VALUES (?, ?)`, rowID, rec.SearchableText); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func normalizeRecord(rec Record) Record {
	rec.Kind = strings.TrimSpace(rec.Kind)
	if rec.Kind == "" {
		rec.Kind = KindTaskCompletion
	}
	rec.OS = strings.TrimSpace(rec.OS)
	if rec.OS == "" {
		rec.OS = runtime.GOOS
	}
	rec.Arch = strings.TrimSpace(rec.Arch)
	if rec.Arch == "" {
		rec.Arch = runtime.GOARCH
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	rec.SearchableText = strings.TrimSpace(truncate(rec.SearchableText, maxIndexedOutput))
	rec.FinalOutput = strings.TrimSpace(truncate(rec.FinalOutput, maxIndexedOutput))
	if rec.ID == "" {
		rec.ID = deterministicID(rec)
	}
	return rec
}

func deterministicID(rec Record) string {
	input, _ := json.Marshal(rec.ToolInput)
	sum := sha256.Sum256([]byte(strings.Join([]string{rec.Kind, rec.OS, rec.SearchableText, rec.ToolName, string(input)}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func (s *SQLiteStore) Search(ctx context.Context, q Query) ([]Match, error) {
	q.Text = strings.TrimSpace(q.Text)
	if q.Text == "" {
		return nil, nil
	}
	if q.Limit <= 0 {
		q.Limit = defaultSearchLimit
	}
	if q.OS == "" {
		q.OS = runtime.GOOS
	}

	if s.ftsEnabled {
		matches, err := s.searchFTS(ctx, q)
		if err == nil {
			return matches, nil
		}
	}
	return s.searchLexical(ctx, q)
}

func (s *SQLiteStore) searchFTS(ctx context.Context, q Query) ([]Match, error) {
	matchQuery := ftsQuery(q.Text)
	if matchQuery == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT m.id, m.kind, m.source_run_id, m.source_seq, m.source_kind, m.os, m.arch, m.shell, m.cwd,
		m.tool_name, m.tool_input_json, m.final_output, m.searchable_text, m.created_at, bm25(task_memory_fts) AS rank
		FROM task_memory_fts JOIN task_memories m ON m.rowid = task_memory_fts.rowid
		WHERE task_memory_fts MATCH ?
		ORDER BY rank LIMIT ?`, matchQuery, max(q.Limit*8, q.Limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []Match
	for rows.Next() {
		rec, rank, err := scanRecordWithRank(rows)
		if err != nil {
			return nil, err
		}
		matches = append(matches, Match{Record: rec, Score: scoreRecord(rec, q, 10-rank)})
	}
	return topMatches(matches, q.Limit), rows.Err()
}

func (s *SQLiteStore) searchLexical(ctx context.Context, q Query) ([]Match, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, source_run_id, source_seq, source_kind, os, arch, shell, cwd,
		tool_name, tool_input_json, final_output, searchable_text, created_at FROM task_memories ORDER BY created_at DESC LIMIT 5000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	queryTokens := tokenSet(q.Text)
	var matches []Match
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		overlap := tokenOverlap(queryTokens, tokenSet(rec.SearchableText))
		if overlap <= 0 {
			continue
		}
		matches = append(matches, Match{Record: rec, Score: scoreRecord(rec, q, float64(overlap))})
	}
	return topMatches(matches, q.Limit), rows.Err()
}

// Recent returns the newest records for browsing, most recent first. It is the
// no-query counterpart to Search, used by list views such as the GUI memory tab.
func (s *SQLiteStore) Recent(ctx context.Context, limit int) ([]Record, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, source_run_id, source_seq, source_kind, os, arch, shell, cwd,
		tool_name, tool_input_json, final_output, searchable_text, created_at FROM task_memories ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []Record
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

type recordScanner interface{ Scan(dest ...any) error }

func scanRecordWithRank(rows *sql.Rows) (Record, float64, error) {
	var rank float64
	rec, err := scanRecordFields(rows, &rank)
	return rec, rank, err
}

func scanRecord(rows recordScanner) (Record, error) {
	return scanRecordFields(rows, nil)
}

func scanRecordFields(rows recordScanner, rank *float64) (Record, error) {
	var rec Record
	var inputJSON, createdAt string
	dest := []any{&rec.ID, &rec.Kind, &rec.SourceRunID, &rec.SourceSeq, &rec.SourceKind, &rec.OS, &rec.Arch, &rec.Shell, &rec.CWD, &rec.ToolName, &inputJSON, &rec.FinalOutput, &rec.SearchableText, &createdAt}
	if rank != nil {
		dest = append(dest, rank)
	}
	if err := rows.Scan(dest...); err != nil {
		return Record{}, err
	}
	_ = json.Unmarshal([]byte(inputJSON), &rec.ToolInput)
	rec.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return rec, nil
}

func scoreRecord(rec Record, q Query, base float64) float64 {
	score := base + kindBoost(rec.Kind)
	if rec.OS != "" && q.OS != "" {
		if rec.OS == q.OS {
			score += 3
		} else {
			score -= 8
		}
	}
	if q.IncludeCWD && rec.CWD != "" && q.CWD != "" && rec.CWD == q.CWD {
		score += 1
	}
	return score
}

func kindBoost(kind string) float64 {
	switch kind {
	case KindToolSuccess:
		return 5
	case KindTaskCompletion:
		return 2
	case KindTaskRequest:
		return 0
	default:
		return 0
	}
}

func topMatches(matches []Match, limit int) []Match {
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func ftsQuery(text string) string {
	tokens := tokens(text)
	if len(tokens) == 0 {
		return ""
	}
	if len(tokens) > 12 {
		tokens = tokens[:12]
	}
	return strings.Join(tokens, " OR ")
}

func tokens(text string) []string {
	raw := tokenPattern.FindAllString(strings.ToLower(text), -1)
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, token := range raw {
		token = strings.Trim(token, "-_$./~")
		if len(token) < 2 || seen[token] || stopword(token) {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}

func tokenSet(text string) map[string]bool {
	set := map[string]bool{}
	for _, token := range tokens(text) {
		set[token] = true
	}
	return set
}

func tokenOverlap(a, b map[string]bool) int {
	count := 0
	for token := range a {
		if b[token] {
			count++
		}
	}
	return count
}

func stopword(token string) bool {
	switch token {
	case "the", "and", "or", "to", "of", "in", "is", "it", "my", "me", "for", "with", "from", "this", "that", "what", "where", "how", "please", "give", "show":
		return true
	default:
		return false
	}
}

func (s *SQLiteStore) Stats(ctx context.Context) (Stats, error) {
	var stats Stats
	stats.FTSEnabled = s.ftsEnabled
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_memories`).Scan(&stats.Records); err != nil {
		return Stats{}, err
	}
	return stats, nil
}

func (s *SQLiteStore) Clear(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_memories`); err != nil {
		return err
	}
	if s.ftsEnabled {
		// External-content FTS5 tables do not support a plain DELETE; the
		// 'delete-all' command is the supported way to clear the index.
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_memory_fts(task_memory_fts) VALUES('delete-all')`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func FormatMatches(matches []Match) string {
	if len(matches) == 0 {
		return "No relevant task memories found."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Found %d relevant task memories:\n", len(matches))
	for i, match := range matches {
		rec := match.Record
		fmt.Fprintf(&b, "\n%d. %s", i+1, rec.Kind)
		if rec.OS != "" {
			fmt.Fprintf(&b, " (%s)", rec.OS)
		}
		if rec.ToolName != "" {
			fmt.Fprintf(&b, "\n   Tool: %s", rec.ToolName)
		}
		if len(rec.ToolInput) > 0 {
			encoded, _ := json.Marshal(rec.ToolInput)
			fmt.Fprintf(&b, "\n   Input: %s", encoded)
		}
		if rec.FinalOutput != "" {
			fmt.Fprintf(&b, "\n   Previous output: %s", oneLine(truncate(rec.FinalOutput, maxSnippetOutput)))
		}
		fmt.Fprintf(&b, "\n   Memory: %s\n", oneLine(truncate(rec.SearchableText, maxSnippetOutput)))
	}
	return strings.TrimRight(b.String(), "\n")
}

func BuildSearchableText(parts ...string) string {
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			lines = append(lines, part)
		}
	}
	return truncate(strings.Join(lines, "\n"), maxIndexedOutput)
}

func ToolInputJSON(input map[string]any) string {
	if len(input) == 0 {
		return "{}"
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return fmt.Sprint(input)
	}
	return string(encoded)
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "... [truncated]"
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
