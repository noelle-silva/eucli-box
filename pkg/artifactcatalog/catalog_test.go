package artifactcatalog

import (
	"testing"

	"eucli-box/pkg/types"
)

func TestLoadReturnsCompleteReleaseRoster(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if catalog.Platform != types.ReleasePlatformWindowsX64 || len(catalog.Artifacts) != 10 {
		t.Fatalf("catalog = %#v", catalog)
	}
	for _, identity := range []types.ReleaseArtifactIdentity{
		{Kind: types.ReleaseArtifactKindTool, ID: "context7"},
		{Kind: types.ReleaseArtifactKindTool, ID: "shell_command"},
		{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"},
	} {
		if !catalog.Contains(identity) {
			t.Fatalf("catalog does not contain %#v", identity)
		}
	}
}

func TestResolveTargetRejectsNonReleaseArtifacts(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, target := range []string{"box:eucli-box", "tool:missing", "plugin:../escape"} {
		if _, err := catalog.ResolveTarget(target); err == nil {
			t.Fatalf("ResolveTarget(%q) error = nil", target)
		}
	}
	if _, err := catalog.ResolveTarget("tool:context7"); err != nil {
		t.Fatalf("ResolveTarget(tool:context7) error = %v", err)
	}
}
