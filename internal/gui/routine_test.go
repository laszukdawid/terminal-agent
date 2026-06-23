package gui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	appservice "github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/routines"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/assert"
)

func TestRoutineToolsWebSearchRoundTrip(t *testing.T) {
	assert.Nil(t, routineToolsFromWebSearch(false), "off means default policy (nil)")
	assert.False(t, routineToolsAllowWebSearch(nil))

	enabled := routineToolsFromWebSearch(true)
	assert.Contains(t, enabled, tools.ToolNameWebsearch)
	assert.Contains(t, enabled, tools.ToolNameRead)
	assert.True(t, routineToolsAllowWebSearch(enabled), "the produced list reads back as web-search-enabled")
}

func TestParseOptionalInt(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantNil bool
		want    int
		wantErr bool
	}{
		{name: "empty is nil", in: "  ", wantNil: true},
		{name: "valid", in: "500", want: 500},
		{name: "zero", in: "0", want: 0},
		{name: "invalid", in: "ten", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOptionalInt(tt.in)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			assert.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
		})
	}
}

func TestSplitNonEmptyLines(t *testing.T) {
	assert.Nil(t, splitNonEmptyLines("  \n  \n"))
	assert.Equal(t, []string{"a", "b"}, splitNonEmptyLines("a\n  \nb\n"))
}

func TestRoutineTextHelpers(t *testing.T) {
	assert.Equal(t, "(default)", orDefaultText("  "))
	assert.Equal(t, "gpt", orDefaultText("gpt"))

	assert.Equal(t, "(default)", routineBudgetText(nil))
	assert.Equal(t, "unlimited", routineBudgetText(ptr(0)))
	assert.Equal(t, "500", routineBudgetText(ptr(500)))

	assert.Equal(t, "default (external-facing disabled)", routineToolPolicyText(nil))
	assert.Equal(t, "(none)", routineToolPolicyText([]string{}))
	assert.Equal(t, "read, websearch", routineToolPolicyText([]string{"read", "websearch"}))

	assert.Equal(t, "never", routineTimeText(time.Time{}))
	assert.Equal(t, "named", routineTitle(routines.Routine{Name: "named", ID: "id"}))
	assert.Equal(t, "id", routineTitle(routines.Routine{ID: "id"}))
}

func TestRoutineStatusColorDistinct(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	palette := currentBrandPalette()
	assert.Equal(t, palette.accentGreen, routineStatusColor(routines.StatusActive))
	assert.Equal(t, palette.disabledText, routineStatusColor(routines.StatusInactive))
	assert.Equal(t, palette.error, routineStatusColor(routines.StatusError))
	assert.Equal(t, palette.warning, routineStatusColor(statusRunning))
}

func TestSubmitIgnoredInBrowseModes(t *testing.T) {
	// In a browse mode submit() must return before touching the (here nil) popup or
	// service, so no run starts. If the isBrowseMode guard regressed, this would
	// dereference the nil popup and panic.
	for _, mode := range []guiMode{guiModeHistory, guiModeRoutine} {
		g := &App{state: &state{mode: mode}}
		g.submit()
		assert.False(t, g.state.isRunning, "submit must not start a run in %s mode", mode)
	}
}

func TestRoutineToolsRepresentable(t *testing.T) {
	assert.True(t, routineToolsRepresentable(nil), "default policy is representable")
	assert.True(t, routineToolsRepresentable(routineToolsFromWebSearch(true)), "local+websearch set is representable")
	assert.False(t, routineToolsRepresentable([]string{"unix"}), "a custom list is not representable (preserved on edit)")
	assert.False(t, routineToolsRepresentable([]string{}), "an explicit empty list is custom")

	assert.True(t, equalStringSet([]string{"a", "b"}, []string{"b", "a"}))
	assert.False(t, equalStringSet([]string{"a"}, []string{"a", "b"}))
	assert.False(t, equalStringSet([]string{"a", "a"}, []string{"a", "b"}))
}

func TestSetRoutinesPopulatesBody(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	a.Settings().SetTheme(newBrandTheme()) // provides the bold/monospace font the cards use

	p := &popupWindow{routineBody: container.NewVBox()}

	tb := 100
	views := []appservice.RoutineView{
		{Routine: routines.Routine{ID: "a", Prompt: "do a", TokenBudget: &tb}, Status: routines.StatusActive, Frequency: "Manual"},
		{Routine: routines.Routine{ID: "b", Prompt: "do b"}, Status: routines.StatusInactive, Frequency: "0 9 * * *"},
	}
	p.setRoutines(views, "", nil, "")
	assert.Len(t, p.routineBody.Objects, 2)

	p.setRoutines(nil, "", nil, "")
	assert.Len(t, p.routineBody.Objects, 1, "empty state is a single element")

	p.setRoutines(nil, "boom", nil, "")
	assert.Len(t, p.routineBody.Objects, 1, "error state is a single element")
}

