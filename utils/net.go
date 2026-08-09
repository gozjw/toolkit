package utils

import (
	"net"
	"os/exec"
	"regexp"
	"strconv"
)

var LocalHost string

func IsLocalIP(ip string) bool {
	if LocalHost != "" && ip == LocalHost {
		return true
	}
	return ip == "127.0.0.1" || ip == "[::1]"
}

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
				LocalHost = ipNet.IP.String()
				return LocalHost, ""
			}
		}
	}
	return "127.0.0.1", "（未联网）"
}

// 获取所有处于LISTENING的TCP端口集合
func getListenPorts() map[int64]bool {
	cmd := exec.Command("netstat", "-ano", "-p", "tcp")
	output, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	text := string(output)
	reg := regexp.MustCompile(`:(\d+)\s+.*LISTENING`)
	result := make(map[int64]bool)
	matches := reg.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		portStr := m[1]
		p, e := strconv.Atoi(portStr)
		if e == nil {
			result[int64(p)] = true
		}
	}
	return result
}

// 获取可用端口
func GetFreePort(p int64) int64 {
	occupied := getListenPorts()
	for {
		if !occupied[p] {
			return p
		}
		p++
	}
}
