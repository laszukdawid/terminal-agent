package sessionlog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
