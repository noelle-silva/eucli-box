package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestActionDefinitionUnmarshal(t *testing.T) {
	var text actionDefinition
	if err := json.Unmarshal([]byte(`"echo helloworld"`), &text); err != nil {
		t.Fatalf("字符串形态解析失败：%v", err)
	}
	if text.Command != "echo helloworld" || len(text.Artifact) != 0 {
		t.Fatalf("字符串形态 = %#v", text)
	}

	var object actionDefinition
	if err := json.Unmarshal([]byte(`{"command":"go run demo","artifact":{"path":"output.path"}}`), &object); err != nil {
		t.Fatalf("对象形态解析失败：%v", err)
	}
	if object.Command != "go run demo" || object.Artifact["path"] != "output.path" {
		t.Fatalf("对象形态 = %#v", object)
	}
}

func TestValueAtPath(t *testing.T) {
	root := map[string]any{"outer": map[string]any{"inner": "value"}, "size": 42.0}
	if value, ok := valueAtPath(root, "outer.inner"); !ok || value != "value" {
		t.Fatalf("valueAtPath(outer.inner) = %q, %v", value, ok)
	}
	for _, path := range []string{"outer", "outer.inner.deep", "size", "missing"} {
		if _, ok := valueAtPath(root, path); ok {
			t.Fatalf("valueAtPath(%q) 应该取不到", path)
		}
	}
}

func TestRunMainProducesReceiptWithArtifact(t *testing.T) {
	protocolFile := filepath.Join(t.TempDir(), protocolFileName)
	content := `{
  "runner": "go run",
  "actions": {
    "demo": {
      "command": "echo {\"name\":\"box.zip\",\"size\":\"42\"}",
      "artifact": {"name": "name", "size": "size"}
    }
  }
}
`
	if err := os.WriteFile(protocolFile, []byte(content), 0o644); err != nil {
		t.Fatalf("写入协议文件失败：%v", err)
	}

	var output bytes.Buffer
	if status := runMain([]string{"demo"}, protocolFile, &output); status != 0 {
		t.Fatalf("runMain() = %d，输出：%s", status, output.String())
	}
	parsed := unmarshalReceipt(t, output.String())
	if parsed.ContractVersion != contractVersion || parsed.Status != statusSucceeded {
		t.Fatalf("receipt = %#v", parsed)
	}
	if parsed.ExitCode == nil || *parsed.ExitCode != 0 || parsed.Error != "" {
		t.Fatalf("receipt = %#v", parsed)
	}
	result, ok := parsed.Data.Result.(map[string]any)
	if !ok || result["name"] != "box.zip" {
		t.Fatalf("data.result = %#v", parsed.Data.Result)
	}
	if parsed.Data.Artifact["name"] != "box.zip" || parsed.Data.Artifact["size"] != "42" {
		t.Fatalf("data.artifact = %#v", parsed.Data.Artifact)
	}
}

func TestRunMainUnknownActionFails(t *testing.T) {
	protocolFile := filepath.Join(t.TempDir(), protocolFileName)
	if err := os.WriteFile(protocolFile, []byte(`{"actions":{"known":"echo ok"}}`), 0o644); err != nil {
		t.Fatalf("写入协议文件失败：%v", err)
	}

	var output bytes.Buffer
	if status := runMain([]string{"missing"}, protocolFile, &output); status == 0 {
		t.Fatal("未知动作应该以非零状态结束")
	}
	parsed := unmarshalReceipt(t, output.String())
	if parsed.Status != statusFailed || parsed.ExitCode != nil || parsed.Error == "" {
		t.Fatalf("receipt = %#v", parsed)
	}
	if len(parsed.Data.Artifact) != 0 || parsed.Data.Result != nil {
		t.Fatalf("data = %#v", parsed.Data)
	}
}

func unmarshalReceipt(t *testing.T, output string) receipt {
	t.Helper()
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := lines[index]
		if !strings.HasPrefix(line, receiptPrefix) {
			continue
		}
		var parsed receipt
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, receiptPrefix)), &parsed); err != nil {
			t.Fatalf("回执解析失败：%v（%q）", err, line)
		}
		return parsed
	}
	t.Fatalf("输出中没有回执行：%q", output)
	return receipt{}
}

