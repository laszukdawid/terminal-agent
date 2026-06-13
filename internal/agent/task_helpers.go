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

func runTaskTool(ctx context.Context, tool tools.Tool, input map[string]any, dirs TaskDirs, output io.Writer, progress func(string)) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	execCtx := tools.ToolExecutionContext{
		RootDir:    dirs.RootDir,
		CurrentDir: dirs.CurrentDir,
		Output:     output,
		Progress:   progress,
	}
	if permissionCategoryFor(tool) == tools.PermissionWrite {
		execCtx.AllowedRootDirs = []string{dirs.RootDir}
		execCtx.AllowedPaths = dirs.WriteAllowedPaths
	} else {
		execCtx.AllowedRootDirs = dirs.ReadAllowedRoots
	}
	if contextAwareTool, ok := tool.(tools.ContextAwareTool); ok {
		return contextAwareTool.RunSchemaContext(ctx, input, execCtx)
	}
	if contextualTool, ok := tool.(tools.ContextualTool); ok {
		return contextualTool.RunSchemaWithContext(input, execCtx)
	}
	return tool.RunSchema(input)
}

func additionalWritePathForFilePath(path string, dirs TaskDirs) string {
	absPath := resolveTaskPathForScope(path, dirs)
	if absPath == "" || writePathAllowed(absPath, dirs) {
		return ""
	}
	return absPath
}

func additionalRootForDirPath(path string, dirs TaskDirs) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	absPath := resolveTaskPathForScope(path, dirs)
	if absPath == "" || readPathAllowed(absPath, dirs) {
		return ""
	}
	return absPath
}

func resolveTaskPathForScope(path string, dirs TaskDirs) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if !filepath.IsAbs(trimmed) {
		base := dirs.CurrentDir
		if base == "" {
			base = dirs.RootDir
		}
		trimmed = filepath.Join(base, trimmed)
	}
	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		return ""
	}
	return filepath.Clean(absPath)
}

func readPathAllowed(path string, dirs TaskDirs) bool {
	return tools.PathAllowedInContext(path, tools.ToolExecutionContext{
		RootDir:         dirs.RootDir,
		CurrentDir:      dirs.CurrentDir,
		AllowedRootDirs: dirs.ReadAllowedRoots,
	})
}

func writePathAllowed(path string, dirs TaskDirs) bool {
	return tools.PathAllowedInContext(path, tools.ToolExecutionContext{
		RootDir:         dirs.RootDir,
		CurrentDir:      dirs.CurrentDir,
		AllowedRootDirs: []string{dirs.RootDir},
		AllowedPaths:    dirs.WriteAllowedPaths,
	})
}

func appendAllowedRoot(roots []string, root string) []string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return roots
	}
	absRoot = filepath.Clean(absRoot)
	for _, existing := range roots {
		existingAbs, err := filepath.Abs(existing)
		if err == nil && filepath.Clean(existingAbs) == absRoot {
			return roots
		}
	}
	return append(roots, absRoot)
}

func appendAllowedPath(paths []string, path string) []string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return paths
	}
	absPath = filepath.Clean(absPath)
	for _, existing := range paths {
		existingAbs, err := filepath.Abs(existing)
		if err == nil && filepath.Clean(existingAbs) == absPath {
			return paths
		}
	}
	return append(paths, absPath)
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

	return normalizeTaskDirs(TaskDirs{RootDir: rootDir, CurrentDir: currentDir, ReadAllowedRoots: dirs.ReadAllowedRoots, WriteAllowedPaths: dirs.WriteAllowedPaths})
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

	return TaskDirs{RootDir: rootDir, CurrentDir: currentDir, ReadAllowedRoots: appendAllowedRoot(dirs.ReadAllowedRoots, rootDir), WriteAllowedPaths: dirs.WriteAllowedPaths}, nil
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

	resolvedDirs, err := normalizeTaskDirs(TaskDirs{RootDir: dirs.RootDir, CurrentDir: nextDir, ReadAllowedRoots: dirs.ReadAllowedRoots, WriteAllowedPaths: dirs.WriteAllowedPaths})
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
