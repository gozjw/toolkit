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

func init() {
	LogImpl = &logImpl{
		pool: sync.Pool{New: func() any { return bytes.NewBuffer(make([]byte, 0, 128)) }},
	}
}

var LogImpl *logImpl

type logImpl struct {
	lock sync.Mutex
	pool sync.Pool
	out  *os.File
}

func (l *logImpl) SetOut(logPath string) {
	const maxLogSize int64 = 10 * 1024 * 1024 // 10MB

	var flag int = os.O_CREATE | os.O_WRONLY
	stat, err := os.Stat(logPath)
	if err == nil {
		if stat.Size() >= maxLogSize {
			flag |= os.O_TRUNC
		} else {
			flag |= os.O_APPEND
		}
	} else {
		flag |= os.O_APPEND
	}

	file, err := os.OpenFile(logPath, flag, 0644)
	if err != nil {
		panic(err)
	}

	l.out = file
}

func (l *logImpl) Clean() {
	if l.out != nil {
		l.out.Close()
	}
}

func (l *logImpl) output(calldepth int, level string, id string, params ...any) {
	var logBuf = l.pool.Get().(*bytes.Buffer)
	defer l.pool.Put(logBuf)

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

	l.lock.Lock()
	defer l.lock.Unlock()
	if l.out != nil {
		l.out.Write(logBuf.Bytes())
	}
	os.Stdout.Write(logBuf.Bytes())
}

type Logger struct {
	ID string
}

func (l *Logger) Log(calldepth int, level string, params ...any) {
	LogImpl.output(2+calldepth, level, l.ID, params...)
}

func (l *Logger) Logf(calldepth int, level string, format string, params ...any) {
	LogImpl.output(2+calldepth, level, l.ID, fmt.Sprintf(format, params...))
}

func (l *Logger) Print(params ...any) {
	LogImpl.output(2, "inf", l.ID, params...)
}

func (l *Logger) Printf(format string, params ...any) {
	LogImpl.output(2, "inf", l.ID, fmt.Sprintf(format, params...))
}

func (l *Logger) Error(params ...any) {
	LogImpl.output(2, "err", l.ID, params...)
}

func (l *Logger) Errorf(format string, params ...any) {
	LogImpl.output(2, "err", l.ID, fmt.Sprintf(format, params...))
}
