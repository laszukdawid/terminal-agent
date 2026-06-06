package gui

import (
	"context"

	"github.com/laszukdawid/terminal-agent/internal/voice"
)

type state struct {
	input        string
	question     string
	output       string
	status       string
	spinnerFrame int
	errorText    string
	isRunning    bool
	isVisible    bool
	showRequest  bool
	cancelFunc   context.CancelFunc
	voiceState   voice.State
	voiceError   string
}

func (s *state) resetOutput() {
	s.question = ""
	s.output = ""
	s.status = ""
	s.spinnerFrame = 0
	s.errorText = ""
	s.showRequest = false
}

func (s *state) setRunning(cancel context.CancelFunc) {
	s.isRunning = true
	s.cancelFunc = cancel
	s.status = "thinking"
	s.spinnerFrame = 0
	s.errorText = ""
}

func (s *state) clearRunning() {
	s.isRunning = false
	s.cancelFunc = nil
	s.status = ""
	s.spinnerFrame = 0
}
