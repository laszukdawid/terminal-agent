package app

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	terminalContextOutputLimit = 4000
	BashReaderInstallHint      = "agent plugin install bash-reader"
)

type terminalContextEntry struct {
	Timestamp  int64
	ExitCode   int
	OutputPath string
	Command    string
	Output     string
}

const terminalContextTemplate = `<context>
Terminal context (latest commands):

%s
</context>`

const terminalContextEntryTemplate = `[%d] $ %s
exit_code: %d
%s`

func BuildContextFromTerminal(maxEntries int) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user home directory: %w", err)
	}

	scriptPath := BashReaderScriptPath(homeDir)
	if _, err := os.Stat(scriptPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("--use-terminal-context requires the bash-reader plugin. Install it with: %s", BashReaderInstallHint)
		}
		return "", fmt.Errorf("failed to check bash-reader plugin installation: %w", err)
	}

	indexPath := filepath.Join(TerminalContextDir(homeDir), "index.log")
	entries, err := readLastTerminalContextEntries(indexPath, maxEntries)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no terminal context found yet. Run a few bash commands and try again")
		}
		return "", err
	}

	if len(entries) == 0 {
		return "", fmt.Errorf("no terminal context found yet. Run a few bash commands and try again")
	}

	return fmt.Sprintf(terminalContextTemplate, formatTerminalContextEntries(entries)), nil
}

func formatTerminalContextEntries(entries []terminalContextEntry) string {
	parts := make([]string, 0, len(entries))
	for i, entry := range entries {
		output := "output: <no output>"
		if strings.TrimSpace(entry.Output) != "" {
			output = "output:\n" + entry.Output
			if !strings.HasSuffix(output, "\n") {
				output += "\n"
			}
			output = strings.TrimRight(output, "\n")
		}
		parts = append(parts, fmt.Sprintf(terminalContextEntryTemplate, i+1, entry.Command, entry.ExitCode, output))
	}
	return strings.Join(parts, "\n\n")
}

func BashReaderScriptPath(homeDir string) string {
	return filepath.Join(homeDir, ".config", "terminal-agent", "plugins", "bash-reader", "init.bash")
}

func BashRCPath(homeDir string) string {
	return filepath.Join(homeDir, ".bashrc")
}

func TerminalContextDir(homeDir string) string {
	return filepath.Join(homeDir, ".local", "share", "terminal-agent", "terminal-context")
}

func readLastTerminalContextEntries(indexPath string, maxEntries int) ([]terminalContextEntry, error) {
	file, err := os.Open(indexPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read terminal context index: %w", err)
	}

	start := 0
	if len(lines) > maxEntries {
		start = len(lines) - maxEntries
	}

	var entries []terminalContextEntry
	for _, line := range lines[start:] {
		entry, err := parseTerminalContextLine(line)
		if err != nil {
			continue
		}

		output, err := os.ReadFile(entry.OutputPath)
		if err == nil {
			entry.Output = truncateTerminalOutput(strings.TrimSpace(string(output)), terminalContextOutputLimit)
		} else {
			entry.Output = ""
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func parseTerminalContextLine(line string) (terminalContextEntry, error) {
	parts := strings.SplitN(line, "\t", 4)
	if len(parts) != 4 {
		return terminalContextEntry{}, fmt.Errorf("invalid terminal context line")
	}

	timestamp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return terminalContextEntry{}, fmt.Errorf("invalid timestamp")
	}

	exitCode, err := strconv.Atoi(parts[1])
	if err != nil {
		return terminalContextEntry{}, fmt.Errorf("invalid exit code")
	}

	commandBytes, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return terminalContextEntry{}, fmt.Errorf("invalid command encoding")
	}

	return terminalContextEntry{
		Timestamp:  timestamp,
		ExitCode:   exitCode,
		OutputPath: parts[2],
		Command:    strings.TrimSpace(string(commandBytes)),
	}, nil
}

func truncateTerminalOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}

	return output[:maxLen] + "\n...[truncated]"
}
