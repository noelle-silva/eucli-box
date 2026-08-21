//go:build windows

package toolcalling

import (
	"os/exec"
	"strconv"
)

func configureToolProcess(cmd *exec.Cmd) {}

func terminateToolProcessTree(pid int) error {
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}
