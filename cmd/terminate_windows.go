//go:build windows

package cmd

import (
	"os"

	"golang.org/x/sys/windows"
)

func terminateProcess(proc *os.Process) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(proc.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.TerminateProcess(handle, 1)
}
