//go:build windows

package contacts

import "os/exec"

func openFile(path string) error {
	// rundll32's FileProtocolHandler opens the file with its associated
	// application; the path is a separate argument so Go quotes spaces.
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // reap the child process
	return nil
}
