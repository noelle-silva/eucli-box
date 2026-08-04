//go:build !windows

package localrun

import "os"

func atomicReplace(source string, target string) error {
	return os.Rename(source, target)
}
