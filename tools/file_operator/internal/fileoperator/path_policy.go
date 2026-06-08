package fileoperator

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type PathPolicy struct {
	hostRoot string
	roots    []string
}

type ResolvedPath struct {
	Requested string
	Absolute  string
	Display   string
	Root      string
}

func newPathPolicy(hostWorkingDirectory string, config Config) (PathPolicy, error) {
	hostRootInput, err := filepath.Abs(strings.TrimSpace(hostWorkingDirectory))
	if err != nil {
		return PathPolicy{}, fmt.Errorf("resolve host working directory: %w", err)
	}
	info, err := os.Stat(hostRootInput)
	if err != nil {
		return PathPolicy{}, fmt.Errorf("host working directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return PathPolicy{}, fmt.Errorf("host working directory is not a directory")
	}
	hostRootReal, err := filepath.EvalSymlinks(hostRootInput)
	if err != nil {
		return PathPolicy{}, fmt.Errorf("resolve host working directory symlink: %w", err)
	}
	hostRoot, err := filepath.Abs(hostRootReal)
	if err != nil {
		return PathPolicy{}, fmt.Errorf("resolve real host working directory: %w", err)
	}
	hostRoot = filepath.Clean(hostRoot)
	roots := config.WorkspaceRoots
	if len(roots) == 0 {
		roots = []string{hostRoot}
	}
	resolvedRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if !filepath.IsAbs(root) {
			root = filepath.Join(hostRoot, root)
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return PathPolicy{}, fmt.Errorf("resolve workspace root %q: %w", root, err)
		}
		info, err := os.Stat(absRoot)
		if err != nil {
			return PathPolicy{}, fmt.Errorf("workspace root does not exist %q: %w", absRoot, err)
		}
		if !info.IsDir() {
			return PathPolicy{}, fmt.Errorf("workspace root is not a directory: %q", absRoot)
		}
		realRoot, err := filepath.EvalSymlinks(absRoot)
		if err != nil {
			return PathPolicy{}, fmt.Errorf("resolve workspace root symlink %q: %w", absRoot, err)
		}
		resolvedRoots = append(resolvedRoots, filepath.Clean(realRoot))
	}
	if len(resolvedRoots) == 0 {
		return PathPolicy{}, fmt.Errorf("at least one workspace root is required")
	}
	return PathPolicy{hostRoot: hostRoot, roots: resolvedRoots}, nil
}

func (p PathPolicy) Resolve(inputPath string) (ResolvedPath, error) {
	requested := strings.TrimSpace(inputPath)
	if requested == "" {
		requested = "."
	}
	if strings.ContainsRune(requested, '\x00') {
		return ResolvedPath{}, fmt.Errorf("path cannot contain null bytes")
	}
	resolved := requested
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(p.hostRoot, resolved)
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("resolve path: %w", err)
	}
	abs, err = p.canonicalPath(filepath.Clean(abs))
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("resolve real path: %w", err)
	}
	root, ok := p.allowedRoot(abs)
	if !ok {
		return ResolvedPath{}, fmt.Errorf("path is outside workspace roots: %s", abs)
	}
	display := abs
	if rel, err := filepath.Rel(p.hostRoot, abs); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		display = filepath.ToSlash(rel)
	}
	return ResolvedPath{Requested: requested, Absolute: abs, Display: display, Root: root}, nil
}

func (p PathPolicy) ResolveExisting(inputPath string) (ResolvedPath, error) {
	resolved, err := p.Resolve(inputPath)
	if err != nil {
		return ResolvedPath{}, err
	}
	if _, err := os.Stat(resolved.Absolute); err != nil {
		return ResolvedPath{}, err
	}
	return resolved, nil
}

func (p PathPolicy) canonicalPath(path string) (string, error) {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		absReal, err := filepath.Abs(real)
		if err != nil {
			return "", err
		}
		return filepath.Clean(absReal), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent, err := nearestExistingParent(path)
	if err != nil {
		return "", err
	}
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(realParent, rel)), nil
}

func (p PathPolicy) allowedRoot(path string) (string, bool) {
	for _, root := range p.roots {
		if pathWithin(root, path) {
			return root, true
		}
	}
	return "", false
}

func pathWithin(base string, child string) bool {
	base = filepath.Clean(base)
	child = filepath.Clean(child)
	if runtime.GOOS == "windows" {
		base = strings.ToLower(base)
		child = strings.ToLower(child)
	}
	rel, err := filepath.Rel(base, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func nearestExistingParent(path string) (string, error) {
	current := filepath.Clean(path)
	for {
		info, err := os.Stat(current)
		if err == nil {
			if info.IsDir() {
				return current, nil
			}
			return filepath.Dir(current), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing parent found for %s", path)
		}
		current = parent
	}
}

func ensureParentInsidePolicy(policy PathPolicy, path string) error {
	parent, err := nearestExistingParent(path)
	if err != nil {
		return err
	}
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return err
	}
	if _, ok := policy.allowedRoot(filepath.Clean(realParent)); !ok {
		return fmt.Errorf("parent directory is outside workspace roots: %s", realParent)
	}
	return nil
}
