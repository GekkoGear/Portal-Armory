// Package hqlink adds every vehicle spawner that sits inside an HQ's HQArea
// polygon to that HQ's VehicleSpawners list, editing the spatial text in
// place so nothing else in the file changes.
//
// It has no file, console or UI code, so the standalone .exe and Portal
// Armory can both call Link directly.
package hqlink

import "bytes"

// HQStatus says what happened to one HQ.
type HQStatus string

const (
	Added        HQStatus = "vehicles added"
	UpToDate     HQStatus = "already up to date"
	NothingFound HQStatus = "no vehicle spawners inside its polygon - left alone"
	NoArea       HQStatus = "skipped - no HQArea polygon linked"
	BadArea      HQStatus = "skipped - HQArea polygon not found in the file"
	BadList      HQStatus = "skipped - VehicleSpawners is not a normal list"
	NoPlace      HQStatus = "skipped - couldn't find where to add VehicleSpawners"
)

// HQResult is the outcome for one HQ.
type HQResult struct {
	ID, Name      string
	AreaID        string
	Status        HQStatus
	Added         []string // spawners added this run
	AlreadyLinked []string // spawners inside the polygon that were already in the list
	CreatedList   bool     // the HQ had no VehicleSpawners list, so one was made
	Note          string   // extra warning, empty when fine
}

// Overlap is a spawner inside two or more different HQ polygons. It is added to none of them.
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

var bom = []byte{0xEF, 0xBB, 0xBF}

// Link returns the updated spatial and a report. When nothing needs adding,
// the returned bytes are identical to the input.
func Link(in []byte) ([]byte, Report, error) {
	prefix := []byte(nil)
	b := in
	if bytes.HasPrefix(b, bom) {
		prefix, b = bom, b[len(bom):]
	}
	s, err := parseSpatial(b)
	if err != nil {
		return nil, Report{}, err
	}
	rep := Report{Minified: !bytes.Contains(b, []byte("\n"))}

	// Which spawners sit inside which HQ polygons.
	inside := map[int][]string{}            // HQ index -> spawners in its polygon (file order)
	polysOf := map[string]map[string]bool{} // spawner -> distinct polygon ids it's in
	results := make([]HQResult, len(s.HQs))
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
		results[i] = r
		if r.Status != "" {
			continue
		}
		for _, sp := range s.Spawners {
			if insideXZ(sp.X, sp.Z, poly) {
				inside[i] = append(inside[i], sp.ID)
				if polysOf[sp.ID] == nil {
					polysOf[sp.ID] = map[string]bool{}
				}
				polysOf[sp.ID][h.AreaID] = true
			}
		}
	}

	// Spawners inside more than one different polygon.
	overlapping := map[string]bool{}
	for _, sp := range s.Spawners {
		if len(polysOf[sp.ID]) > 1 {
			overlapping[sp.ID] = true
			o := Overlap{SpawnerID: sp.ID}
			for i, h := range s.HQs {
				for _, id := range inside[i] {
					if id == sp.ID {
						o.HQIDs = append(o.HQIDs, h.ID)
					}
				}
			}
			rep.Overlaps = append(rep.Overlaps, o)
		}
	}

	var edits []edit
	for i, h := range s.HQs {
		r := &results[i]
		if r.Status != "" {
			continue
		}
		existing := map[string]bool{}
		for _, v := range h.Vehicles {
			existing[v] = true
		}
		for _, id := range inside[i] {
			switch {
			case existing[id]:
				r.AlreadyLinked = append(r.AlreadyLinked, id)
			case !overlapping[id]:
				r.Added = append(r.Added, id)
			}
		}
		switch {
		case len(inside[i]) == 0:
			r.Status = NothingFound
		case len(r.Added) == 0:
			r.Status = UpToDate
		default:
			o := s.Objects[h.Index]
			e, ok := hqEdits(b, o, r.Added)
			if !ok {
				r.Status, r.Added = NoPlace, nil
				continue
			}
			r.Status = Added
			r.CreatedList = !h.HasVehicles
			if _, has := o.Fields["linked"]; !has {
				r.Note = "this HQ has no 'linked' list; open it in Godot to check the vehicles show up"
			}
			edits = append(edits, e...)
		}
	}
	rep.HQs = results

	if len(edits) == 0 {
		return in, rep, nil
	}
	rep.Changed = true
	out := applyEdits(b, edits)
	return append(prefix, out...), rep, nil
}