func TestParseOptionalNonNegativeInt(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantNil bool
		want    int
		wantErr bool
	}{
		{name: "empty is nil", in: "  ", wantNil: true},
		{name: "zero ok", in: "0", want: 0},
		{name: "positive ok", in: "500", want: 500},
		{name: "negative rejected", in: "-1", wantErr: true},
		{name: "non-numeric rejected", in: "x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOptionalNonNegativeInt(tt.in)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			assert.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	assert.Equal(t, "short", truncateRunes("short", 10))
	assert.Equal(t, "exactly10c", truncateRunes("exactly10c", 10))
	assert.Equal(t, "abcde…", truncateRunes("abcdefghij", 5))
}

func TestRoutineLogText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "jsonl transcript is pretty-printed and separated by a blank line",
			in:   `{"seq":1,"text":"a"}` + "\n" + `{"seq":2,"text":"b"}`,
			want: "{\n  \"seq\": 1,\n  \"text\": \"a\"\n}\n\n{\n  \"seq\": 2,\n  \"text\": \"b\"\n}",
		},
		{
			name: "blank and trailing lines are skipped",
			in:   `{"a":1}` + "\n\n" + `{"b":2}` + "\n",
			want: "{\n  \"a\": 1\n}\n\n{\n  \"b\": 2\n}",
		},
		{
			name: "non-json content (markdown summary) is returned unchanged",
			in:   "# Routine: Daily standup\n\n- Status: success",
			want: "# Routine: Daily standup\n\n- Status: success",
		},
		{
			name: "partially-json content falls back to the raw text",
			in:   `{"a":1}` + "\n" + "not json",
			want: `{"a":1}` + "\n" + "not json",
		},
		{
			name: "empty input is returned unchanged",
			in:   "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, routineLogText(tt.in))
		})
	}
}

func TestParseRoutineSummary(t *testing.T) {
	full := "# Routine: Projects staleness\n\n" +
		"- ID: projects-staleness\n- Schedule: 10 * * * *\n- Status: success\n\n" +
		"## Output\n\nOutdated vs each repo's default branch:\n- a: 1 commit behind\n"
	withError := "# Routine: Test\n\n- ID: test\n- Status: error\n\n## Error\n\nboom\n\n## Output\n\n(none)\n"
	nestedHeadings := "# Routine: T\n\n- ID: t\n\n## Output\n\nSummary\n\n## Details\n\nmore output\n"

	tests := []struct {
		name       string
		in         string
		wantTitle  string
		wantPairs  []string
		wantOutput string
		wantErr    string
		wantOK     bool
	}{
		{
			name:       "full summary splits title, metadata, and output",
			in:         full,
			wantTitle:  "Routine: Projects staleness",
			wantPairs:  []string{"ID: projects-staleness", "Schedule: 10 * * * *", "Status: success"},
			wantOutput: "Outdated vs each repo's default branch:\n- a: 1 commit behind",
			wantOK:     true,
		},
		{
			name:       "error section is captured separately from output",
			in:         withError,
			wantTitle:  "Routine: Test",
			wantPairs:  []string{"ID: test", "Status: error"},
			wantOutput: "(none)",
			wantErr:    "boom",
			wantOK:     true,
		},
		{
			name:       "headings inside the output are preserved, not split",
			in:         nestedHeadings,
			wantTitle:  "Routine: T",
			wantPairs:  []string{"ID: t"},
			wantOutput: "Summary\n\n## Details\n\nmore output",
			wantOK:     true,
		},
		{
			name:      "title and metadata with no output section still parse",
			in:        "# Routine: T\n\n- ID: t\n",
			wantTitle: "Routine: T",
			wantPairs: []string{"ID: t"},
			wantOK:    true,
		},
		{
			name:       "CRLF line endings parse cleanly",
			in:         "# Routine: T\r\n\r\n- ID: t\r\n- Status: success\r\n\r\n## Output\r\n\r\nall good\r\n",
			wantTitle:  "Routine: T",
			wantPairs:  []string{"ID: t", "Status: success"},
			wantOutput: "all good",
			wantOK:     true,
		},
		{
			name:       "an inline '## Output' (not a heading line) is not a section boundary",
			in:         "# Routine: T\n\n- ID: t\n\n## Output\n\nsee the ## Output note inline\n",
			wantTitle:  "Routine: T",
			wantPairs:  []string{"ID: t"},
			wantOutput: "see the ## Output note inline",
			wantOK:     true,
		},
		{
			name:   "no metadata bullets is not a recognized summary",
			in:     "# Routine: T\n\n## Output\n\nhi\n",
			wantOK: false,
		},
		{
			name:   "no title is not a recognized summary",
			in:     "- ID: t\n- Status: ok\n",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, pairs, output, errText, ok := parseRoutineSummary(tt.in)
			assert.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}
			assert.Equal(t, tt.wantTitle, title)
			assert.Equal(t, tt.wantPairs, pairs)
			assert.Equal(t, tt.wantOutput, output)
			assert.Equal(t, tt.wantErr, errText)
		})
	}
}
