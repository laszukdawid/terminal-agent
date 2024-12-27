package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/utils"
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
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
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

	var logs []HistoryLog
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var log HistoryLog
		if err := json.Unmarshal(scanner.Bytes(), &log); err != nil {
			return nil, fmt.Errorf("failed to unmarshal log: %w", err)
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
