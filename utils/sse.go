package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type SSEClient struct {
	IP       string      `json:"ip"`
	CreateAt string      `json:"time"`
	ch       chan string `json:"-"`
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
		if IsLocalIp(c.IP) {
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

func (t *SSEManager) IPs() []byte {
	t.Mutex.Lock()
	defer t.Mutex.Unlock()
	var list []SSEClient
	for _, c := range t.clients {
		list = append(list, c)
	}
	b, _ := json.Marshal(list)
	return b
}

func (t *SSEManager) SSE(w http.ResponseWriter, r *http.Request, ip string) {
	if !IsGuiMode() {
		http.Error(w, "不支持SSE", http.StatusInternalServerError)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "不支持流式输出", http.StatusInternalServerError)
		return
	}

	// SSE 必须头部
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := make(chan string, 5)
	t.Mutex.Lock()
	t.clients[w] = SSEClient{
		IP:       ip,
		ch:       ch,
		CreateAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	t.Mutex.Unlock()

	defer func() {
		t.Mutex.Lock()
		delete(t.clients, w)
		t.Mutex.Unlock()
		close(ch)
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			// SSE 标准格式 data:xxx\n\n
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}
