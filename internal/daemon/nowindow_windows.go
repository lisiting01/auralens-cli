//go:build windows

package daemon

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func applyNoWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow,
	}
}
