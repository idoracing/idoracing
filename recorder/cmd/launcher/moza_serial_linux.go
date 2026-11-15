//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// findMozaSerialPort discovers the Moza CDC-ACM serial port on the Linux host.
func findMozaSerialPort() string {
	matches, _ := filepath.Glob("/dev/serial/by-id/*MOZA*")
	if len(matches) > 0 {
		return matches[0]
	}
	matches, _ = filepath.Glob("/dev/serial/by-id/*Gudsen*")
	if len(matches) > 0 {
		return matches[0]
	}
	for i := 0; i <= 3; i++ {
		p := fmt.Sprintf("/dev/ttyACM%d", i)
		if fi, err := os.Stat(p); err == nil && (fi.Mode()&os.ModeDevice) != 0 {
			return p
		}
	}
	return ""
}

// configureMozaSerialPort configures raw mode 115200 8N1 on a Linux character device fd.
func configureMozaSerialPort(fd uintptr) error {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return errno
	}

	termios.Iflag = 0
	termios.Oflag = 0
	termios.Cflag = syscall.B115200 | syscall.CS8 | syscall.CREAD | syscall.CLOCAL
	termios.Lflag = 0
	termios.Cc[syscall.VMIN] = 0
	termios.Cc[syscall.VTIME] = 1

	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return errno
	}
	return nil
}

func openMozaPortFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK, 0666)
}
