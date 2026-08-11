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
	std  bool
}

func (t *logImpl) SetOut(logPath string, std bool) {
	const maxLogSize int64 = 10 * 1024 * 1024

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
	t.Clean()
	t.out = file
	t.std = std
}

func (t *logImpl) Clean() {
	if t.out != nil {
		t.out.Close()
	}
}

func (t *logImpl) output(calldepth int, level string, id string, params ...any) {
	var logBuf = t.pool.Get().(*bytes.Buffer)
	defer t.pool.Put(logBuf)

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

	t.lock.Lock()
	defer t.lock.Unlock()
	if t.out != nil {
		t.out.Write(logBuf.Bytes())
	}
	if t.std {
		os.Stdout.Write(logBuf.Bytes())
	}
}
