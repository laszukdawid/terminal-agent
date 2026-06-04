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
