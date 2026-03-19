package privileged

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var forbiddenShellFragments = []string{"&&", "&", "||", "|", ";", ">", "<", "`", "$(", "\r", "\n"}

func (l *Layer) resolveWorkspaceDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		path = "."
	}

	resolved, err := resolveAgainstRoots(path, l.workspaceRoots[0], l.workspaceRoots, l.workspaceRoots[0])
	if err != nil {
		return "", err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("workspace directory %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", deniedErrorf("workspace directory %q is not a directory", resolved)
	}

	return resolved, nil
}

func (l *Layer) resolveWritePath(path, baseDirectory string) (string, error) {
	return resolveAgainstRoots(path, baseDirectory, l.writableRoots, l.writableRoots[0])
}

func resolveAgainstRoots(path, baseDirectory string, roots []string, defaultBase string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", deniedErrorf("path is required")
	}

	candidate, err := resolveCandidatePath(path, baseDirectory, defaultBase)
	if err != nil {
		return "", err
	}

	for _, root := range roots {
		if isWithinRoot(candidate, root) {
			return candidate, nil
		}
	}

	return "", deniedErrorf("path %q is outside allowed roots", candidate)
}

func resolveCandidatePath(path, baseDirectory, defaultBase string) (string, error) {
	path = strings.TrimSpace(path)
	baseDirectory = strings.TrimSpace(baseDirectory)

	if filepath.IsAbs(path) {
		return resolvePathAllowMissing(path)
	}

	base := defaultBase
	if baseDirectory != "" {
		base = baseDirectory
	}
	if strings.TrimSpace(base) == "" {
		return "", deniedErrorf("a base directory is required for relative paths")
	}
	if !filepath.IsAbs(base) {
		base = filepath.Join(defaultBase, base)
	}

	resolvedBase, err := resolvePathAllowMissing(base)
	if err != nil {
		return "", fmt.Errorf("resolve base directory %q: %w", base, err)
	}

	return resolvePathAllowMissing(filepath.Join(resolvedBase, path))
}

func resolvePathAllowMissing(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path %q: %w", path, err)
	}

	absolute = filepath.Clean(absolute)

	current := absolute
	missing := make([]string, 0, 4)
	for {
		_, err := os.Lstat(current)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect path %q: %w", current, err)
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}

		missing = append(missing, filepath.Base(current))
		current = parent
	}

	resolvedBase, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", fmt.Errorf("resolve symlinks for %q: %w", current, err)
	}

	for index := len(missing) - 1; index >= 0; index-- {
		resolvedBase = filepath.Join(resolvedBase, missing[index])
	}

	return filepath.Clean(resolvedBase), nil
}

func isWithinRoot(path, root string) bool {
	path = comparablePath(path)
	root = comparablePath(root)

	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}

	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func comparablePath(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func commandIsSafe(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return deniedErrorf("shell command is required")
	}

	for _, fragment := range forbiddenShellFragments {
		if strings.Contains(command, fragment) {
			return deniedErrorf("shell command contains forbidden fragment %q", fragment)
		}
	}

	return nil
}

func (l *Layer) commandIsAutoApproved(command string) bool {
	command = strings.TrimSpace(command)
	for _, entry := range l.shellAllowlist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if command == entry {
			return true
		}
		if strings.HasPrefix(command, entry) && len(command) > len(entry) {
			next := command[len(entry)]
			if next == ' ' || next == '\t' {
				return true
			}
		}
	}

	return false
}
