package utils

import (
	"fmt"
	"net/http"
	"sync"
)

type SSEManager struct {
	clients map[http.ResponseWriter]chan string
	Mutex   sync.Mutex
}

func NewSSEManager() *SSEManager {
	return &SSEManager{
		clients: make(map[http.ResponseWriter]chan string),
	}
}

func (t *SSEManager) Broadcast(msg string) {
	t.Mutex.Lock()
	defer t.Mutex.Unlock()
	for _, ch := range t.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (t *SSEManager) SSE(w http.ResponseWriter, r *http.Request) {
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
	t.clients[w] = ch
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