func TestBuildStorePackageProducesInstallableArchive(t *testing.T) {
	root := t.TempDir()
	protocolDir := filepath.Join(root, ".fast-window-dev-protocol")
	writeTestFile(t, filepath.Join(root, "internal", "boxrelease", "release.json"), "{\n  \"version\": \"0.1.2\",\n  \"dataVersion\": \"1.0.0\"\n}\n")
	writeTestFile(t, filepath.Join(protocolDir, "assets", "icon.svg"), "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 64 64\"></svg>\n")
	writeTestFile(t, filepath.Join(protocolDir, "fw-app.json"), `{
  "type": "service-app",
  "id": "eucli-box",
  "name": "eucli-box",
  "description": "eucli-box 商店化测试。",
  "versionSource": "internal/boxrelease/release.json",
  "package": {
    "windowsExecutable": "eucli-box.exe",
    "icon": ".fast-window-dev-protocol/assets/icon.svg"
  },
  "service": {
    "ready": { "type": "log", "match": "is ready" },
    "stop": { "type": "terminate" }
  },
  "displayMode": "default",
  "commands": []
}
`)
	baseZip := filepath.Join(root, "base", "eucli-box_0.1.2_windows-x64.zip")
	writeTestZip(t, baseZip, map[string]string{
		"eucli-box.exe":        "fake-exe",
		"README.md":            "# readme",
		"release-product.json": "{}",
	})

	result, err := buildStorePackage(protocolDir, map[string]string{"path": baseZip})
	if err != nil {
		t.Fatalf("buildStorePackage() error = %v", err)
	}
	if result["name"] != "eucli-box_0.1.2_windows-x64.zip" {
		t.Fatalf("name = %q", result["name"])
	}
	if !strings.HasSuffix(filepath.ToSlash(result["path"]), ".fast-window-dev-protocol/dist/eucli-box_0.1.2_windows-x64.zip") {
		t.Fatalf("path = %q", result["path"])
	}
	if len(result["sha256"]) != 64 {
		t.Fatalf("sha256 = %q", result["sha256"])
	}

	names := listZipNames(t, result["path"])
	for _, name := range []string{"fw-app.json", "assets/icon.svg", "eucli-box.exe", "README.md", "release-product.json"} {
		if !containsString(names, name) {
			t.Fatalf("商店包缺少 %s：%v", name, names)
		}
	}
	if containsString(names, "fw-app.service.json") {
		t.Fatalf("商店包不应再携带独立服务声明：%v", names)
	}

	var manifest struct {
		Type     string `json:"type"`
		ID       string `json:"id"`
		Name     string `json:"name"`
		Version  string `json:"version"`
		Package  struct {
			WindowsExecutable string `json:"windowsExecutable"`
			Icon              string `json:"icon"`
		} `json:"package"`
		Service struct {
			Ready struct {
				Type  string `json:"type"`
				Match string `json:"match"`
			} `json:"ready"`
			Stop struct {
				Type string `json:"type"`
			} `json:"stop"`
		} `json:"service"`
		DisplayMode string `json:"displayMode"`
	}
	if err := json.Unmarshal([]byte(readZipEntry(t, result["path"], "fw-app.json")), &manifest); err != nil {
		t.Fatalf("fw-app.json 解析失败：%v", err)
	}
	if manifest.Type != appTypeServiceApp || manifest.ID != "eucli-box" || manifest.Name != "eucli-box" || manifest.Version != "0.1.2" {
		t.Fatalf("fw-app.json = %#v", manifest)
	}
	if manifest.Package.WindowsExecutable != "eucli-box.exe" || manifest.Package.Icon != "assets/icon.svg" || manifest.DisplayMode != "default" {
		t.Fatalf("fw-app.json = %#v", manifest)
	}
	if manifest.Service.Ready.Type != "log" || manifest.Service.Ready.Match != "is ready" || manifest.Service.Stop.Type != "terminate" {
		t.Fatalf("fw-app.json service = %#v", manifest.Service)
	}
	if icon := readZipEntry(t, result["path"], "assets/icon.svg"); !strings.Contains(icon, "<svg") {
		t.Fatalf("图标内容异常：%q", icon)
	}
}

func TestBuildStorePackageKeepsDesktopAppWithoutService(t *testing.T) {
	root := t.TempDir()
	protocolDir := filepath.Join(root, ".fast-window-dev-protocol")
	writeTestFile(t, filepath.Join(root, "internal", "boxrelease", "release.json"), "{\n  \"version\": \"0.1.2\"\n}\n")
	writeTestFile(t, filepath.Join(protocolDir, "assets", "icon.svg"), "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 64 64\"></svg>\n")
	writeTestFile(t, filepath.Join(protocolDir, "fw-app.json"), `{
  "type": "desktop-app",
  "id": "eucli-box",
  "name": "eucli-box",
  "description": "桌面应用商店化测试。",
  "versionSource": "internal/boxrelease/release.json",
  "package": {
    "windowsExecutable": "eucli-box.exe",
    "icon": ".fast-window-dev-protocol/assets/icon.svg"
  },
  "displayMode": "default",
  "commands": []
}
`)
	baseZip := filepath.Join(root, "base", "eucli-box_0.1.2_windows-x64.zip")
	writeTestZip(t, baseZip, map[string]string{"eucli-box.exe": "fake-exe"})

	result, err := buildStorePackage(protocolDir, map[string]string{"path": baseZip})
	if err != nil {
		t.Fatalf("buildStorePackage() error = %v", err)
	}
	payload := readZipEntry(t, result["path"], "fw-app.json")
	if strings.Contains(payload, "service") {
		t.Fatalf("桌面应用清单不应带 service 段：%s", payload)
	}
	if strings.Contains(payload, "versionSource") {
		t.Fatalf("商店清单不应保留 versionSource：%s", payload)
	}
}

