//go:build windows

package localrun

import (
	"path/filepath"
	"testing"
)

func TestWritePrivateJSONAllowsCurrentUserAtomicReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.json")
	if err := WritePrivateJSON(path, map[string]string{"status": "first"}); err != nil {
		t.Fatalf("write first private JSON: %v", err)
	}
	if err := WritePrivateJSON(path, map[string]string{"status": "second"}); err != nil {
		t.Fatalf("replace private JSON: %v", err)
	}
}
