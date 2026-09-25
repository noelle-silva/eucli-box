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

// TestRelativeWriteUsesWorkspaceBase 验证工作区注入时，相对路径写入
// 落在工作区首个注册目录下，宿主目录不参与。
func TestRelativeWriteUsesWorkspaceBase(t *testing.T) {
	hostDir := t.TempDir()
	workspaceDir := t.TempDir()

	output := Execute(context.Background(), types.ToolExecutionInput{
		ActionID:             "test-action",
		ToolName:             "file_editor",
		Arguments:            map[string]any{"action": "write", "path": "inside.txt", "content": "workspace-write\n"},
		DefaultConfig:        map[string]any{},
		HostWorkingDirectory: hostDir,
		Workspace:            &types.ToolWorkspaceContext{ID: "w1", Name: "工作区", Directories: []types.WorkspaceDirectory{{Path: workspaceDir, Alias: "work"}}},
	})
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	data, err := os.ReadFile(filepath.Join(workspaceDir, "inside.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "workspace-write\n" {
		t.Fatalf("workspace write content = %q", string(data))
	}
	if _, err := os.Stat(filepath.Join(hostDir, "inside.txt")); err == nil {
		t.Fatal("relative write must not land in host working directory")
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

func TestEditAdaptsToCRLFLineEndings(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "crlf.txt")
	writeTestFileBytes(t, target, []byte("alpha\r\nbeta\r\ngamma\r\n"))

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":    "edit",
		"path":      "crlf.txt",
		"oldString": "beta\ngamma",
		"newString": "BETA\nGAMMA",
	}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "alpha\r\nBETA\r\nGAMMA\r\n" {
		t.Fatalf("content = %q", string(data))
	}
}

func TestApplyPatchHandlesSeparatedHunks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "hunks.txt")
	writeTestFile(t, target, "line1\nline2\nline3\nline4\nline5\nline6\n")
	patchText := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: hunks.txt",
		"@@",
		" line1",
		"-line2",
		"+LINE2",
		"@@",
		" line5",
		"-line6",
		"+LINE6",
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
	if string(data) != "line1\nLINE2\nline3\nline4\nline5\nLINE6\n" {
		t.Fatalf("content = %q", string(data))
	}
}

func TestApplyPatchTreatsBlankLineAsEmptyContext(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "blank.txt")
	writeTestFile(t, target, "alpha\n\nbeta\n")
	patchText := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: blank.txt",
		"@@",
		" alpha",
		"",
		"-beta",
		"+gamma",
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
	if string(data) != "alpha\n\ngamma\n" {
		t.Fatalf("content = %q", string(data))
	}
}

func TestApplyPatchKeepsCRLFFileStyle(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "crlf.txt")
	writeTestFileBytes(t, target, []byte("line1\r\nline2\r\nline3\r\n"))
	patchText := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: crlf.txt",
		"@@",
		" line1",
		"-line2",
		"+LINE2",
		"@@",
		"-line3",
		"+LINE3",
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
	if string(data) != "line1\r\nLINE2\r\nLINE3\r\n" {
		t.Fatalf("content = %q", string(data))
	}
}

func TestApplyPatchMatchesFileWithoutTrailingNewline(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "eof.txt")
	writeTestFile(t, target, "alpha\nbeta")
	patchText := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: eof.txt",
		"@@",
		"-alpha",
		"+ALPHA",
		"@@",
		"-beta",
		"+BETA",
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
	if string(data) != "ALPHA\nBETA" {
		t.Fatalf("content = %q", string(data))
	}
}

func TestApplyPatchMovesAndUpdates(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "source.txt"), "alpha\n")
	patchText := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: source.txt",
		"*** Move to: moved.txt",
		"@@",
		"-alpha",
		"+beta",
		"*** End Patch",
	}, "\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":    "apply_patch",
		"patchText": patchText,
	}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if _, err := os.Stat(filepath.Join(root, "source.txt")); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "moved.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "beta\n" {
		t.Fatalf("moved content = %q", string(data))
	}
}

func TestApplyPatchStrictSyntaxRejections(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a.txt"), "alpha\n")
	writeTestFile(t, filepath.Join(root, "b.txt"), "beta\n")
	cases := map[string]string{
		"trailing content": strings.Join([]string{
			"*** Begin Patch",
			"*** Update File: a.txt",
			"@@",
			"-alpha",
			"+changed",
			"*** End Patch",
			"junk",
		}, "\n"),
		"separator with content": strings.Join([]string{
			"*** Begin Patch",
			"*** Update File: a.txt",
			"@@ section one",
			"-alpha",
			"+changed",
			"*** End Patch",
		}, "\n"),
		"move on add": strings.Join([]string{
			"*** Begin Patch",
			"*** Add File: added.txt",
			"*** Move to: other.txt",
			"+data",
			"*** End Patch",
		}, "\n"),
		"delete with content": strings.Join([]string{
			"*** Begin Patch",
			"*** Delete File: b.txt",
			"+data",
			"*** End Patch",
		}, "\n"),
		"same file twice": strings.Join([]string{
			"*** Begin Patch",
			"*** Update File: a.txt",
			"@@",
			"-alpha",
			"+changed",
			"*** Update File: a.txt",
			"@@",
			"-changed",
			"+again",
			"*** End Patch",
		}, "\n"),
	}
	for name, patchText := range cases {
		output := Execute(context.Background(), toolInput(root, map[string]any{
			"action":    "apply_patch",
			"patchText": patchText,
		}))
		if output.Status != types.ToolStatusFailed {
			t.Fatalf("%s: status = %s, want failed", name, output.Status)
		}
	}
}

