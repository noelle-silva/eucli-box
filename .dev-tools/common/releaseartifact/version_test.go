package releaseartifact

import (
	"os"
	"path/filepath"
	"testing"

	"eucli-box/pkg/types"
)

func TestNextDevelopmentVersionUsesOutputAreaHistory(t *testing.T) {
	outputRoot := t.TempDir()
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"}
	versionRoot := filepath.Join(outputRoot, outputDirectoryName(identity))
	for _, version := range []string{"0.1.0.1", "0.1.0.3", "0.1.9.6", "0.1.0.2"} {
		if err := os.MkdirAll(filepath.Join(versionRoot, version), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", version, err)
		}
	}
	next, err := NextDevelopmentVersion(outputRoot, identity, "0.1.0")
	if err != nil {
		t.Fatalf("NextDevelopmentVersion() error = %v", err)
	}
	if next != "0.1.0.4" {
		t.Fatalf("next version = %s, want 0.1.0.4", next)
	}
}

func TestNextDevelopmentVersionResetsTailOnNewBaseline(t *testing.T) {
	outputRoot := t.TempDir()
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "demo"}
	versionRoot := filepath.Join(outputRoot, outputDirectoryName(identity))
	if err := os.MkdirAll(filepath.Join(versionRoot, "0.1.9.6"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	next, err := NextDevelopmentVersion(outputRoot, identity, "0.2.0")
	if err != nil {
		t.Fatalf("NextDevelopmentVersion() error = %v", err)
	}
	if next != "0.2.0.1" {
		t.Fatalf("next version = %s, want 0.2.0.1", next)
	}
}

func TestNextDevelopmentVersionRejectsFormalSourceVersion(t *testing.T) {
	outputRoot := t.TempDir()
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"}
	if _, err := NextDevelopmentVersion(outputRoot, identity, "0.2.0.1"); err == nil {
		t.Fatal("NextDevelopmentVersion() should reject a development source version")
	}
}
