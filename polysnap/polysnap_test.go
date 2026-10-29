package polysnap

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

type xz struct{ x, z float64 }

func spatial(vols map[string][]xz, order []string) []byte {
	var parts []string
	for _, name := range order {
		var pts []string
		for _, p := range vols[name] {
			pts = append(pts, fmt.Sprintf(`{"x": %v, "y": 5.5, "z": %v}`, p.x, p.z))
		}
		parts = append(parts, fmt.Sprintf(`{"name": "%s", "type": "PolygonVolume", "id": "Sectors/%s", "height": 10, "color": "ff0000", "points": [%s]}`,
			name, name, strings.Join(pts, ", ")))
	}
	return []byte(`{"Portal_Dynamic": [` + strings.Join(parts, ",\n") + `, {"name": "Cam", "type": "DeployCam", "id": "Cam", "position": {"x": 1.23456789, "y": 2, "z": 3}}], "Static": []}`)
}

func pointsOf(t *testing.T, data []byte, name string) []xz {
	t.Helper()
	var doc struct {
		Portal_Dynamic []struct {
			Name   string
			Points []struct{ X, Y, Z float64 }
		}
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	for _, o := range doc.Portal_Dynamic {
		if o.Name == name {
			var out []xz
			for _, p := range o.Points {
				if p.Y != 5.5 {
					t.Fatalf("%s: height changed to %v", name, p.Y)
				}
				out = append(out, xz{p.X, p.Z})
			}
			return out
		}
	}
	t.Fatalf("volume %s not found", name)
	return nil
}

func TestTopDownSnapping(t *testing.T) {
	in := spatial(map[string][]xz{
		"A": {{0, 0}, {10, 0}, {10, 10}, {0, 10}},
		"B": {{10.03, 0.02}, {20, 0}, {20, 10}, {10.01, 9.98}}, // two corners near A
		"C": {{9.97, 10.04}, {20.02, 10.01}, {15, 20}},         // 3-way corner at (10,10)
		"G": {{5, 10.05}, {5, 15}, {3, 15}},                    // corner on A's top edge
		"F": {{0, -0.3}, {-5, -5}, {5, -5}},                    // near miss, 0.3 m
	}, []string{"A", "B", "C", "G", "F"})

	res := Process(in, "Test.spatial.json")
	if res.Snapped == nil {
		t.Fatalf("failed: %s %v", res.Failed, res.Issues)
	}
	// A never moves.
	if got := pointsOf(t, res.Snapped, "A"); got[1] != (xz{10, 0}) || got[2] != (xz{10, 10}) {
		t.Errorf("A moved: %v", got)
	}
	// B and C land exactly on A's corners; C's second corner on B's.
	b, c := pointsOf(t, res.Snapped, "B"), pointsOf(t, res.Snapped, "C")
	if b[0] != (xz{10, 0}) || b[3] != (xz{10, 10}) || c[0] != (xz{10, 10}) || c[1] != (xz{20, 10}) {
		t.Errorf("corner snaps wrong: B=%v C=%v", b, c)
	}
	// G slides onto A's top edge (z = 10), keeping x.
	if g := pointsOf(t, res.Snapped, "G"); g[0] != (xz{5, 10}) {
		t.Errorf("edge move wrong: %v", g[0])
	}
	// F is only reported.
	if f := pointsOf(t, res.Snapped, "F"); f[0] != (xz{0, -0.3}) || len(res.NearMisses) != 1 {
		t.Errorf("near miss changed or not reported: %v %v", f[0], res.NearMisses)
	}
	if res.CornerSnaps != 4 || res.EdgeSnaps != 1 || len(res.Issues) != 0 {
		t.Errorf("counts: corners %d edges %d issues %v", res.CornerSnaps, res.EdgeSnaps, res.Issues)
	}

	// Running again on the output changes nothing.
	again := Process(res.Snapped, "Test.spatial.json")
	if string(again.Snapped) != string(res.Snapped) || again.Moved != 0 {
		t.Errorf("second pass moved %d points", again.Moved)
	}
}

func TestOnlyPointNumbersChange(t *testing.T) {
	in := spatial(map[string][]xz{
		"A": {{0, 0}, {10, 0}, {10, 10}},
		"B": {{10.05, 0.01}, {20, 0}, {20, 10}},
	}, []string{"A", "B"})
	res := Process(in, "x.spatial.json")
	// Same length lines except the one changed number; everything else byte-identical.
	want := strings.Replace(string(in), `"x": 10.05, "y": 5.5, "z": 0.01`, `"x": 10, "y": 5.5, "z": 0`, 1)
	if string(res.Snapped) != want {
		t.Errorf("unexpected bytes changed:\n%s", res.Snapped)
	}
}

func TestMinifyRules(t *testing.T) {
	in := spatial(map[string][]xz{"A": {{0.12345, 0}, {10, 0}, {10, 10}}}, []string{"A"})
	res := Process(in, "x.spatial.json")
	got := string(res.Minified)
	for _, want := range []string{
		`"name":"a"`, `"id":"b/a"`, // names and id segments shortened
		`{"x":0.123,"y":5.5,"z":0}`, // points to 3 decimals
		`"position":{"x":1.2346,`,   // positions to 4 decimals
	} {
		if !strings.Contains(got, want) {
			t.Errorf("minified output missing %s\n%s", want, got)
		}
	}
	if strings.Contains(got, "color") || strings.Contains(got, " ") {
		t.Errorf("color not stripped or whitespace left: %s", got)
	}
	if already := Process(in, "x.spatial.minified.json"); already.Minified != nil {
		t.Error("already-minified input was minified again")
	}
}

func TestBadInput(t *testing.T) {
	if res := Process([]byte(`{"a":`), "x.json"); res.Snapped != nil || res.Failed == "" {
		t.Error("invalid JSON not rejected")
	}
	res := Process([]byte(`{"Portal_Dynamic":[{"name":"x","type":"PolygonVolume","points":[{"x":0,"y":0,"z":0}]}]}`), "x.json")
	if res.Snapped == nil || len(res.Issues) == 0 {
		t.Error("too-few-points volume not reported")
	}
}

func TestDotnetFormatting(t *testing.T) {
	cases := map[float64]string{1.6e-05: "1.6E-05", 0.0001: "0.0001", 1e15: "1E+15", 123.4: "123.4", -0.5: "-0.5", 40000: "40000"}
	for v, want := range cases {
		if got := formatDotnet(v); got != want {
			t.Errorf("formatDotnet(%v) = %s, want %s", v, got, want)
		}
	}
	if r := dotnetRoundAway(2.5, 0); r != 3 {
		t.Errorf("round away 2.5 -> %v", r)
	}
	if r := dotnetRoundAway(-1.23455, 4); math.Abs(r-(-1.2346)) > 1e-12 && math.Abs(r-(-1.2345)) > 1e-12 {
		t.Errorf("round -1.23455 -> %v", r)
	}
}

func TestRenderPreview(t *testing.T) {
	in := spatial(map[string][]xz{
		"A": {{0, 0}, {10, 0}, {10, 10}},
		"B": {{10.05, 0.01}, {20, 0}, {20, 10}},
	}, []string{"A", "B"})
	for _, res := range []*Result{Process(in, "x.spatial.json"), Process([]byte(`{"Portal_Dynamic":[]}`), "empty.json")} {
		png, err := RenderPreview(res)
		if err != nil || len(png) < 8 || string(png[1:4]) != "PNG" {
			t.Fatalf("preview failed: %v", err)
		}
	}
}
