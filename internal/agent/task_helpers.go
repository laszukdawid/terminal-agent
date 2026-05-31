package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/tools"
)

func selectTaskRawOutput(outputs []taskToolOutput) taskToolOutput {
	for i := len(outputs) - 1; i >= 0; i-- {
		output := outputs[i]
		if !isTaskDisplayOrientedTool(output.ToolName) {
			continue
		}
		if !strings.ContainsAny(output.Output, "\n\t") {
			continue
		}

		return output
	}

	return taskToolOutput{}
}

func runTaskTool(ctx context.Context, tool tools.Tool, input map[string]any, dirs TaskDirs, output io.Writer) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	execCtx := tools.ToolExecutionContext{
		RootDir:    dirs.RootDir,
		CurrentDir: dirs.CurrentDir,
		Output:     output,
	}
	if contextAwareTool, ok := tool.(tools.ContextAwareTool); ok {
		return contextAwareTool.RunSchemaContext(ctx, input, execCtx)
	}
	if contextualTool, ok := tool.(tools.ContextualTool); ok {
		return contextualTool.RunSchemaWithContext(input, execCtx)
	}
	return tool.RunSchema(input)
}

func resolveInitialTaskDirs(dirs TaskDirs, cfg config.Config) (TaskDirs, error) {
	rootDir := strings.TrimSpace(dirs.RootDir)
	if rootDir == "" && cfg != nil {
		rootDir = cfg.GetWorkingDir()
	}
	if rootDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return TaskDirs{}, fmt.Errorf("failed to resolve task root directory: %w", err)
		}
		rootDir = cwd
	}

	currentDir := strings.TrimSpace(dirs.CurrentDir)
	if currentDir == "" {
		currentDir = rootDir
	}

	return normalizeTaskDirs(TaskDirs{RootDir: rootDir, CurrentDir: currentDir})
}

func normalizeTaskDirs(dirs TaskDirs) (TaskDirs, error) {
	rootDir, err := filepath.Abs(dirs.RootDir)
	if err != nil {
		return TaskDirs{}, fmt.Errorf("failed to resolve task root directory: %w", err)
	}
	currentDir, err := filepath.Abs(dirs.CurrentDir)
	if err != nil {
		return TaskDirs{}, fmt.Errorf("failed to resolve task current directory: %w", err)
	}

	if err := validateTaskDirectory(rootDir, rootDir); err != nil {
		return TaskDirs{}, err
	}
	if err := validateTaskDirectory(currentDir, rootDir); err != nil {
		return TaskDirs{}, err
	}

	return TaskDirs{RootDir: rootDir, CurrentDir: currentDir}, nil
}

func validateTaskDirectory(path string, rootDir string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to access directory %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	rel, err := filepath.Rel(rootDir, path)
	if err != nil {
		return fmt.Errorf("failed to validate directory %s: %w", path, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("directory outside task root: %s", path)
	}
	return nil
}

func changeTaskDirectory(input map[string]any, dirs *TaskDirs) (string, error) {
	path, ok := input["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}

	var nextDir string
	if filepath.IsAbs(path) {
		nextDir = filepath.Clean(path)
	} else {
		nextDir = filepath.Join(dirs.CurrentDir, path)
	}

	resolvedDirs, err := normalizeTaskDirs(TaskDirs{RootDir: dirs.RootDir, CurrentDir: nextDir})
	if err != nil {
		return "", err
	}

	*dirs = resolvedDirs
	return fmt.Sprintf("changed current directory to %s", dirs.CurrentDir), nil
}

func taskToolInputRequestsFinal(input map[string]any) bool {
	requested, ok := input["final"].(bool)
	return ok && requested
}

const finalDirectOutputDescriptionMarker = "returned directly without another"

func toolSupportsFinal(tool tools.Tool) bool {
	schema := tools.EffectiveTaskInputSchema(tool)
	propertiesRaw, _ := schema["properties"]
	properties, ok := normalizeSchemaMap(propertiesRaw)
	if !ok {
		return false
	}
	finalSchema, ok := normalizeSchemaDefinition(properties["final"])
	if !ok {
		return false
	}
	typeName, _ := finalSchema["type"].(string)
	if typeName != "boolean" {
		return false
	}
	desc, _ := finalSchema["description"].(string)
	return strings.Contains(desc, finalDirectOutputDescriptionMarker)
}

func isTaskDisplayOrientedTool(toolName string) bool {
	switch toolName {
	case tools.ToolNameUnix, tools.ToolNamePython, tools.ToolNameFileSearch, tools.ToolNameRead:
		return true
	default:
		return false
	}
}
