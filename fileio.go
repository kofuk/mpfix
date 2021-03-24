package main

import (
	"io"
	"os"
)

func moveFile(oldPath, newPath string) error {
	if os.Rename(oldPath, newPath) == nil {
		return nil
	}

	out, err := os.Create(newPath)
	if err != nil {
		return err
	}
	defer out.Close()

	in, err := os.Open(oldPath)
	if err != nil {
		return err
	}
	defer in.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	os.Remove(oldPath)

	return nil
}
