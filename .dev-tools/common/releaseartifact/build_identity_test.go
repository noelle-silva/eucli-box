package releaseartifact

import (
	"strings"
	"testing"

	"eucli-box/pkg/types"
)

// TestResolveBuildIdentityDevBuildBypassesRoster 钉住开发构建不查名册：
// 名册之外的发布物也能按 kind:id 直接解析并本地铺货。
func TestResolveBuildIdentityDevBuildBypassesRoster(t *testing.T) {
	identity, err := resolveBuildIdentity("tool:not-in-roster", true)
	if err != nil {
		t.Fatalf("resolveBuildIdentity() error = %v", err)
	}
	if identity.Kind != types.ReleaseArtifactKindTool || identity.ID != "not-in-roster" {
		t.Fatalf("identity = %#v", identity)
	}
	plugin, err := resolveBuildIdentity("plugin:not-in-roster", true)
	if err != nil {
		t.Fatalf("resolveBuildIdentity(plugin) error = %v", err)
	}
	if plugin.Kind != types.ReleaseArtifactKindPlugin {
		t.Fatalf("plugin identity = %#v", plugin)
	}
}

// TestResolveBuildIdentityDevBuildRejectsInvalidTarget 钉住开发构建仍校验身份格式。
func TestResolveBuildIdentityDevBuildRejectsInvalidTarget(t *testing.T) {
	for _, target := range []string{"eucli-box", "tool:", "box:whatever", "tool:../escape", "tool:Upper"} {
		if _, err := resolveBuildIdentity(target, true); err == nil {
			t.Fatalf("target %q must be rejected in dev build", target)
		}
	}
}

// TestResolveBuildIdentityFormalBuildKeepsRosterGate 钉住正式构建仍以名册为闸门：
// 名册外发布物拒绝，名册内发布物放行。
func TestResolveBuildIdentityFormalBuildKeepsRosterGate(t *testing.T) {
	if _, err := resolveBuildIdentity("tool:not-in-roster", false); err == nil {
		t.Fatal("formal build must reject an artifact outside the roster")
	} else if !strings.Contains(err.Error(), "白名单") {
		t.Fatalf("formal build rejection should mention the roster: %v", err)
	}
	identity, err := resolveBuildIdentity("tool:context7", false)
	if err != nil {
		t.Fatalf("resolveBuildIdentity(context7) error = %v", err)
	}
	if identity.Kind != types.ReleaseArtifactKindTool || identity.ID != "context7" {
		t.Fatalf("identity = %#v", identity)
	}
}
