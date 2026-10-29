package polysnap

import (
	"fmt"
	"math"
)

// priorityAlign lines volumes up in the order they appear in the file.
// The first volume is never touched. Each later volume's points are moved onto
// volumes ABOVE it in the list (already final), never the other way round, so
// a point that has been placed is never moved again.
//
// For each point of the current volume:
//  1. If an earlier volume has a corner within snapDistance, copy the closest
//     such corner's exact position.
//  2. Otherwise, if an earlier volume has an edge within snapDistance, slide the
//     point straight onto the closest such edge.
func priorityAlign(vols []volume, pts []point, rep *report) (cornerSnaps, edgeSnaps, already int) {
	g := &grid{size: snapDistance, cells: map[cellKey][]int{}}
	type bbox struct{ minX, maxX, minZ, maxZ float64 }
	boxes := make([]bbox, len(vols))

	finalize := func(vi int) {
		b := bbox{math.MaxFloat64, -math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64}
		for _, pi := range vols[vi].pts {
			p := cur(pts[pi])
			k := g.key(p.x, p.z)
			g.cells[k] = append(g.cells[k], pi)
			b.minX, b.maxX = math.Min(b.minX, p.x), math.Max(b.maxX, p.x)
			b.minZ, b.maxZ = math.Min(b.minZ, p.z), math.Max(b.maxZ, p.z)
		}
		boxes[vi] = b
	}

	type plan struct {
		kind   int // 0 none, 1 corner, 2 edge
		target int // corner: point index; edge: first end point
		end2   int // edge: second end point
		vol    int
		edgeK  int
		d      float64
		nx, nz float64
	}

	for vi := range vols {
		if vi == 0 {
			finalize(0)
			continue
		}
		v := vols[vi]
		plans := make([]plan, len(v.pts))
		for k, pi := range v.pts {
			p := pts[pi]
			best := plan{d: math.MaxFloat64}
			// 1. closest corner on an earlier volume
			g.within(p.x, p.z, snapDistance, func(j int) {
				q := cur(pts[j])
				d := dist(p, q)
				if d <= snapDistance && (d < best.d || (d == best.d && q.vol < best.vol)) {
					best = plan{kind: 1, target: j, vol: q.vol, d: d, nx: q.x, nz: q.z}
				}
			})
			if best.kind == 0 {
				// 2. closest edge on an earlier volume
				for wi := 0; wi < vi; wi++ {
					b := boxes[wi]
					if p.x < b.minX-snapDistance || p.x > b.maxX+snapDistance || p.z < b.minZ-snapDistance || p.z > b.maxZ+snapDistance {
						continue
					}
					w := vols[wi]
					n := len(w.pts)
					for e := 0; e < n; e++ {
						a, c := cur(pts[w.pts[e]]), cur(pts[w.pts[(e+1)%n]])
						if d := segDist(p, a, c); d <= snapDistance && d < best.d {
							dx, dz := c.x-a.x, c.z-a.z
							t := math.Max(0, math.Min(1, ((p.x-a.x)*dx+(p.z-a.z)*dz)/(dx*dx+dz*dz)))
							best = plan{kind: 2, target: w.pts[e], end2: w.pts[(e+1)%n], vol: wi, edgeK: e, d: d,
								nx: round6(a.x + t*dx), nz: round6(a.z + t*dz)}
						}
					}
				}
			}
			plans[k] = best
		}

		// Two points of this volume landing on the same corner would collapse its edge.
		claimed := map[int]int{} // target corner -> plan index
		for k, pl := range plans {
			if pl.kind != 1 {
				continue
			}
			if o, ok := claimed[pl.target]; ok {
				loser := k
				if pl.d < plans[o].d {
					loser, claimed[pl.target] = o, k
				}
				rep.issue("Volume \"%s\" points %d and %d are both closest to %s pt %d. Only the closer one (point %d) was snapped there; point %d was left as-is.",
					v.name, pts[v.pts[o]].idx+1, pts[v.pts[k]].idx+1, vols[pl.vol].name, pts[pl.target].idx+1,
					pts[v.pts[claimed[pl.target]]].idx+1, pts[v.pts[loser]].idx+1)
				plans[loser].kind = 0
				continue
			}
			claimed[pl.target] = k
		}

		for k, pl := range plans {
			pi := v.pts[k]
			p := &pts[pi]
			switch pl.kind {
			case 1:
				if pl.d == 0 {
					already++
					continue
				}
				p.newX, p.newZ, p.moved, p.how, p.onto = pl.nx, pl.nz, true, 1, fmt.Sprintf("%s pt %d", vols[pl.vol].name, pts[pl.target].idx+1)
				cornerSnaps++
				rep.snapped = append(rep.snapped, fmt.Sprintf("%s pt %d -> %s pt %d (%s, %s), moved %.4fm",
					v.name, p.idx+1, vols[pl.vol].name, pts[pl.target].idx+1, formatNum(pl.nx), formatNum(pl.nz), pl.d))
			case 2:
				if pl.d < 2e-6 {
					continue // already on the line
				}
				w := vols[pl.vol]
				label := fmt.Sprintf("%s pt %d %s -> edge between points %d and %d of %s",
					v.name, p.idx+1, fmtXZ(*p), pl.edgeK+1, (pl.edgeK+1)%len(w.pts)+1, w.name)
				// If the spot on the line is right next to an earlier corner, a
				// re-run would snap it to that corner instead, so leave it for review.
				target := point{x: pl.nx, z: pl.nz}
				clash := -1
				g.within(pl.nx, pl.nz, snapDistance, func(j int) {
					if clash < 0 && dist(target, cur(pts[j])) <= snapDistance {
						clash = j
					}
				})
				if clash >= 0 {
					rep.issue("Did not move %s: that spot is within %.2fm of %s pt %d. Line this spot up by hand.",
						label, snapDistance, vols[pts[clash].vol].name, pts[clash].idx+1)
					continue
				}
				p.newX, p.newZ, p.moved, p.how, p.onto = pl.nx, pl.nz, true, 2, fmt.Sprintf("%s edge %d-%d", w.name, pl.edgeK+1, (pl.edgeK+1)%len(w.pts)+1)
				edgeSnaps++
				rep.tJunction = append(rep.tJunction, fmt.Sprintf("%s, moved %.4fm to (%s, %s)", label, pl.d, formatNum(pl.nx), formatNum(pl.nz)))
			}
		}
		finalize(vi)
	}

	// Corners of EARLIER volumes that sit on a LATER volume's edge can't be fixed
	// without moving the earlier volume or adding a point, so list them.
	for vi := range vols {
		for _, pi := range vols[vi].pts {
			p := cur(pts[pi])
			for wi := vi + 1; wi < len(vols); wi++ {
				w := vols[wi]
				near := false
				for _, qi := range w.pts {
					if dist(p, cur(pts[qi])) <= snapDistance {
						near = true
						break
					}
				}
				if near {
					continue
				}
				n := len(w.pts)
				for e := 0; e < n; e++ {
					d := segDist(p, cur(pts[w.pts[e]]), cur(pts[w.pts[(e+1)%n]]))
					if d > 2e-6 && d <= snapDistance {
						rep.onEdge = append(rep.onEdge, fmt.Sprintf("%s pt %d %s is %.3fm from the edge between points %d and %d of %s (which is lower in the list)",
							vols[vi].name, p.idx+1, fmtXZ(p), d, e+1, (e+1)%n+1, w.name))
						break
					}
				}
			}
		}
	}
	return
}
