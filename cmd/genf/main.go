package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"toolkit/utils"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/term"
)

const (
	chunkSize = 4 * 1024 * 1024 // 4MiB
	saltSize  = 16
	nonceSize = chacha20poly1305.NonceSizeX

	geFileSuffix = ".ge"
)

var workDir string
var programPath string

func main() {
	programPath, _ = filepath.Abs(os.Args[0])
	if len(os.Args) > 1 && runWithArg() {
		return
	}
	runNoArg()
}

func runNoArg() {
	if workDir == "" {
		workDir, _ = filepath.Abs(".")
	}
	var menu = fmt.Sprintf("%s\n1.加密文件\n2.解密文件\n3.清除屏幕\nq.退出\n请选择：", getTitle())
	var op string
	for {
		op = ""
		fmt.Print(menu)
		fmt.Scanln(&op)
		enList, deList := sortsFile(getDirFile(workDir))
		switch op {
		case "1":
			enList = selectAeDeFile(enList, "加密")
			if len(enList) > 0 {
				enAndDeFile(enList, []GeFile{})
			}
		case "2":
			deList = selectAeDeFile(deList, "解密")
			if len(deList) > 0 {
				enAndDeFile([]GeFile{}, deList)
			}
		case "3":
			utils.ClearScreen()
			goto flesh
		case "q":
			return
		}
		fmt.Println()
	flesh:
	}
}

func getTitle() string {
	t := "-----文件加解密"
	if workDir != "" {
		t += "(" + workDir + ")"
	}
	return t + "-----"
}

func selectAeDeFile(src []GeFile, desc string) (target []GeFile) {
	if len(src) == 0 {
		return
	}

	var opSign = "（已" + desc + "）"
	fmt.Printf("\n%s列表：\n", desc)
	for i, v := range src {
		if v.OpSign {
			fmt.Println(i+1, v.Path, opSign)
		} else {
			fmt.Println(i+1, v.Path)
		}
	}
	fmt.Println(len(src)+1, "全部")

	fmt.Printf("\n选择需要%s文件的序号，多个文件以/分隔，如1/2/3\n请选择：", desc)
	var indexMap = make(map[int]struct{})
	var input string
	fmt.Scanln(&input)
	for _, v := range strings.Split(input, "/") {
		v = strings.TrimSpace(v)
		index, err := strconv.Atoi(v)
		if err != nil {
			continue
		}
		index--
		if index == len(src) {
			return src
		}
		if index < 0 || index >= len(src) {
			continue
		}
		if _, ok := indexMap[index]; ok {
			continue
		}
		target = append(target, src[index])
		indexMap[index] = struct{}{}
	}
	return
}

func runWithArg() bool {
	fileList := make([]string, 0)
	for _, arg := range os.Args[1:] {
		fileInfo, err := os.Stat(arg)
		if err != nil {
			continue
		}
		abs, err := filepath.Abs(arg)
		if err != nil {
			continue
		}
		if fileInfo.IsDir() {
			workDir = abs
		} else {
			if abs == programPath || utils.IsIgnoreFile(fileInfo) {
				continue
			}
			fileList = append(fileList, abs)
		}
	}

	enList, deList := sortsFile(fileList)

	if workDir != "" && len(enList) == 0 && len(deList) == 0 {
		return false
	}

	fmt.Println(getTitle())
	enAndDeFile(enList, deList)
	fmt.Println("操作完成")
	time.Sleep(time.Second * 2)
	return true
}

type GeFile struct {
	Path   string
	OpSign bool
}

func sortsFile(allFileList []string) (enList, deList []GeFile) {
	allFileMap := make(map[string]struct{})
	list := make([]string, 0)
	for _, fp := range allFileList {
		if _, ok := allFileMap[fp]; ok {
			continue
		}
		list = append(list, fp)
		allFileMap[fp] = struct{}{}
	}

	for _, abs := range list {
		fileName := filepath.Base(abs)

		if strings.HasSuffix(fileName, geFileSuffix) {
			_, ok := allFileMap[strings.TrimSuffix(abs, geFileSuffix)]
			deList = append(deList, GeFile{Path: abs, OpSign: ok})
		} else {
			_, ok := allFileMap[abs+geFileSuffix]
			enList = append(enList, GeFile{Path: abs, OpSign: ok})
		}
	}
	return
}

