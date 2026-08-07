package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
)

const currentProgramSchemaVersion = 1

// currentProgramRecord 只记录当前启用版本和已核对的发布身份，不记录用户配置和密钥。
type currentProgramRecord struct {
	SchemaVersion    int                          `json:"schemaVersion"`
	Artifact         types.ReleaseArtifactIdentity `json:"artifact"`
	Version          string                       `json:"version"`
	Platform         string                       `json:"platform"`
	ProgramDirectory string                       `json:"programDirectory"`
	Status           string                       `json:"status"`
}

// CurrentProgram 表示当前启用版本的可核对事实。
type CurrentProgram struct {
	Version          string
	ProgramDirectory string
}

// PreparedProgram 表示一次已经完整准备但尚未启用的新版本。
type PreparedProgram struct {
	Version   string
	Directory string
	Files     []types.ReleaseFileRecord
}

// ProgramStore 拥有单个发布物的版本目录和当前版本记录；
// 不下载、不访问官方来源、不判断适用范围、不判断活动。
type ProgramStore struct {
	root     string
	identity types.ReleaseArtifactIdentity
}

// NewProgramStore 构造当前发布物自己的程序根目录。
func NewProgramStore(root string, identity types.ReleaseArtifactIdentity) (ProgramStore, error) {
	if err := validateIdentity(identity); err != nil {
		return ProgramStore{}, err
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return ProgramStore{}, err
	}
	root = filepath.Clean(root)
	if filepath.Base(root) != identity.ID {
		return ProgramStore{}, fmt.Errorf("程序根目录必须属于发布物 %s", identity.ID)
	}
	return ProgramStore{root: root, identity: identity}, nil
}

// PrepareVersion 把完整且已验证的目录复制到 versions/<version> 的新临时目录，再原子改名为正式版本目录。
// 已存在同版本且内容资料一致时可以复用；内容不一致时直接失败，不覆盖。
func (s ProgramStore) PrepareVersion(ctx context.Context, sourceDir string, product types.ReleaseProductRecord, files []types.ReleaseFileRecord) (PreparedProgram, error) {
	if product.Artifact != s.identity {
		return PreparedProgram{}, fmt.Errorf("产品记录身份与程序根目录身份不一致")
	}
	if err := ValidateReleaseProductRecord(product); err != nil {
		return PreparedProgram{}, err
	}
	if err := ValidateVersion(product.Version); err != nil {
		return PreparedProgram{}, fmt.Errorf("产品版本无效：%w", err)
	}
	prepared := PreparedProgram{Version: product.Version, Files: append([]types.ReleaseFileRecord(nil), files...)}
	prepared.Directory = s.versionDirectory(product.Version)
	if existing, err := s.verifiedVersionDirectory(prepared.Directory, prepared.Files); err == nil {
		prepared.Directory = existing
		return prepared, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return PreparedProgram{}, fmt.Errorf("已存在版本目录资料不一致：%w", err)
	}
	if _, err := existingDirectory(sourceDir); err != nil {
		return PreparedProgram{}, err
	}
	if err := EnsurePlainDirectory(sourceDir); err != nil {
		return PreparedProgram{}, err
	}
	versionsRoot := filepath.Join(s.root, "versions")
	if err := os.MkdirAll(versionsRoot, 0o755); err != nil {
		return PreparedProgram{}, fmt.Errorf("建立版本目录失败：%w", err)
	}
	if err := EnsurePlainDirectory(versionsRoot); err != nil {
		return PreparedProgram{}, err
	}
	temporary, err := os.MkdirTemp(versionsRoot, ".prepare-"+product.Version+"-*")
	if err != nil {
		return PreparedProgram{}, fmt.Errorf("建立版本准备临时目录失败：%w", err)
	}
	temporaryCleanup := func() { _ = os.RemoveAll(temporary) }
	if err := copyPlainTree(sourceDir, temporary); err != nil {
		temporaryCleanup()
		return PreparedProgram{}, err
	}
	if _, err := CompareFileRecords(temporary, prepared.Files); err != nil {
		temporaryCleanup()
		return PreparedProgram{}, fmt.Errorf("准备版本内容核对失败：%w", err)
	}
	if err := os.Rename(temporary, prepared.Directory); err != nil {
		temporaryCleanup()
		return PreparedProgram{}, fmt.Errorf("启用版本目录失败：%w", err)
	}
	return prepared, nil
}

