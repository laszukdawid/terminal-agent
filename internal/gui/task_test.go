package gui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	appservice "github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/sessionlog"
)

// recordingService captures the most recent Ask/Task request and returns a
// pre-closed event channel so the consume goroutine exits immediately, letting
// tests assert on what was dispatched without driving the fyne event loop.
type recordingService struct {
	mu          sync.Mutex
	askCalls    int
	taskCalls   int
	lastAskReq  appservice.AskRequest
	lastTaskReq appservice.TaskRequest
}

func closedEvents() <-chan appservice.Event {
	ch := make(chan appservice.Event)
	close(ch)
	return ch
}

func (s *recordingService) Ask(context.Context, appservice.AskRequest) (appservice.AskResult, error) {
	return appservice.AskResult{}, nil
}

func (s *recordingService) AskEvents(_ context.Context, req appservice.AskRequest) (<-chan appservice.Event, error) {
	s.mu.Lock()
	s.askCalls++
	s.lastAskReq = req
	s.mu.Unlock()
	return closedEvents(), nil
}

func (s *recordingService) Chat(context.Context, appservice.ChatRequest) (appservice.ChatResult, error) {
	return appservice.ChatResult{}, nil
}

func (s *recordingService) ChatEvents(context.Context, appservice.ChatRequest) (<-chan appservice.Event, error) {
	return closedEvents(), nil
}

func (s *recordingService) TaskEvents(_ context.Context, req appservice.TaskRequest) (<-chan appservice.Event, error) {
	s.mu.Lock()
	s.taskCalls++
	s.lastTaskReq = req
	s.mu.Unlock()
	return closedEvents(), nil
}

func newRecordingApp(t *testing.T) (*App, *recordingService) {
	t.Helper()
	service := &recordingService{}
	g := NewApp(service, voiceGUIConfig{}, AppOptions{
		AppID:   "terminal-agent-task-test",
		FyneApp: test.NewApp(),
	})
	t.Cleanup(func() { g.fyneApp.Quit() })
	return g, service
}

func TestDefaultModeIsAsk(t *testing.T) {
	g, _ := newRecordingApp(t)
	if g.state.mode != guiModeAsk {
		t.Fatalf("default mode = %q, want %q", g.state.mode, guiModeAsk)
	}
}

func TestSidebarLabelsIncludeHistory(t *testing.T) {
	g, _ := newRecordingApp(t)
	if g.popup.navAsk.label != "ASK" {
		t.Fatalf("ask nav label = %q, want ASK", g.popup.navAsk.label)
	}
	if g.popup.navTask.label != "TASK" {
		t.Fatalf("task nav label = %q, want TASK", g.popup.navTask.label)
	}
	if g.popup.navHistory.label != sectionHistory {
		t.Fatalf("history nav label = %q, want %q", g.popup.navHistory.label, sectionHistory)
	}
}

func TestDisplayCwdDoesNotTruncateLongPath(t *testing.T) {
	path := "/tmp/.local/config/terminal-agent"
	if got := displayCwd(path); got != path {
		t.Fatalf("displayCwd() = %q, want %q", got, path)
	}
}

func TestMarqueeDoubleTapCopiesFullText(t *testing.T) {
	path := "~/.local/config/terminal-agent"
	label := newMarqueeLabel(path, brandMutedGreen, 12)
	var copied string
	label.SetOnCopy(func(text string) {
		copied = text
	})
	label.Resize(label.MinSize())
	label.CreateRenderer().Layout(fyne.NewSize(80, label.MinSize().Height))
	label.advance()

	label.DoubleTapped(nil)

	if copied != path {
		t.Fatalf("copied marquee text = %q, want %q", copied, path)
	}
}

func TestMarqueeTapCopiesFullText(t *testing.T) {
	path := "~/.local/config/terminal-agent"
	label := newMarqueeLabel(path, brandMutedGreen, 12)
	var copied string
	label.SetOnCopy(func(text string) {
		copied = text
	})

	label.Tapped(nil)

	if copied != path {
		t.Fatalf("copied marquee text = %q, want %q", copied, path)
	}
}

