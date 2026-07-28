package releaseops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceTopLevelJSONVersionOnlyChangesTopLevelValue(t *testing.T) {
	source := []byte("{\n  \"version\": \"0.1.0\",\n  \"nested\": {\"version\": \"9.9.9\"}\n}\n")
	updated, err := replaceTopLevelJSONVersion(source, "0.1.0", "0.1.1")
	if err != nil {
		t.Fatalf("replaceTopLevelJSONVersion() error = %v", err)
	}
	got := string(updated)
	if !strings.Contains(got, `"version": "0.1.1"`) || !strings.Contains(got, `{"version": "9.9.9"}`) {
		t.Fatalf("updated JSON = %s", got)
	}
}

func TestReplaceTOMLVersionTargetsRequestedPackage(t *testing.T) {
	cargoTOML := "[package]\nname = \"ai-studio-app\"\nversion = \"0.1.9\"\n\n[dependencies]\nexample = \"0.1.9\"\n"
	updated, err := replaceTOMLVersion(cargoTOML, packageVersion, "0.1.9", "0.1.10")
	if err != nil {
		t.Fatalf("replaceTOMLVersion() error = %v", err)
	}
	if strings.Count(updated, `0.1.10`) != 1 || !strings.Contains(updated, `example = "0.1.9"`) {
		t.Fatalf("updated Cargo.toml = %s", updated)
	}

	cargoLock := "[[package]]\nname = \"ai-studio-app\"\nversion = \"0.1.9\"\n\n[[package]]\nname = \"other\"\nversion = \"0.1.9\"\n"
	updated, err = replaceTOMLVersion(cargoLock, aiStudioLockVersion, "0.1.9", "0.1.10")
	if err != nil {
		t.Fatalf("replaceTOMLVersion() error = %v", err)
	}
	if strings.Count(updated, `version = "0.1.10"`) != 1 || strings.Count(updated, `version = "0.1.9"`) != 1 {
		t.Fatalf("updated Cargo.lock = %s", updated)
	}
}

func TestApplyChangesRollsBackWhenVerificationFails(t *testing.T) {
	directory := t.TempDir()
	first := filepath.Join(directory, "first.txt")
	second := filepath.Join(directory, "second.txt")
	writeTestFile(t, first, "first-original")
	writeTestFile(t, second, "second-original")

	err := applyChanges([]fileChange{
		{path: first, payload: []byte("first-updated"), mode: 0o644},
		{path: second, payload: []byte("second-updated"), mode: 0o644},
	}, func() error {
		return os.ErrInvalid
	})
	if err == nil {
		t.Fatal("applyChanges() should fail")
	}
	if got := readTestFile(t, first); got != "first-original" {
		t.Fatalf("first file = %q", got)
	}
	if got := readTestFile(t, second); got != "second-original" {
		t.Fatalf("second file = %q", got)
	}
}

func TestSetVersionUpdatesOneToolAndChangelog(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "tools", "demo")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeTestFile(t, filepath.Join(directory, "tool.json"), `{
  "id": "demo",
  "name": "演示工具",
  "description": "用于测试版本调整。",
  "version": "0.1.0",
  "eucliBoxCompatibility": {"minimumVersion": "0.1.0", "maximumVersionExclusive": "0.2.0"},
  "defaultInvocationMode": "sync",
  "type": "local"
}
`)
	writeTestFile(t, filepath.Join(directory, "README.md"), "# 演示工具\n\n这是用于版本调整验证的中文工具说明。\n")
	writeTestFile(t, filepath.Join(directory, "CHANGELOG.md"), "# 更新记录\n\n## 0.1.0\n\n- 建立工具。\n")

	result, err := SetVersion(root, "tool:demo", "0.1.1", "补充版本调整验证")
	if err != nil {
		t.Fatalf("SetVersion() error = %v", err)
	}
	if result.PreviousVersion != "0.1.0" || result.Version != "0.1.1" {
		t.Fatalf("result = %#v", result)
	}
	toolJSON := readTestFile(t, filepath.Join(directory, "tool.json"))
	if !strings.Contains(toolJSON, `"version": "0.1.1"`) {
		t.Fatalf("tool.json = %s", toolJSON)
	}
	changelog := readTestFile(t, filepath.Join(directory, "CHANGELOG.md"))
	if !strings.Contains(changelog, "## 0.1.1") || !strings.Contains(changelog, "补充版本调整验证") {
		t.Fatalf("CHANGELOG.md = %s", changelog)
	}
}

func TestSetVersionKeepsClientPackageVersionsInSync(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "clients", "eucli-studio")
	tauriDirectory := filepath.Join(directory, "src-tauri")
	if err := os.MkdirAll(tauriDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeTestFile(t, filepath.Join(directory, "release.json"), `{
  "version": "0.1.9",
  "eucliBoxCompatibility": {"minimumVersion": "0.1.0", "maximumVersionExclusive": "0.2.0"}
}
`)
	writeTestFile(t, filepath.Join(directory, "README.md"), "# 桌面客户端\n\n这是用于版本同步验证的中文客户端说明。\n")
	writeTestFile(t, filepath.Join(directory, "CHANGELOG.md"), "# 更新记录\n\n## 0.1.9\n\n- 建立客户端。\n")
	writeTestFile(t, filepath.Join(directory, "package.json"), "{\n  \"name\": \"client\",\n  \"version\": \"0.1.9\"\n}\n")
	writeTestFile(t, filepath.Join(tauriDirectory, "tauri.conf.json"), "{\n  \"version\": \"0.1.9\"\n}\n")
	writeTestFile(t, filepath.Join(tauriDirectory, "tauri.conf.dev.json"), "{\n  \"version\": \"0.1.9\"\n}\n")
	writeTestFile(t, filepath.Join(tauriDirectory, "Cargo.toml"), "[package]\nname = \"ai-studio-app\"\nversion = \"0.1.9\"\n")
	writeTestFile(t, filepath.Join(tauriDirectory, "Cargo.lock"), "version = 4\n\n[[package]]\nname = \"ai-studio-app\"\nversion = \"0.1.9\"\n")

	result, err := SetVersion(root, "eucli-studio", "0.1.10", "验证客户端版本同步")
	if err != nil {
		t.Fatalf("SetVersion() error = %v", err)
	}
	if result.Version != "0.1.10" {
		t.Fatalf("result = %#v", result)
	}
	paths := clientVersionFiles(directory)
	jsonFiles := append([]string{}, paths.jsonFiles...)
	jsonFiles = append(jsonFiles, filepath.Join(directory, "release.json"))
	for _, path := range jsonFiles {
		if !strings.Contains(readTestFile(t, path), `"version": "0.1.10"`) {
			t.Fatalf("%s did not receive the new version", path)
		}
	}
	if !strings.Contains(readTestFile(t, paths.cargoTOML), `version = "0.1.10"`) {
		t.Fatal("Cargo.toml did not receive the new version")
	}
	if !strings.Contains(readTestFile(t, paths.cargoLock), `version = "0.1.10"`) {
		t.Fatal("Cargo.lock did not receive the new version")
	}
}

func TestPrependChangelogVersionRejectsDuplicateVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CHANGELOG.md")
	writeTestFile(t, path, "# 更新记录\n\n## 0.1.1\n\n- 已有记录。\n")
	if _, err := prependChangelogVersion(path, "0.1.1", "重复记录"); err == nil {
		t.Fatal("prependChangelogVersion() should reject duplicate versions")
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(payload)
}
