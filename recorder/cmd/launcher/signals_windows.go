//go:build windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

func notifyInterruptSignals(sigChan chan os.Signal) {
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
}
