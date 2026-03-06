package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolvePath(path string, workDir string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if workDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to resolve working directory: %w", err)
		}
		workDir = cwd
	}

	root, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve working directory: %w", err)
	}

	var absPath string
	if filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	} else {
		absPath = filepath.Join(root, path)
	}

	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return "", fmt.Errorf("failed to validate path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path outside working directory: %s", absPath)
	}

	return absPath, nil
}

func resolveRoot(root string, workDir string) (string, error) {
	if root == "" {
		return resolvePath(".", workDir)
	}
	return resolvePath(root, workDir)
}
