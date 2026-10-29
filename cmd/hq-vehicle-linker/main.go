// HQ Vehicle Linker: drag a .spatial.json onto the .exe. For each HQ it
// finds the vehicle spawners inside the HQ's HQArea polygon and shows them
// as checkboxes in the browser; checked ones are added to that HQ's
// VehicleSpawners list. The original file is never changed; the result goes
// to a Desktop folder named after the spatial, with a report.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/GekkoGear/portal-armory/hqlink"
)

func main() {
	// One reader for the console: Enter cancels the checklist, and later closes the window.
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	code := run(os.Args[1:], desktopDir(), browserPicker(lines, os.Stdout, openBrowser), os.Stdout)
	fmt.Println()
	fmt.Println("Press Enter to close.")
	<-lines
	os.Exit(code)
}

func run(args []string, desktop string, choose picker, out io.Writer) int {
	say := func(format string, a ...any) { fmt.Fprintf(out, format+"\n", a...) }
	say("HQ Vehicle Linker v" + hqlink.Version)
	say("")
	if len(args) != 1 {
		say("Drag a .spatial.json file onto HQ Vehicle Linker.exe (one file at a time).")
		return 2
	}
	in := args[0]
	data, err := os.ReadFile(in)
	if err != nil {
		say("Couldn't read %s: %v", in, err)
		return 1
	}
	found, err := hqlink.Find(data)
	if err != nil {
		if errors.Is(err, hqlink.ErrNotSpatial) {
			say("%s is not a spatial file (no Portal_Dynamic list found).", filepath.Base(in))
		} else {
			say("Couldn't process %s: %v", filepath.Base(in), err)
		}
		return 1
	}
	pick := map[string][]string{}
	if found.FoundCount() > 0 {
		var ok bool
		pick, ok = choose(found, filepath.Base(in))
		if !ok {
			say("Cancelled - nothing was changed or saved.")
			return 0
		}
	}
	result, rep, err := hqlink.Apply(data, pick)
	if err != nil {
		say("Couldn't process %s: %v", filepath.Base(in), err)
		return 1
	}

	dir := outputDir(desktop, in)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		say("Couldn't create %s: %v", dir, err)
		return 1
	}
	if rep.Changed {
		target := filepath.Join(dir, filepath.Base(in))
		if sameFile(target, in) {
			target = filepath.Join(dir, baseName(in)+" - linked.spatial.json")
		}
		if err := os.WriteFile(target, result, 0o644); err != nil {
			say("Couldn't write %s: %v", target, err)
			return 1
		}
		say("Saved: %s", target)
	}
	_, _, reportName := hqlink.OutputNames(in)
	report := filepath.Join(dir, reportName)
	if err := os.WriteFile(report, []byte(rep.Text(filepath.Base(in))), 0o644); err != nil {
		say("Couldn't write the report: %v", err)
		return 1
	}

	say("")
	for _, h := range rep.HQs {
		name := hqLabel(h)
		if h.Status == hqlink.Added {
			extra := ""
			if h.CreatedList {
				extra = ", new vehicle list created"
			}
			if len(h.NotPicked) > 0 {
				extra += fmt.Sprintf(", %d left unchecked", len(h.NotPicked))
			}
			say("  %s: +%d added%s", name, len(h.Added), extra)
		} else {
			say("  %s: %s", name, h.Status)
		}
	}
	if len(rep.Overlaps) > 0 {
		say("")
		say("  %d spawner(s) sit inside more than one HQ polygon and were NOT added - see the report.", len(rep.Overlaps))
	}
	say("")
	if rep.Changed {
		say("Added %d vehicle spawner(s). Original file untouched.", rep.AddedCount())
	} else {
		say("No changes were needed - no new spatial was written.")
	}
	say("Report: %s", report)
	return 0
}

func sameFile(a, b string) bool {
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(sa, sb)
}
