//go:build !windows

package core

import "log"

func beepFallback(err error) {
	log.Printf("Error playing beep: %v", err)
}
