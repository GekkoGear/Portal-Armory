//go:build windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// FOLDERID_Desktop. Follows OneDrive / moved Desktops.
var folderIDDesktop = syscall.GUID{Data1: 0xB4BFCC3A, Data2: 0xDB2C, Data3: 0x424C,
	Data4: [8]byte{0xB0, 0x29, 0x7F, 0xE9, 0x9A, 0x87, 0xC6, 0x41}}

func desktopDir() string {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	ole32 := syscall.NewLazyDLL("ole32.dll")
	get := shell32.NewProc("SHGetKnownFolderPath")
	free := ole32.NewProc("CoTaskMemFree")
	var p *uint16
	r, _, _ := get.Call(uintptr(unsafe.Pointer(&folderIDDesktop)), 0, 0, uintptr(unsafe.Pointer(&p)))
	if p != nil {
		defer free.Call(uintptr(unsafe.Pointer(p))) // freed even when the call fails
	}
	if r == 0 && p != nil {
		n := 0
		for *(*uint16)(unsafe.Add(unsafe.Pointer(p), n*2)) != 0 {
			n++
		}
		if path := syscall.UTF16ToString(unsafe.Slice(p, n)); path != "" {
			return path
		}
	}
	return fallbackDesktop()
}

func fallbackDesktop() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Desktop")
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}
