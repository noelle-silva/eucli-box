package filereader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

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

// TestRelativePathUsesWorkspaceBase 验证工作区注入时，相对路径以工作区
// 首个注册目录为基准解析，宿主目录不再参与。
func TestRelativePathUsesWorkspaceBase(t *testing.T) {
	hostDir := t.TempDir()
	workspaceDir := t.TempDir()
	writeTestFile(t, filepath.Join(workspaceDir, "target.txt"), "workspace-content\n")

	output := Execute(context.Background(), types.ToolExecutionInput{
		ActionID:             "test-action",
		ToolName:             "file_reader",
		Arguments:            map[string]any{"action": "read", "path": "target.txt"},
		DefaultConfig:        map[string]any{},
		HostWorkingDirectory: hostDir,
		Workspace:            &types.ToolWorkspaceContext{ID: "w1", Name: "工作区", Directories: []types.WorkspaceDirectory{{Path: workspaceDir, Alias: "work"}}},
	})
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if !strings.Contains(output.Content, "workspace-content") {
		t.Fatalf("content = %q", output.Content)
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

func TestGlobSemantics(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "root.txt"), "root\n")
	writeTestFile(t, filepath.Join(root, "sub", "nested.txt"), "nested\n")
	writeTestFile(t, filepath.Join(root, "sub", "deep", "inner.txt"), "inner\n")
	writeTestFile(t, filepath.Join(root, "upper.TXT"), "upper\n")

	cases := []struct {
		pattern string
		want    []string
	}{
		{"*.txt", []string{"root.txt"}},
		{"**/*.txt", []string{"root.txt", "sub/deep/inner.txt", "sub/nested.txt"}},
		{"sub/*.txt", []string{"sub/nested.txt"}},
		{"**/root.txt", []string{"root.txt"}},
		{"*.TXT", []string{"upper.TXT"}},
		{"sub/**", []string{"sub/", "sub/deep/", "sub/deep/inner.txt", "sub/nested.txt"}},
	}
	for _, testCase := range cases {
		output := Execute(context.Background(), toolInput(root, map[string]any{
			"action":  "glob",
			"path":    ".",
			"pattern": testCase.pattern,
		}))
		if output.Status != types.ToolStatusSuccess {
			t.Fatalf("glob %q status = %s, error = %s", testCase.pattern, output.Status, output.Error)
		}
		got := globResultPaths(t, output)
		if !slices.Equal(got, testCase.want) {
			t.Fatalf("glob %q = %v, want %v", testCase.pattern, got, testCase.want)
		}
	}
}

func TestGlobRejectsInvalidPattern(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a.txt"), "a\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":  "glob",
		"path":    ".",
		"pattern": "sub//a.txt",
	}))
	if output.Status != types.ToolStatusFailed {
		t.Fatalf("status = %s, want failed", output.Status)
	}
}

func TestReadContinuationFactsVisibleInContent(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "notes.txt"), "alpha\nbeta\ngamma\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action": "read",
		"path":   "notes.txt",
		"limit":  2,
	}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	envelope := lastEnvelopeLine(output.Content)
	for _, fragment := range []string{"action=read", "hash=" + output.Metadata["hash"].(string), "totalLines=3", "returnedLines=2", "nextOffset=3", "truncated=true"} {
		if !strings.Contains(envelope, fragment) {
			t.Fatalf("envelope %q missing %q", envelope, fragment)
		}
	}
}

func TestReadEnvelopeSurvivesOutputTruncation(t *testing.T) {
	root := t.TempDir()
	var builder strings.Builder
	for index := 0; index < 40; index++ {
		builder.WriteString("a long enough line of text to overflow the limit\n")
	}
	writeTestFile(t, filepath.Join(root, "big.txt"), builder.String())

	input := toolInput(root, map[string]any{
		"action": "read",
		"path":   "big.txt",
	})
	input.DefaultConfig["maxOutputChars"] = 160
	output := Execute(context.Background(), input)
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	envelope := lastEnvelopeLine(output.Content)
	if !strings.Contains(envelope, "truncated=true") {
		t.Fatalf("envelope did not survive truncation: %q", output.Content)
	}
	if output.Metadata["truncated"] != true {
		t.Fatalf("metadata truncated = %#v", output.Metadata["truncated"])
	}
}

