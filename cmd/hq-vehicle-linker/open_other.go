//go:build !windows

package main

import "os/exec"

// openBrowser opens url in the default browser (best effort off Windows).
func openBrowser(url string) error {
	return exec.Command("xdg-open", url).Start()
}
