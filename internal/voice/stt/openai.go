package stt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/auth"
	"github.com/laszukdawid/terminal-agent/internal/voice"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

type openAITranscriptionNewFunc func(context.Context, openai.AudioTranscriptionNewParams) (*openai.Transcription, error)

type OpenAITranscriber struct {
	newTranscription openAITranscriptionNewFunc
	model            openai.AudioModel
	language         string
}

func NewOpenAITranscriber(model string, language string) (*OpenAITranscriber, error) {
	resolvedAuth, err := auth.NewManager().ResolveOpenAIAPIKeyAuth()
	if err != nil {
		return nil, fmt.Errorf("openai speech-to-text auth is not configured: %w", err)
	}
	clientOptions := []option.RequestOption{option.WithAPIKey(resolvedAuth.Token)}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		clientOptions = append(clientOptions, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(clientOptions...)
	return newOpenAITranscriberWithClient(func(ctx context.Context, params openai.AudioTranscriptionNewParams) (*openai.Transcription, error) {
		return client.Audio.Transcriptions.New(ctx, params)
	}, model, language)
}

func newOpenAITranscriberWithClient(newTranscription openAITranscriptionNewFunc, model string, language string) (*OpenAITranscriber, error) {
	audioModel, err := openAIAudioModel(model)
	if err != nil {
		return nil, err
	}
	return &OpenAITranscriber{
		newTranscription: newTranscription,
		model:            audioModel,
		language:         strings.TrimSpace(language),
	}, nil
}

func openAIAudioModel(model string) (openai.AudioModel, error) {
	switch strings.TrimSpace(model) {
	case string(openai.AudioModelGPT4oMiniTranscribe):
		return openai.AudioModelGPT4oMiniTranscribe, nil
	case string(openai.AudioModelGPT4oTranscribe):
		return openai.AudioModelGPT4oTranscribe, nil
	case string(openai.AudioModelWhisper1):
		return openai.AudioModelWhisper1, nil
	default:
		return "", fmt.Errorf("unsupported OpenAI speech-to-text model %q", model)
	}
}

func (t *OpenAITranscriber) Transcribe(ctx context.Context, rec voice.Recording) (voice.Transcript, error) {
	if rec.Format != voice.AudioFormatWAV {
		return voice.Transcript{}, fmt.Errorf("openai speech-to-text requires wav audio, got %q", rec.Format)
	}
	if len(rec.Data) == 0 {
		return voice.Transcript{}, voice.ErrEmptyRecording
	}
	params := openai.AudioTranscriptionNewParams{
		File:  namedBytesReader{name: "audio.wav", Reader: bytes.NewReader(rec.Data)},
		Model: t.model,
	}
	if t.language != "" {
		params.Language = param.NewOpt(t.language)
	}
	res, err := t.newTranscription(ctx, params)
	if err != nil {
		return voice.Transcript{}, err
	}
	return voice.Transcript{Text: res.Text}, nil
}

type namedBytesReader struct {
	name string
	*bytes.Reader
}

func (r namedBytesReader) Name() string {
	return r.name
}
