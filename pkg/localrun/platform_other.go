//go:build !windows

package localrun

import "time"

func ProtectFileForCurrentUser(path string) error {
	return ErrWindowsOnly
}

func ProcessStartedAt(pid int) (time.Time, error) {
	return time.Time{}, ErrWindowsOnly
}

func ProcessMatches(pid int, startedAt time.Time) (bool, error) {
	return false, ErrWindowsOnly
}
