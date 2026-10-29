//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

// desktopDir asks Windows where the Desktop really is (handles OneDrive
// redirection), falling back to %USERPROFILE%\Desktop.
func desktopDir() string {
	// FOLDERID_Desktop {B4BFCC3A-DB2C-424C-B029-7FE99A87C641}
	guid := syscall.GUID{
		Data1: 0xB4BFCC3A, Data2: 0xDB2C, Data3: 0x424C,
		Data4: [8]byte{0xB0, 0x29, 0x7F, 0xE9, 0x9A, 0x87, 0xC6, 0x41},
	}
	shell32 := syscall.NewLazyDLL("shell32.dll")
	ole32 := syscall.NewLazyDLL("ole32.dll")
	proc := shell32.NewProc("SHGetKnownFolderPath")
	free := ole32.NewProc("CoTaskMemFree")
	if proc.Find() == nil {
		var p *uint16
		r, _, _ := proc.Call(uintptr(unsafe.Pointer(&guid)), 0, 0, uintptr(unsafe.Pointer(&p)))
		if r == 0 && p != nil {
			path := syscall.UTF16ToString((*[32768]uint16)(unsafe.Pointer(p))[:])
			free.Call(uintptr(unsafe.Pointer(p)))
			if path != "" {
				return path
			}
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Desktop")
}

// openFile shows a file with its default Windows app (e.g. Photos for a PNG).
func openFile(path string) {
	exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
}
