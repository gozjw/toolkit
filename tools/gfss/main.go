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
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"toolkit/utils"

	"github.com/amalfra/etag/v3"
	"github.com/energye/systray"
	"github.com/hymkor/trash-go"
	"golang.org/x/sys/windows"
)

//go:embed index.html
var indexHTMl []byte
var indexETag string

//go:embed app.ico
var iconData []byte
var iconETag string

const maxTextSize = 2 * 1024 * 1024
const maxFileSize = 3 * 1024 * 1024 * 1024
const defaultWorkDir = "upload"
const appGuiMutex = `Global\FileShareServerGuiMutex_92746185032975`

var serverName = "文件共享"
var hostName string
var execPath string
var workDir string
var showDir string
var logPath = "false"
var useTrash bool
var port int64

var textBuf bytes.Buffer
var reqMux sync.RWMutex

var tmpSuffix = ".part"
var tfTracker *TmpFileTracker
var dlTracker *DownloadTracker

var sseMgr *utils.SSEManager
var log = utils.Ctx{}

func main() {
	if utils.IsGuiMode {
		ok, mutex := utils.CheckSingleInstance(appGuiMutex)
		if !ok {
			os.Exit(1)
		}
		defer windows.CloseHandle(mutex)
	}

	var useLogFile bool
	flag.StringVar(&workDir, "d", "", "工作目录")
	flag.Int64Var(&port, "p", 9527, "端口号")
	flag.BoolVar(&useLogFile, "l", false, "启用日志")
	flag.BoolVar(&useTrash, "t", false, "启用回收站")
	flag.Parse()

	hostName, _ = os.Hostname()
	execPath, _ = os.Executable()

	if useLogFile || utils.IsGuiMode {
		logPath = filepath.Join(filepath.Dir(execPath), "gfss.log")
		utils.LogImpl.SetOut(logPath)
	}
	defer utils.LogImpl.Clean()

	sseMgr = utils.NewSSEManager()
	dlTracker = NewDownloadTracker()
	tfTracker = NewTmpFileTracker()
	defer tfTracker.Clean()

	setWorkDir()
	port = utils.GetFreePort(port)
	addr := fmt.Sprintf(":%d", port)
	host, ipMsg := utils.GetIP()

	indexETag = etag.Generate(string(indexHTMl), true)
	iconETag = etag.Generate(string(iconData), true)

	log.Info("====================================")
	log.Infof("网站名称：%s", serverName)
	log.Infof("网站地址：http://%s:%d %s", host, port, ipMsg)
	log.Infof("设备名称：%s", hostName)
	log.Infof("工作目录：%s", workDir)
	log.Infof("启用日志：%s", logPath)
	log.Infof("启用回收站：%t", useTrash)
	log.Info("====================================")

	server := &http.Server{
		Addr:        addr,
		Handler:     &Engine{},
		IdleTimeout: 10 * time.Second,
	}
	go func() {
		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			log.Errorf("服务启动失败: %v\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	if utils.IsGuiMode {
		go showTray(quit)
	}

	sig := <-quit
	log.Infof("收到关闭信号: %v", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

func showTray(q chan os.Signal) {
	// 防止右键不显示菜单，需要禁止 Go 调度器切换系统线程
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	localLink := fmt.Sprintf("http://127.0.0.1:%d", port)
	systray.Run(func() {
		systray.SetIcon(iconData)
		systray.SetTitle(serverName)
		systray.SetTooltip(fmt.Sprintf("%s(%d)", serverName, port))
		systray.SetOnDClick(func(systray.IMenu) {
			if !sseMgr.HasLocalConn() {
				utils.CmdStart(localLink)
			}
		})
		systray.SetOnRClick(func(menu systray.IMenu) {
			menu.ShowMenu()
		})
		systray.AddMenuItem("打开界面", "").Click(func() {
			if !sseMgr.HasLocalConn() {
				utils.CmdStart(localLink)
			}
		})
		systray.AddMenuItem("打开日志", "").Click(func() {
			utils.ExplorerOpen(logPath)
		})
		systray.AddMenuItem("查看连接", "").Click(func() {
			sseMgr.BroadcastLocal("ips", sseMgr.IPs())
		})
		systray.AddSeparator()
		trashMenu := systray.AddMenuItemCheckbox("启用回收站", "", useTrash)
		trashMenu.Click(func() {
			if trashMenu.Checked() {
				trashMenu.Uncheck()
				useTrash = false
			} else {
				trashMenu.Check()
				useTrash = true
			}
			sseMgr.Broadcast("refresh", nil)
		})
		systray.AddSeparator()
		systray.AddMenuItem("更改文件夹", "").Click(func() {
			dir := utils.SelectFolder("请选择")
			if dir != "" {
				updateWorkDir(dir)
				sseMgr.Broadcast("refresh", nil)
				log.Infof("workDir:%s", workDir)
			}
		})
		systray.AddMenuItem("打开文件夹", "").Click(func() {
			utils.ExplorerOpen(workDir)
		})
		systray.AddSeparator()
		systray.AddMenuItem("退出", "").Click(func() {
			q <- os.Interrupt
			systray.Quit()
		})
	}, func() {})
}

var ctxPool = sync.Pool{New: func() any { return &utils.Ctx{} }}

type Engine struct{}

func (*Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var c = ctxPool.Get().(*utils.Ctx)
	defer ctxPool.Put(c)
	c.W, c.R = w, r
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx != -1 {
		c.ID = r.RemoteAddr[:idx]
	} else {
		c.ID = r.RemoteAddr
	}
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path == "/sse" {
			sseMgr.SSE(c)
			return
		} else if r.URL.Path == "/info" {
			info(c)
			return
		} else if r.URL.Path == "/text" {
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
		delFile(c)
		return
	}
	index(c)
}

func setWorkDir() {
	var dir string
	if workDir != "" {
		dir, _ = utils.IsDirExist(utils.NormalizePath(workDir))
	}

	arg0 := flag.Arg(0)
	if dir == "" && arg0 != "" &&
		!strings.HasPrefix(arg0, "-") {
		dir, _ = utils.IsDirExist(arg0)
	}

	if dir == "" {
		for _, v := range []string{".", filepath.Dir(execPath)} {
			absDir, _ := filepath.Abs(v)
			absDir = filepath.Join(absDir, defaultWorkDir)
			err := os.MkdirAll(absDir, 0o755)
			if err == nil {
				dir = absDir
				break
			}
		}
	}

	updateWorkDir(dir)
}

func updateWorkDir(dir string) {
	if dir == "" {
		return
	}
	workDir = dir
	showDir = utils.ShrinkHomePath(workDir)
}

type InfoRsp struct {
	HostName  string `json:"hostName"`
	WorkDir   string `json:"workDir"`
	DelDesc   string `json:"delDesc"`
	IsGuiMode bool   `json:"isGuiMode"`
}

func info(c *utils.Ctx) {
	var rsp = InfoRsp{
		HostName:  hostName,
		WorkDir:   showDir,
		DelDesc:   "删除",
		IsGuiMode: utils.IsGuiMode,
	}
	if useTrash {
		rsp.DelDesc = "移除"
	}
	c.W.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(c.W).Encode(rsp)
}

func modText(c *utils.Ctx) {
	tempBytes, err := io.ReadAll(http.MaxBytesReader(c.W, c.R.Body, maxTextSize))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeErrorRsp(c, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("文本超出%s限制", utils.FormatBytesIEC(maxTextSize)), err)
			return
		}
		writeErrorRsp(c, http.StatusBadRequest, "参数错误", err)
		return
	}

	reqMux.Lock()
	defer reqMux.Unlock()
	textBuf.Reset()
	textBuf.Write(tempBytes)
	c.Info(utils.FormatBytesIEC(int64(textBuf.Len())))
	c.W.Header().Set("Content-Type", "application/json; charset=utf-8")
}

func text(c *utils.Ctx) {
	reqMux.RLock()
	defer reqMux.RUnlock()
	c.W.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.W.Write(textBuf.Bytes())
}

func delFile(c *utils.Ctx) {
	fileName, err := url.PathUnescape(strings.TrimPrefix(c.R.URL.Path, "/"))
	if err != nil || strings.Contains(fileName, "/") {
		writeErrorRsp(c, http.StatusBadRequest, "非法文件路径", err, fileName)
		return
	}

	fp := filepath.Join(workDir, fileName)
	if fp == execPath {
		writeErrorRsp(c, http.StatusBadRequest, "非法文件路径", err, fileName)
		return
	}

	if dlTracker.IsDownloading(fileName) {
		writeErrorRsp(c, http.StatusForbidden, "文件正在被下载", err, fileName)
		return
	}

	if useTrash {
		err = trash.Throw(fp)
		if err != nil {
			writeErrorRsp(c, http.StatusInternalServerError, "放入回收站失败", err, fileName)
			return
		}
		c.Info("t", fileName)
	} else {
		err = os.Remove(fp)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			writeErrorRsp(c, http.StatusInternalServerError, "删除文件失败", err, fileName)
			return
		}
		c.Info("d", fileName)
	}
}

