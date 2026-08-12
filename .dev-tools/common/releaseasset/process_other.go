//go:build !windows

package releaseasset

import "os/exec"

func hideProcessWindow(cmd *exec.Cmd) {}
