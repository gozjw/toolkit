package utils

import (
	"os"
	"path/filepath"
	"strings"
)

func IsDirExist(dir string) (string, bool) {
	if dir != "" {
		absDir, err := filepath.Abs(dir)
		if err == nil {
			info, err := os.Stat(absDir)
			if err == nil && info.IsDir() {
				return absDir, true
			}
		}
	}
	return "", false
}

func ShrinkHomePath(absPath string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		norm := filepath.Clean(absPath)
		return strings.ReplaceAll(norm, string(filepath.Separator), "/")
	}

	absNorm := filepath.Clean(absPath)
	homeNorm := filepath.Clean(home)

	if strings.HasPrefix(absNorm, homeNorm) {
		suffix := absNorm[len(homeNorm):]
		display := "~" + suffix
		return strings.ReplaceAll(display, string(filepath.Separator), "/")
	}

	return strings.ReplaceAll(absNorm, string(filepath.Separator), "/")
}

func ExpandUserHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	remain := strings.TrimPrefix(path, "~")
	return filepath.Join(homeDir, remain)
}

func NormalizePath(raw string) string {
	s1 := ExpandUserHome(raw)
	s2 := os.ExpandEnv(s1)
	return filepath.Clean(s2)
}
