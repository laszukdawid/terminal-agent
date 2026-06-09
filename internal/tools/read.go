package tools

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	readToolDescription = "Read the contents of a file with optional offset and limit for pagination."
)

type ReadTool struct {
	name        string
	description string
	inputSchema map[string]any
	helpText    string
	workDir     string
}

func NewReadTool(workDir string) *ReadTool {
	return &ReadTool{
		name:        ToolNameRead,
		description: readToolDescription,
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]string{
					"type":        "string",
					"description": "File path relative to the working directory (or absolute within it)",
				},
				"offset": map[string]string{
					"type":        "integer",
					"description": "Line number to start reading from (1-indexed, default 1)",
				},
				"limit": map[string]string{
					"type":        "integer",
					"description": "Maximum number of lines to read (default reads all lines from offset)",
				},
			},
			"required": []string{"path"},
		},
		helpText: "Read file contents with optional pagination.",
		workDir:  workDir,
	}
}

func (t *ReadTool) Name() string {
	return t.name
}

func (t *ReadTool) PermissionCategory() PermissionCategory {
	return PermissionRead
}

func (t *ReadTool) Description() string {
	return t.description
}

func (t *ReadTool) InputSchema() map[string]any {
	return t.inputSchema
}

func (t *ReadTool) HelpText() string {
	return t.helpText
}

func (t *ReadTool) ToolStatus(input map[string]any) string {
	path := trimmedStringInput(input, "path")
	if path == "" {
		return ""
	}

	parts := []string{quotedStatusPart("file", path)}
	if offset, ok := integerInput(input, "offset"); ok {
		parts = append(parts, fmt.Sprintf("offset=%d", offset))
	}
	if limit, ok := integerInput(input, "limit"); ok {
		parts = append(parts, fmt.Sprintf("limit=%d", limit))
	}
	return formatStatus("Read: ", parts)
}

func (t *ReadTool) Run(input *string) (string, error) {
	if input == nil {
		return "", fmt.Errorf("path is required")
	}
	return t.RunSchema(map[string]any{"path": *input})
}

func (t *ReadTool) RunSchema(input map[string]any) (string, error) {
	return t.RunSchemaWithContext(input, ToolExecutionContext{RootDir: t.workDir, CurrentDir: t.workDir})
}

func (t *ReadTool) RunSchemaWithContext(input map[string]any, ctx ToolExecutionContext) (string, error) {
	path, ok := input["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}

	resolvedPath, err := resolvePathInContext(path, ctx, t.workDir)
	if err != nil {
		return "", err
	}

	offset := 1
	if rawOffset, ok := input["offset"]; ok {
		switch v := rawOffset.(type) {
		case int:
			offset = v
		case float64:
			offset = int(v)
		}
	}
	if offset < 1 {
		offset = 1
	}

	limit := -1
	if rawLimit, ok := input["limit"]; ok {
		switch v := rawLimit.(type) {
		case int:
			limit = v
		case float64:
			limit = int(v)
		}
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", path)
		}
		return "", fmt.Errorf("failed to access file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file: %s", path)
	}

	file, err := os.Open(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lines := make([]string, 0)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if lineNo < offset {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d: %s", lineNo, scanner.Text()))
		if limit > 0 && len(lines) >= limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	if len(lines) == 0 {
		if lineNo == 0 {
			return "(empty file)", nil
		}
		if offset > lineNo {
			return fmt.Sprintf("offset %d exceeds file length (%d lines)", offset, lineNo), nil
		}
		return "(empty file)", nil
	}

	result := strings.Join(lines, "\n")
	totalLines := lineNo
	if limit > 0 && totalLines > offset+limit-1 {
		result += fmt.Sprintf("\n\n[Showing lines %d-%d of %d total lines in %s]", offset, offset+limit-1, totalLines, filepath.Base(resolvedPath))
	}

	return result, nil
}
