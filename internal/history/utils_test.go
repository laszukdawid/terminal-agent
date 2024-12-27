package history

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func timePtr(t time.Time) *time.Time {
	return &t
}

func TestAugmentHistoryQuery(t *testing.T) {
	cheatingAfter := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cheatingBefore := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		afterStr  string
		beforeStr string
		after     *time.Time
		before    *time.Time
	}{
		{"2024", "2025", &cheatingAfter, &cheatingBefore},
		{"2024", "", &cheatingAfter, nil},
		{"", "2025", nil, &cheatingBefore},
		{"invalid", "invalid", nil, nil},
		{"", "", nil, nil},
	}

	for _, test := range tests {
		t.Run(test.afterStr, func(t *testing.T) {
			hq := &HistoryQuery{
				AfterStr:  test.afterStr,
				BeforeStr: test.beforeStr,
			}
			err := augmentHistoryQuery(hq)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			assert.Equal(t, test.after, hq.After)
			assert.Equal(t, test.before, hq.Before)
		})
	}
}

func TestStrToTime(t *testing.T) {
	nowTime := time.Now()
	tests := []struct {
		input    string
		expected *time.Time
	}{
		{"2024", timePtr(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))},
		{"2024-01-01", timePtr(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))},
		{"12:34:56", timePtr(time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 12, 34, 56, 0, time.UTC))},
		{"2024-01-01T12:34:56", timePtr(time.Date(2024, 1, 1, 12, 34, 56, 0, time.UTC))},
		{nowTime.Format(time.RFC3339), timePtr(time.Date(nowTime.Year(), nowTime.Month(), nowTime.Day(), nowTime.Hour(), nowTime.Minute(), nowTime.Second(), 0, nowTime.Location()))},
		{"invalid", nil},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			actual, err := strToTime(test.input)
			if err != nil {
				if test.expected != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if actual == nil {
				t.Error("expected time, got nil")
				return
			}

			assert.Equal(t, test.expected, actual)
		})
	}

}
