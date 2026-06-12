package sessionlog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readRecords(t *testing.T, path string) []Record {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open session file: %v", err)
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("invalid jsonl line %q: %v", line, err)
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan session file: %v", err)
	}
	return records
}

func TestNewWritesMetaHeaderAndFilename(t *testing.T) {
	dir := t.TempDir()
	user := t.Name()

	rec := New(dir, Meta{Kind: "task", User: user, Provider: "openai", Model: "gpt-4o-mini", Command: "do the thing"})

	if rec.RunID() == "" {
		t.Fatal("expected a generated run id")
	}

	base := filepath.Base(rec.Path())
	if !strings.HasSuffix(base, ".jsonl") {
		t.Fatalf("expected .jsonl filename, got %q", base)
	}
	if !strings.Contains(base, "_task_") {
		t.Fatalf("expected kind in filename, got %q", base)
	}
	shortID := strings.ReplaceAll(rec.RunID(), "-", "")[:6]
	if !strings.Contains(base, shortID) {
		t.Fatalf("expected short id %q in filename %q", shortID, base)
	}

	records := readRecords(t, rec.Path())
	if len(records) != 1 {
		t.Fatalf("expected 1 record (meta header), got %d", len(records))
	}
	meta := records[0]
	if meta.Type != RecordMeta {
		t.Fatalf("expected first record to be meta, got %q", meta.Type)
	}
	if meta.Seq != 1 {
		t.Fatalf("expected meta seq 1, got %d", meta.Seq)
	}
	if meta.Meta == nil || meta.Meta.User != user || meta.Meta.Provider != "openai" {
		t.Fatalf("meta header missing provenance: %+v", meta.Meta)
	}
	if meta.Meta.RunID != rec.RunID() {
		t.Fatalf("meta run id %q != recorder run id %q", meta.Meta.RunID, rec.RunID())
	}
}

func TestWriteAssignsMonotonicSeqAndDefaults(t *testing.T) {
	dir := t.TempDir()
	rec := New(dir, Meta{Kind: "ask"})

	rec.Write(Record{Type: RecordRequest, Text: "hello"})
	rec.Write(Record{Type: RecordCompleted, Text: "world"})

	records := readRecords(t, rec.Path())
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
	for i, r := range records {
		if r.Seq != i+1 {
			t.Fatalf("record %d has seq %d, expected %d", i, r.Seq, i+1)
		}
		if r.RunID != rec.RunID() {
			t.Fatalf("record %d missing run id", i)
		}
		if r.Kind != "ask" {
			t.Fatalf("record %d kind = %q, expected ask", i, r.Kind)
		}
		if r.Timestamp.IsZero() {
			t.Fatalf("record %d missing timestamp", i)
		}
	}
	if records[1].Text != "hello" || records[2].Text != "world" {
		t.Fatalf("unexpected record order/content: %+v", records)
	}
}

func TestNilRecorderWriteIsNoop(t *testing.T) {
	var rec *Recorder
	rec.Write(Record{Type: RecordRequest}) // must not panic
}

func TestRecentSummarizesAskAndTaskSessions(t *testing.T) {
	dir := t.TempDir()
	askTime := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	taskTime := askTime.Add(time.Hour)

	ask := New(dir, Meta{Kind: "ask", Provider: "openai", Model: "gpt-4.1", Command: "what model?", CreatedAt: askTime})
	ask.Write(Record{Type: RecordRequest, Text: "what model?"})
	ask.Write(Record{Type: RecordToolResult, ToolName: "unix", ToolResult: strings.Repeat("large output ", 1024)})
	ask.Write(Record{Type: RecordCompleted, Text: "gpt-4.1", Timestamp: askTime.Add(time.Second)})

	task := New(dir, Meta{Kind: "task", Provider: "anthropic", Model: "claude", Command: "list files", CreatedAt: taskTime})
	task.Write(Record{Type: RecordRequest, Text: "list files"})
	task.Write(Record{Type: RecordCompleted, Text: "README.md", Timestamp: taskTime.Add(2 * time.Second)})

	chat := New(dir, Meta{Kind: "chat", CreatedAt: taskTime.Add(time.Hour)})
	chat.Write(Record{Type: RecordRequest, Text: "ignored"})

	runs, err := Recent(dir, 10)
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("Recent() returned %d runs, want 2", len(runs))
	}
	if runs[0].Kind != "task" || runs[0].Request != "list files" || runs[0].Response != "README.md" {
		t.Fatalf("first summary = %+v, want task run", runs[0])
	}
	if runs[1].Kind != "ask" || runs[1].Provider != "openai" || runs[1].Response != "gpt-4.1" {
		t.Fatalf("second summary = %+v, want ask run", runs[1])
	}
}

func TestRecentMissingDirectoryIsEmpty(t *testing.T) {
	runs, err := Recent(filepath.Join(t.TempDir(), "missing"), 10)
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("Recent() returned %d runs, want empty", len(runs))
	}
}

func TestRecentSkipsMalformedSessionLogs(t *testing.T) {
	dir := t.TempDir()
	createdAt := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	rec := New(dir, Meta{Kind: "ask", Provider: "openai", Model: "gpt-4.1", Command: "hello", CreatedAt: createdAt})
	rec.Write(Record{Type: RecordRequest, Text: "hello"})
	rec.Write(Record{Type: RecordCompleted, Text: "world", Timestamp: createdAt.Add(time.Second)})

	badPath := filepath.Join(dir, "2026-06-07T13-00-00_ask_badlog.jsonl")
	if err := os.WriteFile(badPath, []byte(`{"type":"meta"}`+"\n"+`{not json`+"\n"), 0644); err != nil {
		t.Fatalf("write malformed session log: %v", err)
	}

	runs, err := Recent(dir, 10)
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("Recent() returned %d runs, want 1 valid run", len(runs))
	}
	if runs[0].Request != "hello" || runs[0].Response != "world" {
		t.Fatalf("summary = %+v, want valid run", runs[0])
	}
}
