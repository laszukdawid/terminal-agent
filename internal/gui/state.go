package gui

import (
	"context"
	"strings"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/voice"
)

// guiMode selects which app-layer flow the single input drives. Ask keeps the
// original question/answer experience; Task runs an agentic tool-using session.
type guiMode string

const (
	guiModeAsk  guiMode = "ask"
	guiModeTask guiMode = "task"
)

// taskToolFenceMarker opens and closes the fenced code block that wraps live
// tool stdout/stderr in the Task transcript, keeping shell output readable in
// the markdown renderer.
const taskToolFenceMarker = "```"

type state struct {
	input        string
	question     string
	output       string
	status       string
	spinnerFrame int
	errorText    string
	isRunning    bool
	isVisible    bool
	cancelFunc   context.CancelFunc
	voiceState   voice.State
	voiceError   string
	startTime    time.Time
	completedAt  time.Time
	elapsed      time.Duration

	// mode is the selected sidebar tab. It deliberately persists across runs and
	// is not cleared by resetOutput so the chosen tab stays selected.
	mode      guiMode
	modeViews map[guiMode]modeViewState

	// Per-run Task streaming state. taskSawLiveOutput records whether any live
	// tool output streamed this run; taskLiveOutputTools tracks which tools
	// streamed so completion can avoid duplicating already-shown raw output.
	// taskToolOpen/taskToolProcessID track the currently open fenced output
	// block so consecutive chunks render as one contiguous transcript block.
	taskSawLiveOutput   bool
	taskLiveOutputTools map[string]bool
	taskToolOpen        bool
	taskToolProcessID   int
}

type modeViewState struct {
	input        string
	question     string
	output       string
	status       string
	spinnerFrame int
	errorText    string
	completedAt  time.Time
	elapsed      time.Duration

	taskSawLiveOutput   bool
	taskLiveOutputTools map[string]bool
	taskToolOpen        bool
	taskToolProcessID   int
}

func (s *state) saveModeView() {
	if s.modeViews == nil {
		s.modeViews = map[guiMode]modeViewState{}
	}
	liveOutputTools := map[string]bool{}
	for tool, seen := range s.taskLiveOutputTools {
		liveOutputTools[tool] = seen
	}
	s.modeViews[s.mode] = modeViewState{
		input:               s.input,
		question:            s.question,
		output:              s.output,
		status:              s.status,
		spinnerFrame:        s.spinnerFrame,
		errorText:           s.errorText,
		completedAt:         s.completedAt,
		elapsed:             s.elapsed,
		taskSawLiveOutput:   s.taskSawLiveOutput,
		taskLiveOutputTools: liveOutputTools,
		taskToolOpen:        s.taskToolOpen,
		taskToolProcessID:   s.taskToolProcessID,
	}
}

func (s *state) restoreModeView(mode guiMode) {
	view := modeViewState{}
	if s.modeViews != nil {
		view = s.modeViews[mode]
	}
	s.mode = mode
	s.input = view.input
	s.question = view.question
	s.output = view.output
	s.status = view.status
	s.spinnerFrame = view.spinnerFrame
	s.errorText = view.errorText
	s.completedAt = view.completedAt
	s.elapsed = view.elapsed
	s.taskSawLiveOutput = view.taskSawLiveOutput
	s.taskLiveOutputTools = map[string]bool{}
	for tool, seen := range view.taskLiveOutputTools {
		s.taskLiveOutputTools[tool] = seen
	}
	s.taskToolOpen = view.taskToolOpen
	s.taskToolProcessID = view.taskToolProcessID
}

func (s *state) resetOutput() {
	s.question = ""
	s.output = ""
	s.status = ""
	s.spinnerFrame = 0
	s.errorText = ""
	s.completedAt = time.Time{}
	s.elapsed = 0
	s.resetTaskStreaming()
}

// resetTaskStreaming clears per-run Task transcript streaming state. It runs for
// every run (including Ask) so stale Task state cannot leak into a later run's
// completion handling.
func (s *state) resetTaskStreaming() {
	s.taskSawLiveOutput = false
	s.taskLiveOutputTools = map[string]bool{}
	s.taskToolOpen = false
	s.taskToolProcessID = 0
}

// openTaskToolBlock ensures a fenced output block is open for processID. A new
// process (or no open block) closes any previous fence and opens a fresh one so
// each tool's live output is its own readable block.
func (s *state) openTaskToolBlock(processID int) {
	if s.taskToolOpen && s.taskToolProcessID == processID {
		return
	}
	s.closeTaskToolBlock()
	s.ensureTrailingNewline()
	s.output += taskToolFenceMarker + "\n"
	s.taskToolOpen = true
	s.taskToolProcessID = processID
}

// closeTaskToolBlock closes an open fenced output block, if any.
func (s *state) closeTaskToolBlock() {
	if !s.taskToolOpen {
		return
	}
	s.ensureTrailingNewline()
	s.output += taskToolFenceMarker + "\n"
	s.taskToolOpen = false
	s.taskToolProcessID = 0
}

// appendTaskTranscriptLine closes any open tool-output fence and appends a
// non-output transcript line (status/progress) on its own line.
func (s *state) appendTaskTranscriptLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	s.closeTaskToolBlock()
	s.ensureTrailingNewline()
	s.output += line + "\n"
}

// appendTaskFinalText appends final/raw completion text, separating it from any
// preceding transcript content with a horizontal rule.
func (s *state) appendTaskFinalText(text string) {
	if text == "" {
		return
	}
	if s.output != "" {
		s.ensureTrailingNewline()
		s.output += "\n---\n\n"
	}
	s.output += text
}

// ensureTrailingNewline normalizes the transcript so the next appended block
// starts on a fresh line.
func (s *state) ensureTrailingNewline() {
	if s.output != "" && !strings.HasSuffix(s.output, "\n") {
		s.output += "\n"
	}
}

func (s *state) setRunning(cancel context.CancelFunc) {
	s.isRunning = true
	s.cancelFunc = cancel
	s.status = "thinking"
	s.spinnerFrame = 0
	s.errorText = ""
	s.startTime = time.Now()
}

// markCompleted records the elapsed runtime for the response metadata row.
func (s *state) markCompleted() {
	s.completedAt = time.Now()
	if !s.startTime.IsZero() {
		s.elapsed = s.completedAt.Sub(s.startTime)
	}
}

func (s *state) clearRunning() {
	s.isRunning = false
	s.cancelFunc = nil
	s.status = ""
	s.spinnerFrame = 0
}
