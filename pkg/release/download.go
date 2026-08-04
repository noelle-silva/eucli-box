package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
)

var (
	ErrDownloadSizeMismatch   = errors.New("下载文件大小不一致")
	ErrDownloadDigestMismatch = errors.New("下载文件摘要不一致")
)

type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

type DownloadFileOptions struct {
	Client         HTTPDoer
	URL            string
	TargetPath     string
	ExpectedName   string
	ExpectedSize   int64
	ExpectedSHA256 string
	MaxBytes       int64
	Headers        http.Header
}

func DownloadFile(ctx context.Context, options DownloadFileOptions) (types.ReleaseFileRecord, error) {
	return downloadFile(ctx, options, true)
}

// DownloadFileUnchecked is reserved for remote assets whose response does
// not carry a trusted digest. The caller must verify the returned record.
func DownloadFileUnchecked(ctx context.Context, options DownloadFileOptions) (types.ReleaseFileRecord, error) {
	return downloadFile(ctx, options, false)
}

func downloadFile(ctx context.Context, options DownloadFileOptions, requireDigest bool) (types.ReleaseFileRecord, error) {
	if ctx == nil {
		return types.ReleaseFileRecord{}, fmt.Errorf("下载上下文不能为空")
	}
	if strings.TrimSpace(options.URL) == "" {
		return types.ReleaseFileRecord{}, fmt.Errorf("下载地址不能为空")
	}
	if strings.TrimSpace(options.TargetPath) == "" {
		return types.ReleaseFileRecord{}, fmt.Errorf("下载目标不能为空")
	}
	if strings.TrimSpace(options.ExpectedName) == "" {
		return types.ReleaseFileRecord{}, fmt.Errorf("下载文件名不能为空")
	}
	if options.ExpectedSize < 0 {
		return types.ReleaseFileRecord{}, fmt.Errorf("下载文件大小不能为负数")
	}
	if requireDigest && strings.TrimSpace(options.ExpectedSHA256) == "" {
		return types.ReleaseFileRecord{}, fmt.Errorf("下载文件摘要不能为空")
	}
	if strings.TrimSpace(options.ExpectedSHA256) != "" && !validSHA256(options.ExpectedSHA256) {
		return types.ReleaseFileRecord{}, fmt.Errorf("下载文件摘要无效")
	}

	target, err := filepath.Abs(options.TargetPath)
	if err != nil {
		return types.ReleaseFileRecord{}, fmt.Errorf("下载目标无效：%w", err)
	}
	if filepath.Base(target) != options.ExpectedName {
		return types.ReleaseFileRecord{}, fmt.Errorf("下载目标文件名必须为 %s", options.ExpectedName)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return types.ReleaseFileRecord{}, fmt.Errorf("建立下载目录失败：%w", err)
	}

	client := options.Client
	if client == nil {
		client = &http.Client{}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(options.URL), nil)
	if err != nil {
		return types.ReleaseFileRecord{}, fmt.Errorf("建立下载请求失败：%w", err)
	}
	for key, values := range options.Headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		return types.ReleaseFileRecord{}, fmt.Errorf("下载文件失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return types.ReleaseFileRecord{}, fmt.Errorf("下载文件失败：远端返回 %d", response.StatusCode)
	}

	maxBytes := options.MaxBytes
	if maxBytes <= 0 {
		maxBytes = options.ExpectedSize + 1
	}
	if maxBytes < options.ExpectedSize {
		return types.ReleaseFileRecord{}, fmt.Errorf("下载大小上限小于期望大小")
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".download-*.tmp")
	if err != nil {
		return types.ReleaseFileRecord{}, fmt.Errorf("创建下载临时文件失败：%w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		cleanup()
		return types.ReleaseFileRecord{}, fmt.Errorf("写入下载文件失败：%w", err)
	}
	if written > maxBytes || written != options.ExpectedSize {
		cleanup()
		return types.ReleaseFileRecord{}, ErrDownloadSizeMismatch
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return types.ReleaseFileRecord{}, fmt.Errorf("刷新下载文件失败：%w", err)
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return types.ReleaseFileRecord{}, fmt.Errorf("关闭下载文件失败：%w", err)
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if strings.TrimSpace(options.ExpectedSHA256) != "" && !strings.EqualFold(digest, options.ExpectedSHA256) {
		cleanup()
		return types.ReleaseFileRecord{}, ErrDownloadDigestMismatch
	}
	if err := ReplaceFileAtomic(temporaryPath, target); err != nil {
		cleanup()
		return types.ReleaseFileRecord{}, fmt.Errorf("启用下载文件失败：%w", err)
	}
	return types.ReleaseFileRecord{Name: options.ExpectedName, Size: written, SHA256: digest}, nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}