func index(c *utils.Ctx) {
	r := c.R.URL.Path
	if r == "/" {
		c.Info(c.R.Method, r)
	} else {
		c.Info("404", c.R.Method, r)
	}
	if c.R.Header.Get("If-None-Match") == indexETag {
		c.W.WriteHeader(http.StatusNotModified)
		return
	}
	c.W.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.W.Header().Set("ETag", indexETag)
	c.W.Write(indexHTMl)
}

func list(c *utils.Ctx) {
	list, err := getFiles()
	if err != nil {
		writeErrorRsp(c, http.StatusInternalServerError, "获取文件失败", err)
		return
	}
	c.W.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(c.W).Encode(list)
}

func favicon(c *utils.Ctx) {
	if c.R.Header.Get("If-None-Match") == iconETag {
		c.W.WriteHeader(http.StatusNotModified)
		return
	}
	c.W.Header().Set("Content-Type", "image/ico")
	c.W.Header().Set("ETag", iconETag)
	c.W.Write(iconData)
}

var uploadBufPool = sync.Pool{
	New: func() any {
		return make([]byte, 1*1024*1024)
	},
}

func upload(c *utils.Ctx) {
	var now = time.Now()
	// 使用流式 multipart 解析，避免将整个文件缓存在内存
	mr, err := c.R.MultipartReader()
	if err != nil {
		writeErrorRsp(c, http.StatusBadRequest, "无效表单", err)
		return
	}

	var finalName string
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

		out, err := os.CreateTemp(workDir, "*"+tmpSuffix)
		if err != nil {
			part.Close()
			writeErrorRsp(c, http.StatusInternalServerError, "创建临时文件失败", err, fname)
			return
		}
		fPathTmp := out.Name()
		fnameTmp := filepath.Base(fPathTmp)
		tfTracker.Push(fnameTmp, out)

		defer os.Remove(fPathTmp)

		buf := uploadBufPool.Get().([]byte)
		n, err := io.CopyBuffer(out, io.LimitReader(part, maxFileSize+1), buf)
		uploadBufPool.Put(buf)

		out.Close()
		part.Close()
		tfTracker.Pop(fnameTmp)

		if n > maxFileSize {
			writeErrorRsp(c, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("文件超出%s限制", utils.FormatBytesIEC(maxFileSize)), nil, fname)
			return
		}

		if err != nil {
			writeErrorRsp(c, http.StatusInternalServerError, "保存文件失败", err, fnameTmp)
			return
		}

		baseName := strings.TrimSuffix(fname, filepath.Ext(fname))
		ext := filepath.Ext(fname)
		counter := 0

		var finalPath string
		for {
			if counter == 0 {
				finalPath = filepath.Join(workDir, fname)
			} else {
				finalPath = filepath.Join(workDir, fmt.Sprintf("%s(%d)%s", baseName, counter, ext))
			}

			f, err := os.OpenFile(finalPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
			if err == nil {
				f.Close()
				break
			}

			// 说明是没有写入权限或其他严重错误，直接中断
			if !os.IsExist(err) {
				writeErrorRsp(c, http.StatusInternalServerError, "检查目标文件冲突失败", err, fname)
				return
			}

			counter++
		}

		if err = os.Rename(fPathTmp, finalPath); err != nil {
			os.Remove(finalPath)
			writeErrorRsp(c, http.StatusInternalServerError, "重命名文件失败", err, fnameTmp)
			return
		}

		total += n
		finalName = filepath.Base(finalPath)
	}

	c.W.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if total == 0 {
		writeErrorRsp(c, http.StatusBadRequest, "没有检测到文件上传", nil)
		return
	}

	elapsed := time.Since(now)
	speed := int64(0)
	if elapsed > 0 {
		speed = int64(float64(total) / elapsed.Seconds())
	}
	c.Infof(
		"%s %s %v %s/s",
		finalName,
		utils.FormatBytesIEC(total),
		elapsed.Round(time.Millisecond),
		utils.FormatBytesIEC(speed),
	)
}

