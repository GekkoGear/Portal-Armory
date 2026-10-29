//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

func desktopDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Desktop")
}
