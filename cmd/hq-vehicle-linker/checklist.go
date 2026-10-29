package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/GekkoGear/portal-armory/hqlink"
)

// askPicks shows each HQ's found spawners as a ticked checklist and lets the
// player untick the ones they don't want. End of input keeps what's ticked.
func askPicks(rep hqlink.Report, in *bufio.Reader, out io.Writer) map[string][]string {
	pick := map[string][]string{}
	for _, h := range rep.HQs {
		if len(h.Found) == 0 {
			continue
		}
		ticked := make([]bool, len(h.Found))
		for i := range ticked {
			ticked[i] = true
		}
		name := hqLabel(h)
		for {
			printList(out, name, h.Found, ticked)
			fmt.Fprint(out, "Enter = add the ticked ones. Type numbers to tick/untick (e.g. 2 5-7), A = all, N = none: ")
			line, err := in.ReadString('\n')
			line = strings.TrimSpace(line)
			if line == "" {
				fmt.Fprintln(out)
				break
			}
			if bad := toggle(line, ticked); bad != "" {
				fmt.Fprintf(out, "\n  Didn't understand %q - use numbers from 1 to %d, ranges like 2-4, A or N.\n", bad, len(ticked))
			}
			if err != nil { // input ended mid-line: keep what's ticked
				fmt.Fprintln(out)
				break
			}
		}
		for i, c := range h.Found {
			if ticked[i] {
				pick[h.ID] = append(pick[h.ID], c.SpawnerID)
			}
		}
	}
	return pick
}

func printList(out io.Writer, name string, found []hqlink.Candidate, ticked []bool) {
	fmt.Fprintf(out, "\n%s - %d new vehicle spawner(s) inside its polygon:\n", name, len(found))
	width := 4
	for _, c := range found {
		if len(c.VehicleType) > width {
			width = len(c.VehicleType)
		}
	}
	on := 0
	for i, c := range found {
		box := "[ ]"
		if ticked[i] {
			box = "[x]"
			on++
		}
		t := c.VehicleType
		if t == "" {
			t = "?"
		}
		also := ""
		if len(c.AlsoLinkedTo) > 0 {
			also = "   (already on " + strings.Join(c.AlsoLinkedTo, ", ") + ")"
		}
		fmt.Fprintf(out, "  %s %3d  %-*s  %s%s\n", box, i+1, width, t, c.SpawnerID, also)
	}
	fmt.Fprintf(out, "  %d of %d ticked\n", on, len(found))
}

// toggle applies one line of input to ticked. It returns the first word it
// couldn't understand ("" if all fine); words before it are still applied.
func toggle(line string, ticked []bool) string {
	words := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' })
	for _, w := range words {
		switch strings.ToUpper(w) {
		case "A", "ALL":
			for i := range ticked {
				ticked[i] = true
			}
			continue
		case "N", "NONE":
			for i := range ticked {
				ticked[i] = false
			}
			continue
		}
		lo, hi, ok := parseRange(w, len(ticked))
		if !ok {
			return w
		}
		for i := lo; i <= hi; i++ {
			ticked[i-1] = !ticked[i-1]
		}
	}
	return ""
}

func parseRange(w string, n int) (int, int, bool) {
	a, b, isRange := strings.Cut(w, "-")
	lo, err := strconv.Atoi(a)
	if err != nil {
		return 0, 0, false
	}
	hi := lo
	if isRange {
		if hi, err = strconv.Atoi(b); err != nil {
			return 0, 0, false
		}
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 1 || hi > n {
		return 0, 0, false
	}
	return lo, hi, true
}

// hqLabel is the HQ's name, with its id added when the two differ.
func hqLabel(h hqlink.HQResult) string {
	switch {
	case h.Name == "":
		return h.ID
	case h.Name == h.ID:
		return h.Name
	default:
		return h.Name + " (" + h.ID + ")"
	}
}
