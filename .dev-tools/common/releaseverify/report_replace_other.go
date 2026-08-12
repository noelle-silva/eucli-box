//go:build !windows

package releaseverify

import "os"

func replaceReportFile(source string, destination string) error {
	return os.Rename(source, destination)
}
