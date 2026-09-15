package releaseverify

import (
	"fmt"
	"os"
	"path/filepath"

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

type cleanupEntry struct {
	name string
	path string
}

func prepareRun(repositoryRoot string, runRoot string, tool string) (runPaths, error) {
	run, err := toolkit.PrepareVerificationRun(repositoryRoot, runRoot, tool)
	if err != nil {
		return runPaths{}, err
	}
	paths := runPaths{
		root:        run.Root,
		inputs:      run.Inputs,
		workspace:   run.Workspace,
		environment: run.Environment,
		work:        run.Work,
		temp:        run.Temp,
		cache:       run.Cache,
		evidence:    run.Evidence,
		sharedCache: filepath.Join(run.RepositoryRoot, ".dev-workspace", ".dev-tools-runtime", "cache"),
	}
	if err := toolkit.EnsurePlainDirectoryPath(run.RepositoryRoot, paths.sharedCache, "验证资料目录"); err != nil {
		return runPaths{}, err
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
