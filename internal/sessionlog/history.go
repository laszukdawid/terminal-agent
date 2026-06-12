package sessionlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

const scannerMaxTokenSize = 64 * 1024 * 1024

// Summary is the compact, user-facing shape of a per-run session log.
type Summary struct {
	RunID       string
	Kind        string
	Provider    string
	Model       string
	Cwd         string
	Command     string
	Request     string
	Response    string
	Error       string
	CreatedAt   time.Time
	CompletedAt time.Time
	Path        string
}

// Recent returns the most recent ask/task execution summaries from dir.
func Recent(dir string, limit int) ([]Summary, error) {
	if limit < 0 {
		limit = 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions directory: %w", err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))

	summaries := make([]Summary, 0, min(limit, len(paths)))
	for _, path := range paths {
		summary, ok, err := readSummary(path)
		if err != nil {
			logSkippedSession(path, err)
			continue
		}
		if !ok {
			continue
		}
		summaries = append(summaries, summary)
		if limit > 0 && len(summaries) >= limit {
			break
		}
	}
	return summaries, nil
}

func readSummary(path string) (Summary, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return Summary{}, false, fmt.Errorf("open session log %s: %w", path, err)
	}
	defer f.Close()

	summary := Summary{Path: path}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), scannerMaxTokenSize)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		rec, ok := parseSummaryRecord(path, []byte(line))
		if !ok {
			return Summary{}, false, nil
		}
		applyRecord(&summary, rec)
	}
	if err := scanner.Err(); err != nil {
		logSkippedSession(path, err)
		return Summary{}, false, nil
	}
	if summary.Kind != "ask" && summary.Kind != "task" {
		return Summary{}, false, nil
	}
	return summary, true, nil
}

type summaryRecordHeader struct {
	Type RecordType `json:"type"`
}

type summaryRecord struct {
	RunID     string     `json:"run_id"`
	Kind      string     `json:"kind"`
	Type      RecordType `json:"type"`
	Timestamp time.Time  `json:"timestamp"`
	Meta      *Meta      `json:"meta,omitempty"`
	Text      string     `json:"text,omitempty"`
	Error     string     `json:"error,omitempty"`
}

func parseSummaryRecord(path string, line []byte) (summaryRecord, bool) {
	var header summaryRecordHeader
	if err := json.Unmarshal(line, &header); err != nil {
		logSkippedSession(path, err)
		return summaryRecord{}, false
	}
	if !summaryRecordType(header.Type) {
		return summaryRecord{Type: header.Type}, true
	}
	var rec summaryRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		logSkippedSession(path, err)
		return summaryRecord{}, false
	}
	return rec, true
}

func summaryRecordType(recordType RecordType) bool {
	switch recordType {
	case RecordMeta, RecordRequest, RecordCompleted, RecordFailed:
		return true
	default:
		return false
	}
}

func logSkippedSession(path string, err error) {
	utils.GetLogger().Warn("skipping malformed session log",
		zap.String("path", path),
		zap.Error(err),
	)
}

func applyRecord(summary *Summary, rec summaryRecord) {
	if rec.Meta != nil {
		summary.RunID = rec.Meta.RunID
		summary.Kind = rec.Meta.Kind
		summary.Provider = rec.Meta.Provider
		summary.Model = rec.Meta.Model
		summary.Cwd = rec.Meta.Cwd
		summary.Command = rec.Meta.Command
		summary.CreatedAt = rec.Meta.CreatedAt
	}
	if summary.RunID == "" {
		summary.RunID = rec.RunID
	}
	if summary.Kind == "" {
		summary.Kind = rec.Kind
	}
	switch rec.Type {
	case RecordRequest:
		if summary.Request == "" {
			summary.Request = rec.Text
		}
	case RecordCompleted:
		summary.Response = rec.Text
		summary.CompletedAt = rec.Timestamp
	case RecordFailed:
		summary.Error = rec.Error
		summary.CompletedAt = rec.Timestamp
	}
}
