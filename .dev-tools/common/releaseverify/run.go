package releaseverify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devtools/common/toolkit"
)

type runPaths struct {
	root        string
	inputs      string
	workspace   string
	environment string
	work        string
	temp        string
	cache       string
	evidence    string
	sharedCache string
}

const (
	verificationInputsDirectory      = "inputs"
	verificationWorkspaceDirectory   = "workspace"
	verificationEnvironmentDirectory = "environment"
	verificationTempDirectory        = "temp"
	verificationCacheDirectory       = "cache"
)

// validToolName 校验工具名只包含字母、数字、连字符与下划线。
func validToolName(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

type cleanupEntry struct {
	name string
	path string
}

func prepareRun(repositoryRoot string, runRoot string, tool string) (runPaths, error) {
	repositoryRoot, err := toolkit.ExistingPlainDirectory(repositoryRoot, "仓库根目录")
	if err != nil {
		return runPaths{}, err
	}
	if !validToolName(tool) {
		return runPaths{}, fmt.Errorf("验证工具名只能包含字母、数字、连字符与下划线：%q", tool)
	}
	expectedParent := filepath.Join(repositoryRoot, ".dev-workspace", ".dev-tools-runtime", tool)
	if !toolkit.PathWithin(expectedParent, runRoot) || toolkit.SamePath(expectedParent, runRoot) || !strings.HasPrefix(filepath.Base(runRoot), "run-") {
		return runPaths{}, fmt.Errorf("验证运行目录必须位于 %s 的独立 run-* 目录中", expectedParent)
	}
	if err := toolkit.EnsurePlainDirectoryPath(repositoryRoot, runRoot, "验证运行目录"); err != nil {
		return runPaths{}, err
	}
	if _, err := os.Stat(runRoot); err == nil {
		entries, readErr := os.ReadDir(runRoot)
		if readErr != nil {
			return runPaths{}, readErr
		}
		for _, entry := range entries {
			if entry.Name() != "temp" && entry.Name() != "cache" && entry.Name() != "work" {
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
		work:        filepath.Join(runRoot, "work"),
		temp:        filepath.Join(runRoot, "temp"),
		cache:       filepath.Join(runRoot, "cache"),
		evidence:    filepath.Join(runRoot, "evidence"),
		sharedCache: filepath.Join(repositoryRoot, ".dev-workspace", ".dev-tools-runtime", "cache"),
	}
	for _, path := range []string{paths.inputs, paths.workspace, paths.environment, paths.temp, paths.cache, paths.evidence, paths.sharedCache} {
		if err := toolkit.EnsurePlainDirectoryPath(repositoryRoot, path, "验证资料目录"); err != nil {
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
		"work",
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
		{name: "work", path: p.work},
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
