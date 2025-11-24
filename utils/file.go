package utils

import (
	"os"
	"runtime"
	"strings"
	"syscall"
)

func IsIgnoreFile(info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		if strings.HasSuffix(info.Name(), ".lnk") {
			return true
		}
		stat, ok := info.Sys().(*syscall.Win32FileAttributeData)
		if !ok {
			return true
		}
		return stat.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0 ||
			stat.FileAttributes&syscall.FILE_ATTRIBUTE_SYSTEM != 0
	}
	return info.Mode()&os.ModeSymlink != 0
}
