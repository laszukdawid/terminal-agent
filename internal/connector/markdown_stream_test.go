package connector

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what was
// written. The renderer prints via fmt.Print, so tests capture it this way.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	defer func() { os.Stdout = old }() // restore even if fn panics
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}

// stripANSI removes ANSI escape sequences so assertions can match on text.
func stripANSI(s string) string {
	for {
		start := strings.Index(s, "\033[")
		if start < 0 {
			return s
		}
		end := strings.IndexByte(s[start:], 'm')
		if end < 0 {
			return s
		}
		s = s[:start] + s[start+end+1:]
	}
}

// TestMarkdownStreamRendererStreamsPlainText guards that removing the dead
// code-block state did not change the core behavior: streamed plain text is
// rendered in full.
func TestMarkdownStreamRendererStreamsPlainText(t *testing.T) {
	out := captureStdout(t, func() {
		r, err := NewMarkdownStreamRenderer()
		require.NoError(t, err)
		for _, chunk := range []string{"Hello ", "world, ", "this is a ", "streamed response ", "that exceeds the flush threshold comfortably."} {
			r.ProcessChunk(chunk)
		}
		r.Flush()
	})

	assert.Contains(t, stripANSI(out), "Hello world, this is a streamed response that exceeds the flush threshold comfortably.")
}

// BenchmarkProcessChunk feeds a large simulated stream. After the PERF-1 fix each
// chunk is O(chunk) rather than O(response), so total work is linear in the
// stream length instead of quadratic.
func BenchmarkProcessChunk(b *testing.B) {
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = devnull
	defer func() { os.Stdout = old; devnull.Close() }()

	chunk := strings.Repeat("word ", 20) // ~100 chars, no fences
	const chunks = 5000                  // ~500 KB simulated response

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, _ := NewMarkdownStreamRenderer()
		for j := 0; j < chunks; j++ {
			r.ProcessChunk(chunk)
		}
		r.Flush()
	}
}
