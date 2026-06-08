package fileoperator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PathPolicy struct {
	baseDir string
}

type ResolvedPath struct {
	Requested string
	Absolute  string
	Display   string
}

func newPathPolicy(hostWorkingDirectory string) (PathPolicy, error) {
	baseInput, err := filepath.Abs(strings.TrimSpace(hostWorkingDirectory))
	if err != nil {
		return PathPolicy{}, fmt.Errorf("resolve base directory: %w", err)
	}
	info, err := os.Stat(baseInput)
	if err != nil {
		return PathPolicy{}, fmt.Errorf("base directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return PathPolicy{}, fmt.Errorf("base directory is not a directory")
	}
	baseReal, err := filepath.EvalSymlinks(baseInput)
	if err != nil {
		return PathPolicy{}, fmt.Errorf("resolve base directory symlink: %w", err)
	}
	baseDir, err := filepath.Abs(baseReal)
	if err != nil {
		return PathPolicy{}, fmt.Errorf("resolve real base directory: %w", err)
	}
	return PathPolicy{baseDir: filepath.Clean(baseDir)}, nil
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
		resolved = filepath.Join(p.baseDir, resolved)
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("resolve path: %w", err)
	}
	abs, err = p.canonicalPath(filepath.Clean(abs))
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("resolve real path: %w", err)
	}
	display := abs
	if rel, err := filepath.Rel(p.baseDir, abs); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		display = filepath.ToSlash(rel)
	}
	return ResolvedPath{Requested: requested, Absolute: abs, Display: display}, nil
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

func ensureParentCreatable(path string) error {
	current := filepath.Clean(filepath.Dir(path))
	for {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("parent path is not a directory: %s", current)
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("no existing parent found for %s", path)
		}
		current = parent
	}
}
