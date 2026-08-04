package releaseverify

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/workspace"
)

type runPaths struct {
	root        string
	inputs      string
	workspace   string
	environment string
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

func prepareRun(repositoryRoot string, runRoot string, stage string) (runPaths, error) {
	repositoryRoot, err := existingPlainDirectory(repositoryRoot, "仓库根目录")
	if err != nil {
		return runPaths{}, err
	}
	runRoot, err = filepath.Abs(strings.TrimSpace(runRoot))
	if err != nil || strings.TrimSpace(runRoot) == "" {
		return runPaths{}, fmt.Errorf("验证运行目录无效")
	}
	expectedParent := workspace.VerificationStageRoot(repositoryRoot, stage)
	if !pathWithin(expectedParent, runRoot) || samePath(expectedParent, runRoot) || !strings.HasPrefix(filepath.Base(runRoot), "run-") {
		return runPaths{}, fmt.Errorf("验证运行目录必须位于 %s 的独立 run-* 目录中", expectedParent)
	}
	if err := ensurePlainDirectoryPath(repositoryRoot, runRoot, "验证运行目录"); err != nil {
		return runPaths{}, err
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
		sharedCache: workspace.VerificationCacheRoot(repositoryRoot),
	}
	for _, path := range []string{paths.inputs, paths.workspace, paths.environment, paths.temp, paths.cache, paths.evidence, paths.sharedCache} {
		if err := ensurePlainDirectoryPath(repositoryRoot, path, "验证资料目录"); err != nil {
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

func existingPlainDirectory(value string, label string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(value))
	if err != nil || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s无效", label)
	}
	absolute = filepath.Clean(absolute)
	if err := assertPlainDirectoryChain(absolute, label); err != nil {
		return "", err
	}
	return absolute, nil
}

// ensurePlainDirectoryPath 只沿真实目录建立验证资料路径。
// 每一级都会重新检查，目录联接点和符号链接不能把验证写入带到边界之外。
func ensurePlainDirectoryPath(base string, target string, label string) error {
	base, err := existingPlainDirectory(base, "验证目录根")
	if err != nil {
		return err
	}
	target, err = filepath.Abs(strings.TrimSpace(target))
	if err != nil || strings.TrimSpace(target) == "" {
		return fmt.Errorf("%s无效", label)
	}
	target = filepath.Clean(target)
	if !pathWithin(base, target) {
		return fmt.Errorf("%s越过验证目录根", label)
	}
	relative, err := filepath.Rel(base, target)
	if err != nil {
		return fmt.Errorf("确定%s路径失败：%w", label, err)
	}
	if relative == "." {
		return nil
	}
	current := base
	for _, name := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, name)
		if err := ensurePlainDirectory(current, label); err != nil {
			return err
		}
	}
	return nil
}

func ensurePlainDirectory(path string, label string) error {
	if err := assertPlainDirectory(path, label); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Mkdir(path, 0o755); err != nil && !os.IsExist(err) {
		return fmt.Errorf("建立%s失败：%w", label, err)
	}
	return assertPlainDirectory(path, label)
}

func assertPlainDirectoryChain(path string, label string) error {
	chain := make([]string, 0, 8)
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		chain = append(chain, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	for index := len(chain) - 1; index >= 0; index-- {
		if err := assertPlainDirectory(chain[index], label); err != nil {
			return err
		}
	}
	return nil
}

func assertPlainDirectory(path string, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("读取%s失败：%w", label, err)
	}
	reparsePoint, err := isReparsePoint(path, info)
	if err != nil {
		return fmt.Errorf("检查%s目录边界失败：%w", label, err)
	}
	if reparsePoint {
		return fmt.Errorf("%s不能经过目录联接点或符号链接：%s", label, path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s必须是目录", label)
	}
	return nil
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
