package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	log "github.com/laszukdawid/terminal-agent/internal/utils"
)

type BashExecutor struct {
	workDir string
	output  io.Writer
}

func (b *BashExecutor) Exec(code string) (string, error) {
	return b.ExecContext(context.Background(), code)
}

func (b *BashExecutor) ExecContext(ctx context.Context, code string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	log.Debugw("Executing Unix command", "command", code)
	cmd := exec.CommandContext(ctx, "bash", "-o", "pipefail", "-c", code)
	configureCommandCancellation(cmd)

	// Set working directory if provided
	if b.workDir != "" {
		cmd.Dir = b.workDir
	}

	output, err := runCombinedOutput(cmd, b.output)
	strOutput := string(output)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return strOutput, ctxErr
		}
		return strOutput, fmt.Errorf("bash command returned non-zero status: %w\nOutput: %s", err, strOutput)
	}

	return strings.TrimSpace(strOutput), nil
}

type combinedOutputWriter struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	stream io.Writer
}

func (w *combinedOutputWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.buf.Write(p); err != nil {
		return 0, err
	}
	if w.stream != nil {
		if _, err := w.stream.Write(p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func runCombinedOutput(cmd *exec.Cmd, stream io.Writer) ([]byte, error) {
	writer := &combinedOutputWriter{stream: stream}
	cmd.Stdout = writer
	cmd.Stderr = writer
	err := cmd.Start()
	if err != nil {
		return writer.buf.Bytes(), err
	}
	if processWriter, ok := stream.(ProcessesStartedWriter); ok && cmd.Process != nil {
		processWriter.ProcessStarted(cmd.Process.Pid)
	}
	err = cmd.Wait()
	return writer.buf.Bytes(), err
}
