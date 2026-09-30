package aiimage

import (
	"bytes"
	"fmt"
	stdimage "image"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// referenceImageMinPixels 是参考图最短边的下限：更小的图片视为占位图，
// 上游普遍不接受，本地直接拒绝，避免把参数问题误导成上游服务故障。
const referenceImageMinPixels = 64

// validateReferenceImage 校验参考图能被解码且尺寸可用；
// 不合法时返回不可重试的参数类错误，调用方据此在本地拒绝、不发上游。
func validateReferenceImage(ref promptImage) error {
	config, _, err := stdimage.DecodeConfig(bytes.NewReader(ref.Bytes))
	if err != nil {
		return permanentError(fmt.Errorf("参考图不是可识别的图片格式（支持 PNG / JPEG / WebP / GIF）: %w", err), actionReplaceImage)
	}
	width, height := config.Width, config.Height
	if width < referenceImageMinPixels || height < referenceImageMinPixels {
		return permanentError(
			fmt.Errorf("参考图尺寸过小（%d×%d，最短边至少 %d 像素）", width, height, referenceImageMinPixels),
			actionReplaceImage,
		)
	}
	return nil
}

// referenceLabel 为参考图生成可读标识：data URL 统一称 data URL，
// 其余截断显示，避免整段 base64 进入错误信息。
func referenceLabel(reference string) string {
	if isDirectImageInput(reference) {
		return "data URL"
	}
	const maxRunes = 60
	runes := []rune(reference)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "…"
	}
	return reference
}
