// Package imagereader 是 image_reader 工具的业务动作实现：
// 从文件系统按路径读取一张图片，判定格式与大小并按需压缩，然后经
// 会话附件写入能力把图片加载进当前会话，供模型查看。
package imagereader

import (
	"context"
	"fmt"
	"os"
	"strings"

	"eucli-box/pkg/types"
)

// Execute 执行一次 image_reader 工具动作。
func Execute(ctx context.Context, input types.ToolExecutionInput, session SessionService) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("tool execution cancelled", err)
	}
	if err := rejectConfigArguments(input); err != nil {
		return failure("parse image_reader request", err)
	}
	args, err := parseArguments(input.Arguments)
	if err != nil {
		return failure("parse image_reader request", err)
	}
	config, err := loadConfig(input)
	if err != nil {
		return failure("load image_reader config", err)
	}
	policy, err := newPathPolicy(input)
	if err != nil {
		return failure("resolve image_reader base directory", err)
	}
	resolved, err := policy.Resolve(args.Path)
	if err != nil {
		return failure("parse image_reader request", err)
	}
	return loadImageFile(ctx, resolved, config, session)
}

// loadImageFile 读取并归一单张图片文件：先按文件大小分级，再判定格式，
// 超限时压缩；压缩后仍超限则原地拒绝。
func loadImageFile(ctx context.Context, resolved ResolvedPath, config Config, session SessionService) types.ToolExecutionOutput {
	info, err := os.Stat(resolved.Absolute)
	if err != nil {
		if os.IsNotExist(err) {
			return failure("读取图片失败", fmt.Errorf("文件不存在：%s", resolved.Display))
		}
		return failure("读取图片失败", err)
	}
	if info.IsDir() {
		return failure("读取图片失败", fmt.Errorf("目标是目录而非文件：%s", resolved.Display))
	}
	if !info.Mode().IsRegular() {
		return failure("读取图片失败", fmt.Errorf("目标不是普通文件：%s", resolved.Display))
	}
	if info.Size() > config.MaxSourceBytes {
		return failure("文件太大，拒绝读取", fmt.Errorf("源文件 %d 字节超过读取上限 %d 字节", info.Size(), config.MaxSourceBytes))
	}

	payload, err := os.ReadFile(resolved.Absolute)
	if err != nil {
		return failure("读取图片失败", err)
	}
	if err := ctx.Err(); err != nil {
		return failure("tool execution cancelled", err)
	}

	kind, ok := detectImageKind(payload)
	if !ok {
		return failure("不是受支持的图片文件", fmt.Errorf("仅支持 PNG / JPEG / WebP / GIF：%s", resolved.Display))
	}
	sourceBytes := int64(len(payload))

	compressed := false
	if sourceBytes > config.MaxImageBytes {
		shrunk, err := compressImage(payload, config)
		if err != nil {
			return failure("压缩图片失败", err)
		}
		if int64(len(shrunk)) > config.MaxImageBytes {
			return failure("文件太大，拒绝读取", fmt.Errorf("压缩后 %d 字节仍超过入库上限 %d 字节", len(shrunk), config.MaxImageBytes))
		}
		payload = shrunk
		kind = jpegKind
		compressed = true
	}

	if err := ctx.Err(); err != nil {
		return failure("tool execution cancelled", err)
	}

	width, height, err := imageDimensions(payload)
	if err != nil {
		return failure("读取图片失败", err)
	}

	name := imageName(resolved.Display)
	info_, err := writeSessionImage(ctx, session, name, dataURL(kind, payload))
	if err != nil {
		return failure("把图片加载进会话失败", err)
	}

	content := fmt.Sprintf("已把图片 %s（%d×%d，%s）加载进当前会话。", name, width, height, sizeLabel(compressed))
	return success(content, map[string]any{
		"path":         resolved.Display,
		"name":         name,
		"attachmentId": info_.ID,
		"width":        width,
		"height":       height,
		"mime":         kind.Mime,
		"sourceBytes":  sourceBytes,
		"loadedBytes":  int64(len(payload)),
		"compressed":   compressed,
	})
}

// imageName 从展示路径取文件名作为图片名；取不到时回退「图片」。
func imageName(display string) string {
	base := display
	if index := strings.LastIndexAny(display, "/\\"); index >= 0 {
		base = display[index+1:]
	}
	if strings.TrimSpace(base) == "" {
		return "图片"
	}
	return base
}

// sizeLabel 说明本次入库是否经过压缩。
func sizeLabel(compressed bool) string {
	if compressed {
		return "已压缩"
	}
	return "原图"
}
