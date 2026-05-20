package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func normalizeExecutionContext(ctx ToolExecutionContext, fallbackDir string) (ToolExecutionContext, error) {
	currentDir := ctx.CurrentDir
	if currentDir == "" {
		currentDir = fallbackDir
	}
	if currentDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return ToolExecutionContext{}, fmt.Errorf("failed to resolve working directory: %w", err)
		}
		currentDir = cwd
	}

	rootDir := ctx.RootDir
	if rootDir == "" {
		rootDir = currentDir
	}

	rootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return ToolExecutionContext{}, fmt.Errorf("failed to resolve root directory: %w", err)
	}
	currentDir, err = filepath.Abs(currentDir)
	if err != nil {
		return ToolExecutionContext{}, fmt.Errorf("failed to resolve current directory: %w", err)
	}
	if err := ensureWithinRoot(currentDir, rootDir); err != nil {
		return ToolExecutionContext{}, err
	}

	return ToolExecutionContext{RootDir: rootDir, CurrentDir: currentDir}, nil
}

func ensureWithinRoot(path string, root string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("failed to validate path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("path outside working directory: %s", path)
	}
	return nil
}

func resolvePathInContext(path string, ctx ToolExecutionContext, fallbackDir string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	normalizedCtx, err := normalizeExecutionContext(ctx, fallbackDir)
	if err != nil {
		return "", err
	}

	var absPath string
	if filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	} else {
		absPath = filepath.Join(normalizedCtx.CurrentDir, path)
	}

	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}
	if err := ensureWithinRoot(absPath, normalizedCtx.RootDir); err != nil {
		return "", err
	}

	return absPath, nil
}

func resolveRootInContext(root string, ctx ToolExecutionContext, fallbackDir string) (string, error) {
	normalizedCtx, err := normalizeExecutionContext(ctx, fallbackDir)
	if err != nil {
		return "", err
	}
	if root == "" {
		return normalizedCtx.CurrentDir, nil
	}
	return resolvePathInContext(root, normalizedCtx, fallbackDir)
}

func resolvePath(path string, workDir string) (string, error) {
	return resolvePathInContext(path, ToolExecutionContext{RootDir: workDir, CurrentDir: workDir}, workDir)
}

func resolveRoot(root string, workDir string) (string, error) {
	return resolveRootInContext(root, ToolExecutionContext{RootDir: workDir, CurrentDir: workDir}, workDir)
}
