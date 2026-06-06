package voice

import "context"

type Recorder interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) (Recording, error)
	Cancel(ctx context.Context) error
}
