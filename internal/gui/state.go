package gui

import (
	"context"
	"strings"
)

type state struct {
	input      string
	question   string
	output     string
	statusBase string
	status     string
	errorText  string
	isRunning  bool
	isVisible  bool
	showRequest bool
	thinkingStep int
	cancelFunc context.CancelFunc
}

func (s *state) resetOutput() {
	s.question = ""
	s.output = ""
	s.statusBase = ""
	s.status = ""
	s.errorText = ""
	s.showRequest = false
	s.thinkingStep = 0
}

func (s *state) setRunning(cancel context.CancelFunc) {
	s.isRunning = true
	s.cancelFunc = cancel
	s.statusBase = "Waiting"
	s.status = "Waiting."
	s.errorText = ""
	s.thinkingStep = 1
}

func (s *state) clearRunning() {
	s.isRunning = false
	s.cancelFunc = nil
	s.statusBase = ""
	s.status = ""
	s.thinkingStep = 0
}

func (s *state) advanceThinking(base string) {
	if base == "" {
		s.statusBase = ""
		s.status = ""
		s.thinkingStep = 0
		return
	}
	s.statusBase = base
	s.thinkingStep = (s.thinkingStep % 3) + 1
	s.status = base + strings.Repeat(".", s.thinkingStep)
}
