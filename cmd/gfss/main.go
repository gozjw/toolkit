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

	indexETag = etag.Generate(indexHTMl, true)
	iconETag = etag.Generate(string(icon), true)

	fmt.Printf("----------%s----------\n", serverName)
	fmt.Printf("设备名称：%s\n", hostName)
	fmt.Printf("工作目录：%s\n", workDir)
	fmt.Printf("网页链接：http://%s:%d %s\n", ip, port, ipMsg)
	server := &http.Server{
		Addr:        addr,
		Handler:     &Engine{},
		IdleTimeout: 10 * time.Second,
	}
	defer server.Shutdown(context.Background())
	server.ListenAndServe()
}

type Engine struct{}

func (*Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if strings.HasPrefix(r.URL.Path, "/dl/") {
			downloadHandler(w, r)
		} else if r.URL.Path == "/app.png" {
			faviconHandler(w, r)
		} else if r.URL.Path == "/list" {
			listHandler(w)
		} else if r.URL.Path == "/text" {
			getTextHandler(w)
		} else {
			indexHandler(w, r)
		}
	case http.MethodPost:
		switch r.URL.Path {
		case "/upload":
			uploadHandler(w, r)
		case "/text":
			postTextHandler(w, r)
		}
	case http.MethodDelete:
		deleteHandler(w, r)
	}
}

func postTextHandler(w http.ResponseWriter, r *http.Request) {
	textMux.Lock()
	defer textMux.Unlock()
	text.Reset()
	_, err := text.ReadFrom(r.Body)
	if err != nil {
		writeErrorRsp(w, http.StatusBadRequest, "参数错误", err)
		return
	}
	w.Header().Set("Content-Type", "application/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

func getTextHandler(w http.ResponseWriter) {
	textMux.RLock()
	defer textMux.RUnlock()
	w.Header().Set("Content-Type", "application/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(text.Bytes())
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	fileName, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/"))
	if err != nil || strings.Contains(fileName, "/") {
		writeErrorRsp(w, http.StatusBadRequest, "非法文件路径", err)
		return
	}

	if useTrash {
		err = trash.Throw(filepath.Join(workDir, fileName))
		if err != nil {
			writeErrorRsp(w, http.StatusInternalServerError, "放入回收站失败", err)
			return
		}
	} else {
		err = os.Remove(filepath.Join(workDir, fileName))
		if err != nil {
			writeErrorRsp(w, http.StatusInternalServerError, "删除文件失败", err)
			return
		}
	}

	w.Write([]byte("删除成功"))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("If-None-Match") == indexETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("ETag", indexETag)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(indexHTMl))
}

func listHandler(w http.ResponseWriter) {
	d, err := json.Marshal(getFiles())
	if err != nil {
		writeErrorRsp(w, http.StatusInternalServerError, "获取文件列表失败", err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(d)
}

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("If-None-Match") == iconETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("ETag", iconETag)
	w.Write(icon)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// 使用流式 multipart 解析，避免将整个文件缓存在内存
	mr, err := r.MultipartReader()
	if err != nil {
		writeErrorRsp(w, http.StatusBadRequest, "无效表单", err)
		return
	}

	var saved []string

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErrorRsp(w, http.StatusInternalServerError, "读取文件错误", err)
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
			writeErrorRsp(w, http.StatusBadRequest, "没有文件名", nil)
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
			writeErrorRsp(w, http.StatusInternalServerError, "创建文件失败", nil)
			return
		}

		// 将上传流直接写入磁盘（流式），不将整个文件读入内存
		if _, err := io.Copy(out, part); err != nil {
			out.Close()
			part.Close()
			os.Remove(filePathTmp)
			writeErrorRsp(w, http.StatusInternalServerError, "保存文件失败", err)
			return
		}

		out.Close()
		part.Close()

		if err = os.Rename(filePathTmp, filePath); err != nil {
			os.Remove(filePathTmp)
			writeErrorRsp(w, http.StatusInternalServerError, "重命名文件失败", err)
			return
		}

		saved = append(saved, filepath.Base(filePath))
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if len(saved) == 0 {
		writeErrorRsp(w, http.StatusBadRequest, "没有检测到文件上传", nil)
		return
	}
	for _, n := range saved {
		fmt.Fprintf(w, "%s\n", n)
	}
	fmt.Fprintln(w, "上传完成")
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	fileName, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/dl/"))
	if err != nil || strings.Contains(fileName, "/") {
		writeErrorRsp(w, http.StatusBadRequest, "非法文件路径", nil)
		return
	}

	file, err := os.Open(filepath.Join(workDir, fileName))
	if err != nil {
		if os.IsNotExist(err) {
			writeErrorRsp(w, http.StatusNotFound, "文件不存在", err)
		} else {
			writeErrorRsp(w, http.StatusInternalServerError, "无法打开文件", err)
		}
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		writeErrorRsp(w, http.StatusInternalServerError, "读取文件信息失败", err)
		return
	}

	if fileInfo.IsDir() || utils.IsIgnoreFile(fileInfo) {
		writeErrorRsp(w, http.StatusBadRequest, "非文件路径", nil)
		return
	}

	fileHeader := make([]byte, 512)
	_, err = file.Read(fileHeader)
	if err != nil && err != io.EOF {
		writeErrorRsp(w, http.StatusInternalServerError, "读取文件失败", err)
		return
	}

	ctype := mime.TypeByExtension(filepath.Ext(fileName))
	if ctype == "" {
		ctype = http.DetectContentType(fileHeader)
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", fileName, url.PathEscape(fileName)))
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))

	_, err = file.Seek(0, 0)
	if err != nil {
		writeErrorRsp(w, http.StatusInternalServerError, "重置文件指针失败", err)
		return
	}

	_, err = io.Copy(w, file)
	if err != nil {
		fmt.Printf("Error sending file: %v\n", err)
	}
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

func writeErrorRsp(w http.ResponseWriter, status int, msg string, err error) {
	fmt.Printf("%s Msg：%s ", time.Now().Format(time.DateTime), msg)
	if err == nil {
		fmt.Printf("\n")
	} else {
		fmt.Printf("Error：%v\n", err)
	}
	w.WriteHeader(status)
	w.Write([]byte(msg))
}
