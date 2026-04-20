//go:build !windows

package daemon

import (
	"os"
	"syscall"
)

func processAlive(proc *os.Process) error {
	return proc.Signal(syscall.Signal(0))
}
