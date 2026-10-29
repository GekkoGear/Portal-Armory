// Package hqlink adds the vehicle spawners that sit inside an HQ's HQArea
// polygon to that HQ's VehicleSpawners list, editing the spatial text in
// place so nothing else in the file changes.
//
// It works in two steps so a UI can show a checklist:
//
//	rep, err := hqlink.Find(data)             // what each HQ could get (HQResult.Found)
//	out, rep, err := hqlink.Apply(data, pick) // add only the picked spawners
//
// Link(data) does both and adds everything found.
//
// It has no file, console or UI code, so the standalone .exe and Portal
// Armory can both call it directly.
package hqlink

import "bytes"

// HQStatus says what happened to one HQ.
type HQStatus string

const (
	Found        HQStatus = "vehicles found" // Find only: waiting for a pick
	Added        HQStatus = "vehicles added"
	NonePicked   HQStatus = "vehicles found but none picked - left alone"
	UpToDate     HQStatus = "already up to date"
	NothingFound HQStatus = "no vehicle spawners inside its polygon - left alone"
	NoArea       HQStatus = "skipped - no HQArea polygon linked"
	BadArea      HQStatus = "skipped - HQArea polygon not found in the file"
	BadList      HQStatus = "skipped - VehicleSpawners is not a normal list"
	NoPlace      HQStatus = "skipped - couldn't find where to add VehicleSpawners"
)

// Candidate is a spawner inside an HQ's polygon that isn't in its list yet.
type Candidate struct {
	SpawnerID   string
	VehicleType string // e.g. "Abrams"; empty if the spawner has none set
}

// HQResult is the outcome for one HQ.
type HQResult struct {
	ID, Name      string
	AreaID        string
	Status        HQStatus
	Found         []Candidate // new spawners inside the polygon (the checklist), file order
	Added         []string    // spawners added (Apply/Link only)
	NotPicked     []string    // found but not picked (Apply/Link only)
	AlreadyLinked []string    // spawners inside the polygon that were already in the list
	CreatedList   bool        // the HQ had no VehicleSpawners list, so one was made
	Note          string      // extra warning, empty when fine
}

// Overlap is a spawner inside two or more different HQ polygons. It is never offered or added.
type Overlap struct {
	SpawnerID string
	HQIDs     []string
}

// Report describes a whole run.
type Report struct {
	HQs      []HQResult
	Overlaps []Overlap
	Changed  bool
	Minified bool
}

// AddedCount is the total number of spawners added across all HQs.
func (r Report) AddedCount() int {
	n := 0
	for _, h := range r.HQs {
		n += len(h.Added)
	}
	return n
}

// FoundCount is the total number of checklist entries across all HQs.
func (r Report) FoundCount() int {
	n := 0
	for _, h := range r.HQs {
		n += len(h.Found)
	}
	return n
}

// PickAll picks every Found spawner of every HQ.
func (r Report) PickAll() map[string][]string {
	pick := map[string][]string{}
	for _, h := range r.HQs {
		for _, c := range h.Found {
			pick[h.ID] = append(pick[h.ID], c.SpawnerID)
		}
	}
	return pick
}

var bom = []byte{0xEF, 0xBB, 0xBF}

type analysis struct {
	b       []byte // input without BOM
	prefix  []byte
	s       spatial
	results []HQResult
	rep     Report
}

