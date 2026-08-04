//go:build windows

package localrun

import "testing"

func TestAcquireDataLockExcludesSecondProcessHandle(t *testing.T) {
	dataDir := t.TempDir()
	first, err := AcquireDataLock(dataDir)
	if err != nil {
		t.Fatalf("first AcquireDataLock() error = %v", err)
	}
	defer first.Release()
	second, err := AcquireDataLock(dataDir)
	if second != nil || err == nil || err.Error() == "" {
		t.Fatalf("second AcquireDataLock() = %#v, %v; want an exclusive failure", second, err)
	}
}
