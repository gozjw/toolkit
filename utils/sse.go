package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

type SSEClient struct {
	IP       string
	OS       string
	Browser  string
	CreateAt time.Time
	ch       chan string
}

type SSEManager struct {
	clients map[http.ResponseWriter]SSEClient
	Mutex   sync.Mutex
}

func NewSSEManager() *SSEManager {
	return &SSEManager{
		clients: make(map[http.ResponseWriter]SSEClient),
	}
}

type SSEData struct {
	Event string `json:"event"`
	Data  any    `json:"data,omitempty"`
}

func (t *SSEManager) BroadcastLocal(event string, data any) {
	b, _ := json.Marshal(SSEData{
		Event: event,
		Data:  data,
	})
	t.broadcast(true, b)
}

func (t *SSEManager) Broadcast(event string, data any) {
	b, _ := json.Marshal(SSEData{
		Event: event,
		Data:  data,
	})
	t.broadcast(false, b)
}

func (t *SSEManager) broadcast(local bool, b []byte) {
	if len(b) == 0 {
		return
	}
	t.Mutex.Lock()
	defer t.Mutex.Unlock()
	for _, c := range t.clients {
		if local && !IsLocalIP(c.IP) {
			continue
		}
		select {
		case c.ch <- string(b):
		default:
		}
	}
}

type SSEClientRsp struct {
	IP       string `json:"ip"`
	UA       string `json:"ua"`
	CreateAt string `json:"createAt"`
	IsLocal  bool   `json:"isLocal"`
	createAt time.Time
}

func (t *SSEManager) IPs() (list []SSEClientRsp) {
	t.Mutex.Lock()
	defer t.Mutex.Unlock()
	for _, c := range t.clients {
		list = append(list, SSEClientRsp{
			IP:       c.IP,
			UA:       fmt.Sprintf("%s(%s)", c.Browser, c.OS),
			CreateAt: c.CreateAt.Format("2006-01-02 15:04:05"),
			createAt: c.CreateAt,
			IsLocal:  IsLocalIP(c.IP),
		})
	}
	slices.SortFunc(list, func(a, b SSEClientRsp) int {
		return b.createAt.Compare(a.createAt)
	})
	return
}

func (t *SSEManager) SSE(c *Ctx) {
	flusher, ok := c.W.(http.Flusher)
	if !ok {
		http.Error(c.W, "不支持流式输出", http.StatusInternalServerError)
		return
	}

	// SSE 必须头部
	c.W.Header().Set("Content-Type", "text/event-stream")
	c.W.Header().Set("Cache-Control", "no-cache")
	c.W.Header().Set("Connection", "keep-alive")
	c.W.Header().Set("X-Accel-Buffering", "no")

	os, browser := ParseUserAgent(c.R.Header.Get("user-agent"))
	ch := make(chan string, 5)
	t.Mutex.Lock()
	t.clients[c.W] = SSEClient{
		IP:       c.ID,
		OS:       os,
		Browser:  browser,
		ch:       ch,
		CreateAt: time.Now(),
	}
	t.Mutex.Unlock()

	defer func() {
		t.Mutex.Lock()
		delete(t.clients, c.W)
		t.Mutex.Unlock()
		close(ch)
	}()

	for {
		select {
		case <-c.R.Context().Done():
			return
		case msg := <-ch:
			// SSE 标准格式 data:xxx\n\n
			fmt.Fprintf(c.W, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

var browserRules = []struct {
	keyword string
	name    string
}{
	// App内置WebView
	{"micromessenger", "微信内置浏览器"},
	{"alipay", "支付宝内置浏览器"},
	{"dingtalk", "钉钉内置浏览器"},
	{"douyin", "抖音内置浏览器"},

	// 移动端国产浏览器
	{"miuibrowser", "小米浏览器"},
	{"quark", "夸克浏览器"},
	{"qqbrowser", "QQ浏览器"},
	{"huaweibrowser", "华为浏览器"},
	{"opbrowser", "OPPO浏览器"},
	{"vivobrowser", "VIVO浏览器"},
	{"360browser", "360手机浏览器"},
	{"sogoumse", "搜狗手机浏览器"},

	// PC国产套壳浏览器
	{"360se", "360浏览器"},
	{"360ee", "360浏览器"},
	{"sogou", "搜狗浏览器"},
	{"lbbrowser", "猎豹浏览器"},
	{"2345explorer", "2345浏览器"},
	{"cent", "百分浏览器"},
	{"twinkstar", "星愿浏览器"},
	{"quarkpc", "夸克PC版"},

	// 国外Chromium系
	{"edg", "Edge"},
	{"vivaldi", "Vivaldi"},
	{"brave", "Brave"},
	{"chrome", "Chrome"},

	// 非Chromium
	{"firefox", "Firefox"},
	{"safari", "Safari"},
}

var osRules = []struct {
	keyword string
	name    string
}{
	{"harmonyos", "HarmonyOS"},
	{"android", "Android"},
	{"android tv", "Android TV"},
	{"iphone", "iOS"},
	{"ipad", "iOS"},
	{"ipod", "iOS"},
	{"win", "Windows"},
	{"mac", "macOS"},
	{"linux", "Linux"},
}

func ParseUserAgent(ua string) (os string, browser string) {
	os = "未知"
	browser = "未知"
	if ua == "" {
		return
	}
	uaLower := strings.ToLower(ua)

	for _, rule := range browserRules {
		if strings.Contains(uaLower, rule.keyword) {
			browser = rule.name
			break
		}
	}

	for _, rule := range osRules {
		if strings.Contains(uaLower, rule.keyword) {
			os = rule.name
			break
		}
	}
	return
}
