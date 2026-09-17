package releasecatalog

import (
	"testing"

	"eucli-box/pkg/types"
)

func TestLoadReturnsCompleteFixedCatalog(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if catalog.Platform != types.ReleasePlatformWindowsX64 || len(catalog.Sources) != 2 || len(catalog.Artifacts) != 11 {
		t.Fatalf("catalog = %#v", catalog)
	}
	if catalog.SourceRepository != "https://github.com/noelle-silva/eucli-box" {
		t.Fatalf("sourceRepository = %q", catalog.SourceRepository)
	}
	for _, kind := range []string{types.ReleaseArtifactKindTool, types.ReleaseArtifactKindPlugin} {
		source, err := catalog.SourceFor(kind)
		if err != nil {
			t.Fatalf("SourceFor(%s) error = %v", kind, err)
		}
		if source.Owner != "noelle-silva" || source.Name == "" {
			t.Fatalf("source = %#v", source)
		}
	}
	recordRepository, err := catalog.RecordRepository()
	if err != nil {
		t.Fatalf("RecordRepository() error = %v", err)
	}
	if recordRepository != catalog.SourceRepository {
		t.Fatalf("RecordRepository() = %q", recordRepository)
	}
}

func TestTagNameKeepsIndependentArtifactIdentity(t *testing.T) {
	tests := []struct {
		identity types.ReleaseArtifactIdentity
		want     string
	}{
		{identity: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindTool, ID: "context7"}, want: "context7/v0.1.0"},
		{identity: types.ReleaseArtifactIdentity{Kind: types.ReleaseArtifactKindPlugin, ID: "time-plugin"}, want: "time-plugin/v0.1.0"},
	}
	for _, test := range tests {
		got, err := TagName(test.identity, "0.1.0")
		if err != nil {
			t.Fatalf("TagName(%#v) error = %v", test.identity, err)
		}
		if got != test.want {
			t.Fatalf("TagName(%#v) = %q, want %q", test.identity, got, test.want)
		}
	}
}

func TestResolveTargetRejectsNonReleaseArtifacts(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, target := range []string{"eucli-box", "eucli-studio", "tool:missing", "plugin:../escape"} {
		if _, err := catalog.ResolveTarget(target); err == nil {
			t.Fatalf("ResolveTarget(%q) error = nil", target)
		}
	}
}
