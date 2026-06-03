package worktreeoverlay

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

type fileSnapshot struct {
	Exists bool
	Hash   string
	Mode   os.FileMode
}

func snapshotFile(filePath string) (fileSnapshot, error) {
	info, err := os.Lstat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fileSnapshot{}, nil
		}
		return fileSnapshot{}, fmt.Errorf("stat %s: %w", filePath, err)
	}
	if info.IsDir() {
		return fileSnapshot{}, fmt.Errorf("%s is a directory", filePath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fileSnapshot{}, fmt.Errorf("%s is a symlink; symlink overlays are not supported", filePath)
	}
	if !info.Mode().IsRegular() {
		return fileSnapshot{}, fmt.Errorf("%s is not a regular file", filePath)
	}
	hash, err := hashFile(filePath)
	if err != nil {
		return fileSnapshot{}, err
	}
	return fileSnapshot{Exists: true, Hash: hash, Mode: info.Mode().Perm()}, nil
}

func hashFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", filePath, err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash %s: %w", filePath, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyFile(source string, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open %s: %w", source, err)
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent for %s: %w", target, err)
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open %s: %w", target, err)
	}
	defer output.Close()
	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("copy %s to %s: %w", source, target, err)
	}
	return nil
}

func copyFilePreservingMode(source string, target string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("stat %s: %w", source, err)
	}
	if info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", source)
	}
	return copyFile(source, target, info.Mode().Perm())
}

func removeFileAndEmptyParents(root string, filePath string) error {
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", filePath, err)
	}
	parent := filepath.Dir(filePath)
	for pathWithin(root, parent) && !samePath(root, parent) {
		if err := os.Remove(parent); err != nil {
			return nil
		}
		parent = filepath.Dir(parent)
	}
	return nil
}

func validateRepoPath(repoPath string) (string, error) {
	if repoPath == "" || strings.ContainsRune(repoPath, '\x00') {
		return "", fmt.Errorf("empty repository path")
	}
	if filepath.IsAbs(repoPath) || filepath.VolumeName(repoPath) != "" {
		return "", fmt.Errorf("repository path %q must be relative", repoPath)
	}
	cleaned := path.Clean(strings.ReplaceAll(repoPath, "\\", "/"))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("repository path %q escapes repository root", repoPath)
	}
	if cleaned == ".git" || strings.HasPrefix(cleaned, ".git/") {
		return "", fmt.Errorf("repository path %q targets git internals", repoPath)
	}
	return cleaned, nil
}

func repoFile(root string, repoPath string) (string, error) {
	relative, err := validateRepoPath(repoPath)
	if err != nil {
		return "", err
	}
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	if !pathWithin(root, absolute) {
		return "", fmt.Errorf("repository path %q escapes repository root", repoPath)
	}
	return absolute, nil
}

func pathWithin(base string, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func samePath(left string, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
