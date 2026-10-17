//go:build unix

package procmon

import (
	"os"
	"os/exec"
	"syscall"
)

func launch(command string) (*exec.Cmd, *os.File, error) {
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	cmd := exec.Command("bash", "-c", command)
	cmd.Stdout = pw
	cmd.Stderr = pw
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, nil, err
	}
	if err := pw.Close(); err != nil {
		pr.Close()
		return nil, nil, err
	}
	return cmd, pr, nil
}

func signalGroup(pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}
