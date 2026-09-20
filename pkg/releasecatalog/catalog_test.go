package releasecatalog

import (
	"testing"

	"eucli-box/pkg/types"
)

func TestLoadSourcesReturnsFixedOfficialSources(t *testing.T) {
	sources, err := LoadSources()
	if err != nil {
		t.Fatalf("LoadSources() error = %v", err)
	}
	if sources.Platform != types.ReleasePlatformWindowsX64 || len(sources.Sources) != 2 {
		t.Fatalf("sources = %#v", sources)
	}
	if sources.SourceRepository != "https://github.com/noelle-silva/eucli-box" {
		t.Fatalf("sourceRepository = %q", sources.SourceRepository)
	}
	for _, kind := range []string{types.ReleaseArtifactKindTool, types.ReleaseArtifactKindPlugin} {
		source, err := sources.SourceFor(kind)
		if err != nil {
			t.Fatalf("SourceFor(%s) error = %v", kind, err)
		}
		if source.Owner != "noelle-silva" || source.Name == "" {
			t.Fatalf("source = %#v", source)
		}
	}
	recordRepository, err := sources.RecordRepository()
	if err != nil {
		t.Fatalf("RecordRepository() error = %v", err)
	}
	if recordRepository != sources.SourceRepository {
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
