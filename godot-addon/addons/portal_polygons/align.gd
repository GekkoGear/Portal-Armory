@tool
extends RefCounted
## Top-down polygon volume alignment. GDScript port of the Spatial Poly Snap engine.
##
## Volumes are handled in scene tree order. The first volume never moves. Each
## later volume's points move onto the closest corner of a volume above it (within
## the snap distance), or, if no corner is that close, slide straight onto the
## closest edge. A point that has been placed is never moved again, so running it
## twice changes nothing. Points the user placed or rejected are "fixed": they stay
## exactly where the user wants them and later volumes line up with them.

## Gaps smaller than this (meters) already count as lined up. Godot stores points
## as 32-bit numbers, so two corners that share a spot can differ by a hair.
const ALIGNED := 0.0005


## vols:  Array of PackedVector2Array, world X/Z of every volume's points, in tree order.
## names: Array of String, a label per volume (for report lines).
## fixed: Dictionary Vector2i(volume, point) -> Vector2 world position the user chose.
## Returns a Dictionary:
##   after:  Array of PackedVector2Array (same shape as vols)
##   moves:  Dictionary Vector2i -> { kind: "corner"/"edge", onto: String, dist: float }
##   issues: Array of { text: String, at: Vector2 }   (problems to look at)
##   near:   Array of { text: String, at: Vector2 }   (corners close but not snapped)
##   lower:  Array of { text: String, at: Vector2 }   (corners on a lower volume's edge)
static func run(vols: Array, names: Array, fixed: Dictionary, snap: float, near_dist: float) -> Dictionary:
	var after: Array = []
	for v in vols:
		after.append(PackedVector2Array(v))
	var moves := {}
	var issues: Array = []
	var cell := maxf(snap, 0.001)
	var grid := {}   # Vector2i cell -> Array of Vector2i(vol, idx), finished volumes only
	var boxes: Array = []  # Rect2 per finished volume
	boxes.resize(vols.size())

	for vi in range(vols.size()):
		var pts: PackedVector2Array = after[vi]
		var n := pts.size()
		var plans: Array = []
		plans.resize(n)
		for k in range(n):
			var key := Vector2i(vi, k)
			if fixed.has(key):
				pts[k] = fixed[key]
				plans[k] = null
				continue
			if vi == 0:
				plans[k] = null
				continue
			var p: Vector2 = vols[vi][k]
			var best = null
			var best_d := INF
			# 1. closest corner of a volume above
			for j in _near_cells(grid, p, cell, snap):
				var q: Vector2 = after[j.x][j.y]
				var d := p.distance_to(q)
				if d <= snap and (d < best_d or (d == best_d and best != null and j.x < best.target.x)):
					best_d = d
					best = {"kind": "corner", "target": j, "to": q, "dist": d}
			# 2. closest edge of a volume above
			if best == null:
				for wi in range(vi):
					var b: Rect2 = boxes[wi]
					if not b.grow(snap).has_point(p):
						continue
					var w: PackedVector2Array = after[wi]
					var m := w.size()
					for e in range(m):
						var a: Vector2 = w[e]
						var c: Vector2 = w[(e + 1) % m]
						var on := Geometry2D.get_closest_point_to_segment(p, a, c)
						var d2 := p.distance_to(on)
						if d2 <= snap and d2 < best_d:
							best_d = d2
							best = {"kind": "edge", "vol": wi, "edge": e, "to": on, "dist": d2}
			plans[k] = best

		# Two points of this volume picking the same corner would collapse an edge.
		var claimed := {}
		for k in range(n):
			var pl = plans[k]
			if pl == null or pl.kind != "corner":
				continue
			var t: Vector2i = pl.target
			if claimed.has(t):
				var o: int = claimed[t]
				var loser := k
				if pl.dist < plans[o].dist:
					loser = o
					claimed[t] = k
				issues.append({"text": "%s points %d and %d are both closest to %s point %d. Only point %d was snapped; point %d was left as-is." % [
					names[vi], o + 1, k + 1, names[t.x], t.y + 1, claimed[t] + 1, loser + 1], "at": vols[vi][loser]})
				plans[loser] = null
			else:
				claimed[t] = k

		for k in range(n):
			var pl = plans[k]
			if pl == null or pl.dist < ALIGNED:
				continue
			var key := Vector2i(vi, k)
			if pl.kind == "corner":
				pts[k] = pl.to
				moves[key] = {"kind": "corner", "onto": "%s point %d" % [names[pl.target.x], pl.target.y + 1], "dist": pl.dist}
			else:
				var wn: int = after[pl.vol].size()
				var label := "%s edge %d-%d" % [names[pl.vol], pl.edge + 1, (pl.edge + 1) % wn + 1]
				# Landing right next to some corner above would make a re-run snap it there instead.
				var clash = null
				for j in _near_cells(grid, pl.to, cell, snap):
					if pl.to.distance_to(after[j.x][j.y]) <= snap:
						clash = j
						break
				if clash != null:
					issues.append({"text": "%s point %d was not slid onto %s: that spot is within %.2f m of %s point %d. Line it up by hand." % [
						names[vi], k + 1, label, snap, names[clash.x], clash.y + 1], "at": vols[vi][k]})
					continue
				pts[k] = pl.to
				moves[key] = {"kind": "edge", "onto": label, "dist": pl.dist}
		after[vi] = pts

		# Finish this volume: later volumes may line up with it.
		var box := Rect2(pts[0], Vector2.ZERO) if n > 0 else Rect2()
		for k in range(n):
			box = box.expand(pts[k])
			var c2 := _cell(pts[k], cell)
			if not grid.has(c2):
				grid[c2] = []
			grid[c2].append(Vector2i(vi, k))
		boxes[vi] = box

	return {
		"after": after,
		"moves": moves,
		"issues": issues + _shape_issues(vols, after, names),
		"near": _near_misses(after, names, snap, near_dist),
		"lower": _lower_edges(after, names, snap),
	}