func download(c *utils.Ctx) {
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

	dlTracker.Start(fileName)
	defer dlTracker.End(fileName)

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
		c.Errorf("传输失败: %v", err)
		return
	}

	elapsed := time.Since(now)
	speed := int64(0)
	if elapsed > 0 {
		speed = int64(float64(total) / elapsed.Seconds())
	}
	c.Infof(
		"%s %s %v %s/s",
		fileName,
		utils.FormatBytesIEC(total),
		elapsed.Round(time.Millisecond),
		utils.FormatBytesIEC(speed),
	)
}

type fileInfo struct {
	name     string
	createAt time.Time
}

func getFiles() (files []string, err error) {
	files = make([]string, 0)

	fs, err := os.ReadDir(workDir)
	if err != nil {
		return files, err
	}

	var list []fileInfo
	for _, e := range fs {
		if !e.Type().IsRegular() {
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

		fi := fileInfo{name: name, createAt: info.ModTime()}
		stat, ok := info.Sys().(*syscall.Win32FileAttributeData)
		if ok {
			fi.createAt = time.Unix(0, stat.CreationTime.Nanoseconds())
		}
		list = append(list, fi)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].createAt.After(list[j].createAt)
	})

	for _, it := range list {
		files = append(files, it.name)
	}
	return
}

