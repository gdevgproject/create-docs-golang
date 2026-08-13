//go:build !windows

package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func helperSuffix() string {
	return ".updater"
}

func startDetached(executable string, args []string, workingDirectory string, extraEnvironment []string) (int, error) {
	command := exec.Command(executable, args...)
	command.Dir = workingDirectory
	command.Env = append(os.Environ(), extraEnvironment...)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return 0, err
	}
	processID := command.Process.Pid
	return processID, command.Process.Release()
}

func waitForProcessExit(processID int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Kill(processID, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("process %d did not exit within %s", processID, timeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func terminateProcess(processID int, timeout time.Duration) error {
	err := syscall.Kill(processID, syscall.SIGTERM)
	if err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	if err := waitForProcessExit(processID, timeout); err == nil {
		return nil
	}
	if err := syscall.Kill(processID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return waitForProcessExit(processID, 5*time.Second)
}
