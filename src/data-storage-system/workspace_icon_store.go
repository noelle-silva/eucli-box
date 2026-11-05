package datastorage

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"

	"eucli-box/pkg/types"
	"eucli-box/pkg/workspaceicon"
)

// 工作区图标目录内的固定命名：一条存储事实（icon.json）+ 至多一份图像文件（image.<扩展名>）。
// 两者基名不同，事实文件不会被「清理旧图像」的动作带走。
const (
	workspaceIconFactFile      = "icon.json"
	workspaceIconImageBaseName = "image"
)

// SaveWorkspaceIcon 写入工作区身份图标。
// 图标是独立资源：工作区配置文件不参与图标，客户端全量保存工作区也不会碰到它。
func (s *system) SaveWorkspaceIcon(ctx context.Context, workspaceID string, content types.WorkspaceIconContent) error {
	if err := ctx.Err(); err != nil {
		return storageWriteFailed("write cancelled", err)
	}
	if err := s.requireWorkspaceExists(workspaceID); err != nil {
		return err
	}
	normalized, err := workspaceicon.NormalizeContent(content)
	if err != nil {
		return storageInvalid(err.Error(), err)
	}
	iconDir, err := s.paths.workspaceIconDir(workspaceID)
	if err != nil {
		return err
	}
	if err := ensureDirs(iconDir); err != nil {
		return storageWriteFailed("failed to create workspace icon directory", err)
	}
	// NormalizeContent 只放行 image 与 svg 两种载体，这里只按载体分流。
	if normalized.Kind == types.WorkspaceIconKindSVG {
		return s.replaceWorkspaceIcon(ctx, iconDir, types.WorkspaceIcon{Kind: types.WorkspaceIconKindSVG, SVG: normalized.SVG})
	}
	return s.saveWorkspaceIconImage(ctx, iconDir, normalized.DataURL)
}

// saveWorkspaceIconImage 落图像载体：先写图像文件，再落图像路径事实。
func (s *system) saveWorkspaceIconImage(ctx context.Context, iconDir string, dataURL string) error {
	image, err := decodeImageDataURL(dataURL, entityIconImageFormats)
	if err != nil {
		return storageInvalid("workspace icon image must be a png, jpg, or webp data url", err)
	}
	if err := writeEntityIconFile(ctx, iconDir, workspaceIconImageBaseName, image); err != nil {
		return storageWriteFailed("failed to write workspace icon image", err)
	}
	return s.replaceWorkspaceIcon(ctx, iconDir, types.WorkspaceIcon{
		Kind: types.WorkspaceIconKindImage,
		Path: workspaceIconRelativeDir + "/" + workspaceIconImageBaseName + "." + image.Ext,
	})
}

// LoadWorkspaceIcon 读取工作区图标并装配成可渲染形态：图像读成 data URL，SVG 原样返回源码。
// 没有图标时以 not_found 表达——「没有图标」是正常工作区的状态，不是错误兜底。
func (s *system) LoadWorkspaceIcon(ctx context.Context, workspaceID string) (types.WorkspaceIconContent, error) {
	if err := ctx.Err(); err != nil {
		return types.WorkspaceIconContent{}, storageReadFailed("read cancelled", err)
	}
	if err := s.requireWorkspaceExists(workspaceID); err != nil {
		return types.WorkspaceIconContent{}, err
	}
	iconDir, err := s.paths.workspaceIconDir(workspaceID)
	if err != nil {
		return types.WorkspaceIconContent{}, err
	}
	factFile := filepath.Join(iconDir, workspaceIconFactFile)
	if !dataFileExists(factFile) {
		return types.WorkspaceIconContent{}, storageNotFound("workspace icon does not exist", nil)
	}
	fact, err := readJSON[types.WorkspaceIcon](ctx, factFile)
	if err != nil {
		return types.WorkspaceIconContent{}, err
	}
	fact, err = workspaceicon.NormalizeStored(fact)
	if err != nil {
		return types.WorkspaceIconContent{}, storageReadFailed("workspace icon fact is invalid", err)
	}
	if fact.Kind == types.WorkspaceIconKindSVG {
		return types.WorkspaceIconContent{Kind: types.WorkspaceIconKindSVG, SVG: fact.SVG}, nil
	}
	target, err := s.workspaceIconImageTarget(workspaceID, fact.Path)
	if err != nil {
		return types.WorkspaceIconContent{}, err
	}
	mime, ok := entityIconImageMIMEForExtension(filepath.Ext(target))
	if !ok {
		return types.WorkspaceIconContent{}, storageReadFailed("workspace icon image format is not supported", nil)
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		return types.WorkspaceIconContent{}, storageReadFailed("failed to read workspace icon image", err)
	}
	return types.WorkspaceIconContent{
		Kind:    types.WorkspaceIconKindImage,
		DataURL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(payload),
	}, nil
}

// DeleteWorkspaceIcon 清掉工作区图标：图标目录整体随图标事实一起消失，重复删除不再报错。
func (s *system) DeleteWorkspaceIcon(ctx context.Context, workspaceID string) error {
	if err := ctx.Err(); err != nil {
		return storageDeleteFailed("delete cancelled", err)
	}
	if err := s.requireWorkspaceExists(workspaceID); err != nil {
		return err
	}
	iconDir, err := s.paths.workspaceIconDir(workspaceID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(iconDir); err != nil {
		return storageDeleteFailed("failed to delete workspace icon", err)
	}
	return nil
}

// replaceWorkspaceIcon 落一条图标事实，并清掉不属于该载体的旧文件。
// 事实先落盘：万一清理失败，图标语义仍与事实一致，残留文件只是孤立字节。
func (s *system) replaceWorkspaceIcon(ctx context.Context, iconDir string, icon types.WorkspaceIcon) error {
	if err := writeJSON(ctx, filepath.Join(iconDir, workspaceIconFactFile), icon); err != nil {
		return err
	}
	if icon.Kind != types.WorkspaceIconKindImage {
		if err := removeEntityIconFiles(iconDir, workspaceIconImageBaseName); err != nil {
			return storageWriteFailed("failed to remove stale workspace icon image", err)
		}
	}
	return nil
}

// workspaceIconImageTarget 把存储事实里的相对路径解析成图标目录内的真实路径：
// 事实里的路径只允许落在图标目录内，越界即失败。
func (s *system) workspaceIconImageTarget(workspaceID string, relativePath string) (string, error) {
	iconDir, err := s.paths.workspaceIconDir(workspaceID)
	if err != nil {
		return "", err
	}
	workspaceDir, err := s.paths.workspaceDir(workspaceID)
	if err != nil {
		return "", err
	}
	target := filepath.Join(workspaceDir, filepath.FromSlash(relativePath))
	if !isWithin(iconDir, target) {
		return "", storageReadFailed("workspace icon path escapes the icon directory", nil)
	}
	return target, nil
}

func (s *system) requireWorkspaceExists(workspaceID string) error {
	target, err := s.paths.workspaceDataFile(workspaceID)
	if err != nil {
		return err
	}
	if !dataFileExists(target) {
		return storageNotFound("workspace does not exist", nil)
	}
	return nil
}
