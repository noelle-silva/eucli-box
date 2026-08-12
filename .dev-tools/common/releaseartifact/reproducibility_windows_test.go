//go:build windows

package releaseartifact

import (
	"context"
	"reflect"
	"testing"
)

func TestBuildProducesSameArtifactFromSameSource(t *testing.T) {
	repositoryRoot := repositoryRootForTest(t)
	results := make([]BuildResult, 0, 2)
	for index := 0; index < 2; index++ {
		root := t.TempDir()
		result, err := Build(context.Background(), BuildOptions{
			Root:             repositoryRoot,
			Target:           "tool:context7",
			WorkRoot:         root + "\\work",
			OutputRoot:       root + "\\output",
			EvidenceRoot:     root + "\\evidence",
			VerificationOnly: true,
			AssetRoot:        root + "\\assets",
		})
		if err != nil {
			t.Fatalf("build artifact %d: %v", index+1, err)
		}
		if result.WorkDir == "" {
			t.Fatalf("build artifact %d did not return its owned workspace", index+1)
		}
		results = append(results, result)
	}
	if results[0].Manifest.Archive != results[1].Manifest.Archive {
		t.Fatalf("same source produced different archives: %#v != %#v", results[0].Manifest.Archive, results[1].Manifest.Archive)
	}
	if !reflect.DeepEqual(results[0].Manifest.Files, results[1].Manifest.Files) {
		t.Fatal("same source produced different package file records")
	}
}
