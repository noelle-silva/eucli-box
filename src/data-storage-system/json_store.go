package datastorage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func ensureDirs(paths ...string) error {
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// prepareJSON 把一份文档完整编码并写入同目录的临时文件（含 fsync），
// 返回临时文件路径；落名动作留给 commitJSON。
// 这样调用方可以把编解码、目录创建、fsync 放在临界区之外，
// 临界区内只保留一次 rename。
func prepareJSON(ctx context.Context, target string, value any) (string, error) {
	return prepareJSONDocument(ctx, target, value, true)
}

// prepareJSONDerived 用于派生缓存（索引）：不 fsync。
// 索引的权威事实永远在 data.json，索引丢失或损坏只会被判为无效并重建，
// 因此没有必要为它支付落盘同步成本；dirty 位由权威写路径负责同步。
func prepareJSONDerived(ctx context.Context, target string, value any) (string, error) {
	return prepareJSONDocument(ctx, target, value, false)
}

func prepareJSONDocument(ctx context.Context, target string, value any, sync bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", storageWriteFailed("failed to create parent directory", err)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", storageWriteFailed("failed to marshal json", err)
	}
	payload = append(payload, '\n')
	tmp, err := os.CreateTemp(dir, ".tmp-*.json")
	if err != nil {
		return "", storageWriteFailed("failed to create temporary file", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", storageWriteFailed("failed to write temporary file", err)
	}
	if sync {
		if err := tmp.Sync(); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
			return "", storageWriteFailed("failed to sync temporary file", err)
		}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", storageWriteFailed("failed to close temporary file", err)
	}
	return tmpName, nil
}

func commitJSON(temp string, target string) error {
	if err := os.Rename(temp, target); err != nil {
		return storageWriteFailed("failed to replace json file", err)
	}
	return nil
}

func writeJSON(ctx context.Context, target string, value any) error {
	temp, err := prepareJSON(ctx, target, value)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temp) }()
	return commitJSON(temp, target)
}

func readFileBytes(ctx context.Context, target string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, storageReadFailed("read cancelled", err)
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, storageReadFailed("json file does not exist", err)
		}
		return nil, storageReadFailed("failed to read json file", err)
	}
	return payload, nil
}

func decodeJSON[T any](payload []byte) (T, error) {
	var value T
	if err := json.Unmarshal(payload, &value); err != nil {
		return value, storageReadFailed("failed to decode json file", err)
	}
	return value, nil
}

func readJSON[T any](ctx context.Context, target string) (T, error) {
	payload, err := readFileBytes(ctx, target)
	if err != nil {
		var zero T
		return zero, err
	}
	return decodeJSON[T](payload)
}

func dataFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
