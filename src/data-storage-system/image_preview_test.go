package datastorage

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"testing"

	"golang.org/x/image/webp"

	"eucli-box/pkg/types"
)

// testPNGDataURL 生成一张指定尺寸的纯色 PNG data URL。
func testPNGDataURL(t *testing.T, width int, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
}

// TestBuildImagePreviewDownscalesAndEncodesJPEG 钉住小副本规格：
// 长边压到上限内、统一 JPEG 编码。
func TestBuildImagePreviewDownscalesAndEncodesJPEG(t *testing.T) {
	source := testPNGDataURL(t, 4000, 2000)
	image, err := decodeImageDataURL(source, sessionAttachmentAllowedImageMIMEs)
	if err != nil {
		t.Fatalf("decodeImageDataURL() error = %v", err)
	}
	preview, err := buildImagePreview(image.Payload)
	if err != nil {
		t.Fatalf("buildImagePreview() error = %v", err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(preview))
	if err != nil {
		t.Fatalf("preview is not a valid jpeg: %v", err)
	}
	if decoded.Bounds().Dx() != previewMaxDimension || decoded.Bounds().Dy() != previewMaxDimension/2 {
		t.Fatalf("preview bounds = %v", decoded.Bounds())
	}
}

// TestBuildImagePreviewKeepsSmallImageSize 钉住「已经够小则不改尺寸」：
// 小图只换编码格式，不放大。
func TestBuildImagePreviewKeepsSmallImageSize(t *testing.T) {
	source := testPNGDataURL(t, 64, 32)
	image, err := decodeImageDataURL(source, sessionAttachmentAllowedImageMIMEs)
	if err != nil {
		t.Fatalf("decodeImageDataURL() error = %v", err)
	}
	preview, err := buildImagePreview(image.Payload)
	if err != nil {
		t.Fatalf("buildImagePreview() error = %v", err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(preview))
	if err != nil {
		t.Fatalf("preview is not a valid jpeg: %v", err)
	}
	if decoded.Bounds().Dx() != 64 || decoded.Bounds().Dy() != 32 {
		t.Fatalf("preview bounds = %v", decoded.Bounds())
	}
}

// TestBuildImagePreviewFlattensTransparency 钉住透明像素合成白底：
// 全透明 PNG 转 JPEG 后不出现黑块。
func TestBuildImagePreviewFlattensTransparency(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode transparent png: %v", err)
	}
	preview, err := buildImagePreview(buffer.Bytes())
	if err != nil {
		t.Fatalf("buildImagePreview() error = %v", err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(preview))
	if err != nil {
		t.Fatalf("preview is not a valid jpeg: %v", err)
	}
	r, g, b, _ := decoded.At(4, 4).RGBA()
	if r < 0xF000 || g < 0xF000 || b < 0xF000 {
		t.Fatalf("transparent pixel must become white, got r=%d g=%d b=%d", r, g, b)
	}
}

// TestBuildImagePreviewDecodesWebP 钉住 WebP 解码：官方解码器可用，
// 小副本正常生成。
func TestBuildImagePreviewDecodesWebP(t *testing.T) {
	payload := testWebPPayload(t)
	if _, err := webp.Decode(bytes.NewReader(payload)); err != nil {
		t.Fatalf("test webp payload is invalid: %v", err)
	}
	preview, err := buildImagePreview(payload)
	if err != nil {
		t.Fatalf("buildImagePreview(webp) error = %v", err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(preview)); err != nil {
		t.Fatalf("webp preview is not a valid jpeg: %v", err)
	}
}

// testWebPPayload 构造一张最小合法 WebP（VP8L 无损）。
func testWebPPayload(t *testing.T) []byte {
	t.Helper()
	// 1x1 红色像素的 VP8L 无损 WebP。
	encoded, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatalf("decode webp base64: %v", err)
	}
	return encoded
}

// TestSaveSessionImageAttachmentStoresPreviewWhenMultiVersionEnabled 钉住
// 存图多版本：开启时原图与小副本并存，附件带小副本路径。
func TestSaveSessionImageAttachmentStoresPreviewWhenMultiVersionEnabled(t *testing.T) {
	system := newTestSystem(t)
	session := types.Session{ID: "session-preview", RoleID: "developer", Title: "Preview"}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	attachment, err := system.SaveSessionMessageAttachment(context.Background(), "developer", "session-preview", types.RunAttachment{Kind: "image", Name: "大图", DataURL: testPNGDataURL(t, 200, 100)})
	if err != nil {
		t.Fatalf("SaveSessionMessageAttachment() error = %v", err)
	}
	if attachment.PreviewPath == "" {
		t.Fatalf("attachment must carry preview path, got %#v", attachment)
	}
	assertFile(t, filepath.Join(system.paths.root, filepath.FromSlash(attachment.Path)))
	assertFile(t, filepath.Join(system.paths.root, filepath.FromSlash(attachment.PreviewPath)))
	previewDataURL, err := system.LoadSessionAttachmentPreviewImage(context.Background(), attachment.PreviewPath)
	if err != nil {
		t.Fatalf("LoadSessionAttachmentPreviewImage() error = %v", err)
	}
	if !bytes.HasPrefix([]byte(previewDataURL), []byte("data:image/jpeg;base64,")) {
		t.Fatalf("preview data url = %q", previewDataURL)
	}
}

// TestSaveSessionImageAttachmentSkipsPreviewWhenMultiVersionDisabled 钉住
// 存图多版本关闭：只存原图，附件不带小副本路径。
func TestSaveSessionImageAttachmentSkipsPreviewWhenMultiVersionDisabled(t *testing.T) {
	system := newTestSystem(t)
	if _, err := system.SaveConversationImageConfig(context.Background(), types.ConversationImageConfig{MultiVersionEnabled: false, OriginalBudgetCount: 2, HistoryBudgetCount: 6}); err != nil {
		t.Fatalf("SaveConversationImageConfig() error = %v", err)
	}
	session := types.Session{ID: "session-no-preview", RoleID: "developer", Title: "NoPreview"}
	if err := system.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	attachment, err := system.SaveSessionMessageAttachment(context.Background(), "developer", "session-no-preview", types.RunAttachment{Kind: "image", Name: "大图", DataURL: testPNGDataURL(t, 200, 100)})
	if err != nil {
		t.Fatalf("SaveSessionMessageAttachment() error = %v", err)
	}
	if attachment.PreviewPath != "" {
		t.Fatalf("preview path must be empty when multi-version disabled, got %#v", attachment)
	}
}

// TestConversationImageConfigDefaultsAndPersistence 钉住配置默认值与往返。
func TestConversationImageConfigDefaultsAndPersistence(t *testing.T) {
	system := newTestSystem(t)
	config, err := system.LoadConversationImageConfig(context.Background())
	if err != nil {
		t.Fatalf("LoadConversationImageConfig(default) error = %v", err)
	}
	if !config.MultiVersionEnabled || !config.OriginalBudgetEnabled || config.OriginalBudgetCount != types.ConversationImageOriginalBudgetDefault || !config.HistoryBudgetEnabled || config.HistoryBudgetCount != types.ConversationImageHistoryBudgetDefault {
		t.Fatalf("default config = %#v", config)
	}

	saved, err := system.SaveConversationImageConfig(context.Background(), types.ConversationImageConfig{MultiVersionEnabled: true, OriginalBudgetEnabled: true, OriginalBudgetCount: 5, HistoryBudgetEnabled: false, HistoryBudgetCount: 9})
	if err != nil {
		t.Fatalf("SaveConversationImageConfig() error = %v", err)
	}
	loaded, err := system.LoadConversationImageConfig(context.Background())
	if err != nil {
		t.Fatalf("LoadConversationImageConfig(saved) error = %v", err)
	}
	if loaded.OriginalBudgetCount != 5 || loaded.HistoryBudgetCount != 9 || loaded.HistoryBudgetEnabled {
		t.Fatalf("loaded config = %#v saved = %#v", loaded, saved)
	}
}