func writeErrorRsp(c *utils.Ctx, status int, msg string, err error, remarks ...string) {
	if err == nil {
		if len(remarks) > 0 {
			c.Log(1, "err", msg, strings.Join(remarks, " "))
		} else {
			c.Log(1, "err", msg)
		}
	} else {
		if len(remarks) > 0 {
			c.Log(1, "err", fmt.Sprintf("%s %v", msg, err), strings.Join(remarks, " "))
		} else {
			c.Log(1, "err", msg, err)
		}
	}
	c.W.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.W.WriteHeader(status)
	c.W.Write([]byte(msg))
}

type TmpFileTracker struct {
	mux   sync.Mutex
	files map[string]*os.File
}

func NewTmpFileTracker() *TmpFileTracker {
	return &TmpFileTracker{
		files: make(map[string]*os.File),
	}
}

func (t *TmpFileTracker) Push(name string, f *os.File) {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.files[name] = f
}

func (t *TmpFileTracker) Pop(name string) {
	t.mux.Lock()
	defer t.mux.Unlock()
	delete(t.files, name)
}

func (t *TmpFileTracker) Clean() {
	for name, f := range t.files {
		if f != nil {
			f.Close()
		}
		os.Remove(filepath.Join(workDir, name))
	}
}

type DownloadTracker struct {
	mux   sync.RWMutex
	files map[string]int
}

func NewDownloadTracker() *DownloadTracker {
	return &DownloadTracker{
		files: make(map[string]int),
	}
}

func (t *DownloadTracker) Start(name string) {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.files[name]++
}

func (t *DownloadTracker) End(name string) {
	t.mux.Lock()
	defer t.mux.Unlock()

	if t.files[name] <= 1 {
		delete(t.files, name)
		return
	}

	t.files[name]--
}

func (t *DownloadTracker) IsDownloading(name string) bool {
	t.mux.RLock()
	defer t.mux.RUnlock()

	return t.files[name] > 0
}
