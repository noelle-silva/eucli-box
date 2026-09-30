package releaseasset

import (
	"os"
	"path/filepath"
	"testing"
)

// toolSourcesExist identifies the primary repository root: the product tool
// source area and the development assets area must both be present.
func toolSourcesExist(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "tools")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, ".dev-tools", "go.mod")); err != nil {
		return false
	}
	return true
}

func findRepositoryRootForTest(t *testing.T) string {
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
		if toolSourcesExist(current) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("repository root not found from %s", workingDirectory)
		}
		current = parent
	}
}
