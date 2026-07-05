package gui

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appservice "github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/taskmemory"
)

func seedTaskMemoryDB(t *testing.T, records []taskmemory.Record) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "task_memory.db")
	t.Setenv(appservice.TaskMemoryPathEnv, path)
	store, err := taskmemory.Open(path)
	require.NoError(t, err)
	defer store.Close()
	for _, rec := range records {
		require.NoError(t, store.Add(context.Background(), rec))
	}
}

func testMemoryRecords() []taskmemory.Record {
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	return []taskmemory.Record{
		{
			Kind:           taskmemory.KindToolSuccess,
			OS:             runtime.GOOS,
			ToolName:       "unix",
			ToolInput:      map[string]any{"command": "docker ps"},
			FinalOutput:    "CONTAINER ID  IMAGE",
			SearchableText: "Kind: tool_success\nUser request: list docker containers\nTool: unix\nInput: docker ps\nOutput: CONTAINER ID IMAGE",
			CreatedAt:      base,
		},
		{
			Kind:           taskmemory.KindTaskCompletion,
			OS:             runtime.GOOS,
			SearchableText: "Kind: task_completion\nUser request: what time is it\nFinal answer: noon",
			CreatedAt:      base.Add(time.Minute),
		},
	}
}

func TestSetTaskMemoriesPopulatesBody(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	a.Settings().SetTheme(newBrandTheme())

	records := testMemoryRecords()
	tests := []struct {
		name      string
		records   []taskmemory.Record
		query     string
		errorText string
		notice    string
		wantLen   int
	}{
		{name: "records render one card each", records: records, wantLen: 2},
		{name: "empty index is a single empty state", wantLen: 1},
		{name: "no matches is a single empty state", query: "nothing", wantLen: 1},
		{name: "error is a single label", errorText: "boom", wantLen: 1},
		{name: "notice adds one element above cards", records: records, notice: memoryIndexingDisabledNotice, wantLen: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &popupWindow{memoryBody: container.NewVBox()}
			p.setTaskMemories(tc.records, taskmemory.Stats{Records: len(tc.records)}, tc.query, tc.errorText, tc.notice)
			assert.Len(t, p.memoryBody.Objects, tc.wantLen)
		})
	}
}

func TestMemoryStatsText(t *testing.T) {
	tests := []struct {
		name  string
		stats taskmemory.Stats
		shown int
		query string
		want  string
	}{
		{
			name:  "browsing reports the run count",
			stats: taskmemory.Stats{Records: 2, FTSEnabled: true},
			shown: 1,
			want:  "2 records indexed" + metaSeparator + "1 recent run shown",
		},
		{
			name:  "browsing pluralizes runs",
			stats: taskmemory.Stats{Records: 80, FTSEnabled: true},
			shown: 12,
			want:  "80 records indexed" + metaSeparator + "12 recent runs shown",
		},
		{
			name:  "searching reports match count",
			stats: taskmemory.Stats{Records: 80, FTSEnabled: true},
			shown: 3,
			query: "docker",
			want:  "80 records indexed" + metaSeparator + "3 matches shown",
		},
		{
			name:  "missing FTS is called out",
			stats: taskmemory.Stats{Records: 2},
			shown: 0,
			want:  "2 records indexed" + metaSeparator + "full-text search unavailable",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, memoryStatsText(tc.stats, tc.shown, tc.query))
		})
	}
}

func TestMemoryTitleAndMeta(t *testing.T) {
	created := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	rec := taskmemory.Record{
		Kind:      taskmemory.KindToolSuccess,
		OS:        "linux",
		Arch:      "amd64",
		Shell:     "zsh",
		CWD:       "/tmp/project",
		ToolName:  "unix",
		CreatedAt: created,
	}

	title := memoryTitle(rec)
	assert.Contains(t, title, "TOOL SUCCESS")
	assert.Contains(t, title, created.Local().Format("2006-01-02 15:04"))

	assert.Equal(t, "MEMORY  unknown time", memoryTitle(taskmemory.Record{}))

	meta := memoryMeta(rec)
	assert.Contains(t, meta, "unix")
	assert.Contains(t, meta, "linux/amd64")
	assert.Contains(t, meta, "/tmp/project")

	detail := memoryDetailMeta(taskmemory.Record{Shell: "zsh", SourceRunID: "run-1", SourceSeq: 3})
	assert.Contains(t, detail, "shell: zsh")
	assert.Contains(t, detail, "run run-1 step 3")
}

