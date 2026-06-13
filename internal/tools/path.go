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
	allowedRoots, err := normalizeAllowedRoots(rootDir, ctx.AllowedRootDirs)
	if err != nil {
		return ToolExecutionContext{}, err
	}
	if err := ensureWithinAllowedRoots(currentDir, allowedRoots); err != nil {
		return ToolExecutionContext{}, err
	}

	allowedPaths, err := normalizeAllowedPaths(ctx.AllowedPaths)
	if err != nil {
		return ToolExecutionContext{}, err
	}

	return ToolExecutionContext{RootDir: rootDir, CurrentDir: currentDir, AllowedRootDirs: allowedRoots, AllowedPaths: allowedPaths, Output: ctx.Output, Progress: ctx.Progress}, nil
}

func normalizeAllowedRoots(rootDir string, extraRoots []string) ([]string, error) {
	roots := []string{rootDir}
	for _, root := range extraRoots {
		trimmed := strings.TrimSpace(root)
		if trimmed == "" {
			continue
		}
		absRoot, err := filepath.Abs(trimmed)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve allowed root directory: %w", err)
		}
		roots = append(roots, absRoot)
	}
	return uniqueCleanPaths(roots), nil
}

func uniqueCleanPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	unique := make([]string, 0, len(paths))
	for _, value := range paths {
		cleaned := filepath.Clean(value)
		if seen[cleaned] {
			continue
		}
		seen[cleaned] = true
		unique = append(unique, cleaned)
	}
	return unique
}

func normalizeAllowedPaths(paths []string) ([]string, error) {
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		absPath, err := filepath.Abs(trimmed)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve allowed path: %w", err)
		}
		normalized = append(normalized, absPath)
	}
	return uniqueCleanPaths(normalized), nil
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

func ensureWithinAllowedRoots(path string, roots []string) error {
	for _, root := range roots {
		if ensureWithinRoot(path, root) == nil {
			return nil
		}
	}
	return fmt.Errorf("path outside allowed working directories: %s", path)
}

func ensureAllowedPath(path string, roots []string, exactPaths []string) error {
	cleanPath := filepath.Clean(path)
	for _, exactPath := range exactPaths {
		if cleanPath == exactPath {
			return nil
		}
	}
	return ensureWithinAllowedRoots(cleanPath, roots)
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
	if err := ensureAllowedPath(absPath, normalizedCtx.AllowedRootDirs, normalizedCtx.AllowedPaths); err != nil {
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

// PathAllowedInContext reports whether path resolves inside the context's
// allowed roots or exact allowed paths. An empty or unresolvable path returns false.
func PathAllowedInContext(path string, ctx ToolExecutionContext) bool {
	_, err := resolvePathInContext(path, ctx, ctx.CurrentDir)
	return err == nil
}

func resolvePath(path string, workDir string) (string, error) {
	return resolvePathInContext(path, ToolExecutionContext{RootDir: workDir, CurrentDir: workDir}, workDir)
}

func resolveRoot(root string, workDir string) (string, error) {
	return resolveRootInContext(root, ToolExecutionContext{RootDir: workDir, CurrentDir: workDir}, workDir)
}
