package audio

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMalgoRecorderRejectsConcurrentFinish(t *testing.T) {
	recorder := &MalgoRecorder{started: true, finishing: true}

	_, err := recorder.Stop(context.Background())
	require.Error(t, err)

	err = recorder.Cancel(context.Background())
	require.Error(t, err)
}