func enAndDeFile(enList []GeFile, deList []GeFile) {
	var enLen = len(enList)
	var deLen = len(deList)
	if enLen == 0 && deLen == 0 {
		fmt.Println("\n未选择文件！")
		return
	}

	fmt.Print("\n请设置密码：")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Println("\n设置密码错误", err)
		return
	}
	if len(password) == 0 {
		fmt.Println("\n密码不能为空！")
		return
	}

	fmt.Println()
	if enLen > 0 {
		fmt.Print("请确认密码：")
		confirmPassword, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			fmt.Println("\n输入确认密码错误！", err)
			return
		}
		if !bytes.Equal(password, confirmPassword) {
			fmt.Println("\n确认密码与设置密码不一致！")
			return
		}

		fmt.Println()
		fmt.Printf("\n加密列表(%d)：\n", enLen)
		for i, v := range enList {
			fmt.Println(i+1, v.Path)
		}
	}

	if deLen > 0 {
		fmt.Printf("\n解密列表(%d)：\n", deLen)
		for i, v := range deList {
			fmt.Println(i+1, v.Path)
		}
	}

	var confirm string
	fmt.Print("\n确认操作(y-确认/n-取消)：")
	fmt.Scanln(&confirm)
	if confirm != "y" {
		return
	}

	var start time.Time
	if enLen > 0 {
		fmt.Println()
	}
	for i, v := range enList {
		fmt.Printf("加密(%d/%d)：%s ", i+1, enLen, v.Path)
		start = time.Now()
		err := enFile(password, v.Path)
		if err != nil {
			fmt.Printf(" 错误：%s\n", err.Error())
		} else {
			fmt.Printf(" 用时：%fs\n", time.Since(start).Seconds())
		}
	}

	if deLen > 0 {
		fmt.Println()
	}
	for i, v := range deList {
		fmt.Printf("解密(%d/%d)：%s ", i+1, deLen, v.Path)
		start = time.Now()
		err := deFile(password, v.Path)
		if err != nil {
			fmt.Printf(" 错误：%s\n", err.Error())
		} else {
			fmt.Printf(" 用时：%fs\n", time.Since(start).Seconds())
		}
	}
}

func getDirFile(dir string) (absPaths []string) {
	filepath.Walk(dir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || utils.IsIgnoreFile(info) {
				return err
			}
			absPath, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if absPath == programPath {
				return nil
			}
			absPaths = append(absPaths, absPath)
			return nil
		})
	return
}

func deriveKey(password []byte, salt []byte) []byte {
	return argon2.IDKey(password, salt, 3, 64*1024, 4, 32)
}

func enFile(password []byte, srcPath string) error {
	salt := make([]byte, saltSize)
	_, err := io.ReadFull(rand.Reader, salt)
	if err != nil {
		return err
	}

	key := deriveKey(password, salt)

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return err
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(srcPath + geFileSuffix)
	if err != nil {
		return err
	}
	defer dst.Close()

	dst.Write(salt)
	dst.Write(nonce)
	var csBuf [8]byte

	buf := make([]byte, chunkSize)
	var chunkIndex uint64

	for {
		n, err := io.ReadFull(src, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return err
		}
		if n == 0 {
			return nil
		}

		binary.BigEndian.PutUint64(nonce[nonceSize-8:], chunkIndex)

		ct := aead.Seal(nil, nonce, buf[:n], nil)

		binary.BigEndian.PutUint64(csBuf[:], uint64(len(ct)))
		if _, err := dst.Write(csBuf[:]); err != nil {
			return err
		}
		if _, err := dst.Write(ct); err != nil {
			return err
		}

		chunkIndex++
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
	}
}

func deFile(password []byte, srcPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(src, salt); err != nil {
		return err
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(src, nonce); err != nil {
		return err
	}

	var csBuf [8]byte

	key := deriveKey(password, salt)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return err
	}

	dst, err := os.Create(strings.TrimSuffix(srcPath, geFileSuffix))
	if err != nil {
		return err
	}
	defer dst.Close()

	var chunkIndex uint64
	for {
		if _, err := io.ReadFull(src, csBuf[:]); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		ctLen := int(binary.BigEndian.Uint64(csBuf[:]))
		ct := make([]byte, ctLen)
		if _, err := io.ReadFull(src, ct); err != nil {
			return err
		}

		binary.BigEndian.PutUint64(nonce[nonceSize-8:], chunkIndex)

		pt, err := aead.Open(nil, nonce, ct, nil)
		if err != nil {
			return errors.New("密码错误！")
		}
		if _, err := dst.Write(pt); err != nil {
			return err
		}
		chunkIndex++
	}
}
