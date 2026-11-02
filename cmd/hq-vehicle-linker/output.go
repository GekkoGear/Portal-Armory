package main

import (
	"path/filepath"

	"github.com/GekkoGear/portal-armory/hqlink"
)

// outputDir is <desktop>/<spatial name> - HQ Vehicles.
func outputDir(desktop, inputPath string) string {
	folder, _, _ := hqlink.OutputNames(inputPath)
	return filepath.Join(desktop, folder)
}

// baseName is the spatial's name without .spatial / .minified / .json.
func baseName(inputPath string) string {
	folder, _, _ := hqlink.OutputNames(inputPath)
	return folder[:len(folder)-len(" - HQ Vehicles")]
}
