//go:build !windows

package daemon

import (
	"os/exec"
	"syscall"
)

func applyNoWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
