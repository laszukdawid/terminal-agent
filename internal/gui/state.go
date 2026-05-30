package gui

import "context"

type state struct {
	input        string
	question     string
	output       string
	// responsePrefix is rendered ahead of output. It carries the separator and
	// "Response to: <prompt>" marker so a response stays labelled with the
	// prompt it answers, including after completion.
	responsePrefix string
	status         string
	spinnerFrame   int
	errorText      string
	isRunning      bool
	isVisible      bool
	showRequest    bool
	cancelFunc     context.CancelFunc
}

func (s *state) resetOutput() {
	s.question = ""
	s.output = ""
	s.responsePrefix = ""
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
