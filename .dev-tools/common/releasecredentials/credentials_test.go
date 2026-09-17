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
		"EUCLI_TOOLS_GITHUB_TOKEN=tool-token",
		"EUCLI_PLUGINS_GITHUB_TOKEN=plugin-token",
	}, "\n")+"\n")
	credentials, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	tests := map[string]string{
		types.ReleaseArtifactKindTool:   "tool-token",
		types.ReleaseArtifactKindPlugin: "plugin-token",
	}
	for kind, want := range tests {
		got, err := credentials.TokenFor(kind)
		if err != nil || got != want {
			t.Fatalf("TokenFor(%q) = %q, %v; want %q", kind, got, err, want)
		}
	}
	if _, err := credentials.TokenFor(types.ReleaseArtifactKindBox); err == nil {
		t.Fatal("TokenFor(eucli-box) error = nil")
	}
}

func TestLoadRejectsUnknownAndDuplicateFields(t *testing.T) {
	for name, content := range map[string]string{
		"unknown":   "EUCLI_TOOLS_GITHUB_TOKEN=b\nEUCLI_PLUGINS_GITHUB_TOKEN=c\nEUCLI_BOX_GITHUB_TOKEN=x\n",
		"duplicate": "EUCLI_TOOLS_GITHUB_TOKEN=c\nEUCLI_TOOLS_GITHUB_TOKEN=d\nEUCLI_PLUGINS_GITHUB_TOKEN=e\n",
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
	writeCredentials(t, root, "EUCLI_TOOLS_GITHUB_TOKEN=\nEUCLI_PLUGINS_GITHUB_TOKEN=private-plugin-token\n")
	credentials, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	_, err = credentials.TokenFor(types.ReleaseArtifactKindTool)
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
