//go:build !windows && !linux && !darwin

package contacts

import "errors"

func openFile(path string) error {
	return errors.New("opening files is not supported on this platform")
}