func TestMemoryPreviewSourceDropsCardChromeLines(t *testing.T) {
	rec := taskmemory.Record{SearchableText: "Kind: tool_success\nUser request: list files\nTool: unix\nOS: darwin\nCWD: /tmp"}
	got := memoryPreviewSource(rec)
	assert.NotContains(t, got, "Kind: tool_success")
	assert.NotContains(t, got, "OS: darwin")
	assert.NotContains(t, got, "CWD: /tmp")
	assert.Contains(t, got, "User request: list files")
}

func TestParseMemoryFields(t *testing.T) {
	tests := []struct {
		name string
		text string
		want memoryFields
	}{
		{
			name: "step record with multiline output and empty error",
			text: "Kind: tool_success\nUser request: show calendar\nThought: run cal\nTool: unix\nInput: {\"command\":\"cal\"}\nOutput: July 2026\n 1 2 3\nError:\nOS: darwin\nCWD: /tmp",
			want: memoryFields{Request: "show calendar", Thought: "run cal", Output: "July 2026\n 1 2 3"},
		},
		{
			name: "request record",
			text: "Kind: task_request\nUser request: what time is it?\nOS: darwin\nCWD: /tmp",
			want: memoryFields{Request: "what time is it?"},
		},
		{
			name: "completion record",
			text: "Kind: task_completion\nUser request: what time is it?\nFinal answer: noon\nRaw output: 12:00\nTool: unix\nOS: darwin",
			want: memoryFields{Request: "what time is it?", Answer: "noon"},
		},
		{
			name: "unlabeled text yields nothing",
			text: "free-form note",
			want: memoryFields{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseMemoryFields(tc.text))
		})
	}
}

func TestMemoryCommandText(t *testing.T) {
	tests := []struct {
		name string
		rec  taskmemory.Record
		want string
	}{
		{
			name: "unix command",
			rec:  taskmemory.Record{ToolName: "unix", ToolInput: map[string]any{"command": "cal"}},
			want: "cal",
		},
		{
			name: "python code",
			rec:  taskmemory.Record{ToolName: "python", ToolInput: map[string]any{"code": "print(1)"}},
			want: "print(1)",
		},
		{
			name: "final answer input duplicates the body",
			rec:  taskmemory.Record{ToolName: "final_answer", ToolInput: map[string]any{"answer": "noon"}},
			want: "",
		},
		{
			name: "other tool shows name and input",
			rec:  taskmemory.Record{ToolName: "websearch", ToolInput: map[string]any{"query": "news"}},
			want: `websearch {"query":"news"}`,
		},
		{
			name: "bare tool name adds nothing",
			rec:  taskmemory.Record{ToolName: "unix"},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, memoryCommandText(tc.rec))
		})
	}
}

// collectCanvasTexts flattens the visible strings of a card tree so tests can
// assert on what a user would read.
func collectCanvasTexts(o fyne.CanvasObject) []string {
	switch v := o.(type) {
	case *fyne.Container:
		var out []string
		for _, child := range v.Objects {
			out = append(out, collectCanvasTexts(child)...)
		}
		return out
	case *elidedText:
		return []string{v.Text()}
	case *widget.Label:
		return []string{v.Text}
	case *widget.RichText:
		var b strings.Builder
		for _, seg := range v.Segments {
			b.WriteString(seg.Textual())
		}
		return []string{b.String()}
	case *tappableHistoryCard:
		return collectCanvasTexts(v.content)
	}
	return nil
}

func TestGroupMemoryRecords(t *testing.T) {
	newestFirst := []taskmemory.Record{
		{ID: "c", SourceRunID: "run-2"},
		{ID: "b", SourceRunID: "run-1"},
		{ID: "a", SourceRunID: "run-1"},
		{ID: "solo"},
		{},
	}
	groups := groupMemoryRecords(newestFirst)
	require.Len(t, groups, 4)
	assert.Equal(t, "run-2", groups[0].runID)
	assert.Len(t, groups[1].records, 2, "run-1 records merge into one group")
	assert.Equal(t, "b", groups[1].records[0].ID, "newest record stays first inside the group")
	assert.Len(t, groups[2].records, 1, "record without run id groups by its own id")
	assert.Len(t, groups[3].records, 1, "record without any identity stays alone")
}

