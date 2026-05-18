package commands

import (
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/assert"
)

func TestFormatTaskOutput(t *testing.T) {
	t.Run("plain response only", func(t *testing.T) {
		output := formatTaskOutput(app.TaskResult{Response: "done"}, true)
		assert.Equal(t, "done", output)
	})

	t.Run("direct raw output", func(t *testing.T) {
		output := formatTaskOutput(app.TaskResult{
			Response:        "drwxr-xr-x\t2 user staff\n-rw-r--r--\t1 user staff",
			RawOutputTool:   tools.ToolNameUnix,
			RawOutput:       "drwxr-xr-x\t2 user staff\n-rw-r--r--\t1 user staff",
			DirectRawOutput: true,
		}, false)

		assert.Equal(t, "drwxr-xr-x\t2 user staff\n-rw-r--r--\t1 user staff\n", output)
	})

	t.Run("plain response with raw output", func(t *testing.T) {
		output := formatTaskOutput(app.TaskResult{
			Response:      "Here are the files.",
			RawOutputTool: tools.ToolNameUnix,
			RawOutput:     "drwxr-xr-x\t2 user staff\n-rw-r--r--\t1 user staff",
		}, true)

		assert.Contains(t, output, "Here are the files.\n\n")
		assert.Contains(t, output, "Raw output from "+tools.ToolNameUnix+":\n")
		assert.Contains(t, output, "drwxr-xr-x\t2 user staff\n-rw-r--r--\t1 user staff")
	})
}
