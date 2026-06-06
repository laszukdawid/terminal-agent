package gui

import (
	"context"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/voice"
)

func (g *App) initVoice(options VoiceOptions) {
	g.state.voiceState = voice.StateIdle
	if options.Recorder == nil || options.Transcriber == nil {
		return
	}
	g.voiceController = voice.NewController(options.Recorder, options.Transcriber, voice.ControllerOptions{
		MaxRecordingDuration: g.cfg.GetGUIVoiceMaxRecordingDuration(),
		Callbacks: voice.ControllerCallbacks{
			OnState: func(state voice.State) {
				g.scheduleVoiceUI(func() {
					g.state.voiceState = state
					if state == voice.StateRecording {
						g.state.voiceError = ""
						g.state.errorText = ""
					}
					g.render()
				})
			},
			OnTranscript: func(transcript voice.Transcript) {
				g.scheduleVoiceUI(func() {
					text := strings.TrimSpace(transcript.Text)
					if text == "" {
						g.state.voiceError = voice.ErrEmptyTranscript.Error()
						g.state.errorText = g.state.voiceError
						g.render()
						return
					}
					g.state.input = text
					g.render()
					if g.cfg.GetGUIVoiceAutoSubmit() {
						g.submit()
					}
				})
			},
			OnError: func(err error) {
				g.scheduleVoiceUI(func() {
					g.state.voiceError = err.Error()
					g.state.errorText = g.state.voiceError
					g.render()
				})
			},
			OnCancel: func() {
				g.scheduleVoiceUI(func() {
					g.state.voiceError = ""
					g.render()
				})
			},
		},
	})
	if options.Trigger != nil {
		options.Trigger.Register(g.toggleVoice)
	}
}

func (g *App) toggleVoice() {
	if !g.cfg.GetGUIVoiceEnabled() || g.voiceController == nil {
		return
	}
	if g.state.isRunning && g.state.voiceState == voice.StateIdle {
		return
	}
	_ = g.voiceController.Toggle(context.Background())
}

func (g *App) cancelVoice() {
	if g.voiceController == nil {
		return
	}
	_ = g.voiceController.Cancel(context.Background())
}

func (g *App) scheduleVoiceUI(fn func()) {
	if g.voiceSchedule == nil {
		fn()
		return
	}
	g.voiceSchedule(fn)
}
