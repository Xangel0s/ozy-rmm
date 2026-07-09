//go:build !windows

package main

import "fmt"

func captureScreenJPEG(quality int) ([]byte, error) {
	return nil, fmt.Errorf("screen capture not implemented on this platform")
}

func handleScreenInput(payload string) {
	// no-op on non-Windows
}
