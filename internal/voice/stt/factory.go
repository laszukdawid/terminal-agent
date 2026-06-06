package stt

import (
	"fmt"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/voice"
)

func NewTranscriber(cfg config.Config) (voice.Transcriber, error) {
	switch strings.TrimSpace(cfg.GetGUIVoiceSTTBackend()) {
	case config.DefaultGUIVoiceSTTBackend:
		return NewOpenAITranscriber(cfg.GetGUIVoiceSTTModel(), cfg.GetGUIVoiceSTTLanguage())
	default:
		return nil, fmt.Errorf("unsupported speech-to-text backend %q", cfg.GetGUIVoiceSTTBackend())
	}
}
