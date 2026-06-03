package gui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type envTestConfig struct {
	envFile      string
	loadShell    bool
	shellTimeout time.Duration
}

func (c envTestConfig) GetGUIEnvFile() string {
	return c.envFile
}
func (c envTestConfig) GetGUILoadShellEnvironment() bool { return c.loadShell }
func (c envTestConfig) GetGUIShellEnvironmentTimeout() time.Duration {
	if c.shellTimeout == 0 {
		return time.Second
	}
	return c.shellTimeout
}

func TestParseDotenv(t *testing.T) {
	values := parseDotenv([]byte(`
# comment
OPENAI_API_KEY=plain
export GEMINI_API_KEY="quoted value"
MISTRAL_API_KEY='single quoted'
DISPLAY=ignored
MALFORMED
`))

	if values["OPENAI_API_KEY"] != "plain" {
		t.Fatalf("OPENAI_API_KEY = %q, want plain", values["OPENAI_API_KEY"])
	}
	if values["GEMINI_API_KEY"] != "quoted value" {
		t.Fatalf("GEMINI_API_KEY = %q, want quoted value", values["GEMINI_API_KEY"])
	}
	if values["MISTRAL_API_KEY"] != "single quoted" {
		t.Fatalf("MISTRAL_API_KEY = %q, want single quoted", values["MISTRAL_API_KEY"])
	}
	if _, ok := values["DISPLAY"]; ok {
		t.Fatal("DISPLAY should not be parsed")
	}
}

func TestLoadEnvironmentUsesEnvFileBeforeShell(t *testing.T) {
	clearTestEnv(t, "OPENAI_API_KEY", "GEMINI_API_KEY", "DISPLAY")
	envFile := writeTestEnvFile(t, "OPENAI_API_KEY=file-openai\n")
	shell := writeShellEnvScript(t, "OPENAI_API_KEY=shell-openai", "GEMINI_API_KEY=shell-gemini", "DISPLAY=bad")
	t.Setenv("SHELL", shell)

	result := LoadEnvironment(envTestConfig{envFile: envFile, loadShell: true}, nil)

	if result.EnvFileError != nil {
		t.Fatalf("EnvFileError = %v", result.EnvFileError)
	}
	if !result.EnvFileLoaded || !result.ShellLoaded {
		t.Fatalf("loaded file=%v shell=%v", result.EnvFileLoaded, result.ShellLoaded)
	}
	if got := os.Getenv("OPENAI_API_KEY"); got != "file-openai" {
		t.Fatalf("OPENAI_API_KEY = %q, want file-openai", got)
	}
	if got := os.Getenv("GEMINI_API_KEY"); got != "shell-gemini" {
		t.Fatalf("GEMINI_API_KEY = %q, want shell-gemini", got)
	}
	if got := os.Getenv("DISPLAY"); got != "" {
		t.Fatalf("DISPLAY = %q, want empty", got)
	}
	if result.SourceFor("OPENAI_API_KEY") != envSourceFile {
		t.Fatalf("OPENAI_API_KEY source = %q, want %q", result.SourceFor("OPENAI_API_KEY"), envSourceFile)
	}
	if result.SourceFor("GEMINI_API_KEY") != envSourceShell {
		t.Fatalf("GEMINI_API_KEY source = %q, want %q", result.SourceFor("GEMINI_API_KEY"), envSourceShell)
	}
}

func TestLoadEnvironmentDoesNotOverwriteProcessEnv(t *testing.T) {
	clearTestEnv(t, "OPENAI_API_KEY")
	t.Setenv("OPENAI_API_KEY", "process-openai")
	envFile := writeTestEnvFile(t, "OPENAI_API_KEY=file-openai\n")

	result := LoadEnvironment(envTestConfig{envFile: envFile, loadShell: false}, nil)

	if got := os.Getenv("OPENAI_API_KEY"); got != "process-openai" {
		t.Fatalf("OPENAI_API_KEY = %q, want process-openai", got)
	}
	if result.SourceFor("OPENAI_API_KEY") != envSourceProcess {
		t.Fatalf("source = %q, want %q", result.SourceFor("OPENAI_API_KEY"), envSourceProcess)
	}
}

