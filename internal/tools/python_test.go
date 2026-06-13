package tools

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPythonToolTimeoutReturnsCapturedOutput(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	tool := NewPythonTool(t.TempDir())

	out, err := tool.RunSchema(map[string]any{
		"runner":  "python3",
		"code":    "import time; print('start', flush=True); time.sleep(5)",
		"timeout": "1s",
	})

	assert.NoError(t, err)
	assert.Equal(t, "start", out)
}
