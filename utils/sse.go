package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type SSEClient struct {
	IP       string
	CreateAt string
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

func (t *SSEManager) BroadcastLocal(msg string) {
	t.Mutex.Lock()
	defer t.Mutex.Unlock()
	for _, c := range t.clients {
		if IsLocalIP(c.IP) {
			select {
			case c.ch <- msg:
			default:
			}
		}
	}
}

func (t *SSEManager) Broadcast(msg string) {
	t.Mutex.Lock()
	defer t.Mutex.Unlock()
	for _, c := range t.clients {
		select {
		case c.ch <- msg:
		default:
		}
	}
}

type SSEClientRsp struct {
	IP       string `json:"ip"`
	CreateAt string `json:"time"`
	IsLocal  bool   `json:"is_local"`
}

func (t *SSEManager) IPs() []byte {
	t.Mutex.Lock()
	defer t.Mutex.Unlock()
	var list []SSEClientRsp
	for _, c := range t.clients {
		list = append(list, SSEClientRsp{
			IP:       c.IP,
			CreateAt: c.CreateAt,
			IsLocal:  IsLocalIP(c.IP),
		})
	}
	b, _ := json.Marshal(list)
	return b
}

func (t *SSEManager) SSE(c *Ctx) {
	if !IsGuiMode() {
		http.Error(c.W, "不支持SSE", http.StatusInternalServerError)
		return
	}
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

	ch := make(chan string, 5)
	t.Mutex.Lock()
	t.clients[c.W] = SSEClient{
		IP:       c.ID,
		ch:       ch,
		CreateAt: time.Now().Format("2006-01-02 15:04:05"),
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
