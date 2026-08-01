package releaseverify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type runPaths struct {
	root        string
	inputs      string
	workspace   string
	environment string
	temp        string
	cache       string
	evidence    string
}

const (
	verificationInputsDirectory      = "inputs"
	verificationWorkspaceDirectory   = "workspace"
	verificationEnvironmentDirectory = "environment"
	verificationTempDirectory        = "temp"
	verificationCacheDirectory       = "cache"
)

type cleanupEntry struct {
	name string
	path string
}

func prepareRun(repositoryRoot string, runRoot string, stage string) (runPaths, error) {
	repositoryRoot, err := existingDirectory(repositoryRoot, "仓库根目录")
	if err != nil {
		return runPaths{}, err
	}
	runRoot, err = filepath.Abs(strings.TrimSpace(runRoot))
	if err != nil || strings.TrimSpace(runRoot) == "" {
		return runPaths{}, fmt.Errorf("验证运行目录无效")
	}
	expectedParent := filepath.Join(repositoryRoot, ".release", "verification", "stage-"+stage)
	if !pathWithin(expectedParent, runRoot) || samePath(expectedParent, runRoot) || !strings.HasPrefix(filepath.Base(runRoot), "run-") {
		return runPaths{}, fmt.Errorf("验证运行目录必须位于 %s 的独立 run-* 目录中", expectedParent)
	}
	if _, err := os.Stat(runRoot); err == nil {
		entries, readErr := os.ReadDir(runRoot)
		if readErr != nil {
			return runPaths{}, readErr
		}
		for _, entry := range entries {
			if entry.Name() != "temp" && entry.Name() != "cache" {
				return runPaths{}, fmt.Errorf("验证运行目录包含本次入口之外的已有内容：%s", entry.Name())
			}
			if !entry.IsDir() {
				return runPaths{}, fmt.Errorf("验证运行目录中的预备内容必须是目录：%s", entry.Name())
			}
		}
	} else if !os.IsNotExist(err) {
		return runPaths{}, err
	}
	paths := runPaths{
		root:        runRoot,
		inputs:      filepath.Join(runRoot, "inputs"),
		workspace:   filepath.Join(runRoot, "workspace"),
		environment: filepath.Join(runRoot, "environment"),
		temp:        filepath.Join(runRoot, "temp"),
		cache:       filepath.Join(runRoot, "cache"),
		evidence:    filepath.Join(runRoot, "evidence"),
	}
	for _, path := range []string{paths.root, paths.inputs, paths.workspace, paths.environment, paths.temp, paths.cache, paths.evidence} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return runPaths{}, err
		}
	}
	return paths, nil
}

func (p runPaths) cleanupState() (completed []string, pending []string, stateErr error) {
	entries := p.cleanupEntries()
	for index, entry := range entries {
		if _, err := os.Stat(entry.path); os.IsNotExist(err) {
			completed = append(completed, entry.name)
		} else if err != nil {
			pending = append(pending, cleanupEntryNames(entries[index:])...)
			return completed, pending, fmt.Errorf("读取清理目录 %s 失败：%w", entry.name, err)
		} else {
			pending = append(pending, entry.name)
		}
	}
	return completed, pending, nil
}

func verificationWorkspaceDirectories() []string {
	return []string{
		verificationInputsDirectory,
		verificationWorkspaceDirectory,
		verificationEnvironmentDirectory,
	}
}

func bootstrapDirectories() []string {
	return []string{verificationTempDirectory, verificationCacheDirectory}
}

func disposableDirectories() []string {
	return append(verificationWorkspaceDirectories(), bootstrapDirectories()...)
}

func (p runPaths) verificationCleanupEntries() []cleanupEntry {
	return []cleanupEntry{
		{name: verificationInputsDirectory, path: p.inputs},
		{name: verificationWorkspaceDirectory, path: p.workspace},
		{name: verificationEnvironmentDirectory, path: p.environment},
	}
}

func (p runPaths) bootstrapCleanupEntries() []cleanupEntry {
	return []cleanupEntry{
		{name: verificationTempDirectory, path: p.temp},
		{name: verificationCacheDirectory, path: p.cache},
	}
}

func (p runPaths) cleanupEntries() []cleanupEntry {
	entries := p.verificationCleanupEntries()
	return append(entries, p.bootstrapCleanupEntries()...)
}

func cleanupEntryNames(entries []cleanupEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}
	return names
}

func existingDirectory(value string, label string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(value))
	if err != nil || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s无效", label)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s必须是目录", label)
	}
	return filepath.Clean(absolute), nil
}

func pathWithin(base string, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func samePath(left string, right string) bool {
	left, _ = filepath.Abs(left)
	right, _ = filepath.Abs(right)
	return filepath.Clean(left) == filepath.Clean(right)
}
