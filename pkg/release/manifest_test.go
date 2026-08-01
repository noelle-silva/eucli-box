package release

import (
	"testing"

	"eucli-box/pkg/types"
)

func TestValidateReleaseManifestAcceptsCompleteToolRecord(t *testing.T) {
	manifest := testReleaseManifest()
	if err := ValidateReleaseManifest(manifest); err != nil {
		t.Fatalf("ValidateReleaseManifest() error = %v", err)
	}
}

func TestValidateReleaseManifestRejectsMissingCompatibility(t *testing.T) {
	manifest := testReleaseManifest()
	manifest.Compatibility = nil
	if err := ValidateReleaseManifest(manifest); err == nil {
		t.Fatal("ValidateReleaseManifest() error = nil")
	}
}

func TestValidateReleaseManifestRejectsExternalAssetWithoutPackagePath(t *testing.T) {
	manifest := testReleaseManifest()
	manifest.ExternalAssets = []types.ReleaseExternalAsset{{
		Name:       "runtime",
		Source:     "https://example.com/runtime",
		Version:    "1.0.0",
		FileCount:  1,
		TreeSHA256: SHA256([]byte("runtime")),
	}}
	if err := ValidateReleaseManifest(manifest); err == nil {
		t.Fatal("ValidateReleaseManifest() accepted an external asset without packagePath")
	}
}

func TestValidateArchiveDigestRejectsChangedPayload(t *testing.T) {
	payload := []byte("archive")
	manifest := testReleaseManifest()
	manifest.Archive = types.ReleaseFileRecord{Name: "tool.zip", Size: int64(len(payload)), SHA256: SHA256(payload)}
	if err := ValidateArchiveDigest(manifest, payload); err != nil {
		t.Fatalf("ValidateArchiveDigest() error = %v", err)
	}
	if err := ValidateArchiveDigest(manifest, []byte("changed")); err == nil {
		t.Fatal("ValidateArchiveDigest(changed) error = nil")
	}
}

func testReleaseManifest() types.ReleaseManifest {
	compatibility := types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}
	return types.ReleaseManifest{
		SchemaVersion:  ReleaseManifestSchemaVersion,
		Artifact:       types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"},
		Version:        "0.1.0",
		Platform:       types.ReleasePlatformWindowsX64,
		TagName:        "demo/v0.1.0",
		OfficialSource: "https://github.com/noelle-silva/eucli-box-ai-tools",
		Compatibility:  &compatibility,
		Source:         types.ReleaseSourceRecord{Repository: "https://github.com/noelle-silva/eucli-box", Commit: "0123456789abcdef0123456789abcdef01234567", Recorded: true},
		Archive:        types.ReleaseFileRecord{Name: "tool.zip", Size: 7, SHA256: SHA256([]byte("archive"))},
		Files:          []types.ReleaseFileRecord{{Name: "release-product.json", Size: 2, SHA256: SHA256([]byte("{}"))}},
	}
}
