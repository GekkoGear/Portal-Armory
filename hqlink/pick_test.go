package hqlink

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestFindChangesNothingAndLists(t *testing.T) {
	in := load(t, "small_pretty.json")
	rep, err := Find(in)
	if err != nil {
		t.Fatal(err)
	}
	b := result(t, rep, "HQ_B")
	want := []Candidate{{"VB1", "Abrams", nil}, {"VB2", "Abrams", nil}, {boat, "RHIB", nil}}
	if b.Status != Found || !reflect.DeepEqual(b.Found, want) || len(b.Added) != 0 {
		t.Errorf("HQ_B = %+v", b)
	}
	if a := result(t, rep, "HQ_A"); !reflect.DeepEqual(a.Found, []Candidate{{"VA2", "Abrams", nil}}) || !reflect.DeepEqual(a.AlreadyLinked, []string{"VA1"}) {
		t.Errorf("HQ_A = %+v", a)
	}
	if rep.Changed || rep.FoundCount() != 8 {
		t.Errorf("Changed=%v FoundCount=%d", rep.Changed, rep.FoundCount())
	}
}

func TestApplyOnlyPicked(t *testing.T) {
	in := load(t, "small_pretty.json")
	// Picked out of order, plus an id that isn't a candidate: Found order is used, junk ignored.
	out, rep, err := Apply(in, map[string][]string{"HQ_B": {boat, "VB1", "VOut"}})
	if err != nil {
		t.Fatal(err)
	}
	v, l := vehicles(t, out)
	if !reflect.DeepEqual(v["HQ_B"], []string{"VB1", boat}) {
		t.Errorf("HQ_B list = %v", v["HQ_B"])
	}
	if !reflect.DeepEqual(l["HQ_B"], []string{"HQArea", "InfantrySpawns", "VehicleSpawners"}) {
		t.Errorf("HQ_B linked = %v", l["HQ_B"])
	}
	b := result(t, rep, "HQ_B")
	if !reflect.DeepEqual(b.NotPicked, []string{"VB2"}) {
		t.Errorf("NotPicked = %v", b.NotPicked)
	}
	// Everything else unpicked: lists untouched.
	if !reflect.DeepEqual(v["HQ_A"], []string{"VA1"}) || v["HQ_Share"] != nil {
		t.Errorf("unpicked HQs changed: A=%v Share=%v", v["HQ_A"], v["HQ_Share"])
	}
	if r := result(t, rep, "HQ_Share"); r.Status != NonePicked {
		t.Errorf("HQ_Share = %s", r.Status)
	}
	sameExceptLists(t, in, out)
	if !strings.Contains(rep.Text("x"), "Found but left unchecked (1):") {
		t.Error("report missing not-picked list")
	}
}

func TestApplyNothingPicked(t *testing.T) {
	in := load(t, "small_pretty.json")
	out, rep, err := Apply(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Changed || !bytes.Equal(in, out) {
		t.Error("nothing picked should change nothing")
	}
}

func TestLinkEqualsPickAll(t *testing.T) {
	in := load(t, "small_min.json")
	rep, _ := Find(in)
	a, _, _ := Apply(in, rep.PickAll())
	b, _, _ := Link(in)
	if !bytes.Equal(a, b) {
		t.Error("Link should equal Apply(PickAll)")
	}
}
