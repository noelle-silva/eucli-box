package datamigration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	backupSchemaVersion = 1
	versionFileScope    = "meta/version.json"
	spaceReserveBytes   = 1 << 20
)

// backupManifest 是备份清单：每个文件的相对路径、字节大小与 SHA-256。
type backupManifest struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Files         []backupManifestFile `json:"files"`
	TotalBytes    int64                `json:"totalBytes"`
	CreatedAt     string               `json:"createdAt"`
}

type backupManifestFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	SHA256    string `json:"sha256"`
}

// backupScope 返回恢复资料覆盖范围：所有登记步骤 Scope 的并集加上 meta/version.json。
func backupScope() []string {
	scope := []string{versionFileScope}
	for _, step := range registeredSteps() {
		scope = append(scope, step.Scope...)
	}
	return scope
}

// backupScopeFor 返回指定步骤的恢复范围并集加上 meta/version.json。
func backupScopeFor(stepIDs []string) ([]string, error) {
	byID := make(map[string]Step, len(registry))
	for _, step := range registeredSteps() {
		byID[step.ID] = step
	}
	scope := []string{versionFileScope}
	for _, id := range stepIDs {
		step, exists := byID[id]
		if !exists {
			return nil, fmt.Errorf("backup scope references unregistered step %s", id)
		}
		scope = append(scope, step.Scope...)
	}
	return scope, nil
}

// scopeMatches 判断以 / 分隔的相对路径是否落在范围前缀内。
// 范围条目本身可以是文件也可以是目录前缀。
func scopeMatches(scope []string, slashPath string) bool {
	for _, entry := range scope {
		if slashPath == entry || strings.HasPrefix(slashPath, entry+"/") {
			return true
		}
	}
	return false
}

// scopeContainsDir 判断目录是否位于范围覆盖的树内（用于恢复时清理迁移新产生的目录）。
func scopeContainsDir(scope []string, slashDir string) bool {
	for _, entry := range scope {
		if slashDir == entry || strings.HasPrefix(slashDir, entry+"/") || strings.HasPrefix(entry, slashDir+"/") {
			return true
		}
	}
	return false
}

// collectBackupFiles 遍历数据目录，返回范围内文件的相对路径（/ 分隔）与字节总量。
// 遇到符号链接或重解析点立即失败，不跟随。
func collectBackupFiles(ctx context.Context, dataDir string, scope []string) ([]string, int64, error) {
	files := make([]string, 0)
	var total int64
	err := filepath.WalkDir(dataDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		reparsePoint, err := isReparsePoint(path, info)
		if err != nil {
			return err
		}
		if reparsePoint {
			return migrationPrepareFailed("data directory contains a reparse point", fmt.Errorf("%s", path))
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(dataDir, path)
		if err != nil {
			return err
		}
		slashPath := filepath.ToSlash(relative)
		if scopeMatches(scope, slashPath) {
			files = append(files, slashPath)
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return nil, 0, migrationPrepareFailed("failed to walk data directory for backup", err)
	}
	return files, total, nil
}

// establishBackup 建立本次迁移运行的恢复资料：复制范围内文件、写清单、重新核对。
func establishBackup(ctx context.Context, dataDir string, w workspace, runID string, scope []string) (backupManifest, error) {
	files, total, err := collectBackupFiles(ctx, dataDir, scope)
	if err != nil {
		return backupManifest{}, err
	}
	required := 2*uint64(total) + spaceReserveBytes
	if err := checkDiskSpace(w.dir, required); err != nil {
		return backupManifest{}, migrationPrepareFailed("insufficient space for migration backup", err)
	}
	dataBackupDir := w.backupDataDir(runID)
	if err := os.MkdirAll(dataBackupDir, 0o700); err != nil {
		return backupManifest{}, migrationPrepareFailed("failed to create backup directory", err)
	}
	manifest := backupManifest{
		SchemaVersion: backupSchemaVersion,
		Files:         make([]backupManifestFile, 0, len(files)),
		TotalBytes:    total,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	for _, slashPath := range files {
		source := filepath.Join(dataDir, filepath.FromSlash(slashPath))
		target := filepath.Join(dataBackupDir, filepath.FromSlash(slashPath))
		digest, size, err := copyFileWithHash(source, target)
		if err != nil {
			return backupManifest{}, migrationPrepareFailed("failed to copy "+slashPath+" into backup", err)
		}
		manifest.Files = append(manifest.Files, backupManifestFile{Path: slashPath, SizeBytes: size, SHA256: digest})
	}
	if err := writeBackupManifest(w.manifestFile(runID), manifest); err != nil {
		return backupManifest{}, migrationPrepareFailed("failed to write backup manifest", err)
	}
	loaded, err := readBackupManifest(w.manifestFile(runID))
	if err != nil {
		return backupManifest{}, migrationPrepareFailed("failed to re-read backup manifest", err)
	}
	for _, file := range loaded.Files {
		sourceDigest, err := fileSHA256(filepath.Join(dataDir, filepath.FromSlash(file.Path)))
		if err != nil {
			return backupManifest{}, migrationPrepareFailed("failed to verify source file "+file.Path, err)
		}
		backupDigest, err := fileSHA256(filepath.Join(dataBackupDir, filepath.FromSlash(file.Path)))
		if err != nil {
			return backupManifest{}, migrationPrepareFailed("failed to verify backup file "+file.Path, err)
		}
		if sourceDigest != file.SHA256 || backupDigest != file.SHA256 {
			return backupManifest{}, migrationPrepareFailed("backup verification mismatch for "+file.Path, nil)
		}
	}
	return loaded, nil
}

// restoreFromBackup 按固定顺序恢复数据目录到迁移开始前：
// 写回备份文件 → 删除范围内不在清单中的文件与空目录（从深到浅）→ 逐文件核对。
func restoreFromBackup(ctx context.Context, dataDir string, w workspace, runID string, scope []string) error {
	manifest, err := readBackupManifest(w.manifestFile(runID))
	if err != nil {
		return err
	}
	dataBackupDir := w.backupDataDir(runID)
	manifestPaths := make(map[string]bool, len(manifest.Files))
	for _, file := range manifest.Files {
		manifestPaths[file.Path] = true
	}
	for _, file := range manifest.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		source := filepath.Join(dataBackupDir, filepath.FromSlash(file.Path))
		target := filepath.Join(dataDir, filepath.FromSlash(file.Path))
		if err := copyFileAtomic(source, target); err != nil {
			return fmt.Errorf("failed to restore %s: %w", file.Path, err)
		}
	}
	extraFiles := make([]string, 0)
	extraDirs := make([]string, 0)
	err = filepath.WalkDir(dataDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		reparsePoint, err := isReparsePoint(path, info)
		if err != nil {
			return err
		}
		if reparsePoint {
			return fmt.Errorf("data directory contains a reparse point: %s", path)
		}
		relative, err := filepath.Rel(dataDir, path)
		if err != nil {
			return err
		}
		slashPath := filepath.ToSlash(relative)
		if entry.IsDir() {
			if scopeContainsDir(scope, slashPath) {
				extraDirs = append(extraDirs, slashPath)
			}
			return nil
		}
		if scopeMatches(scope, slashPath) && !manifestPaths[slashPath] {
			extraFiles = append(extraFiles, slashPath)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk data directory for restore: %w", err)
	}
	for _, slashPath := range extraFiles {
		if err := os.Remove(filepath.Join(dataDir, filepath.FromSlash(slashPath))); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove extra file %s: %w", slashPath, err)
		}
	}
	sort.Slice(extraDirs, func(i int, j int) bool {
		return strings.Count(extraDirs[i], "/") > strings.Count(extraDirs[j], "/")
	})
	for _, slashPath := range extraDirs {
		if slashPath == "." {
			continue
		}
		target := filepath.Join(dataDir, filepath.FromSlash(slashPath))
		entries, err := os.ReadDir(target)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("failed to read extra directory %s: %w", slashPath, err)
		}
		if len(entries) > 0 {
			continue
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove extra directory %s: %w", slashPath, err)
		}
	}
	for _, file := range manifest.Files {
		digest, err := fileSHA256(filepath.Join(dataDir, filepath.FromSlash(file.Path)))
		if err != nil {
			return fmt.Errorf("failed to verify restored file %s: %w", file.Path, err)
		}
		if digest != file.SHA256 {
			return fmt.Errorf("restored file %s does not match backup", file.Path)
		}
	}
	return nil
}

// removeBackupRun 清理本次运行的恢复资料；backup/ 因此为空时一并删除。
func removeBackupRun(w workspace, runID string) error {
	runDir := w.backupRunDir(runID)
	if err := os.RemoveAll(runDir); err != nil {
		return fmt.Errorf("failed to remove backup run %s: %w", runID, err)
	}
	if entries, err := os.ReadDir(w.backupRoot()); err == nil && len(entries) == 0 {
		if removeErr := os.Remove(w.backupRoot()); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("failed to remove empty backup directory: %w", removeErr)
		}
	}
	return nil
}

