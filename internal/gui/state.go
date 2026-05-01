package gui

import "context"

type state struct {
	input      string
	question   string
	output     string
	status     string
	errorText  string
	isRunning  bool
	cancelFunc context.CancelFunc
}

func (s *state) resetOutput() {
	s.question = ""
	s.output = ""
	s.status = ""
	s.errorText = ""
}

func (s *state) setRunning(cancel context.CancelFunc) {
	s.isRunning = true
	s.cancelFunc = cancel
	s.status = "Waiting for response..."
	s.errorText = ""
}

func (s *state) clearRunning() {
	s.isRunning = false
	s.cancelFunc = nil
	if s.status == "Waiting for response..." {
		s.status = ""
	}
}
