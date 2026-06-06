package voice

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControllerToggleStartsRecording(t *testing.T) {
	recorder := &fakeRecorder{}
	c := NewController(recorder, &fakeTranscriber{}, ControllerOptions{})

	require.NoError(t, c.Toggle(context.Background()))

	assert.Equal(t, 1, recorder.starts)
	assert.Equal(t, StateRecording, c.State())
}

func TestControllerStopTranscribesAndReturnsTranscript(t *testing.T) {
	recorder := &fakeRecorder{recording: Recording{Data: []byte("wav"), Format: AudioFormatWAV}}
	transcriber := &fakeTranscriber{transcript: Transcript{Text: "hello"}}
	callbacks := &callbackRecorder{}
	c := NewController(recorder, transcriber, ControllerOptions{Callbacks: callbacks.callbacks()})

	require.NoError(t, c.Toggle(context.Background()))
	require.NoError(t, c.Toggle(context.Background()))

	callbacks.waitForTranscript(t)
	assert.Equal(t, 1, recorder.stops)
	assert.Equal(t, 1, transcriber.calls)
	assert.Equal(t, []State{StateRecording, StateTranscribing, StateIdle}, callbacks.states)
	assert.Equal(t, "hello", callbacks.transcript.Text)
	assert.Equal(t, StateIdle, c.State())
}

func TestControllerToggleCancelsTranscription(t *testing.T) {
	recorder := &fakeRecorder{recording: Recording{Data: []byte("wav"), Format: AudioFormatWAV}}
	transcriber := &fakeTranscriber{block: make(chan struct{}), ctxCh: make(chan struct{}, 1)}
	callbacks := &callbackRecorder{}
	c := NewController(recorder, transcriber, ControllerOptions{Callbacks: callbacks.callbacks()})

	require.NoError(t, c.Toggle(context.Background()))
	require.NoError(t, c.Toggle(context.Background()))
	callbacks.waitForState(t, StateTranscribing)
	require.NoError(t, c.Toggle(context.Background()))

	callbacks.waitForCancel(t)
	assert.Equal(t, StateIdle, c.State())
	assert.True(t, transcriber.ctxCancelled(t))
	close(transcriber.block)
}

func TestControllerCancelRecordingDiscardsAudio(t *testing.T) {
	recorder := &fakeRecorder{}
	callbacks := &callbackRecorder{}
	c := NewController(recorder, &fakeTranscriber{}, ControllerOptions{Callbacks: callbacks.callbacks()})

	require.NoError(t, c.Toggle(context.Background()))
	require.NoError(t, c.Cancel(context.Background()))

	assert.Equal(t, 1, recorder.cancels)
	assert.Equal(t, 0, recorder.stops)
	assert.Equal(t, StateIdle, c.State())
	callbacks.waitForCancel(t)
}

func TestControllerErrorsReturnToIdle(t *testing.T) {
	tests := []struct {
		name        string
		recorder    *fakeRecorder
		transcriber *fakeTranscriber
		act         func(*Controller) error
	}{
		{
			name:        "start failure",
			recorder:    &fakeRecorder{startErr: errors.New("start failed")},
			transcriber: &fakeTranscriber{},
			act:         func(c *Controller) error { return c.Toggle(context.Background()) },
		},
		{
			name:        "stop failure",
			recorder:    &fakeRecorder{stopErr: errors.New("stop failed")},
			transcriber: &fakeTranscriber{},
			act: func(c *Controller) error {
				require.NoError(t, c.Toggle(context.Background()))
				return c.Toggle(context.Background())
			},
		},
		{
			name:        "transcribe failure",
			recorder:    &fakeRecorder{recording: Recording{Data: []byte("wav"), Format: AudioFormatWAV}},
			transcriber: &fakeTranscriber{err: errors.New("transcribe failed")},
			act: func(c *Controller) error {
				require.NoError(t, c.Toggle(context.Background()))
				return c.Toggle(context.Background())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callbacks := &callbackRecorder{}
			c := NewController(tt.recorder, tt.transcriber, ControllerOptions{Callbacks: callbacks.callbacks()})

			err := tt.act(c)
			if err == nil {
				callbacks.waitForError(t)
			}

			assert.Equal(t, StateIdle, c.State())
			assert.NotEmpty(t, callbacks.errs)
		})
	}
}

func TestControllerMaxDurationStopsRecording(t *testing.T) {
	recorder := &fakeRecorder{recording: Recording{Data: []byte("wav"), Format: AudioFormatWAV}}
	transcriber := &fakeTranscriber{transcript: Transcript{Text: "done"}}
	callbacks := &callbackRecorder{}
	c := NewController(recorder, transcriber, ControllerOptions{
		MaxRecordingDuration: 10 * time.Millisecond,
		Callbacks:            callbacks.callbacks(),
	})

	require.NoError(t, c.Toggle(context.Background()))

	callbacks.waitForTranscript(t)
	assert.Equal(t, 1, recorder.stops)
	assert.Equal(t, "done", callbacks.transcript.Text)
}

