package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
)

// ArtifactPackageSource 是已经核对过的官方候选的可下载事实；
// 转换只能由 releasecheck 一侧从候选完成。它只携带压缩包事实，
// 不携带任何 Release 附属清单信息。
type ArtifactPackageSource struct {
	Artifact   types.ReleaseArtifactIdentity
	Product    types.ReleaseProductRecord
	ArchiveURL string
	SizeBytes  int64
	SHA256     string
	// Development 标记该来源来自显式开发来源（本地成品），
	// 校验时放行仅供验证的开发标记，但其余核对保持完整。
	Development bool
}

// AcquirePackageOptions 固定一次用户动作的取包范围。
type AcquirePackageOptions struct {
	Source       ArtifactPackageSource
	DownloadDir  string
	ExtractedDir string
	Client       HTTPDoer
	OnProgress   func(DownloadProgress)
}

// AcquireAndValidatePackage 统一完成压缩包下载、大小与 SHA-256 核对、安全解包和包内核对。
// 它不判断适用范围、不判断活动、不切换当前版本、不写当前版本记录。
func AcquireAndValidatePackage(ctx context.Context, options AcquirePackageOptions) (ValidatedPackage, error) {
	if ctx == nil {
		return ValidatedPackage{}, fmt.Errorf("下载上下文不能为空")
	}
	if err := ValidateArtifactPackageSource(options.Source); err != nil {
		return ValidatedPackage{}, err
	}
	if strings.TrimSpace(options.DownloadDir) == "" {
		return ValidatedPackage{}, fmt.Errorf("下载目录不能为空")
	}
	if strings.TrimSpace(options.ExtractedDir) == "" {
		return ValidatedPackage{}, fmt.Errorf("解包目录不能为空")
	}
	if options.Client == nil {
		return ValidatedPackage{}, fmt.Errorf("下载客户端不能为空")
	}

	archiveTarget := filepath.Join(options.DownloadDir, ArchiveFileName(options.Source.Artifact, options.Source.Product.Version))
	if options.Source.Development {
		if err := CopyPlainFile(ctx, options.Source.ArchiveURL, archiveTarget); err != nil {
			return ValidatedPackage{}, fmt.Errorf("复制开发压缩包失败：%w", err)
		}
	} else if _, err := DownloadFile(ctx, DownloadFileOptions{
		Client:         options.Client,
		URL:            options.Source.ArchiveURL,
		TargetPath:     archiveTarget,
		ExpectedName:   ArchiveFileName(options.Source.Artifact, options.Source.Product.Version),
		ExpectedSize:   options.Source.SizeBytes,
		ExpectedSHA256: options.Source.SHA256,
		OnProgress:     options.OnProgress,
	}); err != nil {
		return ValidatedPackage{}, fmt.Errorf("下载压缩包失败：%w", err)
	}

	if err := EnsureEmptyDirectory(options.ExtractedDir); err != nil {
		return ValidatedPackage{}, err
	}
	if err := ExtractArchive(ExtractArchiveOptions{ArchivePath: archiveTarget, TargetDir: options.ExtractedDir}); err != nil {
		return ValidatedPackage{}, fmt.Errorf("解开压缩包失败：%w", err)
	}
	validated, err := ValidateExtractedPackage(ValidateExtractedPackageOptions{Directory: options.ExtractedDir, Product: options.Source.Product})
	if err != nil {
		return ValidatedPackage{}, fmt.Errorf("包内核对失败：%w", err)
	}
	return validated, nil
}

// ValidateArtifactPackageSource 校验官方候选的取包事实。
func ValidateArtifactPackageSource(source ArtifactPackageSource) error {
	if err := validateIdentity(source.Artifact); err != nil {
		return err
	}
	if source.Product.Artifact != source.Artifact {
		return fmt.Errorf("包内身份与候选身份不一致")
	}
	if err := ValidateReleaseProductRecord(source.Product); err != nil {
		return err
	}
	if source.Product.VerificationOnly || !source.Product.Source.Recorded {
		if !source.Development {
			return fmt.Errorf("候选使用了仅供验证或未记录来源的成品")
		}
	}
	if strings.TrimSpace(source.ArchiveURL) == "" {
		return fmt.Errorf("候选压缩包下载地址不能为空")
	}
	if source.SizeBytes <= 0 {
		return fmt.Errorf("候选压缩包大小无效")
	}
	if len(source.SHA256) != sha256.Size*2 {
		return fmt.Errorf("候选压缩包缺少有效 SHA-256")
	}
	if _, err := hex.DecodeString(source.SHA256); err != nil {
		return fmt.Errorf("候选压缩包 SHA-256 无效")
	}
	return nil
}

// ArchiveFileName 返回发布物在指定正式版本的压缩包文件名。
func ArchiveFileName(identity types.ReleaseArtifactIdentity, version string) string {
	name := identity.Kind
	if identity.Kind != types.ReleaseArtifactKindBox {
		name += "-" + identity.ID
	}
	return fmt.Sprintf("%s_%s_%s.zip", name, version, types.ReleasePlatformWindowsX64)
}

// CopyPlainFile 把本地开发成品压包复制到目标路径，并核对大小与 SHA-256。
func CopyPlainFile(ctx context.Context, sourcePath string, targetPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
