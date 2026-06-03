package gui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	envSourceProcess = "process env"
	envSourceFile    = "app env file"
	envSourceShell   = "shell import"
)

var guiEnvironmentAllowlist = map[string]struct{}{
	"ANTHROPIC_API_KEY":     {},
	"AWS_ACCESS_KEY_ID":     {},
	"AWS_DEFAULT_REGION":    {},
	"AWS_PROFILE":           {},
	"AWS_REGION":            {},
	"AWS_SECRET_ACCESS_KEY": {},
	"AWS_SESSION_TOKEN":     {},
	"GEMINI_API_KEY":        {},
	"MIMO_API_KEY":          {},
	"MIMO_BASE_URL":         {},
	"MISTRAL_API_KEY":       {},
	"MISTRAL_BASE_URL":      {},
	"OLLAMA_HOST":           {},
	"OPENAI_API_KEY":        {},
	"OPENAI_BASE_URL":       {},
	"PATH":                  {},
	"TAVILY_KEY":            {},
	"YZMA_LIB":              {},
}

type EnvironmentLoadResult struct {
	EnvFilePath    string
	EnvFileLoaded  bool
	EnvFileWarning error
	EnvFileError   error

	ShellEnabled  bool
	ShellLoaded   bool
	Shell         string
	ShellDuration time.Duration
	ShellError    error

	Sources map[string]string
}

type environmentConfig interface {
	GetGUIEnvFile() string
	GetGUILoadShellEnvironment() bool
	GetGUIShellEnvironmentTimeout() time.Duration
}

func (r EnvironmentLoadResult) SourceFor(key string) string {
	if r.Sources == nil {
		return ""
	}
	return r.Sources[key]
}

func LoadEnvironment(cfg environmentConfig) EnvironmentLoadResult {
	result := EnvironmentLoadResult{
		EnvFilePath:  cfg.GetGUIEnvFile(),
		ShellEnabled: cfg.GetGUILoadShellEnvironment(),
		Shell:        os.Getenv("SHELL"),
		Sources:      processEnvironmentSources(),
	}

	fileEnv, loaded, warning, err := readEnvFile(result.EnvFilePath)
	result.EnvFileLoaded = loaded
	result.EnvFileWarning = warning
	result.EnvFileError = err
	mergeEnvironment(fileEnv, envSourceFile, result.Sources)

	if result.ShellEnabled {
		start := time.Now()
		shellEnv, err := loadShellEnvironment(cfg.GetGUIShellEnvironmentTimeout())
		result.ShellDuration = time.Since(start)
		result.ShellError = err
		if err == nil {
			result.ShellLoaded = true
			mergeEnvironment(shellEnv, envSourceShell, result.Sources)
		}
	}

	return result
}

func processEnvironmentSources() map[string]string {
	sources := map[string]string{}
	for key := range guiEnvironmentAllowlist {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			sources[key] = envSourceProcess
		}
	}
	return sources
}

func mergeEnvironment(values map[string]string, source string, sources map[string]string) {
	for key, rawValue := range values {
		value := strings.TrimSpace(rawValue)
		if value == "" || !isAllowedGUIEnv(key) {
			continue
		}
		if strings.TrimSpace(os.Getenv(key)) != "" {
			continue
		}
		if err := os.Setenv(key, value); err == nil {
			sources[key] = source
		}
	}
}

func isAllowedGUIEnv(key string) bool {
	_, ok := guiEnvironmentAllowlist[key]
	return ok
}

func readEnvFile(path string) (map[string]string, bool, error, error) {
	if strings.TrimSpace(path) == "" {
		return map[string]string{}, false, nil, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, false, nil, nil
		}
		return map[string]string{}, false, nil, err
	}
	return parseDotenv(content), true, checkEnvFilePermissions(path), nil
}

func checkEnvFilePermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s is readable by group or others; consider chmod 600", path)
	}
	return nil
}

func parseDotenv(content []byte) map[string]string {
	values := map[string]string{}
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if !isAllowedGUIEnv(key) {
			continue
		}
		values[key] = unquoteEnvValue(strings.TrimSpace(value))
	}
	return values
}

func unquoteEnvValue(value string) string {
	if len(value) < 2 {
		return value
	}
	quote := value[0]
	if (quote != '\'' && quote != '"') || value[len(value)-1] != quote {
		return value
	}
	if quote == '\'' {
		return value[1 : len(value)-1]
	}
	unquoted, err := strconv.Unquote(value)
	if err != nil {
		return value[1 : len(value)-1]
	}
	return unquoted
}

func loadShellEnvironment(timeout time.Duration) (map[string]string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, "-ic", "env -0")
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return map[string]string{}, ctx.Err()
	}
	if err != nil {
		return map[string]string{}, err
	}
	return parseNullSeparatedEnv(output), nil
}

func parseNullSeparatedEnv(output []byte) map[string]string {
	values := map[string]string{}
	for _, part := range bytes.Split(output, []byte{0}) {
		if len(part) == 0 {
			continue
		}
		key, value, ok := strings.Cut(string(part), "=")
		if !ok || !isAllowedGUIEnv(key) {
			continue
		}
		values[key] = value
	}
	return values
}
