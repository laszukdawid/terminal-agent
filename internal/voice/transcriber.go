package voice

import "context"

type Transcriber interface {
	Transcribe(ctx context.Context, rec Recording) (Transcript, error)
}
