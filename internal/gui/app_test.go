package gui

import "testing"

func TestDisplayStatusUsesShortErrorStatus(t *testing.T) {
	s := &state{
		status:    "thinking",
		errorText: "provider openai request failed with a long response body",
	}

	if got := displayStatus(s); got != "Error" {
		t.Fatalf("displayStatus() = %q, want %q", got, "Error")
	}
}

func TestDisplayStatusUsesNormalStatusWithoutError(t *testing.T) {
	s := &state{status: "responding"}

	if got := displayStatus(s); got != "responding" {
		t.Fatalf("displayStatus() = %q, want %q", got, "responding")
	}
}

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

	withQuestion := &state{question: "question"}
	if got := responseCopyText(withQuestion); got != "question" {
		t.Fatalf("responseCopyText(withQuestion) = %q, want %q", got, "question")
	}
}

func TestHasCopyableResponseIncludesErrors(t *testing.T) {
	if !hasCopyableResponse(&state{errorText: "provider failed"}) {
		t.Fatal("hasCopyableResponse() should be true for errors")
	}
	if hasCopyableResponse(&state{question: "question only"}) {
		t.Fatal("hasCopyableResponse() should be false without output or error")
	}
}