func TestSetModeTogglesModeAndSidebar(t *testing.T) {
	g, _ := newRecordingApp(t)

	g.setMode(guiModeTask)
	if g.state.mode != guiModeTask {
		t.Fatalf("mode after select Task = %q, want %q", g.state.mode, guiModeTask)
	}
	if !g.popup.navTask.active || g.popup.navAsk.active {
		t.Fatalf("sidebar active rows wrong: ask=%v task=%v", g.popup.navAsk.active, g.popup.navTask.active)
	}
	if g.popup.inputHeading.Text != sectionTask {
		t.Fatalf("input heading = %q, want %q", g.popup.inputHeading.Text, sectionTask)
	}
	if g.popup.actionSubtitle.Text != autoApproveHintText {
		t.Fatalf("action subtitle = %q, want %q", g.popup.actionSubtitle.Text, autoApproveHintText)
	}

	g.setMode(guiModeAsk)
	if g.state.mode != guiModeAsk {
		t.Fatalf("mode after select Ask = %q, want %q", g.state.mode, guiModeAsk)
	}
	if g.popup.navTask.active || !g.popup.navAsk.active {
		t.Fatalf("sidebar active rows wrong after revert: ask=%v task=%v", g.popup.navAsk.active, g.popup.navTask.active)
	}
	if g.popup.inputHeading.Text != sectionAsk {
		t.Fatalf("input heading = %q, want %q", g.popup.inputHeading.Text, sectionAsk)
	}
	if g.popup.actionSubtitle.Text != "" {
		t.Fatalf("action subtitle after Ask = %q, want empty", g.popup.actionSubtitle.Text)
	}
}

func TestSetModeHistoryLoadsSessionHistory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(appservice.SessionDirEnv, dir)
	rec := sessionlog.New(dir, sessionlog.Meta{Kind: "ask", Provider: "openai", Model: "gpt-4.1", Command: "hello", CreatedAt: completedAtForTest()})
	rec.Write(sessionlog.Record{Type: sessionlog.RecordRequest, Text: "hello"})
	rec.Write(sessionlog.Record{Type: sessionlog.RecordCompleted, Text: "world", Timestamp: completedAtForTest().Add(time.Second)})
	g, _ := newRecordingApp(t)

	g.setMode(guiModeHistory)

	if g.state.mode != guiModeHistory {
		t.Fatalf("mode after select History = %q, want %q", g.state.mode, guiModeHistory)
	}
	if !g.popup.navHistory.active || g.popup.navAsk.active || g.popup.navTask.active {
		t.Fatalf("sidebar active rows wrong: ask=%v task=%v history=%v", g.popup.navAsk.active, g.popup.navTask.active, g.popup.navHistory.active)
	}
	if !g.popup.historySection.Visible() || g.popup.inputGroup.Visible() || g.popup.responseSection.Visible() {
		t.Fatal("history mode should show history section and hide prompt/response sections")
	}
	if len(g.popup.historyBody.Objects) != 1 {
		t.Fatalf("history rows = %d, want 1", len(g.popup.historyBody.Objects))
	}
}

func TestHistoryFullResponseIsNotTruncated(t *testing.T) {
	long := strings.Repeat("response ", 80)
	run := sessionlog.Summary{Response: long}

	if got := historyFullResponse(run); got != strings.TrimSpace(long) {
		t.Fatalf("historyFullResponse() length = %d, want full length %d", len(got), len(strings.TrimSpace(long)))
	}
	preview, truncated := historyPreview(long, "")
	if !truncated {
		t.Fatal("historyPreview should report truncated content")
	}
	if len([]rune(preview)) >= len([]rune(strings.TrimSpace(long))) {
		t.Fatalf("historyPreview should remain compact; preview len=%d full len=%d", len([]rune(preview)), len([]rune(strings.TrimSpace(long))))
	}
}

