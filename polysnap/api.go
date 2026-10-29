// Package polysnap lines up the polygon volumes in a Battlefield 6 Portal
// .spatial.json and minifies the result the same way defanz0r's JsonMinifier does.
//
// Volumes are lined up top-down in file order: the first volume never moves,
// and each later volume's points are moved onto the closest corner (or, if no
// corner is near, the closest edge) of a volume above it, within SnapDistance.
// A point that has been placed is never moved again. Only the X and Z numbers
// of moved points change; heights and every other byte of the file stay the same.
//
// The package does no file or folder handling, so it can be used both by the
// standalone drag-and-drop exe and by Portal Armory's Portal Finalization tab.
package polysnap

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Version of the Spatial Poly Snap engine.
const Version = "1.0.0"

// Tunables (meters), exposed read-only for UIs that want to display them.
const (
	SnapDistance     = snapDistance
	NearMissDistance = nearMissDistance
)

// Result is everything one run produces. Lists are ready-to-show sentences.
type Result struct {
	File string // file name or path shown in the report

	// Snapped is the fixed spatial. Nil when the file could not be processed
	// (see Failed); in that case nothing should be saved except the report.
	Snapped []byte
	// Minified is Snapped run through the JsonMinifier rules. Nil when the input
	// was already minified, or minifying failed (see Issues / Notes).
	Minified []byte
	// Failed is a one-line reason when Snapped is nil.
	Failed string

	Volumes, Points        int
	CornerSnaps, EdgeSnaps int // points snapped to a corner / slid onto an edge
	Moved                  int // CornerSnaps + EdgeSnaps
	AlreadyAligned         int
	Renamed                int // names/ids shortened by the minifier

	Issues     []string // problems that need attention
	Notes      []string
	Snaps      []string // one line per point snapped to a corner
	EdgeMoves  []string // one line per point slid onto an edge
	OnEdge     []string // corners near a lower volume's edge, left alone
	NearMisses []string // corners 0.10 - 0.50 m apart, left alone
}

// AddIssue lets a caller (e.g. a file-saving wrapper) add its own problem to the report.
func (r *Result) AddIssue(format string, a ...any) {
	r.Issues = append(r.Issues, fmt.Sprintf(format, a...))
}

// Process runs the whole pipeline on one spatial file's bytes. name is the
// file's name or path; it is shown in the report and used to spot input that
// is already minified (a name containing ".minified").
func Process(data []byte, name string) *Result {
	res := &Result{File: name}
	rep := &report{}
	defer func() {
		res.Issues, res.Notes = rep.issues, rep.notes
		res.Snaps, res.EdgeMoves, res.OnEdge, res.NearMisses = rep.snapped, rep.tJunction, rep.onEdge, rep.nearMiss
	}()

	root, err := parseJSON(data)
	if err != nil {
		rep.issue("The file is not valid JSON, so nothing was changed. Parser said: %v", err)
		rep.issue("Make sure you dragged the exported .spatial.json (from BFPortal > Export Current Level), not the .tscn scene.")
		res.Failed = "FAILED: file is not valid JSON. See the report."
		return res
	}

	vols, pts := collectVolumes(root, rep)
	if len(vols) == 0 {
		rep.issue("No polygon volumes were found in this file, so no points were snapped (the file was still minified).")
	}
	res.CornerSnaps, res.EdgeSnaps, res.AlreadyAligned = priorityAlign(vols, pts, rep)
	findNearMisses(vols, pts, rep)
	validatePolygons(vols, pts, rep)
	res.Volumes, res.Points = len(vols), len(pts)

	// Splice the new numbers into the original bytes.
	type edit struct {
		start, end int
		text       string
	}
	var edits []edit
	for i := range pts {
		p := &pts[i]
		if !p.moved {
			continue
		}
		res.Moved++
		edits = append(edits, edit{p.xNode.start, p.xNode.end, formatNum(p.newX)})
		edits = append(edits, edit{p.zNode.start, p.zNode.end, formatNum(p.newZ)})
	}
	sort.Slice(edits, func(a, b int) bool { return edits[a].start < edits[b].start })
	var out strings.Builder
	out.Grow(len(data) + len(edits)*4)
	last := 0
	for _, e := range edits {
		out.Write(data[last:e.start])
		out.WriteString(e.text)
		last = e.end
	}
	out.Write(data[last:])
	snapped := []byte(out.String())

	// Safety check: the new file must parse, and must differ from the original
	// only in the X/Z of the points that were moved.
	if err := verifyOutput(data, snapped, root, pts); err != nil {
		rep.issue("Safety check failed, so the fixed spatial was NOT saved: %v", err)
		res.Failed = "FAILED safety check. Nothing exported. See the report."
		return res
	}
	res.Snapped = snapped

	// Minify the snapped file exactly like defanz0r's JsonMinifier (default settings).
	if strings.Contains(strings.ToLower(filepath.Base(name)), ".minified") {
		rep.note("This file is already minified, so it was not minified again. For best results drag in the normal export.")
	} else if mini, renamed, err := Minify(snapped); err != nil {
		rep.issue("Minifying failed, so no minified file was saved: %v", err)
	} else if _, err := parseJSON(mini); err != nil {
		rep.issue("The minified output did not pass its safety check, so it was not saved: %v", err)
	} else {
		res.Minified, res.Renamed = mini, renamed
	}
	return res
}

