package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckManifestAcceptsCompleteFixture(t *testing.T) {
	root := writeFixture(t)
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
			root := writeFixture(t)
			test.mutate(t, root)
			if _, err := checkManifest(manifestPathOf(root)); err == nil {
				t.Fatal("checkManifest() 应该失败")
			} else if !strings.Contains(err.Error(), test.message) {
				t.Fatalf("err = %v，期望包含 %q", err, test.message)
			}
		})
	}
}

const fixtureManifest = `{
  "schemaVersion": 1,
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

func writeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "internal", "boxrelease", "release.json"), "{\n  \"version\": \"0.1.2\",\n  \"dataVersion\": \"1.0.0\"\n}\n")
	writeTestFile(t, filepath.Join(root, ".fast-window-dev-protocol", "assets", "icon.svg"), "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 64 64\"></svg>\n")
	writeTestFile(t, manifestPathOf(root), fixtureManifest)
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
