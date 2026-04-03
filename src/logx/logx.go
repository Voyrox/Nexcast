package logx

import (
	"fmt"
	"log"
	"os"
)

const (
	reset  = "\x1b[0m"
	cyan   = "\x1b[36m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
	blue   = "\x1b[34m"
)

func Init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime)
}

func Infof(format string, args ...any) {
	log.Printf(colorize("INFO", blue, format), args...)
}

func Successf(format string, args ...any) {
	log.Printf(colorize("OK", green, format), args...)
}

func Warnf(format string, args ...any) {
	log.Printf(colorize("WARN", yellow, format), args...)
}

func Errorf(format string, args ...any) {
	log.Printf(colorize("ERR", red, format), args...)
}

func Eventf(format string, args ...any) {
	log.Printf(colorize("NODE", cyan, format), args...)
}

func Fatalf(format string, args ...any) {
	log.Fatalf(colorize("FATAL", red, format), args...)
}

func Fatal(message string) {
	log.Fatal(colorizeLiteral("FATAL", red, message))
}

func colorize(level, color, format string) string {
	return fmt.Sprintf("%s[%s]%s %s", color, level, reset, format)
}

func colorizeLiteral(level, color, message string) string {
	return fmt.Sprintf("%s[%s]%s %s", color, level, reset, message)
}