func TestWriteExpectedHashRequiresExistingTarget(t *testing.T) {
	root := t.TempDir()
	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":       "write",
		"path":         "created.txt",
		"content":      "new\n",
		"expectedHash": strings.Repeat("0", 64),
	}))
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("status = %s, want failed", output.Status)
	}
	if !strings.Contains(output.Error, "does not exist") {
		t.Fatalf("error = %q", output.Error)
	}
	if _, err := os.Stat(filepath.Join(root, "created.txt")); !os.IsNotExist(err) {
		t.Fatalf("file was created despite hash guard: %v", err)
	}
}

func TestEditCreateRejectsExpectedHashOnMissingTarget(t *testing.T) {
	root := t.TempDir()
	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":       "edit",
		"path":         "created.txt",
		"oldString":    "",
		"newString":    "new\n",
		"expectedHash": strings.Repeat("0", 64),
	}))
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("status = %s, want failed", output.Status)
	}
	if _, err := os.Stat(filepath.Join(root, "created.txt")); !os.IsNotExist(err) {
		t.Fatalf("file was created despite hash guard: %v", err)
	}
}

func TestHashMismatchReportsCurrentHash(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "guarded.txt")
	writeTestFile(t, target, "old\n")
	wrong := strings.Repeat("0", 64)
	wantCurrent := hashBytes([]byte("old\n"))

	writeOutput := Execute(context.Background(), toolInput(root, map[string]any{
		"action":       "write",
		"path":         "guarded.txt",
		"content":      "new\n",
		"expectedHash": wrong,
	}))
	if writeOutput.Status != types.ToolStatusFailed || writeOutput.Metadata["currentHash"] != wantCurrent {
		t.Fatalf("write status = %s, metadata = %#v", writeOutput.Status, writeOutput.Metadata)
	}

	editOutput := Execute(context.Background(), toolInput(root, map[string]any{
		"action":       "edit",
		"path":         "guarded.txt",
		"oldString":    "old",
		"newString":    "new",
		"expectedHash": wrong,
	}))
	if editOutput.Status != types.ToolStatusFailed || editOutput.Metadata["currentHash"] != wantCurrent {
		t.Fatalf("edit status = %s, metadata = %#v", editOutput.Status, editOutput.Metadata)
	}
}

func TestWriteLeavesNoTemporaryFiles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "notes.txt")
	writeTestFile(t, target, "old\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":  "write",
		"path":    "notes.txt",
		"content": "new\n",
	}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "notes.txt" {
		t.Fatalf("leftover files: %v", entryNames(entries))
	}
}

func TestWriteRejectsNullByteContent(t *testing.T) {
	root := t.TempDir()
	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":  "write",
		"path":    "binary.txt",
		"content": "a\x00b",
	}))
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("status = %s, want failed", output.Status)
	}
	if _, err := os.Stat(filepath.Join(root, "binary.txt")); !os.IsNotExist(err) {
		t.Fatalf("null-byte content was written: %v", err)
	}
}

func TestEditEnvelopeCarriesHashes(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "notes.txt")
	writeTestFile(t, target, "old\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":    "edit",
		"path":      "notes.txt",
		"oldString": "old",
		"newString": "new",
	}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	envelope := lastEnvelopeLine(output.Content)
	for _, fragment := range []string{"action=edit", "replacements=1", "hash=" + output.Metadata["hash"].(string), "previousHash=" + hashBytes([]byte("old\n"))} {
		if !strings.Contains(envelope, fragment) {
			t.Fatalf("envelope %q missing %q", envelope, fragment)
		}
	}
}

func TestApplyPatchEnvelopeReportsCounts(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a.txt"), "alpha\n")
	patchText := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: a.txt",
		"@@",
		"-alpha",
		"+changed",
		"*** End Patch",
	}, "\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":    "apply_patch",
		"patchText": patchText,
	}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	envelope := lastEnvelopeLine(output.Content)
	if !strings.Contains(envelope, "action=apply_patch") || !strings.Contains(envelope, "operations=1") || !strings.Contains(envelope, "changedPaths=1") {
		t.Fatalf("envelope = %q", envelope)
	}
}

func TestWriteEnvelopeCarriesHash(t *testing.T) {
	root := t.TempDir()
	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":  "write",
		"path":    "notes.txt",
		"content": "alpha\n",
	}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	envelope := lastEnvelopeLine(output.Content)
	for _, fragment := range []string{"action=write", "created=true", "bytes=6", "hash=" + output.Metadata["hash"].(string)} {
		if !strings.Contains(envelope, fragment) {
			t.Fatalf("envelope %q missing %q", envelope, fragment)
		}
	}
}

func lastEnvelopeLine(content string) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func writeTestFileBytes(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
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
