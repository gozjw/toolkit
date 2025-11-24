package utils

import (
	"fmt"
	"net"
)

func GetIP() (string, string) {
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagRunning <= 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if ok &&
				!ipNet.IP.IsLoopback() &&
				ipNet.IP.To4() != nil {
				return ipNet.IP.String(), ""
			}
		}
	}
	return "localhost", "（未联网）"
}

func GetFreePort(port int64) int64 {
	for {
		address := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", address)
		if err == nil {
			ln.Close()
			return port
		}
		port++
	}
}
