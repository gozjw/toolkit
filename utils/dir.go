package utils

import (
	"os"
	"path/filepath"
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
