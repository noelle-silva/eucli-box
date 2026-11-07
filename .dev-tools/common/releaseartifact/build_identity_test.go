package releaseartifact

import (
	"testing"

	"eucli-box/pkg/types"
)

// TestResolveBuildIdentityDevBuildParsesDirectly 钉住开发构建按 kind:id 直接解析：
// 仓库里新增的工具不需要任何登记就能本地铺货。
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

// TestResolveBuildIdentityFormalBuildHasNoRosterGate 钉住正式构建不再查名单：
// 任何合法的 kind:id 都能解析，能发布什么由仓库里存在的东西决定。
func TestResolveBuildIdentityFormalBuildHasNoRosterGate(t *testing.T) {
	identity, err := resolveBuildIdentity("tool:subagent", false)
	if err != nil {
		t.Fatalf("resolveBuildIdentity(subagent) error = %v", err)
	}
	if identity.Kind != types.ReleaseArtifactKindTool || identity.ID != "subagent" {
		t.Fatalf("identity = %#v", identity)
	}
	for _, target := range []string{"eucli-box", "tool:", "box:whatever"} {
		if _, err := resolveBuildIdentity(target, false); err == nil {
			t.Fatalf("target %q must still be rejected", target)
		}
	}
}