static func _cell(p: Vector2, size: float) -> Vector2i:
	return Vector2i(floori(p.x / size), floori(p.y / size))


static func _near_cells(grid: Dictionary, p: Vector2, size: float, r: float) -> Array:
	var out: Array = []
	var reach := ceili(r / size)
	var c := _cell(p, size)
	for dx in range(-reach, reach + 1):
		for dz in range(-reach, reach + 1):
			var k := Vector2i(c.x + dx, c.y + dz)
			if grid.has(k):
				out.append_array(grid[k])
	return out


## Corners on different volumes that are close but outside the snap distance.
static func _near_misses(after: Array, names: Array, snap: float, near_dist: float) -> Array:
	var cell := maxf(near_dist, 0.001)
	var grid := {}
	for vi in range(after.size()):
		for k in range(after[vi].size()):
			var c := _cell(after[vi][k], cell)
			if not grid.has(c):
				grid[c] = []
			grid[c].append(Vector2i(vi, k))
	var found: Array = []
	for vi in range(after.size()):
		for k in range(after[vi].size()):
			var p: Vector2 = after[vi][k]
			var best_d := INF
			var best := Vector2i(-1, -1)
			for j in _near_cells(grid, p, cell, near_dist):
				if j.x == vi:
					continue
				var d := p.distance_to(after[j.x][j.y])
				if d < best_d:
					best_d = d
					best = j
			var me := Vector2i(vi, k)
			if best.x >= 0 and best_d > snap and best_d <= near_dist and (me.x < best.x or (me.x == best.x and me.y < best.y)):
				found.append({"d": best_d, "text": "%.3f m apart: %s point %d and %s point %d" % [best_d, names[vi], k + 1, names[best.x], best.y + 1], "at": p})
	found.sort_custom(func(a, b): return a.d < b.d)
	return found


## Corners of a higher volume sitting near a lower volume's edge. The top-down
## rule can't move the higher one, so these are only listed.
static func _lower_edges(after: Array, names: Array, snap: float) -> Array:
	var out: Array = []
	var boxes: Array = []
	for w in after:
		var b := Rect2(w[0], Vector2.ZERO) if w.size() > 0 else Rect2()
		for p in w:
			b = b.expand(p)
		boxes.append(b.grow(snap))
	for vi in range(after.size()):
		for k in range(after[vi].size()):
			var p: Vector2 = after[vi][k]
			for wi in range(vi + 1, after.size()):
				if not boxes[wi].has_point(p):
					continue
				var w: PackedVector2Array = after[wi]
				var corner := false
				for q in w:
					if p.distance_to(q) <= snap:
						corner = true
						break
				if corner:
					continue
				for e in range(w.size()):
					var on := Geometry2D.get_closest_point_to_segment(p, w[e], w[(e + 1) % w.size()])
					var d := p.distance_to(on)
					if d > ALIGNED and d <= snap:
						out.append({"text": "%s point %d is %.3f m from %s edge %d-%d (lower in the scene, so it can't be moved onto it)" % [
							names[vi], k + 1, d, names[wi], e + 1, (e + 1) % w.size() + 1], "at": p})
						break
	return out


static func _shape_issues(before: Array, after: Array, names: Array) -> Array:
	var out: Array = []
	for vi in range(after.size()):
		var a: PackedVector2Array = after[vi]
		var b: PackedVector2Array = before[vi]
		if a.size() < 3:
			out.append({"text": "%s has only %d point(s); a volume needs at least 3." % [names[vi], a.size()], "at": a[0] if a.size() > 0 else Vector2.ZERO})
			continue
		var changed := a != b
		if changed and _crosses(a) and not _crosses(b):
			out.append({"text": "%s ends up with edges crossing each other. Check it before finalizing." % names[vi], "at": a[0]})
		elif _crosses(b):
			out.append({"text": "%s already has edges crossing each other in your scene." % names[vi], "at": b[0]})
		for k in range(a.size()):
			var k2 := (k + 1) % a.size()
			if a[k].distance_to(a[k2]) < ALIGNED:
				var when := "already like this in your scene" if b[k].distance_to(b[k2]) < ALIGNED else "after lining up"
				out.append({"text": "%s has points %d and %d stacked on the same spot (%s). Delete one of them." % [names[vi], k + 1, k2 + 1, when], "at": a[k]})
	return out


static func _crosses(p: PackedVector2Array) -> bool:
	var n := p.size()
	if n < 4:
		return false
	for i in range(n):
		for j in range(i + 2, n):
			if i == 0 and j == n - 1:
				continue
			if Geometry2D.segment_intersects_segment(p[i], p[(i + 1) % n], p[j], p[(j + 1) % n]) != null:
				var x: Vector2 = Geometry2D.segment_intersects_segment(p[i], p[(i + 1) % n], p[j], p[(j + 1) % n])
				# touching at an end point isn't crossing
				if x.distance_to(p[i]) > ALIGNED and x.distance_to(p[(i + 1) % n]) > ALIGNED and x.distance_to(p[j]) > ALIGNED and x.distance_to(p[(j + 1) % n]) > ALIGNED:
					return true
	return false
