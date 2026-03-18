package pathutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolveWithinWorkspace(path, workspace string) (string, error) {
	workspaceRoot, err := resolveWorkspaceRoot(workspace)
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed == "." {
		return workspaceRoot, nil
	}

	candidate := trimmed
	if !filepath.IsAbs(trimmed) {
		candidate = filepath.Join(workspaceRoot, trimmed)
	}

	return resolveAndValidate(candidate, workspaceRoot)
}

func ResolveRelativeOnly(path, workspace string) (string, error) {
	workspaceRoot, err := resolveWorkspaceRoot(workspace)
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed == "." {
		return workspaceRoot, nil
	}
	if filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("must not use absolute path")
	}

	return resolveAndValidate(filepath.Join(workspaceRoot, trimmed), workspaceRoot)
}

func resolveWorkspaceRoot(workspace string) (string, error) {
	trimmed := strings.TrimSpace(workspace)
	if trimmed == "" {
		return "", fmt.Errorf("workspace is required")
	}

	absWorkspace, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}

	resolvedWorkspace, err := filepath.EvalSymlinks(absWorkspace)
	if err == nil {
		return resolvedWorkspace, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return filepath.Clean(absWorkspace), nil
	}

	return "", err
}

func resolveAndValidate(path, workspaceRoot string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	resolvedPath, err := resolveExistingPath(absPath)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(workspaceRoot, resolvedPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside the workspace")
	}

	return resolvedPath, nil
}

func resolveExistingPath(path string) (string, error) {
	current := filepath.Clean(path)
	var suffix []string

	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			if len(suffix) == 0 {
				return filepath.Clean(resolved), nil
			}
			return filepath.Join(append([]string{resolved}, reverseStrings(suffix)...)...), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			if len(suffix) == 0 {
				return filepath.Clean(path), nil
			}
			return filepath.Join(append([]string{current}, reverseStrings(suffix)...)...), nil
		}

		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func reverseStrings(values []string) []string {
	reversed := make([]string, 0, len(values))
	for i := len(values) - 1; i >= 0; i-- {
		reversed = append(reversed, values[i])
	}
	return reversed
}
