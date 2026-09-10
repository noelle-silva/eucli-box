package filereader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func TestReadReturnsLineWindowAndHash(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "notes.txt"), "alpha\nbeta\ngamma\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action": "read",
		"path":   "notes.txt",
		"offset": 2,
		"limit":  1,
	}))

	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if !strings.Contains(output.Content, "2: beta") {
		t.Fatalf("content did not include requested line: %q", output.Content)
	}
	if output.Metadata["hash"] == "" || output.Metadata["nextOffset"] != 3 {
		t.Fatalf("metadata = %#v", output.Metadata)
	}
}

func TestReadAbsolutePathOutsideBase(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "external.txt")
	writeTestFile(t, target, "alpha\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action": "read",
		"path":   target,
	}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if !strings.Contains(output.Content, "1: alpha") {
		t.Fatalf("outside read content = %q", output.Content)
	}
}

func TestRelativePathsWorkThroughSymlinkBaseDirectory(t *testing.T) {
	realRoot := t.TempDir()
	writeTestFile(t, filepath.Join(realRoot, "inside.txt"), "inside\n")
	linkRoot := filepath.Join(t.TempDir(), "linked-root")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	output := Execute(context.Background(), types.ToolExecutionInput{
		ActionID: "test-action",
		ToolName: "file_reader",
		Arguments: map[string]any{
			"action": "read",
			"path":   "inside.txt",
		},
		DefaultConfig:        map[string]any{},
		HostWorkingDirectory: linkRoot,
	})

	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if !strings.Contains(output.Content, "inside") {
		t.Fatalf("expected symlink base read content, got %q", output.Content)
	}
}

func TestSearchCanUseAbsolutePathOutsideBase(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	searchRoot := filepath.Join(outside, "search")
	writeTestFile(t, filepath.Join(searchRoot, "hit.txt"), "needle\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":  "glob",
		"path":    searchRoot,
		"pattern": "*.txt",
	}))

	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if !strings.Contains(output.Content, "hit.txt") {
		t.Fatalf("expected outside glob result, got %q", output.Content)
	}

	output = Execute(context.Background(), toolInput(root, map[string]any{
		"action": "grep",
		"path":   searchRoot,
		"query":  "needle",
	}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if !strings.Contains(output.Content, "hit.txt") {
		t.Fatalf("expected outside grep result, got %q", output.Content)
	}
}

func TestToolCallCannotOverrideMaxFileBytes(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "notes.txt"), "alpha\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":       "read",
		"path":         "notes.txt",
		"maxFileBytes": 999999999,
	}))

	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failed status, got %s", output.Status)
	}
	if !strings.Contains(output.Error, "configuration-only") {
		t.Fatalf("expected configuration-only error, got %q", output.Error)
	}
}

func TestGrepMarksTruncatedAtResultLimit(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a.txt"), "hit one\nhit two\n")
	input := toolInput(root, map[string]any{
		"action": "grep",
		"path":   ".",
		"query":  "hit",
	})
	input.DefaultConfig["maxSearchResults"] = 1

	output := Execute(context.Background(), input)

	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if output.Metadata["truncated"] != true {
		t.Fatalf("expected truncated search metadata, got %#v", output.Metadata)
	}
}

func TestGrepSearchesOrdinaryBuildDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "build", "artifact.txt"), "needle\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action": "grep",
		"path":   ".",
		"query":  "needle",
	}))

	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if !strings.Contains(output.Content, "build/artifact.txt") {
		t.Fatalf("expected build directory result, got %q", output.Content)
	}
}

func TestListDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "file1.txt"), "hello\n")
	writeTestFile(t, filepath.Join(root, "subdir", "file2.txt"), "world\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action": "list",
		"path":   ".",
	}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if !strings.Contains(output.Content, "file1.txt") || !strings.Contains(output.Content, "subdir/") {
		t.Fatalf("list output missing entries: %q", output.Content)
	}
}

func toolInput(root string, arguments map[string]any) types.ToolExecutionInput {
	return types.ToolExecutionInput{
		ActionID:             "test-action",
		ToolName:             "file_reader",
		Arguments:            arguments,
		DefaultConfig:        map[string]any{},
		HostWorkingDirectory: root,
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
