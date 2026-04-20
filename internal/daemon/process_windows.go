//go:build windows

package daemon

import (
	"os"

	"golang.org/x/sys/windows"
)

func processAlive(proc *os.Process) error {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(proc.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return err
	}
	if code != 259 { // STILL_ACTIVE = 259
		return os.ErrProcessDone
	}
	return nil
}
