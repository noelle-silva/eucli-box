package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devtools/common/toolruntime"
)

// newScaffoldRoot 建立一个包含真实模板副本的临时仓库根。
func newScaffoldRoot(t *testing.T) string {
	t.Helper()
	repositoryRoot, err := toolruntime.FindRepositoryRoot()
	if err != nil {
		t.Fatalf("FindRepositoryRoot() error = %v", err)
	}
	root := t.TempDir()
	templateSource := filepath.Join(repositoryRoot, "tools", templateDirectoryName)
	templateTarget := filepath.Join(root, "tools", templateDirectoryName)
	if err := copyTestDirectory(templateSource, templateTarget); err != nil {
		t.Fatalf("copy template: %v", err)
	}
	return root
}

func copyTestDirectory(source string, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, payload, 0o644)
	})
}

func testInput() scaffoldInput {
	return scaffoldInput{
		ID:          "demo_tool",
		Name:        "演示工具",
		Description: "用于脚手架验证的演示工具。",
		Version:     "0.1.0",
		PackageName: "demotool",
	}
}

func TestScaffoldGeneratesToolWithRewrittenIdentity(t *testing.T) {
	root := newScaffoldRoot(t)
	result, err := scaffold(root, testInput())
	if err != nil {
		t.Fatalf("scaffold() error = %v", err)
	}
	target := filepath.Join(root, "tools", "demo_tool")
	if result.Directory != target {
		t.Fatalf("directory = %s, want %s", result.Directory, target)
	}
	for _, expected := range []string{
		filepath.Join(target, "cmd", "demo_tool", "main.go"),
		filepath.Join(target, "cmd", "demo_tool", "migration.go"),
		filepath.Join(target, "cmd", "demo_tool", "migrations.go"),
		filepath.Join(target, "cmd", "demo_tool", "migration_test.go"),
		filepath.Join(target, "internal", "demotool", "runner.go"),
		filepath.Join(target, "internal", "datamigration", "execute.go"),
		filepath.Join(target, "tool.json"),
		filepath.Join(target, "toolpack.json"),
		filepath.Join(target, "config.json"),
		filepath.Join(target, "README.md"),
		filepath.Join(target, "CHANGELOG.md"),
	} {
		if _, err := os.Stat(expected); err != nil {
			t.Fatalf("generated file missing %s: %v", expected, err)
		}
	}
	if _, err := os.Stat(filepath.Join(target, "cmd", templateDirectoryName)); !os.IsNotExist(err) {
		t.Fatalf("template entry directory was not renamed")
	}
	if _, err := os.Stat(filepath.Join(target, "internal", templatePackageName)); !os.IsNotExist(err) {
		t.Fatalf("template package directory was not renamed")
	}
}

func TestScaffoldRewritesToolDefinition(t *testing.T) {
	root := newScaffoldRoot(t)
	input := testInput()
	if _, err := scaffold(root, input); err != nil {
		t.Fatalf("scaffold() error = %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(root, "tools", "demo_tool", "tool.json"))
	if err != nil {
		t.Fatalf("read tool.json: %v", err)
	}
	var definition map[string]any
	if err := json.Unmarshal(payload, &definition); err != nil {
		t.Fatalf("decode tool.json: %v", err)
	}
	if definition["id"] != input.ID || definition["name"] != input.Name || definition["description"] != input.Description || definition["version"] != input.Version {
		t.Fatalf("tool definition = %#v", definition)
	}
	if definition["promptDescription"] != input.Description {
		t.Fatalf("promptDescription = %#v", definition["promptDescription"])
	}
	if definition["eucliBoxCompatibility"] == nil {
		t.Fatalf("compatibility metadata was lost")
	}
}

func TestScaffoldRewritesGoImportsAndPackage(t *testing.T) {
	root := newScaffoldRoot(t)
	if _, err := scaffold(root, testInput()); err != nil {
		t.Fatalf("scaffold() error = %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(root, "tools", "demo_tool", "cmd", "demo_tool", "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	content := string(payload)
	if !strings.Contains(content, "eucli-box/tools/demo_tool/internal/demotool") {
		t.Fatalf("import path was not rewritten: %s", content)
	}
	if strings.Contains(content, templateDirectoryName) || strings.Contains(content, templatePackageName) {
		t.Fatalf("template identity leaked into main.go: %s", content)
	}
	runnerPayload, err := os.ReadFile(filepath.Join(root, "tools", "demo_tool", "internal", "demotool", "runner.go"))
	if err != nil {
		t.Fatalf("read runner.go: %v", err)
	}
	if !strings.Contains(string(runnerPayload), "package demotool") {
		t.Fatalf("package clause was not rewritten: %s", runnerPayload)
	}
}

func TestScaffoldRewritesDocuments(t *testing.T) {
	root := newScaffoldRoot(t)
	input := testInput()
	if _, err := scaffold(root, input); err != nil {
		t.Fatalf("scaffold() error = %v", err)
	}
	readme, err := os.ReadFile(filepath.Join(root, "tools", "demo_tool", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	if !strings.Contains(string(readme), "# demo_tool") || !strings.Contains(string(readme), input.Description) {
		t.Fatalf("README.md = %s", readme)
	}
	changelog, err := os.ReadFile(filepath.Join(root, "tools", "demo_tool", "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}
	if !strings.Contains(string(changelog), "## "+input.Version) || !strings.Contains(string(changelog), "demo_tool") {
		t.Fatalf("CHANGELOG.md = %s", changelog)
	}
}

func TestScaffoldRejectsExistingTarget(t *testing.T) {
	root := newScaffoldRoot(t)
	if _, err := scaffold(root, testInput()); err != nil {
		t.Fatalf("scaffold() error = %v", err)
	}
	if _, err := scaffold(root, testInput()); err == nil {
		t.Fatal("scaffold() should reject an existing target directory")
	}
}

func TestScaffoldKeepsTemplateIntact(t *testing.T) {
	root := newScaffoldRoot(t)
	if _, err := scaffold(root, testInput()); err != nil {
		t.Fatalf("scaffold() error = %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(root, "tools", templateDirectoryName, "tool.json"))
	if err != nil {
		t.Fatalf("read template tool.json: %v", err)
	}
	if !strings.Contains(string(payload), `"_tool_template"`) {
		t.Fatalf("template identity was modified: %s", payload)
	}
}

func TestDerivePackageName(t *testing.T) {
	cases := []struct {
		id      string
		want    string
		wantErr bool
	}{
		{id: "file_reader", want: "filereader"},
		{id: "context7", want: "context7"},
		{id: "shell-command", want: "shellcommand"},
		{id: "7zip", wantErr: true},
		{id: "type", wantErr: true},
	}
	for _, testCase := range cases {
		got, err := derivePackageName(testCase.id)
		if testCase.wantErr {
			if err == nil {
				t.Fatalf("derivePackageName(%q) error = nil, want failure", testCase.id)
			}
			continue
		}
		if err != nil || got != testCase.want {
			t.Fatalf("derivePackageName(%q) = %q, %v; want %q", testCase.id, got, err, testCase.want)
		}
	}
}
