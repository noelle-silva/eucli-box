package everything

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	scopeModeAllLocalDrives = "allLocalDrives"
	scopeModeDirectory      = "directory"
)

type searchScope struct {
	Mode         string
	SearchPath   string
	DisplayPaths []string
	IndexPaths   []string
}

func resolveSearchScope(hostWorkingDirectory string, value string) (searchScope, error) {
	path := strings.TrimSpace(value)
	if path == "" {
		paths, err := defaultLocalDriveRoots()
		if err != nil {
			return searchScope{}, err
		}
		return searchScope{Mode: scopeModeAllLocalDrives, DisplayPaths: paths}, nil
	}
	resolved, err := normalizeScopePath(hostWorkingDirectory, path)
	if err != nil {
		return searchScope{}, err
	}
	return searchScope{Mode: scopeModeDirectory, SearchPath: resolved, DisplayPaths: []string{resolved}, IndexPaths: []string{resolved}}, nil
}

func normalizeScopePath(hostWorkingDirectory string, value string) (string, error) {
	path := strings.TrimSpace(value)
	if path == "" {
		return "", nil
	}
	var resolved string
	if filepath.IsAbs(path) {
		resolved = filepath.Clean(path)
	} else {
		if filepath.VolumeName(path) != "" {
			return "", fmt.Errorf("scopePath must be absolute or relative without a volume name")
		}
		base := strings.TrimSpace(hostWorkingDirectory)
		if base == "" {
			return "", fmt.Errorf("hostWorkingDirectory is required when scopePath is relative")
		}
		resolved = filepath.Clean(filepath.Join(base, path))
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("scopePath does not exist: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("scopePath must be a directory")
	}
	return resolved, nil
}

func defaultLocalDriveRoots() ([]string, error) {
	if runtime.GOOS != "windows" {
		root := string(filepath.Separator)
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("default full-disk search root is not available")
		}
		return []string{root}, nil
	}
	roots := []string{}
	for drive := 'A'; drive <= 'Z'; drive++ {
		root := string(drive) + `:\`
		info, err := os.Stat(root)
		if err == nil && info.IsDir() {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("no accessible drive roots found for default full-disk search")
	}
	return roots, nil
}
