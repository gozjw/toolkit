package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"toolkit/utils"

	"github.com/amalfra/etag/v3"
	"github.com/hymkor/trash-go"
)

//go:embed index.html
var indexHTMl []byte
var indexETag string

//go:embed app.png
var icon []byte
var iconETag string

var serverName = "文件共享"
var hostName string
var execPath string
var workDir string
var noTrash bool
var port int64
var tmpSuffix = ".tmp"

var textBuf bytes.Buffer
var reqMux sync.RWMutex

func main() {
	log := utils.Logger{}
	hostName, _ = os.Hostname()
	execPath, _ = os.Executable()

	flag.StringVar(&workDir, "d", workDir, "工作目录")
	flag.BoolVar(&noTrash, "t", false, "不放入回收站")
	flag.Int64Var(&port, "p", 9527, "端口号")
	flag.Parse()

	if workDir == "" {
		workDir = flag.Arg(0)
	}

	workDir = utils.ParseWorkDir(workDir)
	port = utils.GetFreePort(port)
	addr := fmt.Sprintf(":%d", port)
	ip, ipMsg := utils.GetIP()

	indexHTMl = bytes.ReplaceAll(indexHTMl, []byte("{{.serverName}}"), []byte(serverName))
	indexHTMl = bytes.ReplaceAll(indexHTMl, []byte("{{.HostName}}"), []byte(hostName))

	curUser, err := user.Current()
	if err == nil && strings.Contains(workDir, curUser.HomeDir) {
		indexHTMl = bytes.ReplaceAll(indexHTMl, []byte("{{.WorkDir}}"),
			[]byte(strings.ReplaceAll(workDir, curUser.HomeDir, "~")))
	} else {
		indexHTMl = bytes.ReplaceAll(indexHTMl, []byte("{{.WorkDir}}"), []byte(workDir))
	}

	if noTrash {
		indexHTMl = bytes.ReplaceAll(indexHTMl, []byte("{{.TrashDesc}}"), []byte("删除"))
	} else {
		indexHTMl = bytes.ReplaceAll(indexHTMl, []byte("{{.TrashDesc}}"), []byte("移除"))
	}

	indexETag = etag.Generate(string(indexHTMl), true)
	iconETag = etag.Generate(string(icon), true)

	log.Printf("----------%s----------", serverName)
	log.Printf("设备名称：%s", hostName)
	log.Printf("工作目录：%s", workDir)
	log.Printf("网页链接：http://%s:%d %s", ip, port, ipMsg)
	log.Printf("回收站：%t", !noTrash)
	server := &http.Server{
		Addr:        addr,
		Handler:     &Engine{},
		IdleTimeout: 10 * time.Second,
	}
	defer server.Shutdown(context.Background())
	server.ListenAndServe()
}

type Ctx struct {
	W   http.ResponseWriter
	R   *http.Request
	Log utils.Logger
}

var ctxPool = sync.Pool{New: func() any { return &Ctx{} }}

type Engine struct{}

func (*Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var c = ctxPool.Get().(*Ctx)
	defer ctxPool.Put(c)
	c.W, c.R = w, r
	if idx := strings.Index(r.RemoteAddr, ":"); idx != -1 {
		c.Log.ID = r.RemoteAddr[:idx]
	} else {
		c.Log.ID = r.RemoteAddr
	}
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path == "/text" {
			text(c)
			return
		} else if r.URL.Path == "/list" {
			list(c)
			return
		} else if r.URL.Path == "/favicon.ico" {
			favicon(c)
			return
		} else if strings.HasPrefix(r.URL.Path, "/dl/") {
			download(c)
			return
		}
	case http.MethodPost:
		switch r.URL.Path {
		case "/text":
			modText(c)
			return
		case "/upload":
			upload(c)
			return
		}
	case http.MethodDelete:
		delete(c)
		return
	}
	index(c)
}

func modText(c *Ctx) {
	reqMux.Lock()
	defer reqMux.Unlock()
	textBuf.Reset()
	n, err := textBuf.ReadFrom(c.R.Body)
	if err != nil {
		writeErrorRsp(c, http.StatusBadRequest, "参数错误", err)
		return
	}
	c.Log.Print(utils.FormatBytesIEC(n))
	c.W.Header().Set("Content-Type", "application/plain; charset=utf-8")
}