func TestReadDecodesUTF8BOMAndUTF16(t *testing.T) {
	root := t.TempDir()
	bomFile := filepath.Join(root, "bom.txt")
	if err := os.WriteFile(bomFile, append([]byte{0xEF, 0xBB, 0xBF}, []byte("alpha\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	utf16File := filepath.Join(root, "utf16.txt")
	if err := os.WriteFile(utf16File, utf16LEBytes("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bomOutput := Execute(context.Background(), toolInput(root, map[string]any{"action": "read", "path": "bom.txt"}))
	if bomOutput.Status != types.ToolStatusSuccess {
		t.Fatalf("bom status = %s, error = %s", bomOutput.Status, bomOutput.Error)
	}
	if strings.Contains(bomOutput.Content, "\ufeff") || !strings.Contains(bomOutput.Content, "1: alpha") {
		t.Fatalf("bom content = %q", bomOutput.Content)
	}
	if bomOutput.Metadata["encoding"] != "utf-8-bom" {
		t.Fatalf("bom metadata = %#v", bomOutput.Metadata)
	}

	utf16Output := Execute(context.Background(), toolInput(root, map[string]any{"action": "read", "path": "utf16.txt"}))
	if utf16Output.Status != types.ToolStatusSuccess {
		t.Fatalf("utf16 status = %s, error = %s", utf16Output.Status, utf16Output.Error)
	}
	if !strings.Contains(utf16Output.Content, "1: hello") || utf16Output.Metadata["encoding"] != "utf-16le" {
		t.Fatalf("utf16 content = %q, metadata = %#v", utf16Output.Content, utf16Output.Metadata)
	}
}

func TestReadWarnsOnNonUTF8Text(t *testing.T) {
	root := t.TempDir()
	writeTestFileBytes(t, filepath.Join(root, "gbk.txt"), []byte{0xC4, 0xE3, 0xBA, 0xC3, '\n'})

	output := Execute(context.Background(), toolInput(root, map[string]any{"action": "read", "path": "gbk.txt"}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if !strings.Contains(output.Content, "[file_reader warning]") {
		t.Fatalf("content = %q", output.Content)
	}
	if output.Metadata["invalidUTF8"] != true || output.Metadata["utf8ReplacementCount"] != 4 {
		t.Fatalf("metadata = %#v", output.Metadata)
	}
}

func TestReadHashUsesRawBytesAndNormalizesDisplayNewlines(t *testing.T) {
	root := t.TempDir()
	raw := []byte("alpha\r\nbeta\r\n")
	writeTestFileBytes(t, filepath.Join(root, "crlf.txt"), raw)

	output := Execute(context.Background(), toolInput(root, map[string]any{"action": "read", "path": "crlf.txt"}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if strings.Contains(output.Content, "\r") {
		t.Fatalf("content kept carriage returns: %q", output.Content)
	}
	sum := sha256.Sum256(raw)
	if output.Metadata["hash"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash = %v, want raw-byte hash", output.Metadata["hash"])
	}
}

func TestReadDirectoryKeepsReadAction(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "notes.txt"), "alpha\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{"action": "read", "path": "."}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if output.Metadata["action"] != "read" {
		t.Fatalf("metadata action = %#v", output.Metadata["action"])
	}
	if !strings.Contains(lastEnvelopeLine(output.Content), "action=read") {
		t.Fatalf("content = %q", output.Content)
	}
}

func TestReadRejectsNonPositiveWindowArguments(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "notes.txt"), "alpha\n")

	for _, arguments := range []map[string]any{
		{"action": "read", "path": "notes.txt", "limit": 0},
		{"action": "read", "path": "notes.txt", "offset": 0},
		{"action": "list", "path": ".", "limit": -1},
	} {
		output := Execute(context.Background(), toolInput(root, arguments))
		if output.Status != types.ToolStatusFailed {
			t.Fatalf("arguments %v status = %s, want failed", arguments, output.Status)
		}
	}
}

func TestReadEmptyFileStillReturnsEnvelope(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "empty.txt"), "")

	output := Execute(context.Background(), toolInput(root, map[string]any{"action": "read", "path": "empty.txt"}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if !strings.Contains(output.Content, "[file_reader] action=read") {
		t.Fatalf("content = %q", output.Content)
	}
}

func TestGrepIncludeMatchesFileNameAtAnyDepth(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a.go"), "needle\n")
	writeTestFile(t, filepath.Join(root, "sub", "b.go"), "needle\n")
	writeTestFile(t, filepath.Join(root, "sub", "c.txt"), "needle\n")

	output := Execute(context.Background(), toolInput(root, map[string]any{
		"action":  "grep",
		"path":    ".",
		"query":   "needle",
		"include": "*.go",
	}))
	if output.Status != types.ToolStatusSuccess {
		t.Fatalf("status = %s, error = %s", output.Status, output.Error)
	}
	if !strings.Contains(output.Content, "a.go") || !strings.Contains(output.Content, "sub/b.go") || strings.Contains(output.Content, "c.txt") {
		t.Fatalf("content = %q", output.Content)
	}

	scoped := Execute(context.Background(), toolInput(root, map[string]any{
		"action":  "grep",
		"path":    ".",
		"query":   "needle",
		"include": "sub/*.go",
	}))
	if scoped.Status != types.ToolStatusSuccess {
		t.Fatalf("scoped status = %s, error = %s", scoped.Status, scoped.Error)
	}
	if strings.Contains(scoped.Content, "a.go") || !strings.Contains(scoped.Content, "sub/b.go") {
		t.Fatalf("scoped content = %q", scoped.Content)
	}
}

func globResultPaths(t *testing.T, output types.ToolExecutionOutput) []string {
	t.Helper()
	paths := []string{}
	for _, line := range strings.Split(output.Content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[file_reader]") {
			continue
		}
		_, rest, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("unexpected glob line %q", line)
		}
		path, _, _ := strings.Cut(rest, "\t")
		paths = append(paths, path)
	}
	return paths
}

func lastEnvelopeLine(content string) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}

func utf16LEBytes(text string) []byte {
	units := utf16.Encode([]rune(text))
	out := []byte{0xFF, 0xFE}
	for _, unit := range units {
		out = append(out, byte(unit), byte(unit>>8))
	}
	return out
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