func TestHistoryPreviewReportsWhetherContentWasTruncated(t *testing.T) {
	preview, truncated := historyPreview("short response", "fallback")
	if truncated || preview != "short response" {
		t.Fatalf("short preview = %q truncated=%v, want untruncated short response", preview, truncated)
	}

	preview, truncated = historyPreview("", "fallback")
	if truncated || preview != "fallback" {
		t.Fatalf("fallback preview = %q truncated=%v, want untruncated fallback", preview, truncated)
	}

	if historyTruncationCorner() == nil {
		t.Fatal("historyTruncationCorner() returned nil")
	}
}

func TestHistoryDetailPopupSizeFitsCanvas(t *testing.T) {
	size := historyDetailPopupSize(fyne.NewSize(640, 360))
	if size.Width > 640-historyDetailDialogMargin {
		t.Fatalf("popup width = %f, want within canvas margin", size.Width)
	}
	if size.Height > 360-historyDetailDialogMargin {
		t.Fatalf("popup height = %f, want within canvas margin", size.Height)
	}

	defaultSize := historyDetailPopupSize(fyne.NewSize(1200, 900))
	if defaultSize.Height != historyDetailDialogHeight {
		t.Fatalf("default popup height = %f, want %f", defaultSize.Height, float32(historyDetailDialogHeight))
	}
}

func TestHistoryCardIsTappable(t *testing.T) {
	tapped := false
	card, ok := newHistoryCard(sessionlog.Summary{Kind: "ask", Request: "hello", Response: "world"}, func() {
		tapped = true
	}).(*tappableHistoryCard)
	if !ok {
		t.Fatalf("newHistoryCard type = %T, want *tappableHistoryCard", card)
	}

	card.Tapped(nil)

	if !tapped {
		t.Fatal("history card tap did not invoke callback")
	}
}

func TestSetModeRestoresModeSpecificInputOutputAndExport(t *testing.T) {
	g, _ := newRecordingApp(t)

	g.state.input = "ask input"
	g.state.question = "ask question"
	g.state.output = "ask response"
	g.state.completedAt = completedAtForTest()
	g.render()

	g.setMode(guiModeTask)
	if g.state.input != "" || g.state.question != "" || g.state.responseText() != "" {
		t.Fatalf("new Task view should be empty: input=%q question=%q output=%q", g.state.input, g.state.question, g.state.responseText())
	}

	g.state.input = "task input"
	g.state.question = "task question"
	g.state.appendTaskFinalText("task response")
	g.state.completedAt = completedAtForTest()
	g.render()

	g.setMode(guiModeAsk)
	if g.state.input != "ask input" || g.state.question != "ask question" || g.state.responseText() != "ask response" {
		t.Fatalf("Ask view was not restored: input=%q question=%q output=%q", g.state.input, g.state.question, g.state.responseText())
	}
	if got := g.exportContent(g.state.responseText()); !strings.Contains(got, "# Ask\n\nask question") {
		t.Fatalf("Ask export used wrong mode/question:\n%s", got)
	}

	g.setMode(guiModeTask)
	if g.state.input != "task input" || g.state.question != "task question" || g.state.responseText() != "task response" {
		t.Fatalf("Task view was not restored: input=%q question=%q output=%q", g.state.input, g.state.question, g.state.responseText())
	}
	if got := g.exportContent(g.state.responseText()); !strings.Contains(got, "# Task\n\ntask question") {
		t.Fatalf("Task export used wrong mode/question:\n%s", got)
	}
}

func completedAtForTest() time.Time {
	return time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
}

func TestSetModeIgnoredWhileRunning(t *testing.T) {
	g, _ := newRecordingApp(t)
	g.state.isRunning = true

	g.setMode(guiModeTask)
	if g.state.mode != guiModeAsk {
		t.Fatalf("mode changed while running: got %q, want %q", g.state.mode, guiModeAsk)
	}
}

