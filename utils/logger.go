package utils

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var logLock sync.Mutex
var logPool = sync.Pool{New: func() any { return bytes.NewBuffer(make([]byte, 0, 128)) }}

func log(calldepth int, level string, id string, params ...any) {
	var logBuf = logPool.Get().(*bytes.Buffer)
	defer logPool.Put(logBuf)

	logBuf.Reset()
	logBuf.WriteString(time.Now().Format("2006-01-02 15:04:05|"))

	logBuf.WriteString(level)
	logBuf.WriteString("|")

	pc, file, line, ok := runtime.Caller(calldepth)
	logBuf.WriteString(path.Base(file))
	logBuf.WriteString(":")
	logBuf.WriteString(strconv.Itoa(line))
	logBuf.WriteString("|")

	if ok {
		fn := runtime.FuncForPC(pc).Name()
		if idx := strings.LastIndex(fn, "."); idx != -1 {
			fn = fn[idx+1:]
		}
		logBuf.WriteString(fn)
		logBuf.WriteString("|")
	}

	if id != "" {
		logBuf.WriteString(id)
		logBuf.WriteString("|")
	}

	for i, p := range params {
		fmt.Fprintf(logBuf, "%+v", p)
		if i != len(params)-1 {
			logBuf.WriteString(" ")
		}
	}

	logBuf.WriteString("\n")

	logLock.Lock()
	defer logLock.Unlock()
	os.Stdout.Write(logBuf.Bytes())
}

type Logger struct {
	ID string
}

func (l *Logger) Log(calldepth int, level string, params ...any) {
	log(2+calldepth, level, l.ID, params...)
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
