package utils

import (
	"os"
	"path/filepath"
	"strings"
)

func ParseWorkDir(workDir string) string {
	fp, err := filepath.Abs(workDir)
	if err != nil {
		workDir, _ = os.Getwd()
		return workDir
	}
	info, err := os.Stat(fp)
	if err != nil || !info.IsDir() {
		workDir, _ = os.Getwd()
		return workDir
	}
	return fp
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
