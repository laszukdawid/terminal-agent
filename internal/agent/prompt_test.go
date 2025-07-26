package agent

import (
	"strings"
	"testing"
)

func TestGetSystemInfo(t *testing.T) {
	info := GetSystemInfo()

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

	// Test that CurrentTime follows the expected format
	if len(info.CurrentTime) < 10 {
		t.Errorf("CurrentTime seems too short: %s", info.CurrentTime)
	}
}

func TestSystemPromptHeader(t *testing.T) {
	header := SystemPromptHeader()

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
	info := GetSystemInfo()
	if !strings.Contains(header, info.Hostname) {
		t.Errorf("SystemPromptHeader should contain actual hostname: %s", info.Hostname)
	}
}
