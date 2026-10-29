package polysnap

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	snapDistance     = 0.10 // meters: points closer than this (on different volumes) are merged
	nearMissDistance = 0.50 // meters: pairs between snapDistance and this are listed for review
	maxListed        = 300  // cap per report section so the txt stays readable
	toolName         = "Spatial Poly Snap"
)

// ---------- data ----------

type point struct {
	vol   int
	idx   int
	x, y  float64
	z     float64
	xNode *jnode
	zNode *jnode
	newX  float64
	newZ  float64
	moved bool
}

type volume struct {
	name    string
	id      string
	pts     []int // indices into all points
	byShape bool
}

type report struct {
	issues    []string
	snapped   []string
	nearMiss  []string
	tJunction []string
	onEdge    []string
	notes     []string
}

func (r *report) issue(f string, a ...any) { r.issues = append(r.issues, fmt.Sprintf(f, a...)) }
func (r *report) note(f string, a ...any)  { r.notes = append(r.notes, fmt.Sprintf(f, a...)) }

func spatialBaseName(file string) string {
	name := filepath.Base(file)
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".json") {
		name = name[:len(name)-5]
		lower = lower[:len(lower)-5]
	}
	if i := strings.Index(lower, ".spatial"); i >= 0 {
		name = name[:i] + name[i+len(".spatial"):]
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "spatial"
	}
	return sanitize(name)
}

func sanitize(s string) string {
	repl := strings.NewReplacer("<", "_", ">", "_", ":", "_", "\"", "_", "/", "_", "\\", "_", "|", "_", "?", "_", "*", "_")
	return strings.TrimRight(repl.Replace(s), ". ")
}

