package logx

import (
    "fmt"
    "log"
    "os"
    "strings"
    "sync/atomic"
)

type Level int32

const (
    Debug Level = iota
    Info
    Warn
    Error
)

var lvl atomic.Int32

func init() {
    // default to Info
    lvl.Store(int32(Info))
    log.SetOutput(os.Stdout)
    log.SetFlags(log.LstdFlags)
}

func SetLevelFromString(s string) {
    switch strings.ToLower(strings.TrimSpace(s)) {
    case "debug":
        lvl.Store(int32(Debug))
    case "info", "":
        lvl.Store(int32(Info))
    case "warn", "warning":
        lvl.Store(int32(Warn))
    case "err", "error":
        lvl.Store(int32(Error))
    default:
        lvl.Store(int32(Info))
    }
}

func Debugf(format string, args ...any) {
    if Level(lvl.Load()) <= Debug {
        log.Printf(prefix("DEBUG ")+format, args...)
    }
}

func Infof(format string, args ...any) {
    if Level(lvl.Load()) <= Info {
        log.Printf(prefix("INFO  ")+format, args...)
    }
}

func Warnf(format string, args ...any) {
    if Level(lvl.Load()) <= Warn {
        log.Printf(prefix("WARN  ")+format, args...)
    }
}

func Errorf(format string, args ...any) {
    if Level(lvl.Load()) <= Error {
        log.Printf(prefix("ERROR ")+format, args...)
    }
}

func prefix(level string) string { return fmt.Sprintf("[%s] ", level) }

