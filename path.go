package main

import (
	"os"
	"path/filepath"
)

func getInputFiles(inputSpec string) ([]string, error) {
	info, err := os.Stat(inputSpec)
	if err != nil {
		return nil, err
	}
	var result []string
	if info.IsDir() {
		dirents, err := os.ReadDir(inputSpec)
		if err != nil {
			return nil, err
		}
		for _, dirent := range dirents {
			if dirent.IsDir() {
				continue
			}
			name := dirent.Name()
			ext := filepath.Ext(name)
			if ext != ".mp3" {
				continue
			}
			result = append(result, filepath.Join(inputSpec, name))
		}
	} else {
		result = make([]string, 1)
		result[0] = inputSpec
	}

	return result, nil
}
