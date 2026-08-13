//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func helperSuffix() string {
	return ".updater.exe"
}

func startDetached(executable string, args []string, workingDirectory string, extraEnvironment []string) (int, error) {
	command := exec.Command(executable, args...)
	command.Dir = workingDirectory
	command.Env = append(os.Environ(), extraEnvironment...)
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	if err := command.Start(); err != nil {
		return 0, err
	}
	processID := command.Process.Pid
	return processID, command.Process.Release()
}

func waitForProcessExit(processID int, timeout time.Duration) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(processID))
	if err != nil {
		if err == windows.ERROR_INVALID_PARAMETER {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(handle)
	waitMillis := uint32(timeout / time.Millisecond)
	result, err := windows.WaitForSingleObject(handle, waitMillis)
	if err != nil {
		return err
	}
	switch result {
	case windows.WAIT_OBJECT_0:
		return nil
	case uint32(windows.WAIT_TIMEOUT):
		return fmt.Errorf("process %d did not exit within %s", processID, timeout)
	default:
		return fmt.Errorf("unexpected process wait result 0x%x", result)
	}
}

func terminateProcess(processID int, timeout time.Duration) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(processID))
	if err != nil {
		if err == windows.ERROR_INVALID_PARAMETER {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(handle)
	if err := windows.TerminateProcess(handle, 1); err != nil {
		return err
	}
	result, err := windows.WaitForSingleObject(handle, uint32(timeout/time.Millisecond))
	if err != nil {
		return err
	}
	if result != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("process %d did not terminate", processID)
	}
	return nil
}
