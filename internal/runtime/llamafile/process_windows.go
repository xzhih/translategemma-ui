//go:build windows

package llamafile

import (
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/windows"
)

const windowsStillActive = 259

func prepareLaunchCommand(cmd *exec.Cmd) {}

func killManagedProcess(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	return proc.Kill()
}

func managedProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == windowsStillActive
}

func waitManagedProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !managedProcessAlive(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return !managedProcessAlive(pid)
}
