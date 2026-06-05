package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskToolOutputWriter(t *testing.T) {
	t.Run("warns and continues on display error", func(t *testing.T) {
		displayErr := errors.New("display failed")
		var warnedTool string
		var warnedPID int
		var warnedErr error
		writeCalls := 0

		writer := newTaskToolOutputWriter(
			context.Background(),
			tools.ToolNameUnix,
			func(event TaskToolOutputEvent) error {
				if event.Err != nil {
					warnedTool = event.ToolName
					warnedPID = event.ProcessID
					warnedErr = event.Err
					return nil
				}
				writeCalls++
				return displayErr
			},
		)
		processWriter, ok := writer.(interface{ ProcessStarted(int) })
		require.True(t, ok)
		processWriter.ProcessStarted(1234)

		n, err := writer.Write([]byte("first"))
		require.NoError(t, err)
		assert.Equal(t, 5, n)

		n, err = writer.Write([]byte("second"))
		require.NoError(t, err)
		assert.Equal(t, 6, n)

		assert.Equal(t, 1, writeCalls)
		assert.Equal(t, tools.ToolNameUnix, warnedTool)
		assert.Equal(t, 1234, warnedPID)
		assert.ErrorIs(t, warnedErr, displayErr)
	})

	t.Run("returns context error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		writer := newTaskToolOutputWriter(
			ctx,
			tools.ToolNameUnix,
			func(TaskToolOutputEvent) error { return errors.New("display failed") },
		)

		n, err := writer.Write([]byte("output"))

		assert.Equal(t, 0, n)
		assert.ErrorIs(t, err, context.Canceled)
	})
}
