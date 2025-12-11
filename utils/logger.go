package utils

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"sync"
	"time"
)

var logLock sync.Mutex

func log(calldepth int, level string, id string, params ...any) {
	var text = bytes.NewBuffer(make([]byte, 0, 128))
	text.WriteString(time.Now().Format("2006-01-02 15:04:05|"))

	text.WriteString(level)
	text.WriteString("|")

	text.WriteString(id)
	text.WriteString("|")

	pc, file, line, ok := runtime.Caller(calldepth)
	text.WriteString(path.Base(file))
	text.WriteString(":")
	text.WriteString(strconv.Itoa(line))
	text.WriteString("|")

	if ok {
		var fn = runtime.FuncForPC(pc).Name()
		for i := len(fn) - 1; i >= 0; i-- {
			if fn[i] == '.' {
				text.WriteString(fn[i+1:])
				text.WriteString("|")
				break
			}
		}
	}

	var l = text.Len()
	for i, p := range params {
		fmt.Fprintf(text, "%+v", p)
		if i != len(params)-1 {
			text.WriteString(" ")
		}
	}

	if text.Len() == l {
		return
	}

	logLock.Lock()
	defer logLock.Unlock()
	os.Stdout.Write(text.Bytes())
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
