//go:build !windows

package localrun

func acquireDataLock(path string) (*DataLock, error) {
	return nil, ErrWindowsOnly
}
