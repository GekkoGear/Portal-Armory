package hqlink

import (
	"path/filepath"
	"strings"
)

// Version of the HQ Vehicle Linker engine.
const Version = "1.0.0"

// OutputNames gives the folder and file names the tools save under, for an
// input spatial name or path: folder is "<name> - HQ Vehicles", spatial keeps
// the input's file name, report is "HQ Vehicle Linker report.txt".
func OutputNames(inputName string) (folder, spatial, report string) {
	base := filepath.Base(inputName)
	n := strings.TrimSuffix(base, filepath.Ext(base))
	for _, s := range []string{".minified", ".spatial"} {
		n = strings.TrimSuffix(n, s)
	}
	return n + " - HQ Vehicles", base, "HQ Vehicle Linker report.txt"
}
