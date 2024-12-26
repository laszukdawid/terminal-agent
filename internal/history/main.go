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

func augmentHistoryQuery(hq *HistoryQuery) error {

	if after, err := strToTime(hq.AfterStr); err == nil {
		hq.After = after
	}

	if before, err := strToTime(hq.BeforeStr); err == nil {
		hq.Before = before
	}

	return nil
}

func strToTime(input string) (*time.Time, error) {
	var layout string
	if len(input) == 4 {
		// Input is a year, convert to YYYY-01-01
		layout = "2006"
		input += "-01-01"
	} else if len(input) == 10 {
		// Input is already in YYYY-MM-DD format
		layout = "2006-01-02"
	} else if len(input) == 8 {
		// Input is in HH:MM:SS format, that's today
		layout = time.TimeOnly
	} else if len(input) == len("2006-01-02T15:04:05") {
		// Input is in RFC3339 format without time zone (local)
		layout = "2006-01-02T15:04:05"
	} else if len(input) == len(time.RFC3339) {
		// Input is in RFC3339 format
		layout = time.RFC3339
	} else {
		return nil, fmt.Errorf("invalid date format: %s", input)
	}

	t, err := time.Parse(layout, input)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date: %w", err)
	}

	// If the input was in TimeOnly format, set the date to today
	if layout == time.TimeOnly {
		now := time.Now()
		t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	}

	return &t, nil
}
