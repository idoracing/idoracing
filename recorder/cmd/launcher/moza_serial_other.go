//go:build !linux

package main

import (
	"fmt"
	"os"
	"runtime"
)

func findMozaSerialPort() string {
	return ""
}

func configureMozaSerialPort(fd uintptr) error {
	return fmt.Errorf("direct CDC-ACM termios serial not supported on %s", runtime.GOOS)
}

func openMozaPortFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR, 0666)
}
