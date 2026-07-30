package utils

import (
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var GuiMode string

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetStdHandle          = kernel32.NewProc("GetStdHandle")
	procGetConsoleMode        = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode        = kernel32.NewProc("SetConsoleMode")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
)

func init() {
	if isNewConsoleSession() {
		disableQuickEditMode()
	}
}

func IsGuiMode() bool {
	return GuiMode == "1"
}

func isNewConsoleSession() bool {
	var pids [2]uint32
	count, _, _ := procGetConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&pids[0])),
		uintptr(len(pids)),
	)
	return count == 1
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

func OpenFolder(path string) {
	exec.Command("explorer.exe", path).Start()
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

// 单实例锁，通过内核互斥体 Mutex 实现只允许运行一个程序实例
func CheckSingleInstance(s string) (bool, windows.Handle) {
	mutex, err := windows.CreateMutex(nil, false,
		windows.StringToUTF16Ptr(s))
	if err != nil {
		return false, 0
	}

	if windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		_ = windows.CloseHandle(mutex)
		return false, 0
	}

	return true, mutex
}

// SelectFolder 弹出Windows选择文件夹对话框
// title: 窗口提示文字
// 返回选中路径，取消则返回空字符串
func SelectFolder(title string) string {
	type BROWSEINFO struct {
		hwndOwner      windows.HWND
		pidlRoot       uintptr
		pszDisplayName *uint16
		lpszTitle      *uint16
		ulFlags        uint32
		lpfn           uintptr
		lParam         uintptr
		iImage         int32
	}

	const (
		BIF_RETURNONLYFSDIRS = 0x0001
		BIF_NEWDIALOGSTYLE   = 0x0040
	)

	shell32 := windows.NewLazySystemDLL("shell32.dll")
	SHBrowseForFolder := shell32.NewProc("SHBrowseForFolderW")
	SHGetPathFromIDList := shell32.NewProc("SHGetPathFromIDListW")

	titleUTF16, _ := windows.UTF16PtrFromString(title)
	var bi BROWSEINFO
	bi.lpszTitle = titleUTF16
	bi.ulFlags = BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE

	pidl, _, _ := SHBrowseForFolder.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return ""
	}

	var buf [windows.MAX_PATH]uint16
	ok, _, _ := SHGetPathFromIDList.Call(pidl, uintptr(unsafe.Pointer(&buf[0])))
	if ok == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:])
}
