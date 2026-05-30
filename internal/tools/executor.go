package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	log "github.com/laszukdawid/terminal-agent/internal/utils"
)

type BashExecutor struct {
	workDir string
}

func (b *BashExecutor) Exec(code string) (string, error) {
	return b.ExecContext(context.Background(), code)
}

func (b *BashExecutor) ExecContext(ctx context.Context, code string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	log.Debugw("Executing Unix command", "command", code)
	cmd := exec.CommandContext(ctx, "bash", "-c", code)
	configureCommandCancellation(cmd)

	// Set working directory if provided
	if b.workDir != "" {
		cmd.Dir = b.workDir
	}

	// Gather cmd results
	output, err := cmd.CombinedOutput()
	strOutput := string(output)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return strOutput, ctxErr
		}
		return strOutput, fmt.Errorf("bash command returned non-zero status: %w\nOutput: %s", err, strOutput)
	}

	return strings.TrimSpace(strOutput), nil
}
