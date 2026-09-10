package fileeditor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func TestWriteAndEditAbsolutePathOutsideBase(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "external.txt")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":  "write",
		"path":    target,
		"content": "alpha\n",
	}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "alpha\n" {
		t.Fatalf("outside write content = %q", string(data))
	}

	output = Execute(context.Background(), toolInput(root, map[string]any{
		"action":    "edit",
		"path":      target,
		"oldString": "alpha",
		"newString": "beta",
	}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	data, err = os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "beta\n" {
		t.Fatalf("outside edit content = %q", string(data))
	}
}

func TestRelativeWriteWorksThroughSymlinkBaseDirectory(t *testing.T) {
	realRoot := t.TempDir()
	linkRoot := filepath.Join(t.TempDir(), "linked-root")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	output := Execute(context.Background(), types.ToolExecutionInput{
		ActionID: "test-action",
		ToolName: "file_editor",
		Arguments: map[string]any{
			"action":  "write",
			"path":    "inside.txt",
			"content": "inside\n",
		},
		DefaultConfig:        map[string]any{},
		HostWorkingDirectory: linkRoot,
	})

	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	data, err := os.ReadFile(filepath.Join(realRoot, "inside.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "inside\n" {
		t.Fatalf("symlink base write content = %q", string(data))
	}
}

func TestToolCallCannotOverrideMaxFileBytes(t *testing.T) {
	root := t.TempDir()

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":       "write",
		"path":         "notes.txt",
		"content":      "alpha\n",
		"maxFileBytes": 999999999,
	}))

	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failed status, got %s", output.Status)
	}
	if !strings.Contains(output.Error, "configuration-only") {
		t.Fatalf("expected configuration-only error, got %q", output.Error)
	}
}

func TestEditRequiresUniqueOldString(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "dupe.txt")
	writeTestFile(t, target, "same\nsame\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":    "edit",
		"path":      "dupe.txt",
		"oldString": "same",
		"newString": "changed",
	}))

	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failed status, got %s", output.Status)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "same\nsame\n" {
		t.Fatalf("file changed unexpectedly: %q", string(data))
	}
}

func TestWriteRejectsMalformedExpectedHash(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "guarded.txt")
	writeTestFile(t, target, "old\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":       "write",
		"path":         "guarded.txt",
		"content":      "new\n",
		"expectedHash": 123,
	}))

	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failed status, got %s", output.Status)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old\n" {
		t.Fatalf("malformed expectedHash allowed write: %q", string(data))
	}
}

func TestEditRejectsMalformedExpectedHash(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "guarded.txt")
	writeTestFile(t, target, "old\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":       "edit",
		"path":         "guarded.txt",
		"oldString":    "old",
		"newString":    "new",
		"expectedHash": 123,
	}))

	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failed status, got %s", output.Status)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old\n" {
		t.Fatalf("malformed expectedHash allowed edit: %q", string(data))
	}
}

func TestEditRejectsOversizedResult(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "limited.txt")
	writeTestFile(t, target, "old\n")
	input := toolInput(root, map[string]any{
		"action":    "edit",
		"path":      "limited.txt",
		"oldString": "old",
		"newString": strings.Repeat("x", 64),
	})
	input.DefaultConfig["maxFileBytes"] = 16

	output := Execute(context.Background(), input)

	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failed status, got %s", output.Status)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old\n" {
		t.Fatalf("oversized edit changed file unexpectedly: %q", string(data))
	}
}

func TestEditRejectsBinaryResult(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "text.txt")
	writeTestFile(t, target, "old\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":    "edit",
		"path":      "text.txt",
		"oldString": "old",
		"newString": "new\x00binary",
	}))

	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failed status, got %s", output.Status)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old\n" {
		t.Fatalf("binary edit changed file unexpectedly: %q", string(data))
	}
}

func TestApplyPatchRejectsBinaryResult(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "text.txt")
	writeTestFile(t, target, "old\n")
	patchText := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: text.txt",
		"@@",
		"-old",
		"+new\x00binary",
		"*** End Patch",
	}, "\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":    "apply_patch",
		"patchText": patchText,
	}))

	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failed status, got %s", output.Status)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old\n" {
		t.Fatalf("binary patch changed file unexpectedly: %q", string(data))
	}
}

func TestApplyPatchRejectsDeletingBinaryFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "blob.bin")
	original := []byte{'a', 0, 'b'}
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatal(err)
	}
	patchText := strings.Join([]string{
		"*** Begin Patch",
		"*** Delete File: blob.bin",
		"*** End Patch",
	}, "\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":    "apply_patch",
		"patchText": patchText,
	}))

	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failed status, got %s", output.Status)
	}
	if !strings.Contains(output.Error, "binary") {
		t.Fatalf("expected binary rejection error, got %q", output.Error)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("binary delete changed file unexpectedly: %q", string(data))
	}
}

func TestApplyPatchCanAddFileOutsideBase(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "patched.txt")
	patchText := strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: " + target,
		"+outside",
		"*** End Patch",
	}, "\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":    "apply_patch",
		"patchText": patchText,
	}))

	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "outside\n" {
		t.Fatalf("outside patch content = %q", string(data))
	}
}

func TestEditCreateRejectsMalformedExpectedHash(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "created.txt")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":       "edit",
		"path":         "created.txt",
		"oldString":    "",
		"newString":    "new\n",
		"expectedHash": 123,
	}))

	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failed status, got %s", output.Status)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("malformed expectedHash allowed edit create or stat failed unexpectedly: %v", err)
	}
}

func TestApplyPatchUpdatesFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "hello.txt")
	writeTestFile(t, target, "hello\nworld\n")

	patchText := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: hello.txt",
		"@@",
		" hello",
		"-world",
		"+there",
		"*** End Patch",
	}, "\n")
	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":    "apply_patch",
		"patchText": patchText,
	}))

	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\nthere\n" {
		t.Fatalf("patched content = %q", string(data))
	}
}

func TestApplyPatchRejectsAmbiguousUpdate(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "ambiguous.txt")
	writeTestFile(t, target, "same\nsame\n")
	patchText := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: ambiguous.txt",
		"@@",
		"-same",
		"+changed",
		"*** End Patch",
	}, "\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":    "apply_patch",
		"patchText": patchText,
	}))

	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failed status, got %s", output.Status)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "same\nsame\n" {
		t.Fatalf("ambiguous patch changed file unexpectedly: %q", string(data))
	}
}

func TestApplyPatchPlansBeforeWriting(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	writeTestFile(t, first, "alpha\n")
	patchText := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: first.txt",
		"@@",
		"-alpha",
		"+changed",
		"*** Update File: missing.txt",
		"@@",
		"-missing",
		"+changed",
		"*** End Patch",
	}, "\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":    "apply_patch",
		"patchText": patchText,
	}))

	if output.Status != types.ToolStatusFailed {
		t.Fatalf("expected failed status, got %s", output.Status)
	}
	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "alpha\n" {
		t.Fatalf("failed patch left partial write: %q", string(data))
	}
}

func toolInput(root string, arguments map[string]any) types.ToolExecutionInput {
	return types.ToolExecutionInput{
		ActionID:             "test-action",
		ToolName:             "file_editor",
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
