package hqlink

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustLink(t *testing.T, in []byte) ([]byte, Report) {
	t.Helper()
	out, rep, err := Link(in)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(bytes.TrimPrefix(out, bom)) {
		t.Fatal("output is not valid JSON")
	}
	return out, rep
}

func result(t *testing.T, rep Report, id string) HQResult {
	t.Helper()
	for _, h := range rep.HQs {
		if h.ID == id {
			return h
		}
	}
	t.Fatalf("no result for %s", id)
	return HQResult{}
}

// vehicles returns each HQ's VehicleSpawners and linked lists from a spatial.
func vehicles(t *testing.T, b []byte) (map[string][]string, map[string][]string) {
	t.Helper()
	var d struct {
		Portal_Dynamic []struct {
			Type            string
			ID              string
			VehicleSpawners []string
			Linked          []string `json:"linked"`
		}
	}
	if err := json.Unmarshal(bytes.TrimPrefix(b, bom), &d); err != nil {
		t.Fatal(err)
	}
	v, l := map[string][]string{}, map[string][]string{}
	for _, o := range d.Portal_Dynamic {
		if o.Type == "HQ_PlayerSpawner" {
			v[o.ID], l[o.ID] = o.VehicleSpawners, o.Linked
		}
	}
	return v, l
}

const boat = "Veh & \"Boat\" é"

func TestLinkAddsInside(t *testing.T) {
	_, rep := mustLink(t, load(t, "small_pretty.json"))
	a := result(t, rep, "HQ_A")
	if a.Status != Added || !reflect.DeepEqual(a.Added, []string{"VA2"}) || !reflect.DeepEqual(a.AlreadyLinked, []string{"VA1"}) {
		t.Errorf("HQ_A = %+v", a)
	}
	want := []string{"VB1", "VB2", boat}
	for _, id := range []string{"HQ_B", "HQ_Share"} {
		r := result(t, rep, id)
		if !reflect.DeepEqual(r.Added, want) || !r.CreatedList {
			t.Errorf("%s = %+v", id, r)
		}
	}
	if r := result(t, rep, "HQ_EmptyList"); !reflect.DeepEqual(r.Added, []string{"VC1"}) || r.CreatedList {
		t.Errorf("HQ_EmptyList = %+v", r)
	}
	for _, h := range rep.HQs {
		for _, id := range h.Added {
			if id == "VOut" {
				t.Error("VOut is outside every polygon")
			}
		}
	}
	if rep.AddedCount() != 1+3+3+1 {
		t.Errorf("AddedCount = %d", rep.AddedCount())
	}
}

func TestLinkSkips(t *testing.T) {
	out, rep := mustLink(t, load(t, "small_pretty.json"))
	cases := map[string]HQStatus{"HQ_Empty": NothingFound, "HQ_NoArea": NoArea, "HQ_BadArea": BadArea}
	for id, st := range cases {
		if r := result(t, rep, id); r.Status != st || len(r.Added) != 0 {
			t.Errorf("%s = %+v, want %s", id, r, st)
		}
	}
	v, _ := vehicles(t, out)
	if len(v["HQ_Empty"]) != 0 || !reflect.DeepEqual(v["HQ_NoArea"], []string{"VA1"}) {
		t.Errorf("skipped HQs changed: %v", v)
	}
}

func TestLinkWritesLists(t *testing.T) {
	for _, f := range []string{"small_pretty.json", "small_min.json", "small_crlf.json"} {
		out, _ := mustLink(t, load(t, f))
		v, l := vehicles(t, out)
		if !reflect.DeepEqual(v["HQ_A"], []string{"VA1", "VA2"}) {
			t.Errorf("%s HQ_A = %v", f, v["HQ_A"])
		}
		if !reflect.DeepEqual(v["HQ_B"], []string{"VB1", "VB2", boat}) {
			t.Errorf("%s HQ_B = %v", f, v["HQ_B"])
		}
		if !reflect.DeepEqual(l["HQ_B"], []string{"HQArea", "InfantrySpawns", "VehicleSpawners"}) {
			t.Errorf("%s HQ_B linked = %v", f, l["HQ_B"])
		}
		if !reflect.DeepEqual(l["HQ_A"], []string{"HQArea", "InfantrySpawns", "VehicleSpawners"}) {
			t.Errorf("%s HQ_A linked changed: %v", f, l["HQ_A"])
		}
	}
}

func TestLinkPrettyLayout(t *testing.T) {
	out, _ := mustLink(t, load(t, "small_pretty.json"))
	s := string(out)
	wantNew := "            \"InfantrySpawns\": [\n                \"HQ_B/S1\"\n            ],\n" +
		"            \"VehicleSpawners\": [\n                \"VB1\",\n                \"VB2\",\n                \"Veh & \\\"Boat\\\" é\"\n            ],\n" +
		"            \"HQArea\": \"PolyB\","
	if !strings.Contains(s, wantNew) {
		t.Error("new VehicleSpawners list not laid out like InfantrySpawns")
	}
	wantAppend := "            \"VehicleSpawners\": [\n                \"VA1\",\n                \"VA2\"\n            ],"
	if !strings.Contains(s, wantAppend) {
		t.Error("appended entry not laid out like existing entries")
	}
	wantEmpty := "            \"VehicleSpawners\": [\n                \"VC1\"\n            ],"
	if !strings.Contains(s, wantEmpty) {
		t.Error("filled empty list not laid out like InfantrySpawns")
	}
	wantLinked := "                \"InfantrySpawns\",\n                \"VehicleSpawners\"\n            ]"
	if !strings.Contains(s, wantLinked) {
		t.Error("linked entry not laid out like existing entries")
	}
}

