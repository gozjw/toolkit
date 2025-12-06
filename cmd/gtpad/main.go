package main

import (
	"context"
	_ "embed"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"toolkit/utils"

	"github.com/amalfra/etag/v3"
	"github.com/go-vgo/robotgo"
	"github.com/gorilla/websocket"
)

//go:embed index.html
var indexHTML []byte
var indexETag string

//go:embed app.png
var icon []byte
var iconETag string

var (
	port        int64
	moveScale   float64
	scrollScale float64
)

func init() {
	iconETag = etag.Generate(string(icon), true)
	indexETag = etag.Generate(string(indexHTML), true)
	flag.Int64Var(&port, "p", 9526, "端口号")
	flag.Float64Var(&moveScale, "m", 1.5, "鼠标移动灵敏度倍数")
	flag.Float64Var(&scrollScale, "s", 0.1, "滚轮灵敏度倍数")
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	flag.Parse()

	port = utils.GetFreePort(port)
	addr := fmt.Sprintf(":%d", port)

	ip, ipMsg := utils.GetIP()

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == indexETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("ETag", indexETag)
		w.Write(indexHTML)
	})

	mux.HandleFunc("/app.png", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == iconETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("ETag", iconETag)
		w.Write(icon)
	})

	mux.HandleFunc("/ws", handleWS)

	fmt.Println("----------web触控板----------")
	fmt.Printf("鼠标灵敏度：%.2f，滚轮灵敏度：%.2f\n", moveScale, scrollScale)
	fmt.Printf("网页链接：http://%s:%d %s\n", ip, port, ipMsg)

	server := &http.Server{Addr: addr, Handler: mux}
	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			fmt.Println("启动失败：", err)
			signalChannel <- syscall.SIGTERM
		}
	}()

	sigReceived := <-signalChannel
	fmt.Printf("接收信号：%s 服务关闭...\n", sigReceived)
	time.Sleep(time.Second)
	server.Shutdown(context.Background())
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("upgrade error:", err)
		return
	}
	defer conn.Close()

	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if mt != websocket.BinaryMessage || len(msg) == 0 {
			continue
		}

		event := msg[0]
		switch event {

		case 1: // move
			if len(msg) < 9 {
				continue
			}
			dx := math.Float32frombits(binary.LittleEndian.Uint32(msg[1:5])) * float32(moveScale)
			dy := math.Float32frombits(binary.LittleEndian.Uint32(msg[5:9])) * float32(moveScale)
			x, y := robotgo.Location()
			robotgo.Move(x+int(dx), y+int(dy))

		case 2: // click
			if len(msg) < 3 {
				continue
			}
			button := msg[1]
			count := int(msg[2])
			btnName := "left"
			if button == 2 {
				btnName = "right"
			}
			for range count {
				robotgo.Click(btnName)
			}

		case 3: // scroll
			if len(msg) < 5 {
				continue
			}
			robotgo.Scroll(0, int(math.Float32frombits(binary.LittleEndian.Uint32(msg[1:5]))*float32(scrollScale)))

		case 4: // input text
			if len(msg) < 3 {
				continue
			}
			length := binary.LittleEndian.Uint16(msg[1:3])
			if int(length)+3 > len(msg) {
				continue
			}
			text := string(msg[3 : 3+length])
			robotgo.TypeStr(text)

		case 5: // key press
			if len(msg) < 2 {
				continue
			}
			length := int(msg[1])
			if len(msg) < 2+length {
				continue
			}
			key := string(msg[2 : 2+length])
			if key == "ctrl+c" {
				robotgo.KeyDown("control")
				robotgo.KeyDown("c")
				robotgo.KeyUp("c")
				robotgo.KeyUp("control")
			} else {
				robotgo.KeyTap(key)
			}
		}
	}
}
