package logx

import (
	"log"
	"os"
)

var (
	infoLogger    = log.New(os.Stdout, "INFO ", log.LstdFlags|log.Lmicroseconds)
	eventLogger   = log.New(os.Stdout, "EVENT ", log.LstdFlags|log.Lmicroseconds)
	warnLogger    = log.New(os.Stdout, "WARN ", log.LstdFlags|log.Lmicroseconds)
	successLogger = log.New(os.Stdout, "OK ", log.LstdFlags|log.Lmicroseconds)
	errorLogger   = log.New(os.Stderr, "ERROR ", log.LstdFlags|log.Lmicroseconds)
)

// Init exists to keep callers stable; logging is configured via package vars.
func Init() {}

func Infof(format string, args ...any) {
	infoLogger.Printf(format, args...)
}

func Eventf(format string, args ...any) {
	eventLogger.Printf(format, args...)
}

func Warnf(format string, args ...any) {
	warnLogger.Printf(format, args...)
}

func Successf(format string, args ...any) {
	successLogger.Printf(format, args...)
}

func Errorf(format string, args ...any) {
	errorLogger.Printf(format, args...)
}

func Fatalf(format string, args ...any) {
	errorLogger.Printf(format, args...)
	os.Exit(1)
}