func TestControllerManualStopDoesNotRaceMaxDurationStop(t *testing.T) {
	recorder := &fakeRecorder{recording: Recording{Data: []byte("wav"), Format: AudioFormatWAV}, stopBlock: make(chan struct{}), stopCh: make(chan struct{}, 1)}
	transcriber := &fakeTranscriber{transcript: Transcript{Text: "done"}}
	callbacks := &callbackRecorder{}
	c := NewController(recorder, transcriber, ControllerOptions{
		MaxRecordingDuration: time.Millisecond,
		Callbacks:            callbacks.callbacks(),
	})

	require.NoError(t, c.Toggle(context.Background()))
	manualStopDone := make(chan error, 1)
	go func() {
		manualStopDone <- c.Toggle(context.Background())
	}()
	recorder.waitForStopStarted(t)
	time.Sleep(5 * time.Millisecond)
	close(recorder.stopBlock)

	require.NoError(t, <-manualStopDone)
	callbacks.waitForTranscript(t)
	assert.Equal(t, 1, recorder.stops)
}

func TestControllerConcurrentStopsCallRecorderOnce(t *testing.T) {
	recorder := &fakeRecorder{recording: Recording{Data: []byte("wav"), Format: AudioFormatWAV}, stopBlock: make(chan struct{}), stopCh: make(chan struct{}, 2)}
	transcriber := &fakeTranscriber{transcript: Transcript{Text: "done"}}
	callbacks := &callbackRecorder{}
	c := NewController(recorder, transcriber, ControllerOptions{Callbacks: callbacks.callbacks()})

	require.NoError(t, c.Toggle(context.Background()))
	stopDone := make(chan error, 2)
	go func() { stopDone <- c.Toggle(context.Background()) }()
	go func() { stopDone <- c.Toggle(context.Background()) }()
	recorder.waitForStopStarted(t)
	time.Sleep(5 * time.Millisecond)
	close(recorder.stopBlock)

	require.NoError(t, <-stopDone)
	require.NoError(t, <-stopDone)
	assert.Equal(t, 1, recorder.stops)
}

type fakeRecorder struct {
	starts    int
	stops     int
	cancels   int
	startErr  error
	stopErr   error
	cancelErr error
	recording Recording
	stopBlock chan struct{}
	stopCh    chan struct{}
}

func (r *fakeRecorder) Start(context.Context) error {
	r.starts++
	return r.startErr
}

func (r *fakeRecorder) Stop(context.Context) (Recording, error) {
	r.stops++
	if r.stopCh != nil {
		r.stopCh <- struct{}{}
	}
	if r.stopBlock != nil {
		<-r.stopBlock
	}
	return r.recording, r.stopErr
}

func (r *fakeRecorder) Cancel(context.Context) error {
	r.cancels++
	return r.cancelErr
}

func (r *fakeRecorder) waitForStopStarted(t *testing.T) {
	t.Helper()
	select {
	case <-r.stopCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for recorder stop")
	}
}

type fakeTranscriber struct {
	mu         sync.Mutex
	calls      int
	transcript Transcript
	err        error
	block      chan struct{}
	ctx        context.Context
	ctxCh      chan struct{}
}

func (t *fakeTranscriber) Transcribe(ctx context.Context, rec Recording) (Transcript, error) {
	t.mu.Lock()
	t.calls++
	t.ctx = ctx
	t.mu.Unlock()
	if t.ctxCh != nil {
		t.ctxCh <- struct{}{}
	}
	if t.block != nil {
		<-t.block
	}
	return t.transcript, t.err
}

func (t *fakeTranscriber) ctxCancelled(tb testing.TB) bool {
	tb.Helper()
	t.mu.Lock()
	ctx := t.ctx
	t.mu.Unlock()
	if ctx == nil && t.ctxCh != nil {
		select {
		case <-t.ctxCh:
		case <-time.After(time.Second):
			return false
		}
		t.mu.Lock()
		ctx = t.ctx
		t.mu.Unlock()
	}
	select {
	case <-ctx.Done():
		return true
	case <-time.After(time.Second):
		return false
	}
}

type callbackRecorder struct {
	mu         sync.Mutex
	states     []State
	transcript Transcript
	errs       []error
	cancels    int
	stateCh    chan State
	textCh     chan struct{}
	errCh      chan struct{}
	cancelCh   chan struct{}
}

func (r *callbackRecorder) callbacks() ControllerCallbacks {
	r.stateCh = make(chan State, 8)
	r.textCh = make(chan struct{}, 1)
	r.errCh = make(chan struct{}, 1)
	r.cancelCh = make(chan struct{}, 1)
	return ControllerCallbacks{
		OnState: func(state State) {
			r.mu.Lock()
			r.states = append(r.states, state)
			r.mu.Unlock()
			r.stateCh <- state
		},
		OnTranscript: func(transcript Transcript) {
			r.mu.Lock()
			r.transcript = transcript
			r.mu.Unlock()
			r.textCh <- struct{}{}
		},
		OnError: func(err error) {
			r.mu.Lock()
			r.errs = append(r.errs, err)
			r.mu.Unlock()
			r.errCh <- struct{}{}
		},
		OnCancel: func() {
			r.mu.Lock()
			r.cancels++
			r.mu.Unlock()
			r.cancelCh <- struct{}{}
		},
	}
}

func (r *callbackRecorder) waitForTranscript(t *testing.T) {
	t.Helper()
	select {
	case <-r.textCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for transcript")
	}
}

func (r *callbackRecorder) waitForError(t *testing.T) {
	t.Helper()
	select {
	case <-r.errCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error")
	}
}

func (r *callbackRecorder) waitForCancel(t *testing.T) {
	t.Helper()
	select {
	case <-r.cancelCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cancel")
	}
}

func (r *callbackRecorder) waitForState(t *testing.T, want State) {
	t.Helper()
	for {
		select {
		case got := <-r.stateCh:
			if got == want {
				return
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for state %s", want)
		}
	}
}
