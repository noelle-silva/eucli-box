//go:build !windows

package localrun

import (
	"errors"
	"testing"
)

func TestAcquireDataLockRejectsUnsupportedPlatform(t *testing.T) {
	_, err := AcquireDataLock(t.TempDir())
	if !errors.Is(err, ErrWindowsOnly) {
		t.Fatalf("AcquireDataLock() error = %v, want ErrWindowsOnly", err)
	}
}
