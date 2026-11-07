package artifactdiscovery

import (
	"os"
	"path/filepath"
	"testing"

	"eucli-box/pkg/types"
)

// writeFile 在测试目录里落一个文件，父目录按需创建。
func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestParseTargetAcceptsAnyValidIdentity 钉住「没有名单」这件事：
// 只要 kind 与标识合法就能发布，仓库里是否有它不归这里管。
func TestParseTargetAcceptsAnyValidIdentity(t *testing.T) {
	identity, err := ParseTarget("tool:subagent")
	if err != nil {
		t.Fatalf("ParseTarget() error = %v", err)
	}
	if identity.Kind != types.ReleaseArtifactKindTool || identity.ID != "subagent" {
		t.Fatalf("identity = %#v", identity)
	}
	plugin, err := ParseTarget(" plugin:time-plugin ")
	if err != nil {
		t.Fatalf("ParseTarget(plugin) error = %v", err)
	}
	if plugin.Kind != types.ReleaseArtifactKindPlugin || plugin.ID != "time-plugin" {
		t.Fatalf("plugin identity = %#v", plugin)
	}
}

// TestParseTargetRejectsMalformedTarget 验证格式仍然把关：非法形态不会蒙混过关。
func TestParseTargetRejectsMalformedTarget(t *testing.T) {
	for _, target := range []string{"", "subagent", "tool:", "box:whatever", "tool:../escape", "tool:Upper"} {
		if _, err := ParseTarget(target); err == nil {
			t.Fatalf("target %q must be rejected", target)
		}
	}
}

// TestDiscoverReadsRepositoryFacts 钉住发现口径：能发布什么由仓库里
// 实际存在的发布物决定，模板目录不算发布物。
func TestDiscoverReadsRepositoryFacts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "tools", "subagent", "tool.json"), `{"id":"subagent"}`)
	writeFile(t, filepath.Join(root, "tools", "file_reader", "tool.json"), `{"id":"file_reader"}`)
	writeFile(t, filepath.Join(root, "tools", "_tool_template", "tool.json"), `{"id":"_tool_template"}`)
	writeFile(t, filepath.Join(root, "system-plugins", "time-plugin", "manifest.json"), `{"id":"time-plugin"}`)

	identities, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(identities) != 3 {
		t.Fatalf("identities = %#v", identities)
	}
	if identities[0].Kind != types.ReleaseArtifactKindPlugin || identities[0].ID != "time-plugin" {
		t.Fatalf("first identity = %#v", identities[0])
	}
	if identities[1].ID != "file_reader" || identities[2].ID != "subagent" {
		t.Fatalf("tool identities = %#v", identities[1:])
	}
}

// TestDiscoverFailsOnBrokenManifest 验证清单缺 id 时当场报错，不静默跳过。
func TestDiscoverFailsOnBrokenManifest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "tools", "broken", "tool.json"), `{"name":"没有 id"}`)
	if _, err := Discover(root); err == nil {
		t.Fatal("manifest without id must be rejected")
	}
}
