//go:build windows

package llamafile

import (
	"os"
	"os/exec"
)

func prepareLaunchCommand(cmd *exec.Cmd) {}

func killManagedProcess(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	return proc.Kill()
}
