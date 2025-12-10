package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
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
var indexHTMl string
var indexETag string

//go:embed app.png
var icon []byte
var iconETag string

var serverName = "文件共享"
var hostName string
var workDir string
var useTrash bool
var port int64
var tmpSuffix = ".tmp"

var text bytes.Buffer
var textMux sync.RWMutex

func main() {
	hostName, _ = os.Hostname()

	flag.StringVar(&workDir, "d", workDir, "工作目录")
	flag.BoolVar(&useTrash, "t", false, "删除时放入回收站")
	flag.Int64Var(&port, "p", 9527, "端口号")
	flag.Parse()

	if workDir == "" {
		workDir = flag.Arg(0)
	}

	workDir = utils.ParseWorkDir(workDir)
	port = utils.GetFreePort(port)
	addr := fmt.Sprintf(":%d", port)
	ip, ipMsg := utils.GetIP()

	indexHTMl = strings.ReplaceAll(indexHTMl, "{{.serverName}}", serverName)
	indexHTMl = strings.ReplaceAll(indexHTMl, "{{.HostName}}", hostName)
	indexHTMl = strings.ReplaceAll(indexHTMl, "{{.WorkDir}}", workDir)

	if useTrash {
		indexHTMl = strings.ReplaceAll(indexHTMl, "{{.UseTrash}}", "移除")
	} else {
		indexHTMl = strings.ReplaceAll(indexHTMl, "{{.UseTrash}}", "删除")
	}

	indexETag = etag.Generate(indexHTMl, true)
	iconETag = etag.Generate(string(icon), true)

	fmt.Printf("----------%s----------\n", serverName)
	fmt.Printf("设备名称：%s\n", hostName)
	fmt.Printf("工作目录：%s\n", workDir)
	fmt.Printf("网页链接：http://%s:%d %s\n", ip, port, ipMsg)
	fmt.Printf("放入回收站：%t\n", useTrash)
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

type Engine struct{}

func (*Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var c = &Ctx{W: w, R: r}
	c.Log.ID = strings.Split(r.RemoteAddr, ":")[0]
	switch r.Method {
	case http.MethodGet:
		if strings.HasPrefix(r.URL.Path, "/dl/") {
			downloadHandler(c)
		} else if r.URL.Path == "/app.png" {
			faviconHandler(c)
		} else if r.URL.Path == "/list" {
			listHandler(c)
		} else if r.URL.Path == "/text" {
			getTextHandler(c)
		} else {
			indexHandler(c)
		}
	case http.MethodPost:
		switch r.URL.Path {
		case "/upload":
			uploadHandler(c)
		case "/text":
			postTextHandler(c)
		}
	case http.MethodDelete:
		deleteHandler(c)
	}
}

func postTextHandler(c *Ctx) {
	textMux.Lock()
	defer textMux.Unlock()
	text.Reset()
	_, err := text.ReadFrom(c.R.Body)
	if err != nil {
		writeErrorRsp(c, http.StatusBadRequest, "参数错误", err)
		return
	}

	c.Log.Print(text.Len())

	c.W.Header().Set("Content-Type", "application/plain; charset=utf-8")
	c.W.WriteHeader(http.StatusOK)
}

func getTextHandler(c *Ctx) {
	textMux.RLock()
	defer textMux.RUnlock()
	c.W.Header().Set("Content-Type", "application/plain; charset=utf-8")
	c.W.WriteHeader(http.StatusOK)
	c.W.Write(text.Bytes())
}

func deleteHandler(c *Ctx) {
	fileName, err := url.PathUnescape(strings.TrimPrefix(c.R.URL.Path, "/"))
	c.Log.Print(fileName)
	if err != nil || strings.Contains(fileName, "/") {
		writeErrorRsp(c, http.StatusBadRequest, "非法文件路径", err)
		return
	}

	if useTrash {
		err = trash.Throw(filepath.Join(workDir, fileName))
		if err != nil {
			writeErrorRsp(c, http.StatusInternalServerError, "放入回收站失败", err)
			return
		}
	} else {
		err = os.Remove(filepath.Join(workDir, fileName))
		if err != nil {
			writeErrorRsp(c, http.StatusInternalServerError, "删除文件失败", err)
			return
		}
	}

	c.W.Write([]byte("删除成功"))
}

func indexHandler(c *Ctx) {
	if c.R.Header.Get("If-None-Match") == indexETag {
		c.W.WriteHeader(http.StatusNotModified)
		return
	}
	c.W.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.W.Header().Set("ETag", indexETag)
	c.W.WriteHeader(http.StatusOK)
	c.W.Write([]byte(indexHTMl))
}

func listHandler(c *Ctx) {
	d, err := json.Marshal(getFiles())
	if err != nil {
		writeErrorRsp(c, http.StatusInternalServerError, "获取文件列表失败", err)
		return
	}
	c.W.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.W.WriteHeader(http.StatusOK)
	c.W.Write(d)
}

func faviconHandler(c *Ctx) {
	if c.R.Header.Get("If-None-Match") == iconETag {
		c.W.WriteHeader(http.StatusNotModified)
		return
	}
	c.W.Header().Set("Content-Type", "image/png")
	c.W.Header().Set("ETag", iconETag)
	c.W.Write(icon)
}

func uploadHandler(c *Ctx) {
	var now = time.Now()
	// 使用流式 multipart 解析，避免将整个文件缓存在内存
	mr, err := c.R.MultipartReader()
	if err != nil {
		writeErrorRsp(c, http.StatusBadRequest, "无效表单", err)
		return
	}

	var saved []string

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
		out, err := os.Create(filePathTmp)
		if err != nil {
			part.Close()
			writeErrorRsp(c, http.StatusInternalServerError, "创建文件失败", nil)
			return
		}

		// 将上传流直接写入磁盘（流式），不将整个文件读入内存
		if _, err := io.Copy(out, part); err != nil {
			out.Close()
			part.Close()
			os.Remove(filePathTmp)
			writeErrorRsp(c, http.StatusInternalServerError, "保存文件失败", err)
			return
		}

		out.Close()
		part.Close()

		if err = os.Rename(filePathTmp, filePath); err != nil {
			os.Remove(filePathTmp)
			writeErrorRsp(c, http.StatusInternalServerError, "重命名文件失败", err)
			return
		}

		saved = append(saved, filepath.Base(filePath))
	}

	c.W.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if len(saved) == 0 {
		writeErrorRsp(c, http.StatusBadRequest, "没有检测到文件上传", nil)
		return
	}

	c.Log.Printf("use:%g len:%d %s", time.Since(now).Seconds(), len(saved), strings.Join(saved, ","))

	for _, n := range saved {
		fmt.Fprintf(c.W, "%s\n", n)
	}
	fmt.Fprintln(c.W, "上传完成")
}

