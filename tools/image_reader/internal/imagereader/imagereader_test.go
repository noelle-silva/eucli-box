package imagereader

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/toolcontrol"
	"eucli-box/pkg/types"
)

// fakeSession 是会话能力服务的测试替身：记录写入请求并按需拒绝。
type fakeSession struct {
	written     []types.SessionAttachmentWriteRequest
	writeDenied bool
	writeFail   bool
}

func (f *fakeSession) Request(ctx context.Context, capability string, access string, payload any) (toolcontrol.CapabilityResult, error) {
	if capability != types.ToolCapabilitySessionAttachments || access != types.ToolCapabilityAccessWrite {
		return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusFailed, Error: "unexpected capability"}, nil
	}
	request, ok := payload.(types.SessionAttachmentWriteRequest)
	if !ok {
		return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusFailed, Error: "bad payload"}, nil
	}
	if f.writeDenied {
		return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusDenied, Error: toolcontrol.CapabilityDeniedMessage}, nil
	}
	if f.writeFail {
		return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusFailed, Error: "boom"}, nil
	}
	f.written = append(f.written, request)
	return toolcontrol.CapabilityResult{Status: toolcontrol.CapabilityStatusSuccess, Payload: types.SessionAttachmentInfo{ID: "att-1-1", Name: request.Name, Mime: "image/png"}}, nil
}

// encodePNG 生成一张指定尺寸的纯色 PNG。
func encodePNG(t *testing.T, width int, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buffer.Bytes()
}

