package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	log "github.com/laszukdawid/terminal-agent/internal/utils"
)

type BashExecutor struct {
	workDir string
	output  io.Writer
}

type ProcessOptions struct {
	Timeout  time.Duration
	MaxBytes int
}

type ProcessResult struct {
	Output        string
	TimedOut      bool
	Truncated     bool
	CapturedBytes int
}

func (b *BashExecutor) Exec(code string) (string, error) {
	return b.ExecContext(context.Background(), code)
}

func (b *BashExecutor) ExecContext(ctx context.Context, code string) (string, error) {
	result, err := b.ExecContextWithOptions(ctx, code, ProcessOptions{})
	return result.Output, err
}

func (b *BashExecutor) ExecContextWithOptions(ctx context.Context, code string, opts ProcessOptions) (ProcessResult, error) {
	if err := ctx.Err(); err != nil {
		return ProcessResult{}, err
	}

	log.Debugw("Executing Unix command", "command", code)
	processCtx, cancel := processContext(ctx, opts.Timeout)
	defer cancel()
	cmd := exec.CommandContext(processCtx, "bash", "-o", "pipefail", "-c", code)
	configureCommandCancellation(cmd)

	// Set working directory if provided
	if b.workDir != "" {
		cmd.Dir = b.workDir
	}

	result, err := runProcess(ctx, processCtx, cmd, b.output, opts)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result, ctxErr
		}
		return result, fmt.Errorf("bash command returned non-zero status: %w\nOutput: %s", err, result.Output)
	}

	result.Output = strings.TrimSpace(result.Output)
	return result, nil
}

type combinedOutputWriter struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	stream    io.Writer
	maxBytes  int
	truncated bool
}

func (w *combinedOutputWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.maxBytes <= 0 {
		if _, err := w.buf.Write(p); err != nil {
			return 0, err
		}
	} else if w.buf.Len() < w.maxBytes {
		remaining := w.maxBytes - w.buf.Len()
		toWrite := p
		if len(toWrite) > remaining {
			toWrite = toWrite[:remaining]
			w.truncated = true
		}
		if _, err := w.buf.Write(toWrite); err != nil {
			return 0, err
		}
		if len(p) > remaining {
			w.truncated = true
		}
	} else {
		w.truncated = true
	}
	if w.stream != nil {
		if _, err := w.stream.Write(p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func processContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

func runProcess(parentCtx context.Context, processCtx context.Context, cmd *exec.Cmd, stream io.Writer, opts ProcessOptions) (ProcessResult, error) {
	writer := &combinedOutputWriter{stream: stream, maxBytes: opts.MaxBytes}
	cmd.Stdout = writer
	cmd.Stderr = writer

	err := cmd.Start()
	if err == nil {
		if processWriter, ok := stream.(ProcessesStartedWriter); ok && cmd.Process != nil {
			processWriter.ProcessStarted(cmd.Process.Pid)
		}
		err = cmd.Wait()
	}

	result := ProcessResult{
		Output:        string(writer.buf.Bytes()),
		Truncated:     writer.truncated,
		CapturedBytes: writer.buf.Len(),
	}
	if err != nil && parentCtx.Err() == nil && opts.Timeout > 0 && processCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		return result, nil
	}
	return result, err
}

func runCombinedOutput(cmd *exec.Cmd, stream io.Writer) ([]byte, error) {
	result, err := runProcess(context.Background(), context.Background(), cmd, stream, ProcessOptions{})
	return []byte(result.Output), err
}
