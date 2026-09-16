package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devtools/common/releaseops"
	"eucli-box/pkg/release"
)

func TestParseOptionsRejectsAmbiguousArguments(t *testing.T) {
	tests := [][]string{
		{"-message", "说明"},
		{"-version", "0.1.1", "-message", "说明"},
		{"unexpected"},
		{"-bump", "patch", "-message", "说明"},
		{"-version", "0.1.1", "-bump", "patch", "-target", "eucli-box", "-message", "说明"},
		{"-bump", "tiny", "-target", "eucli-box", "-message", "说明"},
	}
	for _, args := range tests {
		if _, err := parseOptions(args); err == nil {
			t.Fatalf("parseOptions(%v) should fail", args)
		}
	}
}

func TestParseOptionsAcceptsBumpLevels(t *testing.T) {
	for _, test := range []struct {
		text  string
		level release.VersionLevel
	}{
		{text: "patch", level: release.LevelPatch},
		{text: "minor", level: release.LevelMinor},
		{text: "major", level: release.LevelMajor},
	} {
		opts, err := parseOptions([]string{"-bump", test.text, "-target", "eucli-box", "-message", "例行版本递增"})
		if err != nil {
			t.Fatalf("parseOptions(%q) error = %v", test.text, err)
		}
		if opts.bump != test.level || opts.version != "" {
			t.Fatalf("options = %#v", opts)
		}
	}
}

func TestRunChecksOneRepositoryArtifact(t *testing.T) {
	artifact, err := releaseops.Resolve("../..", "eucli-box")
	if err != nil {
		t.Fatalf("releaseops.Resolve() error = %v", err)
	}
	var output bytes.Buffer
	if err := run([]string{"-root", "../..", "-target", "eucli-box"}, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(output.String(), "eucli-box "+artifact.Version) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRunBumpsEucliBoxVersionByLevel(t *testing.T) {
	for _, test := range []struct {
		level       string
		wantVersion string
	}{
		{level: "patch", wantVersion: "0.1.3"},
		{level: "minor", wantVersion: "0.2.0"},
		{level: "major", wantVersion: "1.0.0"},
	} {
		root := writeEucliBoxFixture(t)
		var output bytes.Buffer
		if err := run([]string{"-root", root, "-target", "eucli-box", "-bump", test.level, "-message", "例行版本递增"}, &output); err != nil {
			t.Fatalf("run(-bump %s) error = %v", test.level, err)
		}
		if !strings.Contains(output.String(), "0.1.2 调整为 "+test.wantVersion) {
			t.Fatalf("output = %q", output.String())
		}
		metadata := readVersionTestFile(t, filepath.Join(root, "internal", "boxrelease", "release.json"))
		if !strings.Contains(metadata, `"version": "`+test.wantVersion+`"`) {
			t.Fatalf("release.json = %s", metadata)
		}
		changelog := readVersionTestFile(t, filepath.Join(root, "CHANGELOG.md"))
		if !strings.Contains(changelog, "## "+test.wantVersion) || !strings.Contains(changelog, "例行版本递增") {
			t.Fatalf("CHANGELOG.md = %s", changelog)
		}
	}
}

func writeEucliBoxFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	metadataDirectory := filepath.Join(root, "internal", "boxrelease")
	if err := os.MkdirAll(metadataDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeVersionTestFile(t, filepath.Join(metadataDirectory, "release.json"), "{\n  \"version\": \"0.1.2\",\n  \"dataVersion\": \"1.0.0\"\n}\n")
	writeVersionTestFile(t, filepath.Join(root, "README.md"), "# eucli-box\n\n这是用于档位递增验证的中文说明文档。\n")
	writeVersionTestFile(t, filepath.Join(root, "CHANGELOG.md"), "# 更新记录\n\n## 0.1.2 - 2026-08-22\n\n- 建立测试。\n")
	return root
}

func writeVersionTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func readVersionTestFile(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(payload)
}
