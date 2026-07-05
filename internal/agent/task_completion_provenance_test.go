package agent

import (
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/connector"
)

// The final answer step is no longer indexed into task memory on its own, so
// the run result must carry its thought and iteration for the run-level
// completion record to absorb.
func TestFinalAnswerResultCarriesCompletionProvenance(t *testing.T) {
	run := &taskExecutionState{state: &TaskState{Iterations: 3}}
	run.recordSuccess(connector.LlmResponseWithTools{
		Response:  "the clock read 12:00",
		ToolUse:   true,
		ToolName:  ToolNameFinalAnswer,
		ToolInput: map[string]any{"answer": "It is noon."},
	}, "It is noon.")

	result := run.finalAnswerResult("It is noon.")
	if result.Response != "It is noon." {
		t.Fatalf("response = %q, want the final answer", result.Response)
	}
	if result.CompletionThought != "the clock read 12:00" {
		t.Fatalf("completion thought = %q, want the closing step's thought", result.CompletionThought)
	}
	if result.CompletionSourceSeq != 3 {
		t.Fatalf("completion seq = %d, want the closing step's iteration", result.CompletionSourceSeq)
	}
}

// Completion paths whose last step is not a final answer (direct raw output,
// summary fallback) must not smear an unrelated step's provenance onto the
// completion record.
func TestFinalAnswerResultWithoutFinalStepLeavesProvenanceEmpty(t *testing.T) {
	run := &taskExecutionState{state: &TaskState{Iterations: 2}}
	run.recordSuccess(connector.LlmResponseWithTools{
		Response:  "running the listing",
		ToolUse:   true,
		ToolName:  "unix",
		ToolInput: map[string]any{"command": "ls"},
	}, "a b c")

	result := run.finalAnswerResult("a b c")
	if result.CompletionThought != "" || result.CompletionSourceSeq != 0 {
		t.Fatalf("provenance = (%q, %d), want empty for a non-final last step",
			result.CompletionThought, result.CompletionSourceSeq)
	}
}