func TestMemoryRunCardMergesRunRecords(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	a.Settings().SetTheme(newBrandTheme())

	base := time.Date(2026, 7, 5, 11, 51, 0, 0, time.UTC)
	run := memoryRunGroup{runID: "task-a1b2c3", records: []taskmemory.Record{
		{
			Kind: taskmemory.KindTaskCompletion, SourceRunID: "task-a1b2c3",
			OS: "darwin", Arch: "arm64", CWD: "/tmp",
			FinalOutput:    "July 2026 calendar",
			SearchableText: "Kind: task_completion\nUser request: show calendar\nFinal answer: July 2026 calendar",
			CreatedAt:      base.Add(3 * time.Second),
		},
		{
			Kind: taskmemory.KindTaskCompletion, SourceRunID: "task-a1b2c3", ToolName: "final_answer",
			ToolInput: map[string]any{"answer": "July 2026 calendar"}, FinalOutput: "July 2026 calendar",
			SearchableText: "Kind: task_completion\nUser request: show calendar\nTool: final_answer\nOutput: July 2026 calendar",
			CreatedAt:      base.Add(2 * time.Second),
		},
		{
			Kind: taskmemory.KindToolSuccess, SourceRunID: "task-a1b2c3", SourceSeq: 1, ToolName: "unix",
			ToolInput: map[string]any{"command": "cal"}, FinalOutput: "July 2026 calendar",
			SearchableText: "Kind: tool_success\nUser request: show calendar\nTool: unix\nInput: {\"command\":\"cal\"}\nOutput: July 2026 calendar",
			CreatedAt:      base.Add(time.Second),
		},
		{
			Kind: taskmemory.KindTaskRequest, SourceRunID: "task-a1b2c3",
			SearchableText: "Kind: task_request\nUser request: show calendar\nOS: darwin",
			CreatedAt:      base,
		},
	}}

	joined := strings.Join(collectCanvasTexts(memoryRunCardContent(run)), "\n")
	assert.Equal(t, 1, strings.Count(joined, "> show calendar"), "the request appears exactly once")
	assert.Equal(t, 1, strings.Count(joined, "$ cal"), "the command appears exactly once")
	assert.Equal(t, 1, strings.Count(joined, "July 2026 calendar"), "the answer appears exactly once")
	assert.Contains(t, joined, memoryRunTitleKind+"  ")
	assert.Contains(t, joined, "4 records")
	assert.NotContains(t, joined, "final_answer", "the delivery pseudo-tool is not a learned tool")
}

func TestMemoryRunCardShowsErrorForFailedRun(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	a.Settings().SetTheme(newBrandTheme())

	run := memoryRunGroup{runID: "task-d4e5f6", records: []taskmemory.Record{
		{
			Kind: taskmemory.KindToolFailure, SourceRunID: "task-d4e5f6", ToolName: "unix",
			ToolInput:      map[string]any{"command": "docker ps"},
			SearchableText: "Kind: tool_failure\nUser request: list containers\nTool: unix\nInput: {\"command\":\"docker ps\"}\nError: Cannot connect to the Docker daemon",
			CreatedAt:      time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC),
		},
	}}

	joined := strings.Join(collectCanvasTexts(memoryRunCardContent(run)), "\n")
	assert.Contains(t, joined, "> list containers")
	assert.Contains(t, joined, "$ docker ps")
	assert.Contains(t, joined, "Cannot connect to the Docker daemon")
}

func TestMemoryRunDetailShowsAndDismisses(t *testing.T) {
	seedTaskMemoryDB(t, nil)
	a := test.NewApp()
	defer a.Quit()
	g := NewApp(&recordingService{}, voiceGUIConfig{}, AppOptions{FyneApp: a})
	defer g.Quit()

	group := memoryRunGroup{runID: "run-1", records: testMemoryRecords()}
	g.popup.showMemoryRunDetail(group)
	require.NotNil(t, g.popup.memoryDetail)
	assert.True(t, g.popup.memoryDetail.Visible())
	assert.True(t, g.popup.dismissMemoryDetail())
}