func text(c *Ctx) {
	reqMux.RLock()
	defer reqMux.RUnlock()
	c.W.Header().Set("Content-Type", "application/plain; charset=utf-8")
	c.W.Write(textBuf.Bytes())
}

func delete(c *Ctx) {
	reqMux.Lock()
	defer reqMux.Unlock()
	fileName, err := url.PathUnescape(strings.TrimPrefix(c.R.URL.Path, "/"))
	if err != nil || strings.Contains(fileName, "/") {
		writeErrorRsp(c, http.StatusBadRequest, "非法文件路径", err, fileName)
		return
	}

	if noTrash {
		err = os.Remove(filepath.Join(workDir, fileName))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			writeErrorRsp(c, http.StatusInternalServerError, "删除文件失败", err, fileName)
			return
		}
		c.Log.Print("d", fileName)
	} else {
		err = trash.Throw(filepath.Join(workDir, fileName))
		if err != nil {
			writeErrorRsp(c, http.StatusInternalServerError, "放入回收站失败", err, fileName)
			return
		}
		c.Log.Print("t", fileName)
	}
}

func index(c *Ctx) {
	c.Log.Print(c.R.Method, c.R.URL.Path)
	if c.R.Header.Get("If-None-Match") == indexETag {
		c.W.WriteHeader(http.StatusNotModified)
		return
	}
	c.W.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.W.Header().Set("ETag", indexETag)
	c.W.Write(indexHTMl)
}

func list(c *Ctx) {
	list, err := getFiles()
	if err != nil {
		writeErrorRsp(c, http.StatusInternalServerError, "获取文件失败", err)
		return
	}
	d, err := json.Marshal(list)
	if err != nil {
		writeErrorRsp(c, http.StatusInternalServerError, "列表格式化失败", err)
		return
	}
	c.W.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.W.Write(d)
}

func favicon(c *Ctx) {
	if c.R.Header.Get("If-None-Match") == iconETag {
		c.W.WriteHeader(http.StatusNotModified)
		return
	}
	c.W.Header().Set("Content-Type", "image/png")
	c.W.Header().Set("ETag", iconETag)
	c.W.Write(icon)
}

func upload(c *Ctx) {
	var now = time.Now()
	// 使用流式 multipart 解析，避免将整个文件缓存在内存
	mr, err := c.R.MultipartReader()
	if err != nil {
		writeErrorRsp(c, http.StatusBadRequest, "无效表单", err)
		return
	}

	var count int
	var total int64

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErrorRsp(c, http.StatusInternalServerError, "读取文件错误", err)
			return
		}

		// 只处理名为 "file" 的文件字段
		if part.FormName() != "file" {
			part.Close()
			continue
		}

		fname := filepath.Base(part.FileName())
		if fname == "" {
			part.Close()
			writeErrorRsp(c, http.StatusBadRequest, "没有文件名", nil)
			return
		}

		// 构造目标文件路径并处理冲突
		filePath := filepath.Join(workDir, fname)
		baseName := strings.TrimSuffix(fname, filepath.Ext(fname))
		ext := filepath.Ext(fname)
		counter := 1
		for {
			_, statErr := os.Stat(filePath)
			if statErr != nil && os.IsNotExist(statErr) {
				break
			}
			filePath = filepath.Join(workDir, fmt.Sprintf("%s(%d)%s", baseName, counter, ext))
			counter++
		}

		var filePathTmp = filePath + tmpSuffix
		var fnameTmp = filepath.Base(filePathTmp)
		out, err := os.Create(filePathTmp)
		if err != nil {
			part.Close()
			writeErrorRsp(c, http.StatusInternalServerError, "创建文件失败", nil, fnameTmp)
			return
		}

		defer os.Remove(filePathTmp)

		// 将上传流直接写入磁盘（流式），不将整个文件读入内存
		n, err := io.Copy(out, part)
		if err != nil {
			out.Close()
			part.Close()
			writeErrorRsp(c, http.StatusInternalServerError, "保存文件失败", err, fnameTmp)
			return
		}

		out.Close()
		part.Close()

		if err = os.Rename(filePathTmp, filePath); err != nil {
			writeErrorRsp(c, http.StatusInternalServerError, "重命名文件失败", err, fnameTmp)
			return
		}

		count++
		total += n
		c.Log.Print(count, filepath.Base(filePath), utils.FormatBytesIEC(n))
	}

	c.W.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if count == 0 {
		writeErrorRsp(c, http.StatusBadRequest, "没有检测到文件上传", nil)
		return
	}

	elapsed := time.Since(now)
	speed := int64(0)
	if elapsed > 0 {
		speed = int64(float64(total) / elapsed.Seconds())
	}
	c.Log.Printf(
		"total:%s use:%v speed:%s/s",
		utils.FormatBytesIEC(total),
		elapsed.Round(time.Millisecond),
		utils.FormatBytesIEC(speed),
	)
}

