package hqlink

import "math"

// Point is a polygon corner seen from above (Godot X and Z; Y is ignored).
type Point struct{ X, Z float64 }

const edgeTolerance = 1e-6

// insideXZ reports whether (x, z) is inside poly, seen from above.
// Points on an edge or corner count as inside. Fewer than 3 points is never inside.
func insideXZ(x, z float64, poly []Point) bool {
	n := len(poly)
	if n < 3 {
		return false
	}
	for i := 0; i < n; i++ {
		if onSegment(x, z, poly[i], poly[(i+1)%n]) {
			return true
		}
	}
	in := false
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		a, b := poly[i], poly[j]
		if (a.Z > z) != (b.Z > z) {
			cross := (b.X-a.X)*(z-a.Z)/(b.Z-a.Z) + a.X
			if x < cross {
				in = !in
			}
		}
	}
	return in
}

func onSegment(x, z float64, a, b Point) bool {
	cross := (b.X-a.X)*(z-a.Z) - (b.Z-a.Z)*(x-a.X)
	length := math.Hypot(b.X-a.X, b.Z-a.Z)
	if length == 0 {
		return math.Hypot(x-a.X, z-a.Z) <= edgeTolerance
	}
	if math.Abs(cross)/length > edgeTolerance {
		return false
	}
	return x >= math.Min(a.X, b.X)-edgeTolerance && x <= math.Max(a.X, b.X)+edgeTolerance &&
		z >= math.Min(a.Z, b.Z)-edgeTolerance && z <= math.Max(a.Z, b.Z)+edgeTolerance
}
