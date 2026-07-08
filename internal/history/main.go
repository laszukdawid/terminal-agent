package history

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

type History interface {
	// Query interaction history
	Query(HistoryQuery) ([]HistoryLog, error)

	// Log the input and output to a file
	Log(method, query, answer string) error
}

type history struct {
	path string
}

func NewHistory(path string) History {
	return &history{path: path}
}

type HistoryQuery struct {
	AfterStr  string
	BeforeStr string
	After     *time.Time
	Before    *time.Time
}

type HistoryLog struct {
	Method    string `json:"method"`
	Timestamp string `json:"timestamp"`
	Query     string `json:"query"`
	Answer    string `json:"answer"`
}

func (h *history) Query(hQuery HistoryQuery) ([]HistoryLog, error) {
	logs, err := readAllLogs(h.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read all logs: %w", err)
	}

	// Augment the query with the parsed time
	err = augmentHistoryQuery(&hQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to augment history query: %w", err)
	}

	var filteredLogs []HistoryLog
	for _, log := range logs {
		t, err := time.Parse(time.RFC3339, log.Timestamp)
		if err != nil {
			// One corrupt entry must not fail the whole query; skip it.
			utils.Warn("skipping history log with unparseable timestamp",
				zap.String("path", h.path),
				zap.String("timestamp", log.Timestamp),
				zap.Error(err),
			)
			continue
		}

		// If `After` is set, skip if `t` is before `After`
		if hQuery.After != nil && hQuery.After.After(t) {
			continue
		}

		// If `Before` is set, skip if `t` is after `Before`
		if hQuery.Before != nil && hQuery.Before.Before(t) {
			continue
		}

		filteredLogs = append(filteredLogs, log)
	}

	return filteredLogs, nil
}

func readAllLogs(filePath string) ([]HistoryLog, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var logs []HistoryLog
	scanner := utils.NewJSONLScanner(file)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var log HistoryLog
		if err := json.Unmarshal(line, &log); err != nil {
			// A single corrupt append (e.g. a truncated line from a crash
			// mid-write) must not make the whole file unreadable; skip it.
			utils.Warn("skipping malformed history log line",
				zap.String("path", filePath),
				zap.Error(err),
			)
			continue
		}
		logs = append(logs, log)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return logs, nil
}

func (h *history) Log(method, query, answer string) error {
	l := HistoryLog{
		Method:    method,
		Query:     query,
		Answer:    answer,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if err := utils.WriteToJSONLFile(h.path, l); err != nil {
		return fmt.Errorf("failed to write to jsonl file: %w", err)
	}
	return nil
}