// Activate 只写入一个完整的 current.json 一次性切换当前版本，不逐文件改写当前版本目录。
// previousVersion 非空时必须与当前记录一致，防止并发切换。
func (s ProgramStore) Activate(ctx context.Context, prepared PreparedProgram, previousVersion string) error {
	directory, err := s.verifiedVersionDirectory(prepared.Directory, prepared.Files)
	if err != nil {
		return fmt.Errorf("待启用版本不可用：%w", err)
	}
	if previousVersion != "" {
		current, err := s.Current()
		if err == nil {
			if current.Version != previousVersion {
				return fmt.Errorf("当前版本已改变，拒绝并发切换")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	record := currentProgramRecord{
		SchemaVersion:    currentProgramSchemaVersion,
		Artifact:         s.identity,
		Version:          prepared.Version,
		Platform:         types.ReleasePlatformWindowsX64,
		ProgramDirectory: directory,
		Status:           "active",
	}
	if err := writeJSONAtomic(s.currentFile(), record); err != nil {
		return fmt.Errorf("写入当前版本记录失败：%w", err)
	}
	if _, err := s.Current(); err != nil {
		return fmt.Errorf("当前版本记录核对失败：%w", err)
	}
	return nil
}

// Restore 只恢复已有且完整核对的版本；不存在或资料不一致时返回错误。
func (s ProgramStore) Restore(ctx context.Context, version string) error {
	if err := ValidateVersion(version); err != nil {
		return err
	}
	directory := s.versionDirectory(version)
	if err := verifyCompleteVersionDirectory(directory, s.identity); err != nil {
		return err
	}
	record := currentProgramRecord{
		SchemaVersion:    currentProgramSchemaVersion,
		Artifact:         s.identity,
		Version:          version,
		Platform:         types.ReleasePlatformWindowsX64,
		ProgramDirectory: directory,
		Status:           "active",
	}
	if err := writeJSONAtomic(s.currentFile(), record); err != nil {
		return fmt.Errorf("写入当前版本记录失败：%w", err)
	}
	if _, err := s.Current(); err != nil {
		return fmt.Errorf("当前版本记录核对失败：%w", err)
	}
	return nil
}

// Current 读取并核对当前版本记录；当前版本未知时返回 os.ErrNotExist 的包装错误。
func (s ProgramStore) Current() (CurrentProgram, error) {
	var record currentProgramRecord
	if err := readJSONAtomic(s.currentFile(), &record); err != nil {
		return CurrentProgram{}, err
	}
	if record.SchemaVersion != currentProgramSchemaVersion {
		return CurrentProgram{}, fmt.Errorf("当前版本记录 schemaVersion 无效")
	}
	if record.Artifact != s.identity {
		return CurrentProgram{}, fmt.Errorf("当前版本记录身份与程序根目录不一致")
	}
	if err := ValidateVersion(record.Version); err != nil {
		return CurrentProgram{}, fmt.Errorf("当前版本记录版本无效")
	}
	if record.Platform != types.ReleasePlatformWindowsX64 {
		return CurrentProgram{}, fmt.Errorf("当前版本记录平台无效")
	}
	if strings.TrimSpace(record.Status) != "active" {
		return CurrentProgram{}, fmt.Errorf("当前版本记录状态无效")
	}
	directory, err := filepath.Abs(record.ProgramDirectory)
	if err != nil {
		return CurrentProgram{}, fmt.Errorf("当前版本程序目录无效：%w", err)
	}
	expected := s.versionDirectory(record.Version)
	if filepath.Clean(directory) != filepath.Clean(expected) {
		return CurrentProgram{}, fmt.Errorf("当前版本程序目录与版本记录不一致")
	}
	if err := verifyCompleteVersionDirectory(directory, s.identity); err != nil {
		return CurrentProgram{}, err
	}
	return CurrentProgram{Version: record.Version, ProgramDirectory: directory}, nil
}

func (s ProgramStore) currentFile() string {
	return filepath.Join(s.root, "current.json")
}

func (s ProgramStore) versionDirectory(version string) string {
	return filepath.Join(s.root, "versions", version)
}

// verifiedVersionDirectory 核对目录与文件记录逐文件一致；目录不存在返回 os.ErrNotExist 包装错误。
func (s ProgramStore) verifiedVersionDirectory(directory string, files []types.ReleaseFileRecord) (string, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(directory); err != nil {
		return "", err
	}
	if _, err := CompareFileRecords(directory, files); err != nil {
		return "", err
	}
	return directory, nil
}

// verifyCompleteVersionDirectory 核对版本目录存在、非空且身份资料可读。
func verifyCompleteVersionDirectory(directory string, identity types.ReleaseArtifactIdentity) error {
	directory, err := existingDirectory(directory)
	if err != nil {
		return err
	}
	if err := EnsurePlainDirectory(directory); err != nil {
		return err
	}
	files, err := CollectFileRecords(directory)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("版本目录为空")
	}
	payload, err := os.ReadFile(filepath.Join(directory, "release-product.json"))
	if err != nil {
		return fmt.Errorf("版本目录缺少成品身份资料：%w", err)
	}
	product, err := DecodeReleaseProductRecord(payload)
	if err != nil {
		return err
	}
	if product.Artifact != identity {
		return fmt.Errorf("版本目录成品身份与程序根目录不一致")
	}
	return nil
}

// copyPlainTree 复制目录树，拒绝符号链接和重解析点。
func copyPlainTree(source string, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("程序目录不能包含符号链接：%s", path)
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return err
			}
			reparse, err := isReparsePoint(path)
			if err != nil {
				return err
			}
			if reparse {
				return fmt.Errorf("程序目录不能包含重解析点：%s", path)
			}
			return nil
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputCloseErr := input.Close()
		outputCloseErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		if outputCloseErr != nil {
			return outputCloseErr
		}
		return nil
	})
}