func TestMemoryCardShowsIntent(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	a.Settings().SetTheme(newBrandTheme())

	tests := []struct {
		name     string
		rec      taskmemory.Record
		wantAll  []string
		wantNone []string
	}{
		{
			name: "tool success shows request, command, and output",
			rec: taskmemory.Record{
				Kind: taskmemory.KindToolSuccess, ToolName: "unix",
				ToolInput:      map[string]any{"command": "cal"},
				FinalOutput:    "July 2026\nSu Mo Tu",
				SearchableText: "Kind: tool_success\nUser request: show calendar\nThought: run cal\nTool: unix\nInput: {\"command\":\"cal\"}\nOutput: July 2026\nSu Mo Tu\nError:\nOS: darwin\nCWD: /tmp",
			},
			wantAll:  []string{"> show calendar", "$ cal", "July 2026"},
			wantNone: []string{"User request:", "Input:", "OS: darwin", "Error:"},
		},
		{
			name: "task completion shows request and answer",
			rec: taskmemory.Record{
				Kind: taskmemory.KindTaskCompletion, ToolName: "final_answer",
				ToolInput:      map[string]any{"answer": "11:40 AM PDT"},
				FinalOutput:    "11:40 AM PDT",
				SearchableText: "Kind: task_completion\nUser request: what time is it?\nThought: read the clock\nTool: final_answer\nInput: {\"answer\":\"11:40 AM PDT\"}\nOutput: 11:40 AM PDT\nOS: darwin",
			},
			wantAll:  []string{"> what time is it?", "11:40 AM PDT"},
			wantNone: []string{`{"answer"`, "$ "},
		},
		{
			name: "tool failure shows the error",
			rec: taskmemory.Record{
				Kind: taskmemory.KindToolFailure, ToolName: "unix",
				ToolInput:      map[string]any{"command": "docker ps"},
				SearchableText: "Kind: tool_failure\nUser request: list containers\nTool: unix\nInput: {\"command\":\"docker ps\"}\nOutput:\nError: Cannot connect to the Docker daemon\nOS: darwin",
			},
			wantAll: []string{"> list containers", "$ docker ps", "Cannot connect to the Docker daemon"},
		},
		{
			name: "task request shows only the query",
			rec: taskmemory.Record{
				Kind:           taskmemory.KindTaskRequest,
				SearchableText: "Kind: task_request\nUser request: show calendar\nOS: darwin\nCWD: /tmp",
			},
			wantAll:  []string{"> show calendar"},
			wantNone: []string{"OS:", "CWD: /tmp"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			joined := strings.Join(collectCanvasTexts(memoryCardContent(tc.rec)), "\n")
			for _, want := range tc.wantAll {
				assert.Contains(t, joined, want)
			}
			for _, none := range tc.wantNone {
				assert.NotContains(t, joined, none)
			}
		})
	}
}

func TestMemoryModeLoadsRecentAndSearches(t *testing.T) {
	seedTaskMemoryDB(t, testMemoryRecords())
	a := test.NewApp()
	defer a.Quit()
	g := NewApp(&recordingService{}, voiceGUIConfig{}, AppOptions{FyneApp: a})
	defer g.Quit()

	g.setMode(guiModeMemory)
	assert.Len(t, g.popup.memoryBody.Objects, 2, "entering the tab lists all seeded records")

	g.popup.memorySearch.SetText("docker containers")
	g.popup.submitMemorySearch()
	require.NotEmpty(t, g.popup.memoryBody.Objects)
	assert.Contains(t, g.popup.memoryStats.Text(), "matches shown")

	g.popup.memorySearch.SetText("")
	g.popup.submitMemorySearch()
	assert.Len(t, g.popup.memoryBody.Objects, 2, "clearing the query returns to recent browsing")
}

// The brand theme zeroes SizeNameInputBorder, which Fyne's Entry renderer also
// uses as the text cursor width. The search entry must therefore resolve to the
// native-cursor override or it focuses with an invisible cursor (see the GUI
// F.A.Q. in docs/developers.md).
func TestMemorySearchEntryRestoresNativeCursorWidth(t *testing.T) {
	seedTaskMemoryDB(t, nil)
	a := test.NewApp()
	defer a.Quit()
	a.Settings().SetTheme(newBrandTheme())
	g := NewApp(&recordingService{}, voiceGUIConfig{}, AppOptions{FyneApp: a})
	defer g.Quit()

	got := g.popup.memorySearch.Theme().Size(theme.SizeNameInputBorder)
	assert.Equal(t, float32(promptNativeCursorWidth), got, "memory search entry must be wrapped with withNativeCursor")
}

func TestMemoryDetailShowsAndDismisses(t *testing.T) {
	seedTaskMemoryDB(t, nil)
	a := test.NewApp()
	defer a.Quit()
	g := NewApp(&recordingService{}, voiceGUIConfig{}, AppOptions{FyneApp: a})
	defer g.Quit()

	rec := testMemoryRecords()[0]
	g.popup.showMemoryDetail(rec)
	require.NotNil(t, g.popup.memoryDetail)
	assert.True(t, g.popup.memoryDetail.Visible())

	assert.True(t, g.popup.dismissMemoryDetail())
	assert.False(t, g.popup.dismissMemoryDetail(), "second dismiss reports nothing to close")
}

func TestClearTaskMemoriesEmptiesIndex(t *testing.T) {
	seedTaskMemoryDB(t, testMemoryRecords())

	require.NoError(t, clearTaskMemories())

	records, stats, err := fetchTaskMemories("")
	require.NoError(t, err)
	assert.Empty(t, records)
	assert.Equal(t, 0, stats.Records)
}
