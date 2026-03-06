package tools

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	fileSearchToolName        = "file_search"
	fileSearchToolDescription = "Search for files and text using native Go operations."
)

type FileSearchTool struct {
	name        string
	description string
	inputSchema map[string]any
	helpText    string
	workDir     string
}

func NewFileSearchTool(workDir string) *FileSearchTool {
	inputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"root": map[string]string{
				"type":        "string",
				"description": "Root directory for the search (defaults to working directory)",
			},
			"name_pattern": map[string]string{
				"type":        "string",
				"description": "Glob-like pattern to match file paths or names",
			},
			"contains": map[string]string{
				"type":        "string",
				"description": "Text to search for inside files",
			},
			"max_results": map[string]string{
				"type":        "integer",
				"description": "Maximum number of results to return",
			},
		},
	}

	return &FileSearchTool{
		name:        fileSearchToolName,
		description: fileSearchToolDescription,
		inputSchema: inputSchema,
		helpText:    "Search for files and content using native Go operations.",
		workDir:     workDir,
	}
}

func (t *FileSearchTool) Name() string {
	return t.name
}

func (t *FileSearchTool) Description() string {
	return t.description
}

func (t *FileSearchTool) InputSchema() map[string]any {
	return t.inputSchema
}

func (t *FileSearchTool) HelpText() string {
	return t.helpText
}

func (t *FileSearchTool) Run(input *string) (string, error) {
	return t.RunSchema(map[string]any{"contains": *input})
}

func (t *FileSearchTool) RunSchema(input map[string]any) (string, error) {
	root, _ := input["root"].(string)
	namePattern, _ := input["name_pattern"].(string)
	contains, _ := input["contains"].(string)
	maxResults := 200

	if rawMax, ok := input["max_results"]; ok {
		switch v := rawMax.(type) {
		case int:
			maxResults = v
		case float64:
			maxResults = int(v)
		}
	}

	if namePattern == "" && contains == "" {
		return "", fmt.Errorf("name_pattern or contains is required")
	}

	rootPath, err := resolveRoot(root, t.workDir)
	if err != nil {
		return "", err
	}

	pattern := normalizePattern(namePattern)

	results := make([]string, 0, maxResults)
	searchErr := filepath.WalkDir(rootPath, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(rootPath, p)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)

		if namePattern != "" && !matchPattern(pattern, relSlash) {
			return nil
		}

		if contains == "" {
			results = append(results, relSlash)
			if len(results) >= maxResults {
				return fs.SkipAll
			}
			return nil
		}

		file, err := os.Open(p)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if strings.Contains(line, contains) {
				results = append(results, fmt.Sprintf("%s:%d: %s", relSlash, lineNo, line))
				if len(results) >= maxResults {
					file.Close()
					return fs.SkipAll
				}
			}
		}
		file.Close()
		if err := scanner.Err(); err != nil {
			return err
		}

		return nil
	})

	if searchErr != nil {
		return "", fmt.Errorf("search failed: %w", searchErr)
	}

	if len(results) == 0 {
		return "no matches found", nil
	}

	return strings.Join(results, "\n"), nil
}

func normalizePattern(pattern string) string {
	if pattern == "" {
		return ""
	}
	return strings.ReplaceAll(filepath.ToSlash(pattern), "**", "*")
}

func matchPattern(pattern string, relPath string) bool {
	if pattern == "" {
		return true
	}
	if strings.Contains(pattern, "/") {
		matched, err := path.Match(pattern, relPath)
		return err == nil && matched
	}
	matched, err := path.Match(pattern, path.Base(relPath))
	return err == nil && matched
}
