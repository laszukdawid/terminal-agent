package history

import (
	"fmt"
	"time"
)

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
		layout = "2006-01-01"
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