// encodeNoisyPNG 生成一张带噪声的大 PNG：PNG 对噪声几乎不压缩，
// 源图会明显大于降采样后的 JPEG，用于验证压缩触发路径。
func encodeNoisyPNG(t *testing.T, width int, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	seed := uint32(12345)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			seed = seed*1664525 + 1013904223
			img.Set(x, y, color.RGBA{R: uint8(seed >> 24), G: uint8(seed >> 16), B: uint8(seed >> 8), A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode noisy png: %v", err)
	}
	return buffer.Bytes()
}

func writeTempFile(t *testing.T, name string, payload []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func executionInput(path string, userConfig map[string]any) types.ToolExecutionInput {
	return types.ToolExecutionInput{
		Arguments:            map[string]any{"path": path},
		HostWorkingDirectory: filepath.Dir(path),
		UserConfig:           userConfig,
	}
}

func TestDetectImageKindRecognizesMagicNumbers(t *testing.T) {
	png := encodePNG(t, 4, 4)
	var jpegBuffer bytes.Buffer
	if err := jpeg.Encode(&jpegBuffer, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	var gifBuffer bytes.Buffer
	if err := gif.Encode(&gifBuffer, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	cases := []struct {
		name    string
		payload []byte
		mime    string
	}{
		{"png", png, "image/png"},
		{"jpeg", jpegBuffer.Bytes(), "image/jpeg"},
		{"gif", gifBuffer.Bytes(), "image/gif"},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "image/webp"},
	}
	for _, testCase := range cases {
		kind, ok := detectImageKind(testCase.payload)
		if !ok || kind.Mime != testCase.mime {
			t.Fatalf("%s: kind = %#v ok = %v", testCase.name, kind, ok)
		}
	}
	if _, ok := detectImageKind([]byte("plain text not an image")); ok {
		t.Fatalf("plain text must not be recognized as an image")
	}
}

func TestParseArgumentsRequiresPath(t *testing.T) {
	if _, err := parseArguments(map[string]any{}); err == nil {
		t.Fatalf("missing path must fail")
	}
	if _, err := parseArguments(map[string]any{"path": "   "}); err == nil {
		t.Fatalf("blank path must fail")
	}
	args, err := parseArguments(map[string]any{"path": " a.png "})
	if err != nil || args.Path != "a.png" {
		t.Fatalf("args = %#v err = %v", args, err)
	}
}

func TestRejectConfigArguments(t *testing.T) {
	input := types.ToolExecutionInput{Arguments: map[string]any{"path": "a.png", "maxImageBytes": 1}}
	if err := rejectConfigArguments(input); err == nil {
		t.Fatalf("config key in arguments must be rejected")
	}
}

func TestLoadConfigDefaultsAndValidation(t *testing.T) {
	config, err := loadConfig(types.ToolExecutionInput{})
	if err != nil {
		t.Fatalf("defaults: %v", err)
	}
	if config.MaxImageBytes != defaultMaxImageBytes || config.MaxSourceBytes != defaultMaxSourceBytes || config.MaxDimension != defaultMaxDimension || config.JPEGQuality != defaultJPEGQuality {
		t.Fatalf("defaults = %#v", config)
	}
	if _, err := loadConfig(types.ToolExecutionInput{UserConfig: map[string]any{"maxImageBytes": 0}}); err == nil {
		t.Fatalf("zero maxImageBytes must fail")
	}
	if _, err := loadConfig(types.ToolExecutionInput{UserConfig: map[string]any{"maxImageBytes": 100, "maxSourceBytes": 50}}); err == nil {
		t.Fatalf("maxSourceBytes smaller than maxImageBytes must fail")
	}
	if _, err := loadConfig(types.ToolExecutionInput{UserConfig: map[string]any{"jpegQuality": 200}}); err == nil {
		t.Fatalf("out-of-range jpegQuality must fail")
	}
}

func TestCompressImageDownscalesAndReencodesJPEG(t *testing.T) {
	source := encodePNG(t, 400, 200)
	config := Config{MaxImageBytes: 1 << 20, MaxSourceBytes: 1 << 30, MaxDimension: 100, JPEGQuality: 80}
	out, err := compressImage(source, config)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	kind, ok := detectImageKind(out)
	if !ok || kind.Mime != "image/jpeg" {
		t.Fatalf("compressed output must be jpeg, got %#v ok=%v", kind, ok)
	}
	width, height, err := imageDimensions(out)
	if err != nil {
		t.Fatalf("dimensions: %v", err)
	}
	if width != 100 || height != 50 {
		t.Fatalf("downscaled dimensions = %d x %d, want 100 x 50", width, height)
	}
}

func TestExecuteLoadsSmallImageWithoutCompression(t *testing.T) {
	payload := encodePNG(t, 8, 8)
	path := writeTempFile(t, "small.png", payload)
	session := &fakeSession{}
	out := Execute(context.Background(), executionInput(path, nil), session)
	if out.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s content = %s", out.Status, out.Content)
	}
	if out.Metadata["compressed"] != false {
		t.Fatalf("small image must not be compressed: %#v", out.Metadata)
	}
	if len(session.written) != 1 {
		t.Fatalf("expected one write, got %d", len(session.written))
	}
	if !strings.HasPrefix(session.written[0].DataURL, "data:image/png;base64,") {
		t.Fatalf("data url = %q", session.written[0].DataURL[:40])
	}
	if session.written[0].Name != "small.png" {
		t.Fatalf("name = %q", session.written[0].Name)
	}
}

func TestExecuteCompressesLargeImage(t *testing.T) {
	payload := encodeNoisyPNG(t, 800, 800)
	// 阈值取在压缩结果与源图之间：源图超过阈值触发压缩，压缩结果落在阈值内。
	compressed, err := compressImage(payload, Config{MaxImageBytes: 1 << 20, MaxSourceBytes: 1 << 30, MaxDimension: 64, JPEGQuality: 60})
	if err != nil {
		t.Fatalf("pre-compute compressed: %v", err)
	}
	threshold := int64(len(compressed) + 1)
	if int64(len(payload)) <= threshold {
		t.Fatalf("test fixture invalid: source %d not larger than threshold %d", len(payload), threshold)
	}
	path := writeTempFile(t, "large.png", payload)
	session := &fakeSession{}
	input := executionInput(path, map[string]any{"maxImageBytes": threshold, "maxDimension": 64, "jpegQuality": 60})
	out := Execute(context.Background(), input, session)
	if out.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s content = %s", out.Status, out.Content)
	}
	if out.Metadata["compressed"] != true {
		t.Fatalf("large image must be compressed: %#v", out.Metadata)
	}
	if len(session.written) != 1 || !strings.HasPrefix(session.written[0].DataURL, "data:image/jpeg;base64,") {
		t.Fatalf("compressed image must be written as jpeg: %#v", session.written)
	}
}

func TestExecuteRejectsWhenStillOversizedAfterCompression(t *testing.T) {
	payload := encodePNG(t, 300, 300)
	path := writeTempFile(t, "large.png", payload)
	session := &fakeSession{}
	// 阈值大于 0 但远小于任何 JPEG 结果：触发压缩后仍超限，必须原地拒绝。
	input := executionInput(path, map[string]any{"maxImageBytes": int64(8), "maxSourceBytes": int64(1 << 30), "maxDimension": 64, "jpegQuality": 60})
	out := Execute(context.Background(), input, session)
	if out.Status != types.ToolStatusFailed || !strings.Contains(out.Error, "文件太大") {
		t.Fatalf("status = %s error = %s", out.Status, out.Error)
	}
	if len(session.written) != 0 {
		t.Fatalf("oversized-after-compression must not be written")
	}
}

func TestExecuteRejectsOversizedSource(t *testing.T) {
	payload := encodePNG(t, 300, 300)
	path := writeTempFile(t, "huge.png", payload)
	session := &fakeSession{}
	input := executionInput(path, map[string]any{"maxSourceBytes": int64(10), "maxImageBytes": int64(10)})
	out := Execute(context.Background(), input, session)
	if out.Status != types.ToolStatusFailed || !strings.Contains(out.Error, "文件太大") {
		t.Fatalf("status = %s error = %s", out.Status, out.Error)
	}
	if len(session.written) != 0 {
		t.Fatalf("oversized source must not be written")
	}
}

func TestExecuteRejectsNonImage(t *testing.T) {
	path := writeTempFile(t, "notes.png", []byte("this is not an image at all"))
	session := &fakeSession{}
	out := Execute(context.Background(), executionInput(path, nil), session)
	if out.Status != types.ToolStatusFailed || !strings.Contains(out.Error, "不是受支持的图片文件") {
		t.Fatalf("status = %s error = %s", out.Status, out.Error)
	}
	if len(session.written) != 0 {
		t.Fatalf("non-image must not be written")
	}
}

func TestExecuteRejectsMissingFile(t *testing.T) {
	dir := t.TempDir()
	session := &fakeSession{}
	out := Execute(context.Background(), executionInput(filepath.Join(dir, "absent.png"), nil), session)
	if out.Status != types.ToolStatusFailed || !strings.Contains(out.Error, "文件不存在") {
		t.Fatalf("status = %s error = %s", out.Status, out.Error)
	}
}

func TestExecutePropagatesWriteDenial(t *testing.T) {
	payload := encodePNG(t, 8, 8)
	path := writeTempFile(t, "small.png", payload)
	session := &fakeSession{writeDenied: true}
	out := Execute(context.Background(), executionInput(path, nil), session)
	if out.Status != types.ToolStatusFailed || !strings.Contains(out.Error, "未授权") {
		t.Fatalf("status = %s error = %s", out.Status, out.Error)
	}
}

func TestExecuteFailsWithoutSession(t *testing.T) {
	payload := encodePNG(t, 8, 8)
	path := writeTempFile(t, "small.png", payload)
	out := Execute(context.Background(), executionInput(path, nil), nil)
	if out.Status != types.ToolStatusFailed || !strings.Contains(out.Error, "无法把图片加载进会话") {
		t.Fatalf("status = %s error = %s", out.Status, out.Error)
	}
}

func TestDataURLRoundTrip(t *testing.T) {
	payload := encodePNG(t, 4, 4)
	url := dataURL(pngKind, payload)
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("data url prefix = %q", url)
	}
	encoded := strings.TrimPrefix(url, "data:image/png;base64,")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !bytes.Equal(decoded, payload) {
		t.Fatalf("data url round trip failed")
	}
}
