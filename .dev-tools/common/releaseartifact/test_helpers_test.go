package releaseartifact

import (
	"os"
	"path/filepath"
	"testing"
)

func repositoryRootForTest(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	current, err := filepath.Abs(workingDirectory)
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("repository root not found from %s", workingDirectory)
		}
		current = parent
	}
}
