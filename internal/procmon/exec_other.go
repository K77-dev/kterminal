//go:build !unix

package procmon

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

var errUnsupported = fmt.Errorf("background processes are not supported on this platform")

func launch(command string) (*exec.Cmd, *os.File, error) {
	return nil, nil, errUnsupported
}

func signalGroup(pid int, sig syscall.Signal) error {
	return errUnsupported
}
