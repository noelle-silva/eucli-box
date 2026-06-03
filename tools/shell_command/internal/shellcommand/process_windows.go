//go:build windows

package shellcommand

import (
	"os/exec"
	"strconv"
)

func configureProcess(cmd *exec.Cmd) {}

func terminateProcessTree(pid int) error {
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}