func TestBuildStorePackageRequiresArtifactPath(t *testing.T) {
	protocolDir := filepath.Join(t.TempDir(), ".fast-window-dev-protocol")
	if _, err := buildStorePackage(protocolDir, map[string]string{}); err == nil || !strings.Contains(err.Error(), "成品路径") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildStorePackageRejectsMissingIcon(t *testing.T) {
	root := t.TempDir()
	protocolDir := filepath.Join(root, ".fast-window-dev-protocol")
	writeTestFile(t, filepath.Join(root, "internal", "boxrelease", "release.json"), "{\n  \"version\": \"0.1.2\"\n}\n")
	writeTestFile(t, filepath.Join(protocolDir, "fw-app.json"), `{
  "type": "desktop-app",
  "id": "eucli-box",
  "name": "eucli-box",
  "versionSource": "internal/boxrelease/release.json",
  "package": {
    "windowsExecutable": "eucli-box.exe",
    "icon": ".fast-window-dev-protocol/assets/icon.svg"
  },
  "displayMode": "default",
  "commands": []
}
`)
	baseZip := filepath.Join(root, "base", "eucli-box_0.1.2_windows-x64.zip")
	writeTestZip(t, baseZip, map[string]string{"eucli-box.exe": "fake-exe"})

	if _, err := buildStorePackage(protocolDir, map[string]string{"path": baseZip}); err == nil || !strings.Contains(err.Error(), "图标") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadSourceManifestValidatesTypeAndService(t *testing.T) {
	tests := []struct {
		name    string
		content string
		message string
	}{
		{
			name: "未知类别",
			content: `{
  "type": "daemon-app",
  "id": "eucli-box",
  "name": "eucli-box",
  "versionSource": "internal/boxrelease/release.json"
}`,
			message: "type 必须为",
		},
		{
			name: "服务应用缺少服务段",
			content: `{
  "type": "service-app",
  "id": "eucli-box",
  "name": "eucli-box",
  "versionSource": "internal/boxrelease/release.json"
}`,
			message: "必须提供 service 段",
		},
		{
			name: "桌面应用带服务段",
			content: `{
  "type": "desktop-app",
  "id": "eucli-box",
  "name": "eucli-box",
  "versionSource": "internal/boxrelease/release.json",
  "service": { "stop": { "type": "terminate" } }
}`,
			message: "不允许携带 service 段",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), sourceManifestFileName)
			writeTestFile(t, file, test.content)
			if _, err := readSourceManifest(file); err == nil {
				t.Fatal("readSourceManifest() 应该失败")
			} else if !strings.Contains(err.Error(), test.message) {
				t.Fatalf("err = %v，期望包含 %q", err, test.message)
			}
		})
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func writeTestZip(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", zipPath, err)
	}
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Create(%s) error = %v", zipPath, err)
	}
	writer := zip.NewWriter(file)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entryWriter, err := writer.Create(name)
		if err != nil {
			t.Fatalf("Create(%s) error = %v", name, err)
		}
		if _, err := entryWriter.Write([]byte(files[name])); err != nil {
			t.Fatalf("Write(%s) error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close(zip) error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close(file) error = %v", err)
	}
}

func listZipNames(t *testing.T, zipPath string) []string {
	t.Helper()
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("OpenReader(%s) error = %v", zipPath, err)
	}
	defer reader.Close()
	names := make([]string, 0, len(reader.File))
	for _, entry := range reader.File {
		names = append(names, entry.Name)
	}
	sort.Strings(names)
	return names
}

func readZipEntry(t *testing.T, zipPath string, name string) string {
	t.Helper()
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("OpenReader(%s) error = %v", zipPath, err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if entry.Name != name {
			continue
		}
		source, err := entry.Open()
		if err != nil {
			t.Fatalf("Open(%s) error = %v", name, err)
		}
		payload, err := io.ReadAll(source)
		_ = source.Close()
		if err != nil {
			t.Fatalf("ReadAll(%s) error = %v", name, err)
		}
		return string(payload)
	}
	t.Fatalf("zip 中找不到 %s", name)
	return ""
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