// Minify runs only the JsonMinifier step (defanz0r's rules, default settings).
// It returns the minified bytes and how many names/ids were shortened.
func Minify(data []byte) ([]byte, int, error) { return minifyJSON(data) }

// Summary is the one-line result shown in the console and at the top of the report.
func (r *Result) Summary() string {
	if r.Snapped == nil {
		return r.Failed
	}
	miniNote := ""
	if r.Minified != nil {
		pct := 100 * float64(len(r.Snapped)-len(r.Minified)) / float64(len(r.Snapped))
		miniNote = fmt.Sprintf(" Minified: %s -> %s bytes (%.1f%% smaller, %d names/ids shortened).",
			commas(len(r.Snapped)), commas(len(r.Minified)), pct, r.Renamed)
	}
	return fmt.Sprintf("%d polygon volumes, %d points. %d points snapped to corners, %d moved onto edges (%d moved in total). %d already aligned. %d issue(s).%s",
		r.Volumes, r.Points, r.CornerSnaps, r.EdgeSnaps, r.Moved, r.AlreadyAligned, len(r.Issues), miniNote)
}

// Section is one titled list in the report, for UIs that lay the report out themselves.
type Section struct {
	Title, Explain string
	Items          []string
}

// Sections returns the report's lists in display order.
func (r *Result) Sections() []Section {
	s := []Section{{"ISSUES", "Problems that need your attention. If this is empty, the file is clean.", r.Issues}}
	if len(r.Notes) > 0 {
		s = append(s, Section{"NOTES", "", r.Notes})
	}
	return append(s,
		Section{"SNAPPED POINTS", "Volumes are lined up top-down in file order: the first volume never moves, and each later volume's points are snapped onto the closest corner of a volume above it.", r.Snaps},
		Section{fmt.Sprintf("NEAR MISSES (%.2f - %.2f m apart, NOT snapped)", snapDistance, nearMissDistance),
			"Corners on different volumes that are close but outside the snap distance. Line these up by hand if they are meant to touch.", r.NearMisses},
		Section{"CORNERS MOVED ONTO ANOTHER VOLUME'S EDGE",
			"These corners had no corner of a volume above them nearby, but were within the snap distance of one of its edges, so each was slid straight onto that edge line.", r.EdgeMoves},
		Section{"CORNERS NEAR A LOWER VOLUME'S EDGE (NOT changed)",
			"These corners belong to a volume higher in the list, so they were not moved. If you want a clean shared line here, add a point to the lower volume in Godot, or move it above this one in the scene.", r.OnEdge},
	)
}

// ReportText renders the .txt report (Windows line endings).
func (r *Result) ReportText() string {
	var b strings.Builder
	line := strings.Repeat("=", 72)
	fmt.Fprintf(&b, "%s - Report\r\n%s\r\n", toolName, line)
	fmt.Fprintf(&b, "File:          %s\r\n", r.File)
	fmt.Fprintf(&b, "Date:          %s\r\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "Snap distance: %.2f m (a point this close to a corner or edge of a volume above it in the file is snapped onto it)\r\n", snapDistance)
	fmt.Fprintf(&b, "Only point X/Z values were changed. Heights (Y) and everything else are untouched.\r\n")
	fmt.Fprintf(&b, "The .minified file is the snapped file run through defanz0r's JsonMinifier rules (default settings) - upload that one to Portal.\r\n\r\n")
	fmt.Fprintf(&b, "SUMMARY\r\n%s\r\n\r\n", r.Summary())
	for _, sec := range r.Sections() {
		fmt.Fprintf(&b, "%s\r\n%s\r\n", line, sec.Title)
		if sec.Explain != "" {
			fmt.Fprintf(&b, "%s\r\n", sec.Explain)
		}
		fmt.Fprintf(&b, "%s\r\n", line)
		if len(sec.Items) == 0 {
			b.WriteString("None.\r\n\r\n")
			continue
		}
		for i, it := range sec.Items {
			if i == maxListed {
				fmt.Fprintf(&b, "... and %d more.\r\n", len(sec.Items)-maxListed)
				break
			}
			fmt.Fprintf(&b, "%d. %s\r\n", i+1, it)
		}
		b.WriteString("\r\n")
	}
	return b.String()
}

// OutputNames gives the folder and file names the tools save under, for an
// input file name like "Map.spatial.json":
//
//	folder   "Map"
//	snapped  "Map.spatial.json"
//	minified "Map.spatial.minified.json"
//	report   "Map - Snap Report.txt"
func OutputNames(inputName string) (folder, snapped, minified, report string) {
	folder = spatialBaseName(inputName)
	snapped = filepath.Base(inputName)
	ext := filepath.Ext(snapped)
	minified = strings.TrimSuffix(snapped, ext) + ".minified" + ext
	report = folder + " - Snap Report.txt"
	return
}
