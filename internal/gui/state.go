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

// taskToolFenceMarker is the copy/export fence marker used when serializing
// task tool-output blocks back to the legacy markdown transcript shape.
const taskToolFenceMarker = "```"

type state struct {
	input          string
	question       string
	output         string
	taskTranscript []transcriptBlock
	status         string
	errorText      string
	isRunning      bool
	isVisible      bool
	cancelFunc     context.CancelFunc
	voiceState     voice.State
	voiceError     string
	startTime      time.Time
	completedAt    time.Time
	elapsed        time.Duration

	// mode is the selected sidebar tab. It deliberately persists across runs and
	// is not cleared by resetOutput so the chosen tab stays selected.
	mode      guiMode
	modeViews map[guiMode]modeViewState

	// Per-run Task streaming state. taskSawLiveOutput records whether any live
	// tool output streamed this run; taskLiveOutputTools tracks which tools
	// streamed so completion can avoid duplicating already-shown raw output.
	// taskToolOpen/taskToolProcessID track the currently open tool-output block so
	// consecutive chunks render as one contiguous monospace transcript block.
	taskSawLiveOutput   bool
	taskLiveOutputTools map[string]bool
	// taskLiveOutputTruncatedTools records tools whose live output was capped, so
	// completion still surfaces their full raw result instead of suppressing it
	// as an already-streamed duplicate (mirrors the CLI's TruncatedTool de-dupe).
	taskLiveOutputTruncatedTools map[string]bool
	taskToolOpen                 bool
	taskToolProcessID            int
	taskToolBlockIndex           int

	// outputDirty marks that the visible response changed and the response panel
	// needs a (coalesced) re-render; the indicator ticker flushes it.
	outputDirty bool
	// taskCurrentToolFinal is set from the running_tool status: final tools stream
	// unlimited because their output is the run's answer.
	taskCurrentToolFinal bool
	// taskLiveLimiter bounds the current tool block's live output.
	taskLiveLimiter *liveOutputLimiter
}

type modeViewState struct {
	input          string
	question       string
	output         string
	taskTranscript []transcriptBlock
	status         string
	errorText      string
	completedAt    time.Time
	elapsed        time.Duration

	taskSawLiveOutput            bool
	taskLiveOutputTools          map[string]bool
	taskLiveOutputTruncatedTools map[string]bool
	taskToolOpen                 bool
	taskToolProcessID            int
	taskToolBlockIndex           int
}

