package gui

import (
	"context"
	"time"

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
	cancelFunc   context.CancelFunc
	voiceState   voice.State
	voiceError   string
	startTime    time.Time
	completedAt  time.Time
	elapsed      time.Duration
}

func (s *state) resetOutput() {
	s.question = ""
	s.output = ""
	s.status = ""
	s.spinnerFrame = 0
	s.errorText = ""
	s.completedAt = time.Time{}
	s.elapsed = 0
}

func (s *state) setRunning(cancel context.CancelFunc) {
	s.isRunning = true
	s.cancelFunc = cancel
	s.status = "thinking"
	s.spinnerFrame = 0
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
	s.spinnerFrame = 0
}
