package tools

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	pythonToolDescription = "Run Python scripts using python, python3, or uv run python."
)

type PythonTool struct {
	name        string
	description string
	inputSchema map[string]any
	taskSchema  map[string]any
	helpText    string
	workDir     string
}

func NewPythonTool(workDir string) *PythonTool {
	properties := map[string]any{
		"path": map[string]string{
			"type":        "string",
			"description": "Python script path to execute",
		},
		"code": map[string]string{
			"type":        "string",
			"description": "Inline Python code to execute",
		},
		"runner": map[string]any{
			"type":        "string",
			"enum":        []string{"auto", "python3", "python", "uv"},
			"description": "Runner selection: auto, python3, python, or uv",
		},
		"uv_mode": map[string]any{
			"type":        "string",
			"enum":        []string{"python", "script"},
			"description": "uv mode: python (default) or script",
		},
		"args": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type":        "string",
				"description": "Single argument value",
			},
			"description": "Arguments to pass to the Python command",
		},
		"final": map[string]string{
			"type":        "boolean",
			"description": "Set to true only when the script output itself fully answers the user's request and should be returned directly without another model summary round.",
		},
	}

	inputSchema := map[string]any{
		"type":       "object",
		"properties": properties,
	}

	taskSchema := map[string]any{
		"type":       "object",
		"properties": properties,
		"oneOf": []any{
			map[string]any{"required": []string{"path"}},
			map[string]any{"required": []string{"code"}},
		},
	}

	return &PythonTool{
		name:        ToolNamePython,
		description: pythonToolDescription,
		inputSchema: inputSchema,
		taskSchema:  taskSchema,
		helpText:    "Execute Python scripts with python, python3, or uv run python.",
		workDir:     workDir,
	}
}

func (t *PythonTool) Name() string {
	return t.name
}

func (t *PythonTool) PermissionCategory() PermissionCategory {
	return PermissionExecute
}

func (t *PythonTool) Description() string {
	return t.description
}

func (t *PythonTool) InputSchema() map[string]any {
	return t.inputSchema
}

func (t *PythonTool) TaskInputSchema() map[string]any {
	return t.taskSchema
}

func (t *PythonTool) HelpText() string {
	return t.helpText
}

func (t *PythonTool) Run(input *string) (string, error) {
	return t.RunSchema(map[string]any{"code": *input})
}

func (t *PythonTool) RunSchema(input map[string]any) (string, error) {
	return t.RunSchemaWithContext(input, ToolExecutionContext{RootDir: t.workDir, CurrentDir: t.workDir})
}

func (t *PythonTool) RunSchemaWithContext(input map[string]any, ctx ToolExecutionContext) (string, error) {
	pathValue, _ := input["path"].(string)
	code, _ := input["code"].(string)
	runner, _ := input["runner"].(string)
	uvMode, _ := input["uv_mode"].(string)
	args := parseStringArray(input["args"])

	if pathValue == "" && code == "" {
		return "", fmt.Errorf("path or code is required")
	}
	if pathValue != "" && code != "" {
		return "", fmt.Errorf("provide path or code, not both")
	}

	selectedRunner, err := choosePythonRunner(runner)
	if err != nil {
		return "", err
	}

	normalizedCtx, err := normalizeExecutionContext(ctx, t.workDir)
	if err != nil {
		return "", err
	}

	resolvedPath := ""
	if pathValue != "" {
		resolvedPath, err = resolvePathInContext(pathValue, normalizedCtx, t.workDir)
		if err != nil {
			return "", err
		}
	}

	commandName, commandArgs, _, cleanup, err := t.buildCommand(selectedRunner, uvMode, resolvedPath, code, args, normalizedCtx.CurrentDir)
	if err != nil {
		return "", err
	}
	if cleanup != nil {
		defer cleanup()
	}

	cmd := exec.Command(commandName, commandArgs...)
	cmd.Dir = normalizedCtx.CurrentDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("python execution failed: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func (t *PythonTool) buildCommand(runner string, uvMode string, pathValue string, code string, args []string, currentDir string) (string, []string, string, func(), error) {
	cleanup := func() {}

	if runner == "uv" {
		if uvMode == "script" {
			if pathValue == "" {
				if code == "" {
					return "", nil, "", nil, fmt.Errorf("path or code required for uv script mode")
				}
				tmpPath, tmpCleanup, err := t.writeTempScript(code, currentDir)
				if err != nil {
					return "", nil, "", nil, err
				}
				pathValue = tmpPath
				cleanup = tmpCleanup
			}
			commandArgs := append([]string{"run", "--script", pathValue}, args...)
			return "uv", commandArgs, fmt.Sprintf("uv %s", strings.Join(commandArgs, " ")), cleanup, nil
		}

		if pathValue != "" {
			commandArgs := append([]string{"run", "python", pathValue}, args...)
			return "uv", commandArgs, fmt.Sprintf("uv %s", strings.Join(commandArgs, " ")), cleanup, nil
		}
		commandArgs := append([]string{"run", "python", "-c", code}, args...)
		return "uv", commandArgs, fmt.Sprintf("uv %s", strings.Join(commandArgs, " ")), cleanup, nil
	}

	if pathValue != "" {
		commandArgs := append([]string{pathValue}, args...)
		return runner, commandArgs, fmt.Sprintf("%s %s", runner, strings.Join(commandArgs, " ")), cleanup, nil
	}
	commandArgs := append([]string{"-c", code}, args...)
	return runner, commandArgs, fmt.Sprintf("%s %s", runner, strings.Join(commandArgs, " ")), cleanup, nil
}

func (t *PythonTool) writeTempScript(code string, currentDir string) (string, func(), error) {
	if currentDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", nil, fmt.Errorf("failed to resolve working directory: %w", err)
		}
		currentDir = cwd
	}

	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("failed to prepare working directory: %w", err)
	}

	tmp, err := os.CreateTemp(currentDir, "task-python-*.py")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp script: %w", err)
	}
	if _, err := tmp.WriteString(code); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", nil, fmt.Errorf("failed to write temp script: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", nil, fmt.Errorf("failed to close temp script: %w", err)
	}

	cleanup := func() {
		os.Remove(tmp.Name())
	}
	return tmp.Name(), cleanup, nil
}

func choosePythonRunner(requested string) (string, error) {
	switch requested {
	case "", "auto":
		if _, err := exec.LookPath("python3"); err == nil {
			return "python3", nil
		}
		if _, err := exec.LookPath("python"); err == nil {
			return "python", nil
		}
		if _, err := exec.LookPath("uv"); err == nil {
			return "uv", nil
		}
		return "", fmt.Errorf("no python runner found; install python3, python, or uv")
	case "python3", "python", "uv":
		if _, err := exec.LookPath(requested); err != nil {
			return "", fmt.Errorf("%s not available on PATH", requested)
		}
		return requested, nil
	default:
		return "", fmt.Errorf("unsupported runner: %s", requested)
	}
}

func parseStringArray(raw any) []string {
	if raw == nil {
		return nil
	}

	items, ok := raw.([]any)
	if !ok {
		return nil
	}

	values := make([]string, 0, len(items))
	for _, item := range items {
		if str, ok := item.(string); ok {
			values = append(values, str)
		}
	}
	return values
}