func download(c *Ctx) {
	var now = time.Now()
	fileName, err := url.PathUnescape(strings.TrimPrefix(c.R.URL.Path, "/dl/"))
	if err != nil || strings.Contains(fileName, "/") {
		writeErrorRsp(c, http.StatusBadRequest, "非法文件路径", nil, fileName)
		return
	}

	file, err := os.Open(filepath.Join(workDir, fileName))
	if err != nil {
		if os.IsNotExist(err) {
			writeErrorRsp(c, http.StatusNotFound, "文件不存在", err, fileName)
		} else {
			writeErrorRsp(c, http.StatusInternalServerError, "无法打开文件", err, fileName)
		}
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		writeErrorRsp(c, http.StatusInternalServerError, "读取文件信息失败", err, fileName)
		return
	}

	if fileInfo.IsDir() || utils.IsIgnoreFile(fileInfo) {
		writeErrorRsp(c, http.StatusBadRequest, "非文件路径", nil, fileName)
		return
	}

	fileHeader := make([]byte, 512)
	_, err = file.Read(fileHeader)
	if err != nil && err != io.EOF {
		writeErrorRsp(c, http.StatusInternalServerError, "读取文件失败", err, fileName)
		return
	}

	ctype := mime.TypeByExtension(filepath.Ext(fileName))
	if ctype == "" {
		// ctype = http.DetectContentType(fileHeader)
		ctype = "application/octet-stream"
	}
	c.W.Header().Set("Content-Type", ctype)
	c.W.Header().Set("X-Content-Type-Options", "nosniff")
	c.W.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", fileName, url.PathEscape(fileName)))
	c.W.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))

	_, err = file.Seek(0, 0)
	if err != nil {
		writeErrorRsp(c, http.StatusInternalServerError, "重置文件指针失败", err, fileName)
		return
	}

	total, err := io.Copy(c.W, file)
	if err != nil {
		c.Log.Errorf("Error sending file: %v", err)
		return
	}

	elapsed := time.Since(now)
	speed := int64(0)
	if elapsed > 0 {
		speed = int64(float64(total) / elapsed.Seconds())
	}
	c.Log.Printf(
		"%s total:%s use:%v speed:%s/s",
		fileName,
		utils.FormatBytesIEC(total),
		elapsed.Round(time.Millisecond),
		utils.FormatBytesIEC(speed),
	)
}

type fileInfo struct {
	name string
	mod  time.Time
}

func getFiles() (files []string, err error) {
	files = make([]string, 0)

	fs, err := os.ReadDir(workDir)
	if err != nil {
		return files, err
	}

	var list []fileInfo
	for _, e := range fs {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, tmpSuffix) ||
			filepath.Join(workDir, name) == execPath {
			continue
		}
		info, err := e.Info()
		if err != nil || utils.IsIgnoreFile(info) {
			continue
		}
		list = append(list, fileInfo{name: name, mod: info.ModTime()})
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].mod.After(list[j].mod)
	})

	for _, it := range list {
		files = append(files, it.name)
	}
	return
}

func writeErrorRsp(c *Ctx, status int, msg string, err error, remarks ...string) {
	if err == nil {
		if len(remarks) > 0 {
			c.Log.Log(1, "err", msg, strings.Join(remarks, " "))
		} else {
			c.Log.Log(1, "err", msg)
		}
	} else {
		if len(remarks) > 0 {
			c.Log.Log(1, "err", fmt.Sprintf("%s %v", msg, err), strings.Join(remarks, " "))
		} else {
			c.Log.Log(1, "err", msg, err)
		}
	}
	c.W.WriteHeader(status)
	c.W.Write([]byte(msg))
}
