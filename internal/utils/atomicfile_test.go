package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type atomicPayload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestWriteJSONAtomicWritesAndOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "data.json")

	require.NoError(t, WriteJSONAtomic(path, atomicPayload{Name: "first", Count: 1}))

	var got atomicPayload
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(content, &got))
	assert.Equal(t, atomicPayload{Name: "first", Count: 1}, got)
	assert.Contains(t, string(content), "  \"name\"", "output should be indented")

	// A second write replaces the file in place.
	require.NoError(t, WriteJSONAtomic(path, atomicPayload{Name: "second", Count: 2}))
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(content, &got))
	assert.Equal(t, atomicPayload{Name: "second", Count: 2}, got)

	// The temp file must be renamed away, never left behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp-", "temp file should be renamed, not left behind")
	}
}

// TestWriteJSONAtomicLeavesDestinationUntouchedOnEncodeFailure verifies the
// failure-safety that motivates the atomic write: if encoding fails the original
// destination is never created or corrupted.
func TestWriteJSONAtomicLeavesDestinationUntouchedOnEncodeFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	// A channel cannot be marshaled to JSON, forcing the encode to fail.
	err := WriteJSONAtomic(path, make(chan int))
	require.Error(t, err)

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "destination must not be created when encoding fails")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no temp file should remain after a failed write")
}
