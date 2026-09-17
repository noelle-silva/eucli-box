package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckManifestAcceptsCompleteFixture(t *testing.T) {
	root := writeFixture(t, fixtureManifest)
	result, err := checkManifest(manifestPathOf(root))
	if err != nil {
		t.Fatalf("checkManifest() error = %v", err)
	}
	if result.ID != "eucli-box" || result.Version != "0.1.2" {
		t.Fatalf("result = %#v", result)
	}
	if result.Executable != "eucli-box.exe" {
		t.Fatalf("executable = %q", result.Executable)
	}
}

func TestCheckManifestAcceptsServiceAppFixture(t *testing.T) {
	root := writeFixture(t, fixtureServiceManifest)
	if _, err := checkManifest(manifestPathOf(root)); err != nil {
		t.Fatalf("checkManifest() error = %v", err)
	}
}

func TestCheckManifestRejectsBrokenManifests(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, root string)
		message string
	}{
		{
			name: "未知字段",
			mutate: func(t *testing.T, root string) {
				rewriteManifest(t, root, func(text string) string {
					return strings.Replace(text, `"displayMode"`, `"extraField": true, "displayMode"`, 1)
				})
			},
			message: "解析清单文件失败",
		},
		{
			name: "非法类别",
			mutate: func(t *testing.T, root string) {
				rewriteManifest(t, root, func(text string) string {
					return strings.Replace(text, `"type": "desktop-app"`, `"type": "daemon-app"`, 1)
				})
			},
			message: "type 必须为",
		},
		{
			name: "服务应用缺少服务段",
			mutate: func(t *testing.T, root string) {
				rewriteManifest(t, root, func(text string) string {
					return strings.Replace(text, `"type": "desktop-app"`, `"type": "service-app"`, 1)
				})
			},
			message: "必须提供 service 段",
		},
		{
			name: "非法 id",
			mutate: func(t *testing.T, root string) {
				rewriteManifest(t, root, func(text string) string {
					return strings.Replace(text, `"id": "eucli-box"`, `"id": "eucli box"`, 1)
				})
			},
			message: "id 不合法",
		},
		{
			name: "版本文件缺失",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "internal", "boxrelease", "release.json")); err != nil {
					t.Fatalf("Remove() error = %v", err)
				}
			},
			message: "versionSource",
		},
		{
			name: "图标缺失",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, ".fast-window-dev-protocol", "assets", "icon.svg")); err != nil {
					t.Fatalf("Remove() error = %v", err)
				}
			},
			message: "图标",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := writeFixture(t, fixtureManifest)
			test.mutate(t, root)
			if _, err := checkManifest(manifestPathOf(root)); err == nil {
				t.Fatal("checkManifest() 应该失败")
			} else if !strings.Contains(err.Error(), test.message) {
				t.Fatalf("err = %v，期望包含 %q", err, test.message)
			}
		})
	}
}

func TestCheckManifestRejectsBrokenServiceSections(t *testing.T) {
	tests := []struct {
		name    string
		service string
		message string
	}{
		{
			name:    "就绪规则类型不支持",
			service: `"ready": { "type": "port", "match": "ready" }, "stop": { "type": "terminate" }`,
			message: "service.ready.type 必须为 log",
		},
		{
			name:    "就绪规则缺少匹配文本",
			service: `"ready": { "type": "log" }, "stop": { "type": "terminate" }`,
			message: "service.ready.match 不能为空",
		},
		{
			name:    "停止方式不支持",
			service: `"ready": { "type": "log", "match": "ready" }, "stop": { "type": "kill" }`,
			message: "service.stop.type 必须为 terminate",
		},
		{
			name:    "连接端口类型不支持",
			service: `"ready": { "type": "log", "match": "ready" }, "stop": { "type": "terminate" }, "connection": { "port": { "type": "env" } }`,
			message: "connection.port.type 必须为 value 或 file",
		},
		{
			name:    "连接文件路径不安全",
			service: `"ready": { "type": "log", "match": "ready" }, "stop": { "type": "terminate" }, "connection": { "key": { "type": "file", "path": "../box.key" } }`,
			message: "connection.key.path 不安全",
		},
		{
			name:    "JSON 连接文件缺少字段名",
			service: `"ready": { "type": "log", "match": "ready" }, "stop": { "type": "terminate" }, "connection": { "port": { "type": "file", "path": "data/port.json", "format": "json" } }`,
			message: "connection.port.field 不能为空",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := writeFixture(t, fixtureManifest)
			rewriteManifest(t, root, func(text string) string {
				next := strings.Replace(text, `"type": "desktop-app"`, `"type": "service-app"`, 1)
				return strings.Replace(next, `"displayMode"`, `"service": {`+test.service+`},`+"\n  "+`"displayMode"`, 1)
			})
			if _, err := checkManifest(manifestPathOf(root)); err == nil {
				t.Fatal("checkManifest() 应该失败")
			} else if !strings.Contains(err.Error(), test.message) {
				t.Fatalf("err = %v，期望包含 %q", err, test.message)
			}
		})
	}
}

const fixtureManifest = `{
  "type": "desktop-app",
  "id": "eucli-box",
  "name": "eucli-box",
  "description": "本地 AI 工作台业务端。",
  "versionSource": "internal/boxrelease/release.json",
  "package": {
    "windowsExecutable": "eucli-box.exe",
    "icon": ".fast-window-dev-protocol/assets/icon.svg"
  },
  "displayMode": "default",
  "commands": []
}
`

const fixtureServiceManifest = `{
  "type": "service-app",
  "id": "eucli-box",
  "name": "eucli-box",
  "description": "本地 AI 工作台业务端。",
  "versionSource": "internal/boxrelease/release.json",
  "package": {
    "windowsExecutable": "eucli-box.exe",
    "icon": ".fast-window-dev-protocol/assets/icon.svg"
  },
  "service": {
    "start": { "args": ["--port", "8765"], "environment": { "EUCLI_BOX_MODE": "service" } },
    "ready": { "type": "log", "match": "is ready", "timeoutSeconds": 30 },
    "connection": {
      "port": { "type": "file", "path": "data/.meta/port.json", "format": "json", "field": "port" },
      "key": { "type": "file", "path": "data/.meta/box.key", "format": "text" }
    },
    "stop": { "type": "terminate" }
  },
  "displayMode": "default",
  "commands": []
}
`

func writeFixture(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "internal", "boxrelease", "release.json"), "{\n  \"version\": \"0.1.2\",\n  \"dataVersion\": \"1.0.0\"\n}\n")
	writeTestFile(t, filepath.Join(root, ".fast-window-dev-protocol", "assets", "icon.svg"), "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 64 64\"></svg>\n")
	writeTestFile(t, manifestPathOf(root), content)
	return root
}

func rewriteManifest(t *testing.T, root string, transform func(string) string) {
	t.Helper()
	path := manifestPathOf(root)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	writeTestFile(t, path, transform(string(payload)))
}

func manifestPathOf(root string) string {
	return filepath.Join(root, ".fast-window-dev-protocol", manifestFileName)
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