func downloadHandler(c *Ctx) {
	var now = time.Now()
	fileName, err := url.PathUnescape(strings.TrimPrefix(c.R.URL.Path, "/dl/"))
	if err != nil || strings.Contains(fileName, "/") {
		writeErrorRsp(c, http.StatusBadRequest, "非法文件路径", nil)
		return
	}

	file, err := os.Open(filepath.Join(workDir, fileName))
	if err != nil {
		if os.IsNotExist(err) {
			writeErrorRsp(c, http.StatusNotFound, "文件不存在", err)
		} else {
			writeErrorRsp(c, http.StatusInternalServerError, "无法打开文件", err)
		}
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		writeErrorRsp(c, http.StatusInternalServerError, "读取文件信息失败", err)
		return
	}

	if fileInfo.IsDir() || utils.IsIgnoreFile(fileInfo) {
		writeErrorRsp(c, http.StatusBadRequest, "非文件路径", nil)
		return
	}

	fileHeader := make([]byte, 512)
	_, err = file.Read(fileHeader)
	if err != nil && err != io.EOF {
		writeErrorRsp(c, http.StatusInternalServerError, "读取文件失败", err)
		return
	}

	ctype := mime.TypeByExtension(filepath.Ext(fileName))
	if ctype == "" {
		ctype = http.DetectContentType(fileHeader)
	}
	c.W.Header().Set("Content-Type", ctype)
	c.W.Header().Set("X-Content-Type-Options", "nosniff")
	c.W.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", fileName, url.PathEscape(fileName)))
	c.W.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))

	_, err = file.Seek(0, 0)
	if err != nil {
		writeErrorRsp(c, http.StatusInternalServerError, "重置文件指针失败", err)
		return
	}

	_, err = io.Copy(c.W, file)
	if err != nil {
		c.Log.Errorf("Error sending file: %v", err)
		return
	}
	c.Log.Printf("use:%g %s", time.Since(now).Seconds(), fileName)
}

type entryInfo struct {
	name string
	mod  time.Time
}

func getFiles() (files []string) {
	fs, _ := os.ReadDir(workDir)

	var list []entryInfo
	exeName := filepath.Base(os.Args[0])

	for _, e := range fs {
		name := e.Name()
		if name == exeName ||
			strings.HasSuffix(name, tmpSuffix) {
			continue
		}
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || utils.IsIgnoreFile(info) {
			continue
		}
		list = append(list, entryInfo{name: name, mod: info.ModTime()})
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].mod.After(list[j].mod)
	})

	files = make([]string, 0, len(list))
	for _, it := range list {
		files = append(files, it.name)
	}
	return
}

func writeErrorRsp(c *Ctx, status int, msg string, err error) {
	if err == nil {
		c.Log.Log(1, "err", msg)
	} else {
		c.Log.Logf(1, "err", "%s %v", msg, err)
	}
	c.W.WriteHeader(status)
	c.W.Write([]byte(msg))
}
