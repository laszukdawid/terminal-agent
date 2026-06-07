package gui

import (
	"context"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	appservice "github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/voice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVoiceToggleStartsRecording(t *testing.T) {
	g := newVoiceTestApp(t, voiceTestOptions{})

	g.toggleVoice()

	g.scheduler.read(func() {
		assert.Equal(t, voice.StateRecording, g.state.voiceState)
		assert.Equal(t, "LISTENING", g.popup.listenButton.Text)
	})
}

func TestVoiceTranscriptAutoSubmits(t *testing.T) {
	service := newVoiceTestService()
	g := newVoiceTestApp(t, voiceTestOptions{service: service, transcript: "hello from voice"})

	g.toggleVoice()
	g.toggleVoice()

	service.waitForAsk(t)
	assert.Equal(t, "hello from voice", service.askMessage)
	g.scheduler.read(func() {
		assert.Equal(t, "hello from voice", g.popup.input.Text)
		assert.Equal(t, voice.StateIdle, g.state.voiceState)
	})
}

func TestVoiceTranscriptDoesNotSubmitWhenAutoSubmitDisabled(t *testing.T) {
	service := newVoiceTestService()
	g := newVoiceTestApp(t, voiceTestOptions{service: service, cfg: voiceGUIConfig{autoSubmit: false}, transcript: "draft only"})

	g.toggleVoice()
	g.toggleVoice()
	require.Eventually(t, func() bool {
		var ok bool
		g.scheduler.read(func() {
			ok = g.popup.input.Text == "draft only"
		})
		return ok
	}, time.Second, time.Millisecond)

	g.scheduler.read(func() {
		assert.Equal(t, "draft only", g.popup.input.Text)
	})
	assert.Equal(t, 0, service.askCalls)
}

func TestVoiceDoesNotStartWhileAskIsRunning(t *testing.T) {
	recorder := &guiFakeRecorder{}
	g := newVoiceTestApp(t, voiceTestOptions{recorder: recorder})
	g.state.isRunning = true

	g.toggleVoice()

	g.scheduler.read(func() {
		assert.Equal(t, 0, recorder.starts)
		assert.Equal(t, voice.StateIdle, g.state.voiceState)
		assert.Equal(t, voiceBlockedWhileRunningStatus, g.state.voiceError)
		assert.Equal(t, "", g.state.status)
	})
}

func TestVoiceBlockedWhileRespondingDoesNotOverwriteStatus(t *testing.T) {
	g := newVoiceTestApp(t, voiceTestOptions{})
	g.state.isRunning = true
	g.state.status = "responding"

	g.toggleVoice()

	g.scheduler.read(func() {
		assert.Equal(t, "responding", g.state.status)
		assert.Equal(t, voiceBlockedWhileRunningStatus, g.state.voiceError)
	})
}

func TestVoiceButtonDisabledWhileAskIsRunning(t *testing.T) {
	g := newVoiceTestApp(t, voiceTestOptions{})
	g.state.isRunning = true
	g.render()

	g.scheduler.read(func() {
		assert.True(t, g.popup.listenButton.Disabled())
		assert.Equal(t, "BUSY", g.popup.listenButton.Text)
	})
}

func TestVoiceTriggerWorksWhenInputIsNotFocused(t *testing.T) {
	g := newVoiceTestApp(t, voiceTestOptions{})
	g.popup.window.Canvas().Unfocus()

	handler := g.popup.window.Canvas().OnTypedKey()
	require.NotNil(t, handler)
	handler(&fyne.KeyEvent{Name: fyne.KeyF1})

	g.scheduler.read(func() {
		assert.Equal(t, voice.StateRecording, g.state.voiceState)
	})
}

func TestVoiceButtonRestoresInputFocus(t *testing.T) {
	g := newVoiceTestApp(t, voiceTestOptions{})
	g.popup.window.Canvas().Focus(g.popup.listenButton)

	g.popup.listenButton.OnTapped()

	g.scheduler.read(func() {
		assert.Same(t, g.popup.input, g.popup.window.Canvas().Focused())
	})
}

