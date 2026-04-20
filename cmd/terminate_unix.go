//go:build !windows

package cmd

import (
	"os"
	"syscall"
)

func terminateProcess(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}
