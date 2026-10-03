package clipboard

import (
	"os/exec"
	"runtime"
)

func Copy(text string) error {
	switch runtime.GOOS {
	case "darwin":
		return pipe(text, "pbcopy")
	case "windows":
		return pipe(text, "clip")
	default:
		if err := pipe(text, "xclip", "-selection", "clipboard"); err == nil {
			return nil
		}
		return pipe(text, "xsel", "--clipboard", "--input")
	}
}

func pipe(text string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := stdin.Write([]byte(text)); err != nil {
		stdin.Close()
		return err
	}
	if err := stdin.Close(); err != nil {
		return err
	}
	return cmd.Wait()
}