func TestSubmitButtonRestoresInputFocus(t *testing.T) {
	g := newVoiceTestApp(t, voiceTestOptions{})
	g.state.input = "typed question"
	g.render()
	g.popup.window.Canvas().Focus(g.popup.actionButton)

	g.popup.actionButton.OnTapped()

	g.scheduler.read(func() {
		assert.Same(t, g.popup.input, g.popup.window.Canvas().Focused())
	})
}

type voiceTestOptions struct {
	cfg        config.Config
	service    *voiceTestService
	recorder   *guiFakeRecorder
	transcript string
}

type voiceTestApp struct {
	*App
	scheduler *voiceTestScheduler
}

func newVoiceTestApp(t *testing.T, opts voiceTestOptions) *voiceTestApp {
	t.Helper()
	cfg := opts.cfg
	if cfg == nil {
		cfg = voiceGUIConfig{autoSubmit: true}
	}
	service := opts.service
	if service == nil {
		service = newVoiceTestService()
	}
	recorder := opts.recorder
	if recorder == nil {
		recorder = &guiFakeRecorder{recording: voice.Recording{Data: []byte("wav"), Format: voice.AudioFormatWAV}}
	}
	transcript := opts.transcript
	if transcript == "" {
		transcript = "voice prompt"
	}

	scheduler := newVoiceTestScheduler()
	g := NewApp(service, cfg, AppOptions{
		AppID:   "terminal-agent-voice-test",
		FyneApp: test.NewApp(),
		Voice: VoiceOptions{
			Recorder:    recorder,
			Transcriber: &guiFakeTranscriber{transcript: voice.Transcript{Text: transcript}},
			Schedule:    scheduler.schedule,
		},
	})
	t.Cleanup(func() {
		scheduler.close()
		g.fyneApp.Quit()
	})
	return &voiceTestApp{App: g, scheduler: scheduler}
}

type voiceTestScheduler struct {
	jobs chan voiceTestJob
	done chan struct{}
}

type voiceTestJob struct {
	fn   func()
	done chan struct{}
}

func newVoiceTestScheduler() *voiceTestScheduler {
	s := &voiceTestScheduler{
		jobs: make(chan voiceTestJob),
		done: make(chan struct{}),
	}
	go func() {
		defer close(s.done)
		for job := range s.jobs {
			job.fn()
			close(job.done)
		}
	}()
	return s
}

func (s *voiceTestScheduler) schedule(fn func()) {
	done := make(chan struct{})
	s.jobs <- voiceTestJob{fn: fn, done: done}
	<-done
}

func (s *voiceTestScheduler) read(fn func()) {
	s.schedule(fn)
}

func (s *voiceTestScheduler) close() {
	close(s.jobs)
	<-s.done
}

type voiceGUIConfig struct {
	autoSubmit bool
}

