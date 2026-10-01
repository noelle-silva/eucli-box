package imagereader

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// imageKind 是一种受支持的图片格式：MIME 与文件扩展名。
type imageKind struct {
	Mime string
	Ext  string
}

var (
	pngKind  = imageKind{Mime: "image/png", Ext: "png"}
	jpegKind = imageKind{Mime: "image/jpeg", Ext: "jpg"}
	webpKind = imageKind{Mime: "image/webp", Ext: "webp"}
	gifKind  = imageKind{Mime: "image/gif", Ext: "gif"}
)

// detectImageKind 按魔数识别图片格式；非受支持格式返回 ok=false。
// 只认内容不认扩展名：伪装成图片的非图片文件在本地被拒绝。
func detectImageKind(payload []byte) (imageKind, bool) {
	switch {
	case len(payload) >= 8 && bytes.HasPrefix(payload, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return pngKind, true
	case len(payload) >= 3 && payload[0] == 0xff && payload[1] == 0xd8 && payload[2] == 0xff:
		return jpegKind, true
	case len(payload) >= 12 && string(payload[0:4]) == "RIFF" && string(payload[8:12]) == "WEBP":
		return webpKind, true
	case bytes.HasPrefix(payload, []byte("GIF87a")) || bytes.HasPrefix(payload, []byte("GIF89a")):
		return gifKind, true
	default:
		return imageKind{}, false
	}
}

// decodeImage 解码受支持格式的图片：GIF 只取首帧，WebP 由官方扩展库解码。
func decodeImage(payload []byte) (image.Image, error) {
	switch kind, ok := detectImageKind(payload); {
	case !ok:
		return nil, fmt.Errorf("unsupported image format")
	case kind == pngKind:
		decoded, err := png.Decode(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("decode png: %w", err)
		}
		return decoded, nil
	case kind == jpegKind:
		decoded, err := jpeg.Decode(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("decode jpeg: %w", err)
		}
		return decoded, nil
	case kind == gifKind:
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

// compressImage 把图片压缩为 JPEG：等比缩到长边不超过 maxDimension，
// 透明像素合成白底，质量由 jpegQuality 决定。图片本身够小也统一走此路，
// 由调用方决定何时触发。
func compressImage(payload []byte, config Config) ([]byte, error) {
	source, err := decodeImage(payload)
	if err != nil {
		return nil, err
	}
	target := downscale(source, config.MaxDimension)
	flattened := flattenOnWhite(target)
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, flattened, &jpeg.Options{Quality: config.JPEGQuality}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return buffer.Bytes(), nil
}

// downscale 等比缩放到长边不超过 maxDimension；已经够小则原样返回。
func downscale(source image.Image, maxDimension int) image.Image {
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

// imageDimensions 读取图片像素尺寸，供结果事实使用。
func imageDimensions(payload []byte) (int, int, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(payload))
	if err != nil {
		return 0, 0, err
	}
	return config.Width, config.Height, nil
}

// dataURL 把图片字节编码为标准 data URL。
func dataURL(kind imageKind, payload []byte) string {
	return "data:" + kind.Mime + ";base64," + base64.StdEncoding.EncodeToString(payload)
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
