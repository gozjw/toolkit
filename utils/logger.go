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

const maxLogSize int = 10 * 1024 * 1024

var LogImpl *logImpl

type logImpl struct {
	lock sync.Mutex
	pool sync.Pool
	out  *os.File
	std  bool
	size int
	path string
}

func init() {
	LogImpl = &logImpl{
		pool: sync.Pool{New: func() any { return bytes.NewBuffer(make([]byte, 0, 128)) }},
	}
}

func (t *logImpl) SetOut(logPath string, std bool) {
	backupPath := logPath + ".1"
	t.path = logPath

	var size = 0
	stat, err := os.Stat(logPath)
	if err == nil {
		size = int(stat.Size())
		if size >= maxLogSize {
			os.Remove(backupPath)
			os.Rename(logPath, backupPath)
			size = 0
		}
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic(err)
	}

	t.Clean()
	t.out = file
	t.std = std
	t.size = size
}

func (t *logImpl) newOut() {
	backupPath := t.path + ".1"
	os.Remove(backupPath)
	os.Rename(t.path, backupPath)
	file, err := os.OpenFile(t.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic(err)
	}
	t.Clean()
	t.out = file
	t.size = 0
}

func (t *logImpl) Clean() {
	if t.out != nil {
		t.out.Close()
		t.out = nil
	}
}

func (t *logImpl) output(calldepth int, level string, id string, params ...any) {
	logBuf := t.pool.Get().(*bytes.Buffer)
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
		n, _ := t.out.Write(logBuf.Bytes())
		t.size += n
		if t.size >= maxLogSize {
			t.newOut()
		}
	}
	if t.std {
		os.Stdout.Write(logBuf.Bytes())
	}
}
