package datastorage

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// 小副本规格：长边上限与 JPEG 质量。小副本只用于发往模型的请求体，
// 聊天展示与会话事实始终是原图。
const (
	previewMaxDimension = 1568
	previewJPEGQuality  = 80
)

// buildImagePreview 把任意允许格式的图片解码并生成一张压缩小副本（JPEG）。
// 透明像素合成到白底，缩放用高质量重采样；GIF 只取首帧。
func buildImagePreview(payload []byte) ([]byte, error) {
	source, err := decodePreviewSource(payload)
	if err != nil {
		return nil, err
	}
	target := downscaleImage(source, previewMaxDimension)
	flattened := flattenOnWhite(target)
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, flattened, &jpeg.Options{Quality: previewJPEGQuality}); err != nil {
		return nil, fmt.Errorf("encode preview jpeg: %w", err)
	}
	return buffer.Bytes(), nil
}

// decodePreviewSource 按魔数识别格式并解码：PNG / JPEG / GIF 用标准库，
// WebP 由官方扩展库的解码器注册进 image 包。
func decodePreviewSource(payload []byte) (image.Image, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("image payload is empty")
	}
	switch {
	case bytes.HasPrefix(payload, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		decoded, err := png.Decode(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("decode png: %w", err)
		}
		return decoded, nil
	case len(payload) >= 3 && payload[0] == 0xff && payload[1] == 0xd8 && payload[2] == 0xff:
		decoded, err := jpeg.Decode(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("decode jpeg: %w", err)
		}
		return decoded, nil
	case bytes.HasPrefix(payload, []byte("GIF87a")) || bytes.HasPrefix(payload, []byte("GIF89a")):
		decoded, err := gif.Decode(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("decode gif: %w", err)
		}
		return decoded, nil
	default:
		decoded, _, err := image.Decode(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("decode image: %w", err)
		}
		return decoded, nil
	}
}

// downscaleImage 把图片等比缩到长边不超过 maxDimension；已经够小则原样返回。
func downscaleImage(source image.Image, maxDimension int) image.Image {
	bounds := source.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return source
	}
	if width <= maxDimension && height <= maxDimension {
		return source
	}
	scale := float64(maxDimension) / float64(maxInt(width, height))
	targetWidth := maxInt(1, int(math.Round(float64(width)*scale)))
	targetHeight := maxInt(1, int(math.Round(float64(height)*scale)))
	target := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	xdraw.CatmullRom.Scale(target, target.Bounds(), source, bounds, draw.Src, nil)
	return target
}

// flattenOnWhite 把图片合成到白底：透明像素转 JPEG 后不会变黑。
func flattenOnWhite(source image.Image) image.Image {
	bounds := source.Bounds()
	target := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(target, target.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(target, target.Bounds(), source, bounds.Min, draw.Over)
	return target
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

// writePreviewFile 以小副本固定文件名原子写入附件目录；
// 它不触碰同目录的原图文件。
func writePreviewFile(ctx context.Context, dir string, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target := filepath.Join(dir, sessionPreviewFileName)
	tmp, err := os.CreateTemp(dir, ".preview-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(payload); err != nil {
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
