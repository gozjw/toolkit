package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

type SSEClient struct {
	IP       string
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
			CreateAt: c.CreateAt.Format("2006-01-02 15:04:05"),
			createAt: c.CreateAt,
			IsLocal:  IsLocalIP(c.IP),
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].createAt.After(list[j].createAt)
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

	ch := make(chan string, 5)
	t.Mutex.Lock()
	t.clients[c.W] = SSEClient{
		IP:       c.ID,
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
