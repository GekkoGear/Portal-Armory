package hqlink

import "testing"

var square = []Point{{0, 0}, {10, 0}, {10, 10}, {0, 10}}

func TestInsideSquare(t *testing.T) {
	if !insideXZ(5, 5, square) {
		t.Error("(5,5) should be inside")
	}
	if insideXZ(15, 5, square) {
		t.Error("(15,5) should be outside")
	}
}

func TestInsideConcaveL(t *testing.T) {
	l := []Point{{0, 0}, {10, 0}, {10, 4}, {4, 4}, {4, 10}, {0, 10}}
	if insideXZ(7, 7, l) {
		t.Error("notch point should be outside")
	}
	if !insideXZ(7, 2, l) || !insideXZ(2, 7, l) {
		t.Error("arm points should be inside")
	}
}

func TestOnEdgeAndVertex(t *testing.T) {
	if !insideXZ(0, 5, square) {
		t.Error("edge point should count as inside")
	}
	if !insideXZ(0, 0, square) {
		t.Error("corner should count as inside")
	}
}

func TestTooFewPoints(t *testing.T) {
	if insideXZ(1, 1, []Point{{0, 0}, {5, 5}}) {
		t.Error("2-point polygon is never inside")
	}
}
