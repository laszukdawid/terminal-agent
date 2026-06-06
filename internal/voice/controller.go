package voice

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

type State string

const (
	StateIdle         State = "idle"
	StateRecording    State = "recording"
	StateTranscribing State = "transcribing"
)

var (
	ErrEmptyRecording  = errors.New("voice recording is empty")
	ErrEmptyTranscript = errors.New("voice transcript is empty")
)

type ControllerCallbacks struct {
	OnState      func(State)
	OnTranscript func(Transcript)
	OnError      func(error)
	OnCancel     func()
}

type ControllerOptions struct {
	MaxRecordingDuration time.Duration
	Callbacks            ControllerCallbacks
}

type Controller struct {
	mu                   sync.Mutex
	recorder             Recorder
	transcriber          Transcriber
	callbacks            ControllerCallbacks
	maxRecording         time.Duration
	state                State
	stopTimer            *time.Timer
	transcribeCancel     context.CancelFunc
	transcribeGeneration int
}

func NewController(recorder Recorder, transcriber Transcriber, options ControllerOptions) *Controller {
	return &Controller{
		recorder:     recorder,
		transcriber:  transcriber,
		callbacks:    options.Callbacks,
		maxRecording: options.MaxRecordingDuration,
		state:        StateIdle,
	}
}

func (c *Controller) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Controller) Toggle(ctx context.Context) error {
	c.mu.Lock()
	state := c.state
	c.mu.Unlock()

	switch state {
	case StateIdle:
		return c.start(ctx)
	case StateRecording:
		return c.stop(ctx)
	case StateTranscribing:
		return c.cancelTranscription()
	default:
		return nil
	}
}

func (c *Controller) Cancel(ctx context.Context) error {
	c.mu.Lock()
	state := c.state
	c.mu.Unlock()

	switch state {
	case StateRecording:
		c.stopMaxTimer()
		err := c.recorder.Cancel(ctx)
		c.setState(StateIdle)
		c.emitCancel()
		return err
	case StateTranscribing:
		return c.cancelTranscription()
	default:
		return nil
	}
}

func (c *Controller) start(ctx context.Context) error {
	if err := c.recorder.Start(ctx); err != nil {
		c.setState(StateIdle)
		c.emitError(err)
		return err
	}
	c.setState(StateRecording)
	c.startMaxTimer()
	return nil
}

func (c *Controller) stop(ctx context.Context) error {
	c.stopMaxTimer()
	rec, err := c.recorder.Stop(ctx)
	if err != nil {
		c.setState(StateIdle)
		c.emitError(err)
		return err
	}
	if len(rec.Data) == 0 {
		c.setState(StateIdle)
		c.emitError(ErrEmptyRecording)
		return nil
	}
	c.setState(StateTranscribing)

	transcribeCtx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.transcribeCancel = cancel
	c.transcribeGeneration++
	generation := c.transcribeGeneration
	c.mu.Unlock()

	go c.transcribe(transcribeCtx, generation, rec)
	return nil
}

func (c *Controller) transcribe(ctx context.Context, generation int, rec Recording) {
	transcript, err := c.transcriber.Transcribe(ctx, rec)
	if ctx.Err() != nil {
		return
	}

	c.mu.Lock()
	current := c.transcribeGeneration
	c.mu.Unlock()
	if generation != current {
		return
	}

	if err != nil {
		c.setState(StateIdle)
		c.emitError(err)
		return
	}
	if strings.TrimSpace(transcript.Text) == "" {
		c.setState(StateIdle)
		c.emitError(ErrEmptyTranscript)
		return
	}
	c.setState(StateIdle)
	c.emitTranscript(transcript)
}

func (c *Controller) cancelTranscription() error {
	c.mu.Lock()
	c.transcribeGeneration++
	cancel := c.transcribeCancel
	c.transcribeCancel = nil
	c.state = StateIdle
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	c.emitState(StateIdle)
	c.emitCancel()
	return nil
}

func (c *Controller) startMaxTimer() {
	if c.maxRecording <= 0 {
		return
	}
	c.stopMaxTimer()
	c.mu.Lock()
	c.stopTimer = time.AfterFunc(c.maxRecording, func() {
		_ = c.stop(context.Background())
	})
	c.mu.Unlock()
}

func (c *Controller) stopMaxTimer() {
	c.mu.Lock()
	timer := c.stopTimer
	c.stopTimer = nil
	c.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
}

func (c *Controller) setState(state State) {
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.emitState(state)
}

func (c *Controller) emitState(state State) {
	if c.callbacks.OnState != nil {
		c.callbacks.OnState(state)
	}
}

func (c *Controller) emitTranscript(transcript Transcript) {
	if c.callbacks.OnTranscript != nil {
		c.callbacks.OnTranscript(transcript)
	}
}

func (c *Controller) emitError(err error) {
	if c.callbacks.OnError != nil {
		c.callbacks.OnError(err)
	}
}

func (c *Controller) emitCancel() {
	if c.callbacks.OnCancel != nil {
		c.callbacks.OnCancel()
	}
}
