package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	fileEditToolName        = "file_edit"
	fileEditToolDescription = "Create and update files using native Go file operations."
)

type FileEditTool struct {
	name        string
	description string
	inputSchema map[string]any
	helpText    string
	workDir     string
}

func NewFileEditTool(workDir string) *FileEditTool {
	inputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]string{
				"type":        "string",
				"description": "File path relative to the working directory (or absolute within it)",
			},
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"write", "append", "replace"},
				"description": "Edit operation: write, append, or replace",
			},
			"content": map[string]string{
				"type":        "string",
				"description": "Content to write or append",
			},
			"search": map[string]string{
				"type":        "string",
				"description": "Text to search for when replacing",
			},
			"replace": map[string]string{
				"type":        "string",
				"description": "Replacement text",
			},
			"count": map[string]string{
				"type":        "integer",
				"description": "Maximum number of replacements to apply",
			},
		},
		"required": []string{"path", "operation"},
	}

	return &FileEditTool{
		name:        fileEditToolName,
		description: fileEditToolDescription,
		inputSchema: inputSchema,
		helpText:    "Edit files using native Go operations.",
		workDir:     workDir,
	}
}

func (t *FileEditTool) Name() string {
	return t.name
}

func (t *FileEditTool) Description() string {
	return t.description
}

func (t *FileEditTool) InputSchema() map[string]any {
	return t.inputSchema
}

func (t *FileEditTool) HelpText() string {
	return t.helpText
}

func (t *FileEditTool) Run(input *string) (string, error) {
	return t.RunSchema(map[string]any{"operation": "write", "path": "", "content": *input})
}

func (t *FileEditTool) RunSchema(input map[string]any) (string, error) {
	path, _ := input["path"].(string)
	operation, _ := input["operation"].(string)
	content, _ := input["content"].(string)
	search, _ := input["search"].(string)
	replace, _ := input["replace"].(string)

	if operation == "" && content != "" {
		operation = "write"
	}
	if operation == "" {
		return "", fmt.Errorf("operation is required")
	}
	resolvedPath, err := resolvePath(path, t.workDir)
	if err != nil {
		return "", err
	}

	return t.runOperation(operation, resolvedPath, content, search, replace, input)
}

func (t *FileEditTool) runOperation(operation string, resolvedPath string, content string, search string, replace string, input map[string]any) (string, error) {

	switch operation {
	case "write":
		if content == "" {
			return "", fmt.Errorf("content is required for write")
		}
		return t.writeFile(resolvedPath, content)
	case "append":
		if content == "" {
			return "", fmt.Errorf("content is required for append")
		}
		return t.appendFile(resolvedPath, content)
	case "replace":
		if search == "" {
			return "", fmt.Errorf("search is required for replace")
		}
		return t.replaceFile(resolvedPath, search, replace, input)
	default:
		return "", fmt.Errorf("unsupported operation: %s", operation)
	}
}

func (t *FileEditTool) writeFile(path string, content string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", fmt.Errorf("failed to replace file: %w", err)
	}

	return fmt.Sprintf("wrote %d bytes to %s", len(content), path), nil
}

func (t *FileEditTool) appendFile(path string, content string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		return "", fmt.Errorf("failed to append file: %w", err)
	}

	return fmt.Sprintf("appended %d bytes to %s", len(content), path), nil
}

func (t *FileEditTool) replaceFile(path string, search string, replace string, input map[string]any) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	original := string(data)

	count := -1
	if rawCount, ok := input["count"]; ok {
		switch v := rawCount.(type) {
		case int:
			count = v
		case float64:
			count = int(v)
		}
	}

	replacements := strings.Count(original, search)
	if replacements == 0 {
		return "", fmt.Errorf("search text not found in %s", path)
	}
	if count > 0 && replacements > count {
		replacements = count
	}

	updated := original
	if count > 0 {
		updated = strings.Replace(original, search, replace, count)
	} else {
		updated = strings.ReplaceAll(original, search, replace)
	}

	if updated == original {
		return "", fmt.Errorf("no changes applied to %s", path)
	}

	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("replaced %d occurrence(s) in %s", replacements, path), nil
}
