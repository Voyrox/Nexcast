package logx

import (
	"log"
	"os"
	"strings"
	"sync"
)

var (
	initOnce sync.Once

	infoLogger    *log.Logger
	eventLogger   *log.Logger
	warnLogger    *log.Logger
	successLogger *log.Logger
	errorLogger   *log.Logger
)

const (
	ansiReset   = "\x1b[0m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
)

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func colorsEnabledFor(f *os.File) bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	if strings.TrimSpace(os.Getenv("FORCE_COLOR")) != "" {
		return true
	}
	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	if term == "dumb" {
		return false
	}
	return isTerminal(f)
}

func prefix(label, color string, enableColor bool) string {
	if !enableColor {
		return label + " "
	}
	return color + label + ansiReset + " "
}

func initLoggers() {
	stdoutColor := colorsEnabledFor(os.Stdout)
	stderrColor := colorsEnabledFor(os.Stderr)
	flags := log.LstdFlags | log.Lmicroseconds

	infoLogger = log.New(os.Stdout, prefix("INFO", ansiCyan, stdoutColor), flags)
	eventLogger = log.New(os.Stdout, prefix("EVENT", ansiMagenta, stdoutColor), flags)
	warnLogger = log.New(os.Stdout, prefix("WARN", ansiYellow, stdoutColor), flags)
	successLogger = log.New(os.Stdout, prefix("OK", ansiGreen, stdoutColor), flags)
	errorLogger = log.New(os.Stderr, prefix("ERROR", ansiRed, stderrColor), flags)
}

// Init configures loggers and is safe to call multiple times.
func Init() {
	initOnce.Do(initLoggers)
}

func ensureInit() {
	Init()
}

func Infof(format string, args ...any) {
	ensureInit()
	infoLogger.Printf(format, args...)
}

func Eventf(format string, args ...any) {
	ensureInit()
	eventLogger.Printf(format, args...)
}

func Warnf(format string, args ...any) {
	ensureInit()
	warnLogger.Printf(format, args...)
}

func Successf(format string, args ...any) {
	ensureInit()
	successLogger.Printf(format, args...)
}

func Errorf(format string, args ...any) {
	ensureInit()
	errorLogger.Printf(format, args...)
}

func Fatalf(format string, args ...any) {
	ensureInit()
	errorLogger.Printf(format, args...)
	os.Exit(1)
}
