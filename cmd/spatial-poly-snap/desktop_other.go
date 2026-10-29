//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

func desktopDir() string {
	if d := os.Getenv("POLYSNAP_DESKTOP"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Desktop")
}
