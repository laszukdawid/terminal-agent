package stt

import (
	"context"
	"sync"

	"github.com/laszukdawid/terminal-agent/internal/voice"
)

type LazyTranscriber struct {
	mu          sync.Mutex
	factory     func() (voice.Transcriber, error)
	transcriber voice.Transcriber
}

func NewLazyTranscriber(factory func() (voice.Transcriber, error)) *LazyTranscriber {
	return &LazyTranscriber{factory: factory}
}

func (t *LazyTranscriber) Transcribe(ctx context.Context, rec voice.Recording) (voice.Transcript, error) {
	transcriber, err := t.get()
	if err != nil {
		return voice.Transcript{}, err
	}
	return transcriber.Transcribe(ctx, rec)
}

func (t *LazyTranscriber) get() (voice.Transcriber, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.transcriber != nil {
		return t.transcriber, nil
	}
	transcriber, err := t.factory()
	if err != nil {
		return nil, err
	}
	t.transcriber = transcriber
	return transcriber, nil
}
