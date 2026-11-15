package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

type Logger struct {
	mu       sync.Mutex
	file     *os.File
	out      io.Writer
	isTTY    bool
	minLevel LogLevel
}

func NewLogger(filePath string, minLevel LogLevel) (*Logger, error) {
	var file *os.File
	if filePath != "" {
		if dir := filepath.Dir(filePath); dir != "" {
			_ = os.MkdirAll(dir, 0755)
		}
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("open log file %s: %w", filePath, err)
		}
		file = f
	}

	isTTY := false
	if fi, err := os.Stdout.Stat(); err == nil {
		isTTY = (fi.Mode() & os.ModeCharDevice) != 0
	}

	return &Logger{
		file:     file,
		out:      os.Stdout,
		isTTY:    isTTY,
		minLevel: minLevel,
	}, nil
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

func (l *Logger) log(level LogLevel, subsys, format string, args ...any) {
	if level < l.minLevel {
		return
	}

	now := time.Now().Format("2006/01/02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	msg = strings.TrimRight(msg, "\r\n")

	var levelStr, colorStart, colorEnd string
	switch level {
	case LevelDebug:
		levelStr = "DEBUG"
		colorStart = "\033[90m"
	case LevelInfo:
		levelStr = "INFO "
		colorStart = "\033[36m"
	case LevelWarn:
		levelStr = "WARN "
		colorStart = "\033[33m"
	case LevelError:
		levelStr = "ERROR"
		colorStart = "\033[31m"
	}
	colorEnd = "\033[0m"

	subsysTag := fmt.Sprintf("%-10s", fmt.Sprintf("[%s]", subsys))

	plainLine := fmt.Sprintf("%s %s %s %s\n", now, levelStr, subsysTag, msg)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Write plain text to log file
	if l.file != nil {
		_, _ = l.file.WriteString(plainLine)
	}

	// Write to terminal (with ANSI colors if TTY)
	if l.out != nil {
		if l.isTTY {
			coloredLine := fmt.Sprintf("%s%s %s%s %s%s%s %s\n",
				colorStart, now, levelStr, colorEnd,
				"\033[1m", subsysTag, colorEnd,
				msg,
			)
			_, _ = l.out.Write([]byte(coloredLine))
		} else {
			_, _ = l.out.Write([]byte(plainLine))
		}
	}
}

func (l *Logger) Info(subsys, format string, args ...any) {
	l.log(LevelInfo, subsys, format, args...)
}

func (l *Logger) Warn(subsys, format string, args ...any) {
	l.log(LevelWarn, subsys, format, args...)
}

func (l *Logger) Error(subsys, format string, args ...any) {
	l.log(LevelError, subsys, format, args...)
}

func (l *Logger) Debug(subsys, format string, args ...any) {
	l.log(LevelDebug, subsys, format, args...)
}

// SubsysWriter returns an io.Writer that logs every written line under the specified subsystem.
type SubsysWriter struct {
	logger *Logger
	subsys string
	buf    []byte
	mu     sync.Mutex
}

func (l *Logger) SubsysWriter(subsys string) *SubsysWriter {
	return &SubsysWriter{
		logger: l,
		subsys: subsys,
	}
}

func (w *SubsysWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf = append(w.buf, p...)
	for {
		idx := -1
		for i, b := range w.buf {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx == -1 {
			break
		}
		line := string(w.buf[:idx])
		w.buf = w.buf[idx+1:]
		line = strings.TrimRight(line, "\r")
		if line != "" {
			w.logger.Info(w.subsys, "%s", line)
		}
	}
	return len(p), nil
}