func commas(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// ---------- finding volumes ----------

func collectVolumes(root *jnode, rep *report) ([]volume, []point) {
	var vols []volume
	var pts []point

	var walk func(n *jnode, byShape bool)
	addVolume := func(n *jnode, byShape bool) {
		name := n.getString("name")
		id := n.getString("id")
		label := name
		if id != "" && id != name {
			label = fmt.Sprintf("%s (id: %s)", name, id)
		}
		if label == "" {
			label = fmt.Sprintf("unnamed volume #%d", len(vols)+1)
		}
		arr := n.get("points")
		if arr == nil || arr.kind != kArray {
			rep.issue("Polygon volume \"%s\" has no points list; left untouched.", label)
			return
		}
		v := volume{name: label, id: id, byShape: byShape}
		var vpts []point
		for i, pn := range arr.vals {
			if pn.kind != kObject {
				rep.issue("Polygon volume \"%s\" point %d is not an {x, y, z} object; volume left untouched.", label, i+1)
				return
			}
			xn, yn, zn := pn.get("x"), pn.get("y"), pn.get("z")
			if xn == nil || zn == nil || xn.kind != kNumber || zn.kind != kNumber {
				rep.issue("Polygon volume \"%s\" point %d is missing a numeric x or z; volume left untouched.", label, i+1)
				return
			}
			y := 0.0
			if yn != nil && yn.kind == kNumber {
				y = yn.num
			}
			if math.IsNaN(xn.num) || math.IsInf(xn.num, 0) || math.IsNaN(zn.num) || math.IsInf(zn.num, 0) {
				rep.issue("Polygon volume \"%s\" point %d has an invalid number; volume left untouched.", label, i+1)
				return
			}
			vpts = append(vpts, point{vol: len(vols), idx: i, x: xn.num, y: y, z: zn.num, xNode: xn, zNode: zn})
		}
		if len(vpts) < 3 {
			rep.issue("Polygon volume \"%s\" has only %d point(s); a volume needs at least 3. Left untouched.", label, len(vpts))
			return
		}
		for _, p := range vpts {
			v.pts = append(v.pts, len(pts))
			pts = append(pts, p)
		}
		vols = append(vols, v)
	}
	walk = func(n *jnode, byShape bool) {
		switch n.kind {
		case kObject:
			t := n.getString("type")
			if !byShape && t == "PolygonVolume" {
				addVolume(n, false)
			} else if byShape && t != "WaypointPath" && looksLikePolygon(n) {
				addVolume(n, true)
			}
			for _, c := range n.vals {
				walk(c, byShape)
			}
		case kArray:
			for _, c := range n.vals {
				walk(c, byShape)
			}
		}
	}
	walk(root, false)
	if len(vols) == 0 && len(rep.issues) == 0 {
		// Fallback: no "type":"PolygonVolume" found. Look for objects that are
		// shaped like one (points list of x/y/z plus a numeric height).
		walk(root, true)
		if len(vols) > 0 {
			rep.note("No objects were labeled \"PolygonVolume\", so volumes were found by their shape (a \"points\" list plus a \"height\"). Double-check the results.")
		}
	}
	return vols, pts
}

func looksLikePolygon(n *jnode) bool {
	h := n.get("height")
	arr := n.get("points")
	if h == nil || h.kind != kNumber || arr == nil || arr.kind != kArray || len(arr.vals) < 3 {
		return false
	}
	for _, p := range arr.vals {
		if p.kind != kObject || p.get("x") == nil || p.get("z") == nil {
			return false
		}
	}
	return true
}

// ---------- snapping ----------

type cellKey struct{ cx, cz int64 }

type grid struct {
	size  float64
	cells map[cellKey][]int
}

func newGrid(pts []point, size float64) *grid {
	g := &grid{size: size, cells: map[cellKey][]int{}}
	for i, p := range pts {
		k := g.key(p.x, p.z)
		g.cells[k] = append(g.cells[k], i)
	}
	return g
}

func (g *grid) key(x, z float64) cellKey {
	return cellKey{int64(math.Floor(x / g.size)), int64(math.Floor(z / g.size))}
}

// within calls fn for every point within radius r of (x, z).
func (g *grid) within(x, z, r float64, fn func(j int)) {
	reach := int64(math.Ceil(r / g.size))
	k := g.key(x, z)
	for dx := -reach; dx <= reach; dx++ {
		for dz := -reach; dz <= reach; dz++ {
			for _, j := range g.cells[cellKey{k.cx + dx, k.cz + dz}] {
				fn(j)
			}
		}
	}
}

func dist(a, b point) float64 { return math.Hypot(a.x-b.x, a.z-b.z) }

func fmtXZ(p point) string { return fmt.Sprintf("(x %s, z %s)", formatNum(p.x), formatNum(p.z)) }

// ---------- review lists ----------

// Near misses: points on different volumes that are close, but not close enough to snap.
func findNearMisses(vols []volume, pts []point, rep *report) {
	g := newGrid(pts, nearMissDistance)
	type pair struct {
		a, b int
		d    float64
	}
	var pairs []pair
	for i := range pts {
		best, bj := math.MaxFloat64, -1
		g.within(pts[i].x, pts[i].z, nearMissDistance, func(j int) {
			if pts[j].vol == pts[i].vol {
				return
			}
			if d := dist(pts[i], pts[j]); d < best {
				best, bj = d, j
			}
		})
		if bj >= 0 && best > snapDistance && best <= nearMissDistance && i < bj {
			pairs = append(pairs, pair{i, bj, best})
		}
	}
	sort.Slice(pairs, func(a, b int) bool { return pairs[a].d < pairs[b].d })
	for _, p := range pairs {
		rep.nearMiss = append(rep.nearMiss, fmt.Sprintf("%.3fm apart: %s pt %d %s  and  %s pt %d %s",
			p.d, vols[pts[p.a].vol].name, pts[p.a].idx+1, fmtXZ(pts[p.a]),
			vols[pts[p.b].vol].name, pts[p.b].idx+1, fmtXZ(pts[p.b])))
	}
}

func cur(p point) point {
	if p.moved {
		p.x, p.z = p.newX, p.newZ
	}
	return p
}

func segDist(p, a, b point) float64 {
	dx, dz := b.x-a.x, b.z-a.z
	l2 := dx*dx + dz*dz
	if l2 == 0 {
		return dist(p, a)
	}
	t := ((p.x-a.x)*dx + (p.z-a.z)*dz) / l2
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(p.x-(a.x+t*dx), p.z-(a.z+t*dz))
}

// ---------- polygon checks ----------

func validatePolygons(vols []volume, pts []point, rep *report) {
	for _, v := range vols {
		before := make([]point, len(v.pts))
		after := make([]point, len(v.pts))
		changed := false
		for k, pi := range v.pts {
			before[k] = pts[pi]
			after[k] = cur(pts[pi])
			changed = changed || pts[pi].moved
		}
		if selfIntersects(before) {
			rep.issue("Volume \"%s\" has edges that cross each other (it was like this before snapping). The game may treat its area unpredictably.", v.name)
		} else if changed && selfIntersects(after) {
			rep.issue("Volume \"%s\" ended up with crossing edges after snapping. Check it in Godot.", v.name)
		}
		for k := range after {
			k2 := (k + 1) % len(after)
			if dist(after[k], after[k2]) < 1e-6 {
				when := "after snapping"
				if dist(before[k], before[k2]) < 1e-6 {
					when = "already like this in your file"
				}
				rep.issue("Volume \"%s\" has two points stacked in the same spot, points %d and %d (%s). Delete one of them in Godot.", v.name, k+1, k2+1, when)
			}
		}
		if math.Abs(area(after)) < 0.01 {
			rep.issue("Volume \"%s\" has almost no area (%.4f m^2).", v.name, math.Abs(area(after)))
		}
	}
}

func area(p []point) float64 {
	s := 0.0
	for i := range p {
		j := (i + 1) % len(p)
		s += p[i].x*p[j].z - p[j].x*p[i].z
	}
	return s / 2
}

func selfIntersects(p []point) bool {
	n := len(p)
	if n < 4 {
		return false
	}
	for i := 0; i < n; i++ {
		a1, a2 := p[i], p[(i+1)%n]
		for j := i + 2; j < n; j++ {
			if i == 0 && j == n-1 {
				continue // adjacent via wraparound
			}
			b1, b2 := p[j], p[(j+1)%n]
			if segsCross(a1, a2, b1, b2) {
				return true
			}
		}
	}
	return false
}

func segsCross(a, b, c, d point) bool {
	o := func(p, q, r point) float64 { return (q.x-p.x)*(r.z-p.z) - (q.z-p.z)*(r.x-p.x) }
	d1, d2, d3, d4 := o(c, d, a), o(c, d, b), o(a, b, c), o(a, b, d)
	return ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) && ((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0))
}

