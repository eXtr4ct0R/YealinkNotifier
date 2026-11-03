//go:build windows

package core

import (
	"log"
	"syscall"
)

// mbOK: the MessageBeep sound choice.
const mbOK = 0

func beepFallback(err error) {
	u, loadErr := syscall.LoadLibrary("user32.dll")
	if loadErr != nil {
		log.Printf("Error playing beep: %v (also %v)", err, loadErr)
		return
	}
	defer syscall.FreeLibrary(u)
	proc, procErr := syscall.GetProcAddress(u, "MessageBeep")
	if procErr != nil || proc == 0 {
		log.Printf("Error playing beep: %v (also %v)", err, procErr)
		return
	}
	r, _, e := syscall.SyscallN(proc, mbOK)
	if r == 0 {
		log.Printf("Error playing beep: %v (also %v)", err, e)
	}
}
