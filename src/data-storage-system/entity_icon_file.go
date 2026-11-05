package datastorage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// entityIconImageFormats 是实体图标允许的图片格式：MIME 是键、落盘扩展名是值。
// 角色头像、群组头像与工作区图标共用同一份白名单，扩展名反查也由它派生，
// 同一份格式集合不在多处各写一份。
var entityIconImageFormats = map[string]string{
	"image/png":  "png",
	"image/jpeg": "jpg",
	"image/webp": "webp",
}

// entityIconImageMIMEForExtension 用同一份白名单按落盘扩展名反查 MIME。
func entityIconImageMIMEForExtension(ext string) (string, bool) {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	for mime, candidate := range entityIconImageFormats {
		if candidate == ext {
			return mime, true
		}
	}
	return "", false
}

// writeEntityIconFile 以「替换式单文件」语义把一张图片写进实体图标目录：
// 先写临时文件并 fsync，清掉同基名的旧图标，最后落名。
// 目录内同一基名只保留一个扩展名，读取方永远只面对一份图标文件。
func writeEntityIconFile(ctx context.Context, dir string, baseName string, image storedImage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target := filepath.Join(dir, baseName+"."+image.Ext)
	tmp, err := os.CreateTemp(dir, "."+baseName+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(image.Payload); err != nil {
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
	if err := removeEntityIconFiles(dir, baseName); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

// removeEntityIconFiles 清掉目录内同一基名的图标文件（任意扩展名）：
// 扩展名切换后旧文件不留在原地。
func removeEntityIconFiles(dir string, baseName string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	prefix := baseName + "."
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		err := os.Remove(filepath.Join(dir, entry.Name()))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
