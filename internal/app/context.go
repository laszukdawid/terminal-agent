package app

import (
	"fmt"
	"os"
	"strings"
)

func BuildContextFromFiles(files []string) (string, error) {
	var contextParts []string
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("failed to read file %s: %w", file, err)
		}
		contextParts = append(contextParts, fmt.Sprintf("<context>\n%s\n</context>", strings.TrimSpace(string(content))))
	}
	return strings.Join(contextParts, "\n\n"), nil
}