func (c voiceGUIConfig) GetDefaultProvider() string                     { return "openai" }
func (c voiceGUIConfig) GetDefaultModelId() string                      { return "gpt-4o-mini" }
func (c voiceGUIConfig) GetModelIdForProvider(string) string            { return "gpt-4o-mini" }
func (c voiceGUIConfig) GetLlamaModels() map[string]string              { return nil }
func (c voiceGUIConfig) GetDevice() string                              { return "auto" }
func (c voiceGUIConfig) GetGUIEnvFile() string                          { return "" }
func (c voiceGUIConfig) GetGUILoadShellEnvironment() bool               { return false }
func (c voiceGUIConfig) GetGUIShellEnvironmentTimeout() time.Duration   { return time.Second }
func (c voiceGUIConfig) GetGUIVoiceEnabled() bool                       { return true }
func (c voiceGUIConfig) GetGUIVoiceTriggerKey() string                  { return config.DefaultGUIVoiceTriggerKey }
func (c voiceGUIConfig) GetGUIVoiceAutoSubmit() bool                    { return c.autoSubmit }
func (c voiceGUIConfig) GetGUIVoiceMaxRecordingDuration() time.Duration { return time.Minute }
func (c voiceGUIConfig) GetGUIVoiceSTTBackend() string                  { return config.DefaultGUIVoiceSTTBackend }
func (c voiceGUIConfig) GetGUIVoiceSTTModel() string                    { return config.DefaultGUIVoiceSTTModel }
func (c voiceGUIConfig) GetGUIVoiceSTTLanguage() string                 { return "" }
func (c voiceGUIConfig) SetDefaultProvider(string) error                { return nil }
func (c voiceGUIConfig) SetDefaultModelId(string) error                 { return nil }
func (c voiceGUIConfig) SetDevice(string) error                         { return nil }
func (c voiceGUIConfig) GetMcpFilePath() string                         { return "" }
func (c voiceGUIConfig) SetMcpFilePath(string) error                    { return nil }
func (c voiceGUIConfig) GetConfiguredWorkingDir() string                { return "" }
func (c voiceGUIConfig) GetWorkingDir() string                          { return "" }
func (c voiceGUIConfig) SetWorkingDir(string) error                     { return nil }
func (c voiceGUIConfig) GetMaxTokens() int                              { return 0 }
func (c voiceGUIConfig) GetTaskTimeout() time.Duration                  { return 0 }
func (c voiceGUIConfig) GetTaskLiveOutputLimit() int                    { return 0 }
func (c voiceGUIConfig) GetMemory() bool                                { return false }
func (c voiceGUIConfig) SetMemory(bool) error                           { return nil }
func (c voiceGUIConfig) GetPermissions() config.Permissions             { return config.Permissions{} }
func (c voiceGUIConfig) GetProjectContext() bool                        { return false }

type voiceTestService struct {
	mu         sync.Mutex
	askCalls   int
	askMessage string
	events     chan appservice.Event
	askCh      chan struct{}
}

func newVoiceTestService() *voiceTestService {
	events := make(chan appservice.Event)
	close(events)
	return &voiceTestService{events: events, askCh: make(chan struct{}, 1)}
}

func (s *voiceTestService) Ask(context.Context, appservice.AskRequest) (appservice.AskResult, error) {
	return appservice.AskResult{}, nil
}

func (s *voiceTestService) AskEvents(_ context.Context, req appservice.AskRequest) (<-chan appservice.Event, error) {
	s.mu.Lock()
	s.askCalls++
	s.askMessage = req.Message
	if s.askCh == nil {
		s.askCh = make(chan struct{}, 1)
	}
	s.mu.Unlock()
	s.askCh <- struct{}{}
	return s.events, nil
}

func (s *voiceTestService) Chat(context.Context, appservice.ChatRequest) (appservice.ChatResult, error) {
	return appservice.ChatResult{}, nil
}

func (s *voiceTestService) ChatEvents(context.Context, appservice.ChatRequest) (<-chan appservice.Event, error) {
	return nil, nil
}

func (s *voiceTestService) TaskEvents(context.Context, appservice.TaskRequest) (<-chan appservice.Event, error) {
	return nil, nil
}

func (s *voiceTestService) waitForAsk(t *testing.T) {
	t.Helper()
	select {
	case <-s.askCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for AskEvents")
	}
}

type guiFakeRecorder struct {
	starts    int
	stops     int
	cancels   int
	recording voice.Recording
}

func (r *guiFakeRecorder) Start(context.Context) error {
	r.starts++
	return nil
}

func (r *guiFakeRecorder) Stop(context.Context) (voice.Recording, error) {
	r.stops++
	return r.recording, nil
}

func (r *guiFakeRecorder) Cancel(context.Context) error {
	r.cancels++
	return nil
}

type guiFakeTranscriber struct {
	transcript voice.Transcript
}

func (t *guiFakeTranscriber) Transcribe(context.Context, voice.Recording) (voice.Transcript, error) {
	return t.transcript, nil
}

func waitForVoiceState(t *testing.T, g *App, want voice.State) {
	t.Helper()
	require.Eventually(t, func() bool { return g.state.voiceState == want }, time.Second, time.Millisecond)
}
