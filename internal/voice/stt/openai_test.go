package stt

import (
	"context"
	"io"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/voice"
	openai "github.com/openai/openai-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAITranscriberBuildsRequest(t *testing.T) {
	client := &fakeOpenAITranscriptionClient{transcription: &openai.Transcription{Text: "hello"}}
	transcriber, err := newOpenAITranscriberWithClient(client.new, "gpt-4o-mini-transcribe", "en")
	require.NoError(t, err)

	transcript, err := transcriber.Transcribe(context.Background(), voice.Recording{
		Data:     []byte("wav bytes"),
		Format:   voice.AudioFormatWAV,
		MIMEType: "audio/wav",
	})
	require.NoError(t, err)

	assert.Equal(t, "hello", transcript.Text)
	assert.Equal(t, openai.AudioModelGPT4oMiniTranscribe, client.params.Model)
	assert.Equal(t, "en", client.params.Language.Value)
	named, ok := client.params.File.(interface{ Name() string })
	require.True(t, ok)
	assert.Equal(t, "audio.wav", named.Name())
	data, err := io.ReadAll(client.params.File)
	require.NoError(t, err)
	assert.Equal(t, []byte("wav bytes"), data)
}

func TestOpenAITranscriberOmitsEmptyLanguage(t *testing.T) {
	client := &fakeOpenAITranscriptionClient{transcription: &openai.Transcription{Text: "hello"}}
	transcriber, err := newOpenAITranscriberWithClient(client.new, "whisper-1", "")
	require.NoError(t, err)

	_, err = transcriber.Transcribe(context.Background(), voice.Recording{Data: []byte("wav"), Format: voice.AudioFormatWAV})
	require.NoError(t, err)

	assert.Equal(t, openai.AudioModelWhisper1, client.params.Model)
	assert.True(t, client.params.Language.IsOmitted())
}

func TestOpenAITranscriberRejectsUnsupportedInput(t *testing.T) {
	client := &fakeOpenAITranscriptionClient{transcription: &openai.Transcription{Text: "hello"}}
	transcriber, err := newOpenAITranscriberWithClient(client.new, "gpt-4o-mini-transcribe", "")
	require.NoError(t, err)

	_, err = transcriber.Transcribe(context.Background(), voice.Recording{Data: []byte("pcm"), Format: voice.AudioFormatPCM16LE})
	require.Error(t, err)
	assert.Equal(t, 0, client.calls)
}

func TestOpenAITranscriberRejectsUnknownModel(t *testing.T) {
	_, err := newOpenAITranscriberWithClient((&fakeOpenAITranscriptionClient{}).new, "not-a-model", "")
	require.Error(t, err)
}

type fakeOpenAITranscriptionClient struct {
	calls         int
	params        openai.AudioTranscriptionNewParams
	transcription *openai.Transcription
	err           error
}

func (c *fakeOpenAITranscriptionClient) new(_ context.Context, params openai.AudioTranscriptionNewParams) (*openai.Transcription, error) {
	c.calls++
	c.params = params
	return c.transcription, c.err
}