func TestLoadEnvironmentReportsEnvFilePermissionWarningWithoutBlockingLoad(t *testing.T) {
	clearTestEnv(t, "OPENAI_API_KEY")
	path := filepath.Join(t.TempDir(), "gui.env")
	if err := os.WriteFile(path, []byte("OPENAI_API_KEY=file-openai\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	result := LoadEnvironment(envTestConfig{envFile: path, loadShell: false}, nil)

	if !result.EnvFileLoaded {
		t.Fatal("env file should be loaded")
	}
	if result.EnvFileError != nil {
		t.Fatalf("EnvFileError = %v, want nil", result.EnvFileError)
	}
	if result.EnvFileWarning == nil {
		t.Fatal("EnvFileWarning should be set")
	}
	if got := os.Getenv("OPENAI_API_KEY"); got != "file-openai" {
		t.Fatalf("OPENAI_API_KEY = %q, want file-openai", got)
	}
}

func TestLoadEnvironmentReloadOverwritesImportedValues(t *testing.T) {
	clearTestEnv(t, "OPENAI_API_KEY", "GEMINI_API_KEY")
	dir := t.TempDir()
	envFile := filepath.Join(dir, "gui.env")
	if err := os.WriteFile(envFile, []byte("OPENAI_API_KEY=file-one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shell := writeShellEnvScript(t, "GEMINI_API_KEY=shell-one")
	t.Setenv("SHELL", shell)

	first := LoadEnvironment(envTestConfig{envFile: envFile, loadShell: true}, nil)
	if err := os.WriteFile(envFile, []byte("OPENAI_API_KEY=file-two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shell = writeShellEnvScript(t, "GEMINI_API_KEY=shell-two", "OPENAI_API_KEY=shell-two")
	t.Setenv("SHELL", shell)

	second := LoadEnvironment(envTestConfig{envFile: envFile, loadShell: true}, first.Sources)

	if got := os.Getenv("OPENAI_API_KEY"); got != "file-two" {
		t.Fatalf("OPENAI_API_KEY = %q, want file-two", got)
	}
	if got := os.Getenv("GEMINI_API_KEY"); got != "shell-two" {
		t.Fatalf("GEMINI_API_KEY = %q, want shell-two", got)
	}
	if second.SourceFor("OPENAI_API_KEY") != envSourceFile {
		t.Fatalf("OPENAI_API_KEY source = %q, want %q", second.SourceFor("OPENAI_API_KEY"), envSourceFile)
	}
}

func TestLoadEnvironmentReloadLetsShellFillRemovedEnvFileValue(t *testing.T) {
	clearTestEnv(t, "OPENAI_API_KEY")
	dir := t.TempDir()
	envFile := filepath.Join(dir, "gui.env")
	if err := os.WriteFile(envFile, []byte("OPENAI_API_KEY=file-one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shell := writeShellEnvScript(t, "OPENAI_API_KEY=shell-one")
	t.Setenv("SHELL", shell)

	first := LoadEnvironment(envTestConfig{envFile: envFile, loadShell: true}, nil)
	if err := os.WriteFile(envFile, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	shell = writeShellEnvScript(t, "OPENAI_API_KEY=shell-two")
	t.Setenv("SHELL", shell)

	second := LoadEnvironment(envTestConfig{envFile: envFile, loadShell: true}, first.Sources)

	if got := os.Getenv("OPENAI_API_KEY"); got != "shell-two" {
		t.Fatalf("OPENAI_API_KEY = %q, want shell-two", got)
	}
	if second.SourceFor("OPENAI_API_KEY") != envSourceShell {
		t.Fatalf("OPENAI_API_KEY source = %q, want %q", second.SourceFor("OPENAI_API_KEY"), envSourceShell)
	}
}

func clearTestEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		t.Setenv(key, "")
	}
}

func writeTestEnvFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gui.env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeShellEnvScript(t *testing.T, entries ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shell")
	content := "#!/bin/sh\nprintf '"
	for _, entry := range entries {
		content += entry + "\\0"
	}
	content += "'\n"
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
