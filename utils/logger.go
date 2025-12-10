package utils

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"sync"
	"time"
)

var logLock sync.Mutex

func log(calldepth int, level string, id string, params ...any) {
	var now = time.Now().Format("2006-01-02 15:04:05")
	_, file, line, _ := runtime.Caller(calldepth)
	var msg string
	for i, p := range params {
		msg += fmt.Sprintf("%+v", p)
		if i != len(params)-1 {
			msg += " "
		}
	}
	logLock.Lock()
	defer logLock.Unlock()
	fmt.Fprintf(os.Stdout, "%s|%s|%s:%d|%s|%s\n", now, level, path.Base(file), line, id, msg)
}

type Logger struct {
	ID string
}

func (l *Logger) Log(calldepth int, level string, params ...any) {
	log(2+calldepth, level, l.ID, params)
}

func (l *Logger) Logf(calldepth int, level string, format string, params ...any) {
	log(2+calldepth, level, l.ID, fmt.Sprintf(format, params...))
}

func (l *Logger) Print(params ...any) {
	log(2, "inf", l.ID, params...)
}

func (l *Logger) Printf(format string, params ...any) {
	log(2, "inf", l.ID, fmt.Sprintf(format, params...))
}

func (l *Logger) Error(params ...any) {
	log(2, "err", l.ID, params...)
}

func (l *Logger) Errorf(format string, params ...any) {
	log(2, "err", l.ID, fmt.Sprintf(format, params...))
}