func (s *state) saveModeView() {
	if s.modeViews == nil {
		s.modeViews = map[guiMode]modeViewState{}
	}
	liveOutputTools := map[string]bool{}
	for tool, seen := range s.taskLiveOutputTools {
		liveOutputTools[tool] = seen
	}
	truncatedTools := map[string]bool{}
	for tool, seen := range s.taskLiveOutputTruncatedTools {
		truncatedTools[tool] = seen
	}
	s.modeViews[s.mode] = modeViewState{
		input:                        s.input,
		question:                     s.question,
		output:                       s.output,
		taskTranscript:               cloneTranscriptBlocks(s.taskTranscript),
		status:                       s.status,
		errorText:                    s.errorText,
		completedAt:                  s.completedAt,
		elapsed:                      s.elapsed,
		taskSawLiveOutput:            s.taskSawLiveOutput,
		taskLiveOutputTools:          liveOutputTools,
		taskLiveOutputTruncatedTools: truncatedTools,
		taskToolOpen:                 s.taskToolOpen,
		taskToolProcessID:            s.taskToolProcessID,
		taskToolBlockIndex:           s.taskToolBlockIndex,
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
	s.taskTranscript = cloneTranscriptBlocks(view.taskTranscript)
	s.status = view.status
	s.errorText = view.errorText
	s.completedAt = view.completedAt
	s.elapsed = view.elapsed
	s.taskSawLiveOutput = view.taskSawLiveOutput
	s.taskLiveOutputTools = map[string]bool{}
	for tool, seen := range view.taskLiveOutputTools {
		s.taskLiveOutputTools[tool] = seen
	}
	s.taskLiveOutputTruncatedTools = map[string]bool{}
	for tool, seen := range view.taskLiveOutputTruncatedTools {
		s.taskLiveOutputTruncatedTools[tool] = seen
	}
	s.taskToolOpen = view.taskToolOpen
	s.taskToolProcessID = view.taskToolProcessID
	s.taskToolBlockIndex = view.taskToolBlockIndex
}

func (s *state) resetOutput() {
	s.question = ""
	s.output = ""
	s.taskTranscript = nil
	s.status = ""
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
	s.taskLiveOutputTruncatedTools = map[string]bool{}
	s.taskToolOpen = false
	s.taskToolProcessID = 0
	s.taskToolBlockIndex = -1
	s.outputDirty = false
	s.taskCurrentToolFinal = false
	s.taskLiveLimiter = nil
}

// openTaskToolBlock ensures a tool-output block is open for processID, bounding
// it to maxLines lines (0 = unlimited). A new process (or no open block) closes
// any previous block and opens a fresh one with a fresh limiter so each tool's
// live output is its own readable, bounded block.
func (s *state) openTaskToolBlock(processID, maxLines int) {
	if s.taskToolOpen && s.taskToolProcessID == processID {
		return
	}
	s.closeTaskToolBlock()
	s.taskTranscript = append(s.taskTranscript, transcriptBlock{Kind: transcriptBlockToolOutput})
	s.taskToolOpen = true
	s.taskToolProcessID = processID
	s.taskToolBlockIndex = len(s.taskTranscript) - 1
	s.taskLiveLimiter = newLiveOutputLimiter(maxLines, liveOutputMaxLineChars)
}

// appendTaskOutput appends live tool output to the open block, applying the
// block's line cap. Output beyond the cap is replaced by a one-time truncation
// marker (see liveOutputLimiter).
func (s *state) appendTaskOutput(text string) {
	if s.taskLiveLimiter != nil {
		text = s.taskLiveLimiter.Filter(text)
	}
	if text == "" || s.taskToolBlockIndex < 0 || s.taskToolBlockIndex >= len(s.taskTranscript) {
		return
	}
	s.taskTranscript[s.taskToolBlockIndex].Chunks = append(s.taskTranscript[s.taskToolBlockIndex].Chunks, text)
}

// closeTaskToolBlock closes an open tool-output block, if any.
func (s *state) closeTaskToolBlock() {
	if !s.taskToolOpen {
		return
	}
	s.taskToolOpen = false
	s.taskToolProcessID = 0
	s.taskToolBlockIndex = -1
	s.taskLiveLimiter = nil
}

// appendTaskTranscriptLine closes any open tool-output block and appends a
// non-output transcript line (status/progress) on its own line.
func (s *state) appendTaskTranscriptLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	s.closeTaskToolBlock()
	s.taskTranscript = append(s.taskTranscript, transcriptBlock{Kind: transcriptBlockProse, Text: line})
}

// appendTaskFinalText appends final/raw completion text, separating it from any
// preceding transcript content with a horizontal rule.
func (s *state) appendTaskFinalText(text string) {
	if text == "" {
		return
	}
	s.closeTaskToolBlock()
	s.taskTranscript = append(s.taskTranscript, transcriptBlock{Kind: transcriptBlockFinal, Text: text})
}

func (s *state) appendTaskRawOutput(text string) {
	if text == "" {
		return
	}
	s.closeTaskToolBlock()
	s.taskTranscript = append(s.taskTranscript, transcriptBlock{Kind: transcriptBlockToolOutput, Chunks: []string{text}})
}

func (s *state) hasResponseContent() bool {
	return s.output != "" || len(s.taskTranscript) > 0
}

func (s *state) responseText() string {
	if len(s.taskTranscript) > 0 {
		return serializeTranscript(s.taskTranscript)
	}
	return s.output
}

func (s *state) setRunning(cancel context.CancelFunc) {
	s.isRunning = true
	s.cancelFunc = cancel
	s.status = "thinking"
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
}
