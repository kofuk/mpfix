package main

import (
	"path/filepath"
	"strings"
)

func getOutPath(inPath string) string {
	absPath, _ := filepath.Abs(inPath)
	return filepath.Join("/tmp", strings.ReplaceAll(absPath, "/", "!"))
}