func TestSubmitAskDispatchesAskRequest(t *testing.T) {
	g, service := newRecordingApp(t)
	g.state.mode = guiModeAsk
	g.popup.input.SetText("what is this?")

	g.submit()

	if service.askCalls != 1 {
		t.Fatalf("AskEvents calls = %d, want 1", service.askCalls)
	}
	if service.taskCalls != 0 {
		t.Fatalf("TaskEvents calls = %d, want 0", service.taskCalls)
	}
	req := service.lastAskReq
	if req.Message != "what is this?" {
		t.Fatalf("ask message = %q, want %q", req.Message, "what is this?")
	}
	if !req.Stream {
		t.Fatal("ask request should stream")
	}
	if req.Provider != "openai" || req.Model != "gpt-4o-mini" {
		t.Fatalf("ask provider/model = %q/%q", req.Provider, req.Model)
	}
}

func TestSubmitTaskDispatchesTaskRequest(t *testing.T) {
	g, service := newRecordingApp(t)
	g.state.mode = guiModeTask
	g.popup.input.SetText("list the files")

	g.submit()

	if service.taskCalls != 1 {
		t.Fatalf("TaskEvents calls = %d, want 1", service.taskCalls)
	}
	if service.askCalls != 0 {
		t.Fatalf("AskEvents calls = %d, want 0", service.askCalls)
	}
	req := service.lastTaskReq
	if req.Message != "list the files" {
		t.Fatalf("task message = %q, want %q", req.Message, "list the files")
	}
	if !req.AutoApprove {
		t.Fatal("task request must set AutoApprove: true")
	}
	if req.Provider != "openai" || req.Model != "gpt-4o-mini" {
		t.Fatalf("task provider/model = %q/%q", req.Provider, req.Model)
	}
	if req.Device != "auto" {
		t.Fatalf("task device = %q, want auto", req.Device)
	}
	if req.Config == nil {
		t.Fatal("task request should carry config")
	}
}

func TestSubmitEmptyInputUsesModeAwareMessage(t *testing.T) {
	g, service := newRecordingApp(t)
	g.state.mode = guiModeTask
	g.popup.input.SetText("   ")

	g.submit()

	if service.taskCalls != 0 || service.askCalls != 0 {
		t.Fatal("empty submit should not dispatch a request")
	}
	if g.state.errorText != "Task cannot be empty." {
		t.Fatalf("error text = %q, want %q", g.state.errorText, "Task cannot be empty.")
	}
}

func TestExportContentUsesModeAwareHeading(t *testing.T) {
	cases := []struct {
		name string
		mode guiMode
		want string
	}{
		{name: "ask", mode: guiModeAsk, want: "# Ask\n\n"},
		{name: "task", mode: guiModeTask, want: "# Task\n\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &App{cfg: voiceGUIConfig{}, state: &state{mode: tc.mode, question: "do it"}}
			got := g.exportContent("body")
			if !strings.Contains(got, tc.want) {
				t.Fatalf("exportContent missing %q in:\n%s", tc.want, got)
			}
		})
	}
}

func TestInputAndExportHeadingHelpers(t *testing.T) {
	if got := inputHeadingForMode(guiModeAsk); got != sectionAsk {
		t.Fatalf("inputHeadingForMode(ask) = %q", got)
	}
	if got := inputHeadingForMode(guiModeTask); got != sectionTask {
		t.Fatalf("inputHeadingForMode(task) = %q", got)
	}
	if got := emptyInputMessage(guiModeAsk); got != "Question cannot be empty." {
		t.Fatalf("emptyInputMessage(ask) = %q", got)
	}
	if got := emptyInputMessage(guiModeTask); got != "Task cannot be empty." {
		t.Fatalf("emptyInputMessage(task) = %q", got)
	}
}

func TestNavRowSetActiveTogglesStyling(t *testing.T) {
	row := newNavRow("TASK", iconPathTask, false, nil)

	row.setActive(true)
	if !row.active || !row.text.TextStyle.Bold {
		t.Fatalf("setActive(true): active=%v bold=%v", row.active, row.text.TextStyle.Bold)
	}
	if row.text.Color != brandAccentGreen {
		t.Fatal("active row text should use accent green")
	}

	row.setActive(false)
	if row.active || row.text.TextStyle.Bold {
		t.Fatalf("setActive(false): active=%v bold=%v", row.active, row.text.TextStyle.Bold)
	}
	if row.text.Color != brandSecondaryText {
		t.Fatal("inactive row text should use secondary color")
	}
}

