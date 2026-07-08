package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeHistoryFile writes raw lines to a temp history file and returns its path.
func writeHistoryFile(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "query_log.jsonl")
	content := strings.Join(lines, "\n")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestHistoryQueryRobustReads(t *testing.T) {
	const goodLine = `{"method":"ask","timestamp":"2024-01-01T00:00:00Z","query":"q1","answer":"a1"}`
	const secondGoodLine = `{"method":"ask","timestamp":"2024-06-01T00:00:00Z","query":"q2","answer":"a2"}`
	bigAnswer := strings.Repeat("x", 200*1024) // > default 64 KB scanner limit
	bigLine := `{"method":"ask","timestamp":"2024-03-01T00:00:00Z","query":"big","answer":"` + bigAnswer + `"}`

	tests := []struct {
		name          string
		lines         []string
		wantQueries   []string
		wantBigAnswer bool
	}{
		{
			name:        "all valid",
			lines:       []string{goodLine, secondGoodLine},
			wantQueries: []string{"q1", "q2"},
		},
		{
			name:        "corrupt json line mid-file is skipped",
			lines:       []string{goodLine, "{not valid json", secondGoodLine},
			wantQueries: []string{"q1", "q2"},
		},
		{
			name:        "truncated last line is skipped",
			lines:       []string{goodLine, secondGoodLine, `{"method":"ask","timesta`},
			wantQueries: []string{"q1", "q2"},
		},
		{
			name:        "blank lines are ignored",
			lines:       []string{goodLine, "", "   ", secondGoodLine},
			wantQueries: []string{"q1", "q2"},
		},
		{
			name:        "entry with unparseable timestamp is skipped",
			lines:       []string{goodLine, `{"method":"ask","timestamp":"not-a-time","query":"bad","answer":"a"}`, secondGoodLine},
			wantQueries: []string{"q1", "q2"},
		},
		{
			name:          "entry larger than 64KB is read",
			lines:         []string{goodLine, bigLine},
			wantQueries:   []string{"q1", "big"},
			wantBigAnswer: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeHistoryFile(t, tc.lines)
			h := NewHistory(path)

			logs, err := h.Query(HistoryQuery{})
			require.NoError(t, err)

			gotQueries := make([]string, 0, len(logs))
			for _, l := range logs {
				gotQueries = append(gotQueries, l.Query)
			}
			assert.Equal(t, tc.wantQueries, gotQueries)

			if tc.wantBigAnswer {
				for _, l := range logs {
					if l.Query == "big" {
						assert.Len(t, l.Answer, len(bigAnswer))
					}
				}
			}
		})
	}
}

func TestHistoryQueryMissingFile(t *testing.T) {
	h := NewHistory(filepath.Join(t.TempDir(), "does-not-exist.jsonl"))
	_, err := h.Query(HistoryQuery{})
	assert.Error(t, err)
}
