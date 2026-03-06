package tools

import (
	"fmt"
	"os/exec"
	"strings"
)

type BashExecutor struct {
	workDir string
}

func (b *BashExecutor) Exec(code string) (string, error) {

	// Prepare command for execution
	fmt.Printf("Executing Unix command: %s\n", code)
	cmd := exec.Command("bash", "-c", code)

	// Set working directory if provided
	if b.workDir != "" {
		fmt.Printf("Working directory: %s\n", b.workDir)
		cmd.Dir = b.workDir
	}

	// Gather cmd results
	output, err := cmd.CombinedOutput()
	strOutput := string(output)
	if err != nil {
		return strOutput, fmt.Errorf("bash command returned non-zero status: %w\nOutput: %s", err, strOutput)
	}

	return strings.TrimSpace(strOutput), nil
}
