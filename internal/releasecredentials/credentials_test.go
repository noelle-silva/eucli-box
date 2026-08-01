package releasecredentials

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

func TestLoadSelectsCredentialByReleaseKind(t *testing.T) {
	root := t.TempDir()
	writeCredentials(t, root, strings.Join([]string{
		"EUCLI_BOX_GITHUB_TOKEN=box-token",
		"EUCLI_TOOLS_GITHUB_TOKEN=tool-token",
		"EUCLI_PLUGINS_GITHUB_TOKEN=plugin-token",
	}, "\n")+"\n")
	credentials, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	tests := map[string]string{
		types.ReleaseArtifactKindBox:    "box-token",
		types.ReleaseArtifactKindTool:   "tool-token",
		types.ReleaseArtifactKindPlugin: "plugin-token",
	}
	for kind, want := range tests {
		got, err := credentials.TokenFor(kind)
		if err != nil || got != want {
			t.Fatalf("TokenFor(%q) = %q, %v; want %q", kind, got, err, want)
		}
	}
}

func TestLoadRejectsUnknownAndDuplicateFields(t *testing.T) {
	for name, content := range map[string]string{
		"unknown":   "EUCLI_BOX_GITHUB_TOKEN=a\nEUCLI_TOOLS_GITHUB_TOKEN=b\nEUCLI_PLUGINS_GITHUB_TOKEN=c\nOTHER=x\n",
		"duplicate": "EUCLI_BOX_GITHUB_TOKEN=a\nEUCLI_BOX_GITHUB_TOKEN=b\nEUCLI_TOOLS_GITHUB_TOKEN=c\nEUCLI_PLUGINS_GITHUB_TOKEN=d\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeCredentials(t, root, content)
			if _, err := Load(root); err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}

func TestTokenForRejectsEmptyCredentialWithoutExposingOthers(t *testing.T) {
	root := t.TempDir()
	writeCredentials(t, root, "EUCLI_BOX_GITHUB_TOKEN=\nEUCLI_TOOLS_GITHUB_TOKEN=private-tool-token\nEUCLI_PLUGINS_GITHUB_TOKEN=private-plugin-token\n")
	credentials, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	_, err = credentials.TokenFor(types.ReleaseArtifactKindBox)
	if err == nil || strings.Contains(err.Error(), "private-") {
		t.Fatalf("TokenFor() error = %v", err)
	}
}

func writeCredentials(t *testing.T, root string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create credentials directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
}