// ---------- output safety check ----------

func verifyOutput(orig, result []byte, origRoot *jnode, pts []point) error {
	newRoot, err := parseJSON(result)
	if err != nil {
		return fmt.Errorf("new file does not parse: %v", err)
	}
	allowed := map[int]float64{} // start offset in original -> expected new value
	for _, p := range pts {
		if p.moved {
			allowed[p.xNode.start] = p.newX
			allowed[p.zNode.start] = p.newZ
		}
	}
	var cmp func(a, b *jnode, path string) error
	cmp = func(a, b *jnode, path string) error {
		if a.kind != b.kind {
			return fmt.Errorf("structure changed at %s", path)
		}
		switch a.kind {
		case kObject, kArray:
			if len(a.vals) != len(b.vals) {
				return fmt.Errorf("item count changed at %s", path)
			}
			for i := range a.vals {
				if a.kind == kObject && a.keys[i] != b.keys[i] {
					return fmt.Errorf("key changed at %s", path)
				}
				if err := cmp(a.vals[i], b.vals[i], path+"/"+strconv.Itoa(i)); err != nil {
					return err
				}
			}
		case kNumber:
			if want, ok := allowed[a.start]; ok {
				if b.num != want {
					return fmt.Errorf("snapped value wrong at %s", path)
				}
			} else if string(orig[a.start:a.end]) != string(result[b.start:b.end]) {
				return fmt.Errorf("an unrelated number changed at %s", path)
			}
		case kString, kBool, kNull:
			if a.str != b.str {
				return fmt.Errorf("value changed at %s", path)
			}
		}
		return nil
	}
	return cmp(origRoot, newRoot, "root")
}

// ---------- numbers ----------

func round6(v float64) float64 {
	r := math.Round(v*1e6) / 1e6
	if r == 0 {
		return 0
	}
	return r
}

func formatNum(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if s == "-0" {
		return "0"
	}
	return s
}
