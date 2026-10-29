// Spatial Poly Snap (standalone)
//
// Drag one or more Battlefield 6 Portal .spatial.json files onto this exe.
// For each, a folder named after the spatial is made on the Desktop holding:
//   - the snapped spatial (polygon volume points lined up),
//   - the snapped spatial minified with defanz0r's JsonMinifier rules,
//   - a "<name> - Snap Report.txt" explaining what changed and any issues,
//   - a "<name> - Changes.png" top-down before/after picture of the changes.
//
// Before anything is saved, the picture opens and the user is asked to confirm.
//
// All the work is done by the polysnap package; this file only handles files.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GekkoGear/portal-armory/polysnap"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println("Spatial Poly Snap v" + polysnap.Version)
		fmt.Println("Drag a .spatial.json file (or several) onto this exe to snap polygon volume points.")
		fmt.Println("Results go to a folder named after the spatial on your Desktop.")
		pause()
		return
	}
	desktop := desktopDir()
	for _, path := range args {
		fmt.Printf("\nProcessing: %s\n", filepath.Base(path))
		outDir, summary := processFile(path, desktop)
		fmt.Println(summary)
		if outDir != "" {
			fmt.Printf("Saved to: %s\n", outDir)
		}
	}
	pause()
}

var stdin = bufio.NewReader(os.Stdin)

func pause() {
	fmt.Print("\nPress Enter to close...")
	stdin.ReadString('\n')
}

// confirm asks a yes/no question. No answer (closed input) counts as no.
func confirm(q string) bool {
	for {
		fmt.Print(q)
		line, err := stdin.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
		if err != nil {
			return false
		}
		fmt.Println("Please type Y or N.")
	}
}

// processFile always tries to write a report; it only writes the spatials
// when the result passed its safety checks.
func processFile(path, desktop string) (string, string) {
	folder, snappedName, miniName, reportName := polysnap.OutputNames(path)
	outDir := filepath.Join(desktop, folder)
	reportPath := filepath.Join(outDir, reportName)

	saveReport := func(res *polysnap.Result) (string, string) {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return "", res.Summary() + "\nCould not create output folder: " + err.Error()
		}
		os.WriteFile(reportPath, []byte(res.ReportText()), 0o644)
		return outDir, res.Summary()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		res := &polysnap.Result{File: path, Failed: "FAILED: could not read the file. See the report."}
		res.AddIssue("Could not read the file: %v", err)
		return saveReport(res)
	}

	res := polysnap.Process(data, path)
	if res.Snapped == nil {
		return saveReport(res)
	}

	// Show the changes and ask before saving anything.
	var picture []byte
	if res.Moved == 0 {
		fmt.Println("No points needed moving, so there is nothing to review.")
	} else {
		fmt.Println(res.Summary())
		var err error
		if picture, err = polysnap.RenderPreview(res); err != nil {
			res.AddIssue("Could not draw the before/after picture: %v", err)
			fmt.Println("Could not draw the before/after picture:", err)
		} else {
			tmp := filepath.Join(os.TempDir(), "Spatial Poly Snap")
			preview := filepath.Join(tmp, folder+" - Changes Preview.png")
			if os.MkdirAll(tmp, 0o755) == nil && os.WriteFile(preview, picture, 0o644) == nil {
				openFile(preview)
				fmt.Println("A top-down before/after picture of the changes has opened:")
				fmt.Println("  " + preview)
			}
		}
		if len(res.Issues) > 0 {
			fmt.Printf("%d issue(s) will be listed in the report.\n", len(res.Issues))
		}
		if !confirm("Export these changes? Type Y to export or N to cancel: ") {
			return "", "Cancelled. Nothing was saved for " + filepath.Base(path) + "."
		}
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", "Could not create output folder: " + err.Error()
	}
	outSpatial := filepath.Join(outDir, snappedName)
	miniPath := filepath.Join(outDir, miniName)
	if samePath(outSpatial, path) {
		res.AddIssue("The dropped file is already inside the output folder; saved the fixed copy with \"(snapped)\" added to the name so the original is not overwritten.")
		ext := filepath.Ext(path)
		outSpatial = strings.TrimSuffix(outSpatial, ext) + " (snapped)" + ext
		miniPath = strings.TrimSuffix(outSpatial, ext) + ".minified" + ext
	}
	if err := os.WriteFile(outSpatial, res.Snapped, 0o644); err != nil {
		res.AddIssue("Could not write the fixed spatial: %v", err)
		res.Snapped, res.Minified = nil, nil
		res.Failed = "FAILED to write output. See the report."
		return saveReport(res)
	}
	if picture != nil {
		if err := os.WriteFile(filepath.Join(outDir, folder+" - Changes.png"), picture, 0o644); err != nil {
			res.AddIssue("Could not save the before/after picture: %v", err)
		}
	}
	if res.Minified != nil {
		if err := os.WriteFile(miniPath, res.Minified, 0o644); err != nil {
			res.AddIssue("Could not write the minified spatial: %v", err)
			res.Minified = nil
		}
	}
	return saveReport(res)
}

func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && strings.EqualFold(aa, bb)
}