func TestIsMeaningfulTaskStatus(t *testing.T) {
	cases := map[string]bool{
		string(agent.TaskStatusRunningTool): true,
		string(agent.TaskStatusThinking):    false,
		string(agent.TaskStatusFinalizing):  false,
		"":                                  false,
	}
	for phase, want := range cases {
		if got := isMeaningfulTaskStatus(phase); got != want {
			t.Fatalf("isMeaningfulTaskStatus(%q) = %v, want %v", phase, got, want)
		}
	}
}

func TestTaskStatusDisplay(t *testing.T) {
	thinking := appservice.Event{Status: string(agent.TaskStatusThinking), Text: "Thinking..."}
	if got := taskStatusDisplay(thinking); got != "Thinking..." {
		t.Fatalf("thinking phase display = %q, want message text", got)
	}
	running := appservice.Event{Status: string(agent.TaskStatusRunningTool), Text: "Running unix..."}
	if got := taskStatusDisplay(running); got != "Running unix..." {
		t.Fatalf("running phase display = %q, want message text", got)
	}
	noText := appservice.Event{Status: string(agent.TaskStatusFinalizing)}
	if got := taskStatusDisplay(noText); got != string(agent.TaskStatusFinalizing) {
		t.Fatalf("empty-text display = %q, want phase string", got)
	}
}

