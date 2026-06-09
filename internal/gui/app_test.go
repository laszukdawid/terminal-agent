package gui

import (
	"strings"
	"testing"
	"time"
)

func TestResponseCopyTextPrioritizesDisplayedError(t *testing.T) {
	s := &state{
		question:  "test question",
		output:    "partial streamed output",
		errorText: "provider failed with full error text",
	}

	if got := responseCopyText(s); got != s.errorText {
		t.Fatalf("responseCopyText() = %q, want error %q", got, s.errorText)
	}
}

func TestResponseCopyTextFallsBackToOutputThenQuestion(t *testing.T) {
	withOutput := &state{question: "question", output: "answer"}
	if got := responseCopyText(withOutput); got != "answer" {
		t.Fatalf("responseCopyText(withOutput) = %q, want %q", got, "answer")
	}

	withTaskTranscript := &state{taskTranscript: []transcriptBlock{{Kind: transcriptBlockProse, Text: "Running unix..."}, {Kind: transcriptBlockFinal, Text: "answer"}}}
	if got := responseCopyText(withTaskTranscript); got != "Running unix...\n\n---\n\nanswer" {
		t.Fatalf("responseCopyText(withTaskTranscript) = %q", got)
	}

	withQuestion := &state{question: "question"}
	if got := responseCopyText(withQuestion); got != "question" {
		t.Fatalf("responseCopyText(withQuestion) = %q, want %q", got, "question")
	}
}

func TestHasCopyableResponseIncludesErrors(t *testing.T) {
	if !hasCopyableResponse(&state{errorText: "provider failed"}) {
		t.Fatal("hasCopyableResponse() should be true for errors")
	}
	if hasCopyableResponse(&state{isRunning: true, output: "partial"}) {
		t.Fatal("hasCopyableResponse() should be false while a response is streaming")
	}
	if hasCopyableResponse(&state{question: "question only"}) {
		t.Fatal("hasCopyableResponse() should be false without output or error")
	}
	if !hasCopyableResponse(&state{taskTranscript: []transcriptBlock{{Kind: transcriptBlockFinal, Text: "answer"}}}) {
		t.Fatal("hasCopyableResponse() should be true for task transcripts")
	}
}

func TestExportContentIncludesQuestionAndResponse(t *testing.T) {
	completedAt := time.Date(2026, 6, 7, 14, 30, 0, 0, time.UTC)
	g := &App{
		cfg: voiceGUIConfig{},
		state: &state{
			question:    "What is Terminal Agent?",
			completedAt: completedAt,
		},
	}

	got := g.exportContent("# Summary\n\nTerminal Agent is a CLI-first AI assistant.")
	wantParts := []string{
		"provider/model: openai / gpt-4o-mini",
		"generated: 2026-06-07 14:30:00",
		"# Ask\n\nWhat is Terminal Agent?",
		"---\n\n# Response\n\n# Summary\n\nTerminal Agent is a CLI-first AI assistant.",
	}
	for _, want := range wantParts {
		if !strings.Contains(got, want) {
			t.Fatalf("exportContent() missing %q in:\n%s", want, got)
		}
	}
}