func analyze(in []byte) (*analysis, error) {
	a := &analysis{b: in}
	if bytes.HasPrefix(in, bom) {
		a.prefix, a.b = bom, in[len(bom):]
	}
	s, err := parseSpatial(a.b)
	if err != nil {
		return nil, err
	}
	a.s = s
	a.rep.Minified = !bytes.Contains(a.b, []byte("\n"))

	inside := map[int][]spawner{}           // HQ index -> spawners in its polygon (file order)
	polysOf := map[string]map[string]bool{} // spawner -> distinct polygon ids it's in
	a.results = make([]HQResult, len(s.HQs))
	for i, h := range s.HQs {
		r := HQResult{ID: h.ID, Name: h.Name, AreaID: h.AreaID}
		poly, ok := s.Polys[h.AreaID]
		switch {
		case h.AreaID == "":
			r.Status = NoArea
		case !ok || len(poly) < 3:
			r.Status = BadArea
		case h.VehiclesBad:
			r.Status = BadList
		}
		a.results[i] = r
		if r.Status != "" {
			continue
		}
		for _, sp := range s.Spawners {
			if insideXZ(sp.X, sp.Z, poly) {
				inside[i] = append(inside[i], sp)
				if polysOf[sp.ID] == nil {
					polysOf[sp.ID] = map[string]bool{}
				}
				polysOf[sp.ID][h.AreaID] = true
			}
		}
	}

	overlapping := map[string]bool{}
	for _, sp := range s.Spawners {
		if len(polysOf[sp.ID]) > 1 {
			overlapping[sp.ID] = true
			o := Overlap{SpawnerID: sp.ID}
			for i, h := range s.HQs {
				for _, in := range inside[i] {
					if in.ID == sp.ID {
						o.HQIDs = append(o.HQIDs, h.ID)
					}
				}
			}
			a.rep.Overlaps = append(a.rep.Overlaps, o)
		}
	}

	for i, h := range s.HQs {
		r := &a.results[i]
		if r.Status != "" {
			continue
		}
		existing := map[string]bool{}
		for _, v := range h.Vehicles {
			existing[v] = true
		}
		seen := map[string]bool{}
		for _, sp := range inside[i] {
			switch {
			case existing[sp.ID]:
				r.AlreadyLinked = append(r.AlreadyLinked, sp.ID)
			case !overlapping[sp.ID] && !seen[sp.ID]:
				seen[sp.ID] = true
				r.Found = append(r.Found, Candidate{SpawnerID: sp.ID, VehicleType: sp.VehicleType})
			}
		}
		switch {
		case len(inside[i]) == 0:
			r.Status = NothingFound
		case len(r.Found) == 0:
			r.Status = UpToDate
		default:
			r.Status = Found
		}
	}
	return a, nil
}

// Find works out, for every HQ, which spawners it could get (HQResult.Found).
// It changes nothing.
func Find(in []byte) (Report, error) {
	a, err := analyze(in)
	if err != nil {
		return Report{}, err
	}
	a.rep.HQs = a.results
	return a.rep, nil
}

// Apply adds the picked spawners. pick maps an HQ's ID to the spawner IDs to
// add; anything not in that HQ's Found list is ignored, and the order used is
// the Found order. When nothing is added, the returned bytes equal the input.
func Apply(in []byte, pick map[string][]string) ([]byte, Report, error) {
	a, err := analyze(in)
	if err != nil {
		return nil, Report{}, err
	}
	var edits []edit
	for i, h := range a.s.HQs {
		r := &a.results[i]
		if r.Status != Found {
			continue
		}
		want := map[string]bool{}
		for _, id := range pick[h.ID] {
			want[id] = true
		}
		for _, c := range r.Found {
			if want[c.SpawnerID] {
				r.Added = append(r.Added, c.SpawnerID)
			} else {
				r.NotPicked = append(r.NotPicked, c.SpawnerID)
			}
		}
		if len(r.Added) == 0 {
			r.Status = NonePicked
			continue
		}
		o := a.s.Objects[h.Index]
		e, ok := hqEdits(a.b, o, r.Added)
		if !ok {
			r.Status, r.Added, r.NotPicked = NoPlace, nil, nil
			continue
		}
		r.Status = Added
		r.CreatedList = !h.HasVehicles
		if _, has := o.Fields["linked"]; !has {
			r.Note = "this HQ has no 'linked' list; open it in Godot to check the vehicles show up"
		}
		edits = append(edits, e...)
	}
	a.rep.HQs = a.results
	if len(edits) == 0 {
		return in, a.rep, nil
	}
	a.rep.Changed = true
	out := append(append([]byte(nil), a.prefix...), applyEdits(a.b, edits)...)
	return out, a.rep, nil
}

// Link adds every spawner Find would offer.
func Link(in []byte) ([]byte, Report, error) {
	rep, err := Find(in)
	if err != nil {
		return nil, Report{}, err
	}
	return Apply(in, rep.PickAll())
}