func writeBackupManifest(path string, manifest backupManifest) error {
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func readBackupManifest(path string) (backupManifest, error) {
	var manifest backupManifest
	payload, err := os.ReadFile(path)
	if err != nil {
		return manifest, fmt.Errorf("failed to read backup manifest: %w", err)
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return manifest, fmt.Errorf("failed to decode backup manifest: %w", err)
	}
	if manifest.SchemaVersion != backupSchemaVersion {
		return manifest, fmt.Errorf("backup manifest schema version %d is not supported", manifest.SchemaVersion)
	}
	for _, file := range manifest.Files {
		clean, err := validateRelativePath(file.Path)
		if err != nil {
			return manifest, fmt.Errorf("backup manifest contains unsafe path: %w", err)
		}
		if clean != file.Path {
			return manifest, fmt.Errorf("backup manifest path is not normalized: %s", file.Path)
		}
		if len(file.SHA256) != 64 {
			return manifest, fmt.Errorf("backup manifest contains invalid sha256 for %s", file.Path)
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return manifest, fmt.Errorf("backup manifest contains invalid sha256 for %s: %w", file.Path, err)
		}
		if file.SizeBytes < 0 {
			return manifest, fmt.Errorf("backup manifest contains invalid size for %s", file.Path)
		}
	}
	return manifest, nil
}

// copyFileWithHash 复制文件并按字节流计算 SHA-256，返回十六进制摘要与字节数。
func copyFileWithHash(source string, target string) (string, int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", 0, err
	}
	input, err := os.Open(source)
	if err != nil {
		return "", 0, err
	}
	defer input.Close()
	tmp, err := os.CreateTemp(filepath.Dir(target), ".tmp-*")
	if err != nil {
		return "", 0, err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	digest := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, digest), input)
	if err != nil {
		_ = tmp.Close()
		return "", 0, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}
	if err := os.Rename(tmpName, target); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(digest.Sum(nil)), size, nil
}

// copyFileAtomic 以临时文件加原子替换的方式复制文件到目标位置。
func copyFileAtomic(source string, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	tmp, err := os.CreateTemp(filepath.Dir(target), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := io.Copy(tmp, input); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

// fileSHA256 返回文件的 SHA-256 十六进制摘要。
func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
