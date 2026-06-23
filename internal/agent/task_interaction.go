package agent

import "errors"

var ErrTaskInteractionRequired = errors.New("task interaction required")

type TaskInteraction interface {
	Confirm(req TaskConfirmationRequest) (TaskConfirmationDecision, error)
	Clarify(req TaskClarificationRequest) (string, error)
}

type TaskConfirmationRequest struct {
	Action string
}

type TaskConfirmationDecision struct {
	Allowed  bool
	Remember bool
	Patterns []string
}

type TaskClarificationRequest struct {
	Question string
}

// DefaultUnattendedClarification is the canned answer returned for any
// clarification request during an unattended run. It keeps the run progressing
// instead of blocking on a human who is not present.
const DefaultUnattendedClarification = "No interactive user is available (automated routine run). " +
	"Proceed using your best judgment and reasonable assumptions; do not ask for further clarification."

// UnattendedInteraction is a TaskInteraction for headless runs (e.g. routines).
// Confirmations are approved (the run also sets AutoApprove, which already
// honors deny rules), and clarifications return a fixed "proceed" response so
// the agent never waits for input that will never come.
type UnattendedInteraction struct {
	// ClarificationResponse overrides the default canned clarification answer
	// when non-empty.
	ClarificationResponse string
}

func (u UnattendedInteraction) Confirm(TaskConfirmationRequest) (TaskConfirmationDecision, error) {
	return TaskConfirmationDecision{Allowed: true}, nil
}

func (u UnattendedInteraction) Clarify(TaskClarificationRequest) (string, error) {
	if u.ClarificationResponse != "" {
		return u.ClarificationResponse, nil
	}
	return DefaultUnattendedClarification, nil
}
