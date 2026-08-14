package background

import (
	"fmt"
	"os"
	"os/exec"
)

// Start relaunches the current executable as a detached child and redirects its
// output to wget-log.
func Start(args []string) error {
	childArgs := make([]string, 0, len(args)+1)
	for _, arg := range args {
		if arg == "-B" {
			continue
		}
		childArgs = append(childArgs, arg)
	}
	childArgs = append(childArgs, "--background-child")

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	logFile, err := os.OpenFile("wget-log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open wget-log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(executable, childArgs...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start background download: %w", err)
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return nil
}
