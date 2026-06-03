package shellcommand

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveWorkdir(hostWorkingDirectory string, requestedWorkdir string) (string, error) {
	hostWorkingDirectory = strings.TrimSpace(hostWorkingDirectory)
	if hostWorkingDirectory == "" {
		return "", fmt.Errorf("hostWorkingDirectory is required")
	}
	hostWorkingDirectory = filepath.Clean(hostWorkingDirectory)
	workdir := strings.TrimSpace(requestedWorkdir)
	if workdir == "" {
		workdir = "."
	}
	if !filepath.IsAbs(workdir) {
		workdir = filepath.Join(hostWorkingDirectory, workdir)
	}
	resolved, err := filepath.Abs(workdir)
	if err != nil {
		return "", fmt.Errorf("resolve workdir: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("workdir does not exist: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workdir is not a directory")
	}
	return resolved, nil
}
