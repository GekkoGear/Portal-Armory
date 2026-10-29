package hqlink

import (
	"fmt"
	"strings"
)

// Text renders the report as plain text with Windows line endings (for Notepad).
func (r Report) Text(inputName string) string {
	var sb strings.Builder
	line := func(format string, a ...any) {
		fmt.Fprintf(&sb, format, a...)
		sb.WriteString("\r\n")
	}
	line("HQ Vehicle Linker report - %s", inputName)
	line("")
	for _, h := range r.HQs {
		switch {
		case h.Name == "" || h.Name == h.ID:
			line("%s", h.ID)
		default:
			line("%s  (%s)", h.Name, h.ID)
		}
		line("  Polygon: %s", orNone(h.AreaID))
		line("  Result: %s", h.Status)
		if h.CreatedList {
			line("  This HQ had no VehicleSpawners list, so one was created.")
		}
		if h.Note != "" {
			line("  Warning: %s", h.Note)
		}
		list := func(title string, ids []string) {
			if len(ids) == 0 {
				return
			}
			line("  %s (%d):", title, len(ids))
			for _, id := range ids {
				line("    %s", id)
			}
		}
		list("Added", h.Added)
		for _, c := range h.Found {
			if len(c.AlsoLinkedTo) > 0 && contains(h.Added, c.SpawnerID) {
				line("  Note: %s was already on %s too.", c.SpawnerID, strings.Join(c.AlsoLinkedTo, ", "))
			}
		}
		list("Found but left unchecked", h.NotPicked)
		list("Already linked", h.AlreadyLinked)
		line("")
	}
	line("Spawners inside more than one HQ polygon (not added - fix the polygons):")
	if len(r.Overlaps) == 0 {
		line("  None")
	}
	for _, o := range r.Overlaps {
		line("  %s  ->  %s", o.SpawnerID, strings.Join(o.HQIDs, ", "))
	}
	line("")
	if r.Changed {
		line("Added %d vehicle spawner(s) in total.", r.AddedCount())
	} else {
		line("No changes were needed.")
	}
	return sb.String()
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
