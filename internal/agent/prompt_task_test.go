package agent

import (
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRenderTaskTemplateReturnsErrorOnExecuteFailure guards BUG-10: a template
// execution failure on the task hot path must surface as an error rather than
// panicking the run goroutine. Referencing a field that does not exist on the
// data value is an execution-time error (not a parse error).
func TestRenderTaskTemplateReturnsErrorOnExecuteFailure(t *testing.T) {
	tmpl := template.Must(template.New("broken").Parse(`{{.Missing.Field}}`))

	out, err := renderTaskTemplate(tmpl, struct{}{})

	require.Error(t, err)
	assert.Empty(t, out)
	assert.Contains(t, err.Error(), "broken", "error should name the failing template")
}
