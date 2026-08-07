package releasecheck

import (
	"context"
	"strings"
	"testing"

	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

func TestLatestCandidateBindsIdentityVersionAndSource(t *testing.T) {
	fixture := newGitHubFixture(t)
	identity := types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}
	fixture.addIndexVersion(identity, "0.1.1")
	fixture.addIndexVersion(identity, "0.1.2")
	checker := fixture.checker(t)
	candidate, err := checker.LatestCandidate(context.Background(), identity)
	if err != nil {
		t.Fatalf("LatestCandidate() error = %v", err)
	}
	if candidate.Artifact != identity || candidate.Version != "0.1.2" {
		t.Fatalf("candidate = %#v", candidate)
	}
	if candidate.Compatibility == nil || candidate.SourceRevision == "" {
		t.Fatalf("candidate facts missing: %#v", candidate)
	}
	if candidate.PublishedAt.IsZero() {
		t.Fatalf("candidate publishedAt missing")
	}
	source, err := candidate.PackageSource()
	if err != nil {
		t.Fatalf("PackageSource() error = %v", err)
	}
	if source.Artifact != identity || source.Product.Version != "0.1.2" {
		t.Fatalf("source = %#v", source)
	}
	if strings.TrimSpace(source.ArchiveURL) == "" || source.SizeBytes <= 0 || len(source.SHA256) != 64 {
		t.Fatalf("source facts missing: %#v", source)
	}
	if fixture.sourceRequests[identity.Kind] != 1 {
		t.Fatalf("source requests = %d, want 1", fixture.sourceRequests[identity.Kind])
	}
}

func TestLatestCandidateRejectsForeignIdentity(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.2")
	checker := fixture.checker(t)
	if _, err := checker.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "not-listed"}); err == nil {
		t.Fatal("LatestCandidate() with foreign identity error = nil")
	}
}

func TestLatestCandidateRejectsMissingArtifactInIndex(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.2")
	checker := fixture.checker(t)
	if _, err := checker.LatestCandidate(context.Background(), types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "zhihu_search"}); err == nil {
		t.Fatal("LatestCandidate() for artifact missing in index error = nil")
	}
}

func TestPackageSourceRejectsInvalidCandidate(t *testing.T) {
	empty := ReleaseCandidate{}
	if _, err := empty.PackageSource(); err == nil {
		t.Fatal("PackageSource() with empty candidate error = nil")
	}
	compatibility := types.EucliBoxCompatibility{MinimumVersion: "0.1.0", MaximumVersionExclusive: "0.2.0"}
	valid := ReleaseCandidate{
		Artifact:         types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "demo"},
		Version:          "0.1.0",
		SourceRevision:   "0123456789abcdef0123456789abcdef01234567",
		SourceRepository: "https://github.com/noelle-silva/eucli-box",
		Compatibility:    &compatibility,
		OfficialSource:   "https://github.com/noelle-silva/eucli-box-ai-tools",
		ArchiveURL:       "https://github.com/noelle-silva/eucli-box-ai-tools/releases/download/demo/v0.1.0/demo_0.1.0_windows-x64.zip",
		SizeBytes:        123,
		SHA256:           release.SHA256([]byte("archive")),
	}
	source, err := valid.PackageSource()
	if err != nil {
		t.Fatalf("PackageSource() error = %v", err)
	}
	if source.Artifact != valid.Artifact || source.Product.Version != "0.1.0" {
		t.Fatalf("source = %#v", source)
	}
	bad := valid
	bad.ArchiveURL = ""
	if _, err := bad.PackageSource(); err == nil {
		t.Fatal("PackageSource() with empty archive url error = nil")
	}
	bad = valid
	bad.SHA256 = ""
	if _, err := bad.PackageSource(); err == nil {
		t.Fatal("PackageSource() with empty sha256 error = nil")
	}
	bad = valid
	bad.SourceRevision = ""
	if _, err := bad.PackageSource(); err == nil {
		t.Fatal("PackageSource() with empty source revision error = nil")
	}
}

func TestCheckDoesNotTreatCandidateAsInstalled(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.2")
	checker := fixture.checker(t)
	snapshot := checker.Check(context.Background(), nil, "0.1.0")
	result := findCheckResult(t, snapshot, "tool", "context7")
	if result.Installed || result.CurrentVersion != "" {
		t.Fatalf("result = %#v", result)
	}
	if !result.UpdateAvailable {
		t.Fatalf("result updateAvailable = false, want true")
	}
}

func TestCheckSkipsInstalledInputWithInvalidVersion(t *testing.T) {
	fixture := newGitHubFixture(t)
	fixture.addIndexVersion(types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, "0.1.2")
	checker := fixture.checker(t)
	snapshot := checker.Check(context.Background(), []InstalledArtifact{
		{Artifact: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, Version: "not-a-version"},
	}, "0.1.0")
	result := findCheckResult(t, snapshot, "tool", "context7")
	if result.Installed {
		t.Fatalf("result = %#v", result)
	}
}
