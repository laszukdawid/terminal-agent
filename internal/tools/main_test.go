package tools

import (
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/config"
)

func TestBuiltinToolsIncludeNativeTools(t *testing.T) {
	tools := GetAllBuiltinTools(config.NewDefaultConfig())

	for _, name := range []string{"file_edit", "file_search", "python"} {
		if tools[name] == nil {
			t.Fatalf("expected builtin tool %q to be registered", name)
		}
	}
}
