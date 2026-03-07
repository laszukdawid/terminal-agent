package agent

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// SystemInfo contains information about the current system environment
type SystemInfo struct {
	Hostname     string
	Username     string
	CurrentTime  string
	WorkingDir   string
	OS           string
	OSVersion    string
	Architecture string
	GoVersion    string
}

// GetSystemInfo collects information about the current system
func GetSystemInfo() SystemInfo {
	info := SystemInfo{
		CurrentTime:  time.Now().Format("2006-01-02 15:04:05 MST"),
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		GoVersion:    runtime.Version(),
	}

	// Get hostname
	if hostname, err := os.Hostname(); err == nil {
		info.Hostname = hostname
	} else {
		info.Hostname = "unknown"
	}

	// Get username
	if currentUser, err := user.Current(); err == nil {
		info.Username = currentUser.Username
	} else {
		info.Username = "unknown"
	}

	// Get working directory
	if wd, err := os.Getwd(); err == nil {
		info.WorkingDir = wd
	} else {
		info.WorkingDir = "unknown"
	}

	// Get OS version
	info.OSVersion = getOSVersion()

	return info
}

// SystemPromptHeader returns the system prompt header with current system information
func SystemPromptHeader() string {
	info := GetSystemInfo()

	osLine := fmt.Sprintf("%s/%s", info.OS, info.Architecture)
	if info.OSVersion != "" {
		osLine = fmt.Sprintf("%s/%s (%s)", info.OS, info.Architecture, info.OSVersion)
	}

	return fmt.Sprintf(`You are a Unix terminal helper.
You are mainly called from Unix terminal, and asked about Unix terminal questions.
You specialize in software development with access to a variety of tools and the ability to instruct and direct a coding agent and a code execution one.

Current system context:
- Hostname: %s
- User: %s
- Time: %s
- Working Directory: %s
- Operating System: %s
`, info.Hostname, info.Username, info.CurrentTime, info.WorkingDir, osLine)
}

// getOSVersion returns a human-readable OS name and version.
func getOSVersion() string {
	switch runtime.GOOS {
	case "linux":
		return getLinuxVersion()
	case "darwin":
		return getDarwinVersion()
	default:
		return ""
	}
}

// getLinuxVersion reads PRETTY_NAME from /etc/os-release.
func getLinuxVersion() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			value := strings.TrimPrefix(line, "PRETTY_NAME=")
			return strings.Trim(value, "\"")
		}
	}
	return ""
}

// getDarwinVersion runs sw_vers to get macOS product name and version.
func getDarwinVersion() string {
	out, err := exec.Command("sw_vers", "-productName").Output()
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(out))

	out, err = exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return name
	}
	version := strings.TrimSpace(string(out))

	return name + " " + version
}

const SystemPromptAsk = `
{{header}}

You don't have any access to tools. In case the user asks to do something, e.g. execute a command,
refer them to other functionalities of yours, e.g. requesting the Task command.

Always strive for accuracy, clarity, and efficiency in your responses. You must be consise.

Remember, you are an AI assistant, and your primary goal is to help the user accomplish their tasks effectively and efficiently while maintaining the integrity and security of their development environment.
`

const SystemPromptTask = `
{{header}}

Remember, you are an AI assistant, and your primary goal is to help the user accomplish their tasks effectively and efficiently while maintaining the integrity and security of their development environment.
Users care about the amount of text so be consise and to the point.

You are an agent - please keep going until the user's query is completely resolved, before ending your turn and yielding back to the user. Only terminate your turn when you are sure that the problem is solved, or if you need more info from the user to solve the problem.
You have access to a variety of tools and the ability to instruct and direct a coding agent and a code execution one. When using the tools, you must provide arguments in accordance with the input schema of the tool. You must also provide a detailed explanation of what you are doing and why, so that the user can understand your reasoning and learn from it.
Prefer native tools for editing and searching files (file_edit, file_search). Use the python tool for running scripts, including uv run python when requested.
When creating a new file, use file_edit with operation "write" and the target path.
If you are not sure about anything pertaining to the user's request, use your tools to read files and gather the relevant information: do NOT guess or make up an answer.

You MUST plan extensively before each function call, and reflect extensively on the outcomes of the previous function calls. DO NOT do this entire process by making function calls only, as this can impair your ability to solve the problem and think insightfully.
`

// ResolvePrompt resolves the system prompt based on the priority hierarchy:
// 1. flagPrompt (CLI --prompt flag) - highest priority
// 2. File-based prompt (system.prompt for ask, task/system.prompt or task_system.prompt for task)
// 3. Default prompt - lowest priority
//
// All prompts support {{header}} template substitution.
// workingDir is the directory to look for prompt files in (from config).
func ResolvePrompt(flagPrompt string, promptType string, workingDir string) (string, error) {
	var rawPrompt string

	if flagPrompt != "" {
		// Highest priority: CLI flag
		rawPrompt = flagPrompt
	} else {
		// Try file-based prompt
		filePrompt, err := readPromptFile(promptType, workingDir)
		if err != nil {
			return "", err
		}
		if filePrompt != "" {
			rawPrompt = filePrompt
		} else {
			// Fall back to default
			if promptType == "task" {
				rawPrompt = SystemPromptTask
			} else {
				rawPrompt = SystemPromptAsk
			}
		}
	}

	// Apply {{header}} substitution
	resolved := strings.Replace(rawPrompt, "{{header}}", SystemPromptHeader(), 1)
	return resolved, nil
}

// readPromptFile attempts to read a prompt file from the working directory.
// For "ask": checks ./ask/system.prompt, then ./ask_system.prompt
// For "task": checks ./task/system.prompt, then ./task_system.prompt
// Returns empty string if no file exists, error if file exists but can't be read.
func readPromptFile(promptType string, workingDir string) (string, error) {
	var paths []string

	if promptType == "task" {
		paths = []string{
			filepath.Join(workingDir, "task", "system.prompt"),
			filepath.Join(workingDir, "task_system.prompt"),
		}
	} else {
		paths = []string{
			filepath.Join(workingDir, "ask", "system.prompt"),
			filepath.Join(workingDir, "ask_system.prompt"),
		}
	}

	for _, path := range paths {
		content, err := tryReadFile(path)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt file %s: %w", path, err)
		}
		if content != "" {
			return content, nil
		}
	}

	return "", nil
}

// tryReadFile attempts to read a file. Returns empty string if file doesn't exist.
// Returns error if file exists but can't be read or is empty.
func tryReadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		// Treat empty file as "not present"
		return "", nil
	}

	return content, nil
}
