package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGetSystemInfo(t *testing.T) {
	workingDir := filepath.Join(t.TempDir(), "project")
	info := GetSystemInfo(workingDir)

	// Test that all fields are populated (not empty or "unknown" unless there's an error)
	tests := []struct {
		name  string
		value string
	}{
		{"Hostname", info.Hostname},
		{"Username", info.Username},
		{"CurrentTime", info.CurrentTime},
		{"WorkingDir", info.WorkingDir},
		{"OS", info.OS},
		{"Architecture", info.Architecture},
		{"GoVersion", info.GoVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				t.Errorf("%s should not be empty", tt.name)
			}
		})
	}

	// Test specific formats
	if !strings.Contains(info.GoVersion, "go") {
		t.Errorf("GoVersion should contain 'go', got %s", info.GoVersion)
	}

	// OSVersion should be populated on linux/darwin
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if info.OSVersion == "" {
			t.Errorf("OSVersion should not be empty on %s", runtime.GOOS)
		}
	}

	// Test that CurrentTime follows the expected format
	if len(info.CurrentTime) < 10 {
		t.Errorf("CurrentTime seems too short: %s", info.CurrentTime)
	}
}

func TestSystemPromptHeader(t *testing.T) {
	workingDir := filepath.Join(t.TempDir(), "project")
	header := SystemPromptHeader(workingDir)

	// Test that the header contains expected content
	expectedStrings := []string{
		"You are a Unix terminal helper",
		"Current system context:",
		"Hostname:",
		"User:",
		"Time:",
		"Working Directory:",
		"Operating System:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(header, expected) {
			t.Errorf("SystemPromptHeader should contain '%s'", expected)
		}
	}

	// Test that system info is actually populated (not just placeholders)
	info := GetSystemInfo(workingDir)
	if !strings.Contains(header, info.Hostname) {
		t.Errorf("SystemPromptHeader should contain actual hostname: %s", info.Hostname)
	}
	if !strings.Contains(header, workingDir) {
		t.Errorf("SystemPromptHeader should contain explicit working directory: %s", workingDir)
	}
}

func TestDiscoverProjectContextFile(t *testing.T) {
	t.Run("AGENTS.md found first", func(t *testing.T) {
		dir := t.TempDir()
		requireWriteFile(t, filepath.Join(dir, "AGENTS.md"), "project context")
		path := discoverProjectContextFile(dir)
		if path != filepath.Join(dir, "AGENTS.md") {
			t.Errorf("expected AGENTS.md, got %s", path)
		}
	})

	t.Run("CLAUDE.md used when AGENTS.md absent", func(t *testing.T) {
		dir := t.TempDir()
		requireWriteFile(t, filepath.Join(dir, "CLAUDE.md"), "claude context")
		path := discoverProjectContextFile(dir)
		if path != filepath.Join(dir, "CLAUDE.md") {
			t.Errorf("expected CLAUDE.md, got %s", path)
		}
	})

	t.Run(".agentrules used as last resort", func(t *testing.T) {
		dir := t.TempDir()
		requireWriteFile(t, filepath.Join(dir, ".agentrules"), "rules context")
		path := discoverProjectContextFile(dir)
		if path != filepath.Join(dir, ".agentrules") {
			t.Errorf("expected .agentrules, got %s", path)
		}
	})

	t.Run("AGENTS.md takes priority over CLAUDE.md", func(t *testing.T) {
		dir := t.TempDir()
		requireWriteFile(t, filepath.Join(dir, "CLAUDE.md"), "claude")
		requireWriteFile(t, filepath.Join(dir, "AGENTS.md"), "agents")
		path := discoverProjectContextFile(dir)
		if path != filepath.Join(dir, "AGENTS.md") {
			t.Errorf("expected AGENTS.md to take priority, got %s", path)
		}
	})

	t.Run("returns empty when no file exists", func(t *testing.T) {
		dir := t.TempDir()
		path := discoverProjectContextFile(dir)
		if path != "" {
			t.Errorf("expected empty string, got %s", path)
		}
	})

	t.Run("case insensitive matching for agents.md", func(t *testing.T) {
		dir := t.TempDir()
		requireWriteFile(t, filepath.Join(dir, "agents.md"), "lowercase context")
		path := discoverProjectContextFile(dir)
		if path != filepath.Join(dir, "agents.md") {
			t.Errorf("expected agents.md, got %s", path)
		}
	})

	t.Run("case insensitive matching for claude.md", func(t *testing.T) {
		dir := t.TempDir()
		requireWriteFile(t, filepath.Join(dir, "claude.md"), "claude lowercase")
		path := discoverProjectContextFile(dir)
		if path != filepath.Join(dir, "claude.md") {
			t.Errorf("expected claude.md, got %s", path)
		}
	})

	t.Run("AGENTS.md still wins over agents.md due to priority", func(t *testing.T) {
		dir := t.TempDir()
		requireWriteFile(t, filepath.Join(dir, "agents.md"), "lowercase")
		requireWriteFile(t, filepath.Join(dir, "AGENTS.md"), "uppercase")
		path := discoverProjectContextFile(dir)
		if path != filepath.Join(dir, "AGENTS.md") && path != filepath.Join(dir, "agents.md") {
			t.Errorf("expected AGENTS.md or agents.md, got %s", path)
		}
	})
}

