package agent

import (
	"context"
	"io"
)

type taskToolOutputWriter struct {
	ctx      context.Context
	toolName string
	pid      int
	disabled bool
	warned   bool
	onOutput func(TaskToolOutputEvent) error
}

func newTaskToolOutputWriter(ctx context.Context, toolName string, onOutput func(TaskToolOutputEvent) error) io.Writer {
	if onOutput == nil {
		return nil
	}
	return &taskToolOutputWriter{
		ctx:      ctx,
		toolName: toolName,
		onOutput: onOutput,
	}
}

func (w *taskToolOutputWriter) ProcessStarted(pid int) {
	w.pid = pid
}

func (w *taskToolOutputWriter) Write(p []byte) (int, error) {
	if w.disabled {
		return len(p), nil
	}
	if err := w.onOutput(TaskToolOutputEvent{ToolName: w.toolName, ProcessID: w.pid, Output: string(p)}); err != nil {
		if ctxErr := w.ctx.Err(); ctxErr != nil {
			return 0, ctxErr
		}
		w.disabled = true
		if !w.warned {
			w.warned = true
			_ = w.onOutput(TaskToolOutputEvent{ToolName: w.toolName, ProcessID: w.pid, Err: err})
		}
	}
	return len(p), nil
}
