package main

import (
	"bytes"
	"strings"
	"testing"

	"devtools/common/releaseops"
)

func TestParseOptionsRejectsAmbiguousArguments(t *testing.T) {
	tests := [][]string{
		{"-message", "说明"},
		{"-version", "0.1.1", "-message", "说明"},
		{"unexpected"},
	}
	for _, args := range tests {
		if _, err := parseOptions(args); err == nil {
			t.Fatalf("parseOptions(%v) should fail", args)
		}
	}
}

func TestRunChecksOneRepositoryArtifact(t *testing.T) {
	artifact, err := releaseops.Resolve("../..", "eucli-box")
	if err != nil {
		t.Fatalf("releaseops.Resolve() error = %v", err)
	}
	var output bytes.Buffer
	if err := run([]string{"-root", "../..", "-target", "eucli-box"}, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(output.String(), "eucli-box "+artifact.Version) {
		t.Fatalf("output = %q", output.String())
	}
}