func TestLinkMinifiedStaysMinified(t *testing.T) {
	out, rep := mustLink(t, load(t, "small_min.json"))
	if bytes.ContainsAny(out, "\n ") && !bytes.Contains(load(t, "small_min.json"), []byte(" ")) {
		t.Error("minified output gained whitespace")
	}
	if !rep.Minified {
		t.Error("Minified should be true")
	}
	if !bytes.Contains(out, []byte(`"InfantrySpawns":["HQ_B/S1"],"VehicleSpawners":["VB1","VB2","Veh & \"Boat\" é"],"HQArea"`)) {
		t.Errorf("minified insert wrong")
	}
}

func TestSpliceCRLF(t *testing.T) {
	out, _ := mustLink(t, load(t, "small_crlf.json"))
	for i, c := range out {
		if c == '\n' && (i == 0 || out[i-1] != '\r') {
			t.Fatalf("bare \\n at byte %d", i)
		}
	}
}

func TestSpliceEscaping(t *testing.T) {
	out, _ := mustLink(t, load(t, "small_pretty.json"))
	if !bytes.Contains(out, []byte(`"Veh & \"Boat\" é"`)) {
		t.Error("id not written the way Godot writes it")
	}
	if bytes.Contains(out, []byte("\\u0026")) {
		t.Error("& was escaped")
	}
}

func TestLinkNoDuplicates(t *testing.T) {
	out, _ := mustLink(t, load(t, "small_pretty.json"))
	out2, rep := mustLink(t, out)
	if rep.Changed || !bytes.Equal(out, out2) {
		t.Error("second run should change nothing")
	}
	if r := result(t, rep, "HQ_B"); r.Status != UpToDate {
		t.Errorf("HQ_B second run = %s", r.Status)
	}
}

func TestLinkOverlap(t *testing.T) {
	out, rep := mustLink(t, load(t, "overlap_pretty.json"))
	if len(rep.Overlaps) != 1 || rep.Overlaps[0].SpawnerID != "VA2" ||
		!reflect.DeepEqual(rep.Overlaps[0].HQIDs, []string{"HQ_A", "HQ_B", "HQ_Share"}) {
		t.Fatalf("Overlaps = %+v", rep.Overlaps)
	}
	v, _ := vehicles(t, out)
	for id, list := range v {
		for _, s := range list {
			if s == "VA2" {
				t.Errorf("VA2 added to %s", id)
			}
		}
	}
	if r := result(t, rep, "HQ_A"); r.Status != UpToDate {
		t.Errorf("HQ_A = %s", r.Status)
	}
}

func TestLinkKeepsBOM(t *testing.T) {
	in := append(append([]byte(nil), bom...), load(t, "small_pretty.json")...)
	out, _ := mustLink(t, in)
	if !bytes.HasPrefix(out, bom) {
		t.Error("BOM lost")
	}
}

func TestLinkNotSpatial(t *testing.T) {
	for _, in := range []string{`{"foo":1}`, `not json`, ``, `[1,2]`} {
		if _, _, err := Link([]byte(in)); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}

// keyOrder lists each object's keys in file order.
func keyOrder(t *testing.T, b []byte) [][]string {
	t.Helper()
	objs, err := locateDynamic(bytes.TrimPrefix(b, bom))
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, o := range objs {
		var ks []string
		for k := range o.Fields {
			ks = append(ks, k)
		}
		sort.Slice(ks, func(i, j int) bool { return o.Fields[ks[i]].KeyStart < o.Fields[ks[j]].KeyStart })
		out = append(out, ks)
	}
	return out
}

// sameExceptLists checks that, ignoring VehicleSpawners and linked, nothing
// changed, and that key order is the same apart from a new VehicleSpawners key.
func sameExceptLists(t *testing.T, in, out []byte) {
	t.Helper()
	strip := func(b []byte) any {
		var d map[string]any
		if err := json.Unmarshal(bytes.TrimPrefix(b, bom), &d); err != nil {
			t.Fatal(err)
		}
		for _, o := range d["Portal_Dynamic"].([]any) {
			if m, ok := o.(map[string]any); ok {
				delete(m, "VehicleSpawners")
				delete(m, "linked")
			}
		}
		return d
	}
	if !reflect.DeepEqual(strip(in), strip(out)) {
		t.Error("something other than VehicleSpawners/linked changed")
	}
	ki, ko := keyOrder(t, in), keyOrder(t, out)
	if len(ki) != len(ko) {
		t.Fatal("object count changed")
	}
	for i := range ki {
		var without []string
		for _, k := range ko[i] {
			if k != "VehicleSpawners" || contains(ki[i], "VehicleSpawners") {
				without = append(without, k)
			}
		}
		if !reflect.DeepEqual(ki[i], without) {
			t.Errorf("object %d key order changed: %v -> %v", i, ki[i], ko[i])
		}
	}
}

func TestLinkOnlyListsChange(t *testing.T) {
	for _, f := range []string{"small_pretty.json", "small_min.json", "small_crlf.json"} {
		in := load(t, f)
		out, _ := mustLink(t, in)
		sameExceptLists(t, in, out)
	}
}

func TestReportText(t *testing.T) {
	_, rep := mustLink(t, load(t, "small_pretty.json"))
	txt := rep.Text("small_pretty.json")
	for _, want := range []string{"Added (1):", "VA2", string(NothingFound), "had no VehicleSpawners list", "Added 8 vehicle spawner(s)", "\r\n"} {
		if !strings.Contains(txt, want) {
			t.Errorf("report missing %q", want)
		}
	}
	for i := range txt {
		if txt[i] == '\n' && (i == 0 || txt[i-1] != '\r') {
			t.Fatal("report has a bare \\n")
		}
	}
}