func TestFormatTaskToolProgress(t *testing.T) {
	cases := []struct {
		name string
		ev   appservice.Event
		want string
	}{
		{name: "tool and text", ev: appservice.Event{ToolName: "unix", Text: "running ls"}, want: "unix: running ls"},
		{name: "text only", ev: appservice.Event{Text: "working"}, want: "working"},
		{name: "tool only", ev: appservice.Event{ToolName: "python"}, want: "python"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatTaskToolProgress(tc.ev); got != tc.want {
				t.Fatalf("formatTaskToolProgress = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShouldAppendTaskFinalOutput(t *testing.T) {
	cases := []struct {
		name  string
		event appservice.Event
		state *state
		want  bool
	}{
		{
			name:  "final output present",
			event: appservice.Event{FinalOutput: "done"},
			state: &state{},
			want:  true,
		},
		{
			name:  "direct raw already streamed is skipped",
			event: appservice.Event{DirectRawOutput: true, RawOutputTool: "unix", RawOutput: "x"},
			state: &state{taskSawLiveOutput: true, taskLiveOutputTools: map[string]bool{"unix": true}},
			want:  false,
		},
		{
			name:  "direct raw truncated is not suppressed",
			event: appservice.Event{DirectRawOutput: true, RawOutputTool: "unix", RawOutput: "x"},
			state: &state{
				taskSawLiveOutput:            true,
				taskLiveOutputTools:          map[string]bool{"unix": true},
				taskLiveOutputTruncatedTools: map[string]bool{"unix": true},
			},
			want: true,
		},
		{
			name:  "raw fallback when nothing streamed",
			event: appservice.Event{RawOutput: "x"},
			state: &state{taskLiveOutputTools: map[string]bool{}},
			want:  true,
		},
		{
			name:  "raw suppressed when live output streamed",
			event: appservice.Event{RawOutput: "x"},
			state: &state{taskSawLiveOutput: true, taskLiveOutputTools: map[string]bool{}},
			want:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldAppendTaskFinalOutput(tc.event, tc.state); got != tc.want {
				t.Fatalf("shouldAppendTaskFinalOutput = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTaskTranscriptStreamingSerializesToCurrentMarkdownShape(t *testing.T) {
	s := &state{}
	s.resetTaskStreaming()

	// A tool-call status line, then live output for that tool, then completion.
	s.appendTaskTranscriptLine("Running unix...")
	s.taskSawLiveOutput = true
	s.taskLiveOutputTools["unix"] = true
	s.openTaskToolBlock(7, 0)
	s.appendTaskOutput("file1\n")
	s.openTaskToolBlock(7, 0) // same process: must not open a second fence
	s.appendTaskOutput("file2\n")
	s.closeTaskToolBlock()
	s.appendTaskFinalText("All done.")

	got := s.responseText()
	if strings.Count(got, taskToolFenceMarker) != 2 {
		t.Fatalf("expected exactly one fenced block (2 markers), got:\n%s", got)
	}
	for _, want := range []string{"Running unix...", "file1", "file2", "All done.", "\n---\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("transcript missing %q in:\n%s", want, got)
		}
	}
	if s.taskToolOpen {
		t.Fatal("tool block should be closed after closeTaskToolBlock")
	}
}

func TestAppendTaskOutputRespectsLineCap(t *testing.T) {
	s := &state{}
	s.resetTaskStreaming()
	s.openTaskToolBlock(1, 2) // cap at 2 lines
	s.appendTaskOutput("one\ntwo\nthree\nfour\n")

	got := s.responseText()
	if !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Fatalf("first two lines should be present:\n%s", got)
	}
	if strings.Contains(got, "three") {
		t.Fatalf("capped output must drop later lines:\n%s", got)
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("expected truncation marker:\n%s", got)
	}
}

func TestAppendTaskOutputUnlimitedForFinal(t *testing.T) {
	s := &state{}
	s.resetTaskStreaming()
	s.openTaskToolBlock(1, 0) // final tool: unlimited
	s.appendTaskOutput("a\nb\nc\nd\ne\nf\ng\n")

	got := s.responseText()
	for _, want := range []string{"a", "g"} {
		if !strings.Contains(got, want) {
			t.Fatalf("unlimited block dropped %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "truncated") {
		t.Fatalf("unlimited block must not truncate:\n%s", got)
	}
}

func TestResetOutputClearsTaskStreaming(t *testing.T) {
	s := &state{
		mode:                guiModeTask,
		taskSawLiveOutput:   true,
		taskLiveOutputTools: map[string]bool{"unix": true},
		taskToolOpen:        true,
		taskToolProcessID:   3,
	}
	s.resetOutput()

	if s.taskSawLiveOutput || s.taskToolOpen || s.taskToolProcessID != 0 || len(s.taskLiveOutputTools) != 0 {
		t.Fatal("resetOutput must clear per-run task streaming state")
	}
	if len(s.taskTranscript) != 0 {
		t.Fatal("resetOutput must clear the task transcript blocks")
	}
	if s.mode != guiModeTask {
		t.Fatal("resetOutput must not reset the selected mode")
	}
}

func TestTaskOutputStreamingKeepsTranscriptInBlocks(t *testing.T) {
	s := &state{}
	s.resetTaskStreaming()
	s.openTaskToolBlock(1, 0)
	s.appendTaskOutput("hello\n")

	if s.output != "" {
		t.Fatalf("task streaming should not flatten into state.output, got %q", s.output)
	}
	if len(s.taskTranscript) != 1 || s.taskTranscript[0].Kind != transcriptBlockToolOutput {
		t.Fatalf("unexpected transcript blocks: %#v", s.taskTranscript)
	}
	if len(s.taskTranscript[0].Chunks) != 1 || s.taskTranscript[0].Chunks[0] != "hello\n" {
		t.Fatalf("tool output should be stored as chunks: %#v", s.taskTranscript[0].Chunks)
	}
}

func TestTaskCompletionRawOutputAppendsToolOutputBlock(t *testing.T) {
	s := &state{}
	s.resetTaskStreaming()

	s.appendTaskCompletionOutput(appservice.Event{RawOutput: "# not markdown\n"})

	if len(s.taskTranscript) != 1 || s.taskTranscript[0].Kind != transcriptBlockToolOutput {
		t.Fatalf("raw output should append as tool-output block: %#v", s.taskTranscript)
	}
	if got := s.responseText(); got != "```\n# not markdown\n```\n" {
		t.Fatalf("serialized raw output = %q", got)
	}
}
