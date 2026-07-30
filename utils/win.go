package utils

import (
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var IsNewConsoleSession bool

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetStdHandle          = kernel32.NewProc("GetStdHandle")
	procGetConsoleMode        = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode        = kernel32.NewProc("SetConsoleMode")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
)

func init() {
	isNewConsoleSession()
	if IsNewConsoleSession {
		disableQuickEditMode()
	}
}

func isNewConsoleSession() {
	var pids [2]uint32
	count, _, _ := procGetConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&pids[0])),
		uintptr(len(pids)),
	)
	IsNewConsoleSession = count == 1
}

func disableQuickEditMode() {
	stdHandle := syscall.STD_INPUT_HANDLE
	hStdin, _, _ := procGetStdHandle.Call(uintptr(stdHandle))
	if hStdin == uintptr(syscall.InvalidHandle) {
		return
	}

	var mode uint32
	ret, _, _ := procGetConsoleMode.Call(hStdin, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return
	}

	mode &^= 0x0040
	mode |= 0x0080

	procSetConsoleMode.Call(hStdin, uintptr(mode))
}

func OpenBrowser(url string) {
	go func() {
		cmd := exec.Command("cmd", "/c", "start", url)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: 0x08000000,
		}
		cmd.Start()
	}()
}

func CheckSingleInstance() (bool, windows.Handle) {
	mutex, err := windows.CreateMutex(nil, false,
		windows.StringToUTF16Ptr(`Global\FileShareServerMutex_92746185032975`))
	if err != nil {
		return false, 0
	}

	if windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		_ = windows.CloseHandle(mutex)
		return false, 0
	}

	return true, mutex
}
