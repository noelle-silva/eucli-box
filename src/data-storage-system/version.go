package datastorage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"eucli-box/pkg/release"
)

// StorageVersion 是数据存储系统的版本事实，保存在 <rootDir>/meta/version.json。
// 版本文件的写入职责属于数据迁移系统，存储系统只负责读取与校验。
type StorageVersion struct {
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// StorageVersionPath 返回版本事实文件的绝对路径。
func StorageVersionPath(rootDir string) string {
	return filepath.Join(rootDir, "meta", "version.json")
}

// ReadStorageVersion 读取数据版本事实。
// 第二个返回值表示版本文件是否存在：文件不存在返回零值与 false。
// 读取、解码或版本字段校验失败返回 storageReadFailed。
func ReadStorageVersion(ctx context.Context, rootDir string) (StorageVersion, bool, error) {
	if err := ctx.Err(); err != nil {
		return StorageVersion{}, false, storageReadFailed("read cancelled", err)
	}
	path := StorageVersionPath(rootDir)
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return StorageVersion{}, false, nil
		}
		return StorageVersion{}, false, storageReadFailed("failed to read storage version file", err)
	}
	var version StorageVersion
	if err := json.Unmarshal(payload, &version); err != nil {
		return StorageVersion{}, false, storageReadFailed("failed to decode storage version file", err)
	}
	if err := release.ValidateVersion(version.Version); err != nil {
		return StorageVersion{}, false, storageReadFailed("storage version is invalid", fmt.Errorf("%w", err))
	}
	return version, true, nil
}

// WriteStorageVersion 以原子替换方式写入数据版本事实。
func WriteStorageVersion(ctx context.Context, rootDir string, version StorageVersion) error {
	if err := release.ValidateVersion(version.Version); err != nil {
		return storageWriteFailed("refusing to write invalid storage version", err)
	}
	if err := writeJSON(ctx, StorageVersionPath(rootDir), version); err != nil {
		return err
	}
	return nil
}