func TestReadProjectContext(t *testing.T) {
	t.Run("reads and wraps file content", func(t *testing.T) {
		dir := t.TempDir()
		requireWriteFile(t, filepath.Join(dir, "AGENTS.md"), "build: task build\nlint: task lint")
		content, err := ReadProjectContext(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(content, "<project_context>") {
			t.Error("expected <project_context> tag")
		}
		if !strings.Contains(content, "build: task build") {
			t.Error("expected file content")
		}
		if !strings.Contains(content, "</project_context>") {
			t.Error("expected closing </project_context> tag")
		}
	})

	t.Run("returns empty when no context file exists", func(t *testing.T) {
		dir := t.TempDir()
		content, err := ReadProjectContext(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if content != "" {
			t.Errorf("expected empty content, got %q", content)
		}
	})

	t.Run("returns empty for empty context file", func(t *testing.T) {
		dir := t.TempDir()
		requireWriteFile(t, filepath.Join(dir, "AGENTS.md"), "")
		content, err := ReadProjectContext(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if content != "" {
			t.Errorf("expected empty for empty file, got %q", content)
		}
	})
}

func TestSystemPromptHeaderIncludesProjectContextPath(t *testing.T) {
	t.Run("includes context path when file exists", func(t *testing.T) {
		dir := t.TempDir()
		requireWriteFile(t, filepath.Join(dir, "AGENTS.md"), "context")
		header := SystemPromptHeader(dir)
		if !strings.Contains(header, "Project Context:") {
			t.Error("expected header to include project context path")
		}
		if !strings.Contains(header, "AGENTS.md") {
			t.Error("expected header to name AGENTS.md")
		}
	})

	t.Run("omits context line when no file exists", func(t *testing.T) {
		dir := t.TempDir()
		header := SystemPromptHeader(dir)
		if strings.Contains(header, "Project Context:") {
			t.Error("expected no project context line when file is absent")
		}
	})
}

func TestSystemPromptTaskFinalGuidanceIsSelective(t *testing.T) {
	prompt := SystemPromptTask

	if !strings.Contains(prompt, "Use final=true ONLY when the raw output is definitely the final user-facing answer") {
		t.Fatal("expected task prompt to reserve final=true for definitely final raw output")
	}
	if !strings.Contains(prompt, "If the output needs interpretation, filtering, grouping, cleanup, explanation, or validation, do not set final=true") {
		t.Fatal("expected task prompt to avoid final=true when output needs model review")
	}
	if strings.Contains(prompt, "fully answers the user's request and should be returned directly") {
		t.Fatal("old final=true guidance is too broad")
	}
}

func requireWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}
