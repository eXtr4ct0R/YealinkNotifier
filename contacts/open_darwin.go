//go:build darwin

package contacts

import "os/exec"

func openFile(path string) error {
	cmd := exec.Command("open", path)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // reap the child process
	return nil
}
