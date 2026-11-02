package hqlink

// Tests against real maps. The maps are never committed (the repo is public);
// point these env vars at local copies to run them:
//   HQLINK_DEADSEAS = Dead Seas spatial (vehicles already linked)
//   HQLINK_BADLANDS = Andy's SDK 5 MP_Badlands_Conquest.spatial.json (no vehicles linked)
//   HQLINK_GULF     = Andy's SDK 5 GulfofLyndon_Conquest_Large.spatial.json (two HQs share a polygon)

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func realMap(t *testing.T, env string) []byte {
	t.Helper()
	p := os.Getenv(env)
	if p == "" {
		t.Skipf("%s not set", env)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDeadSeas(t *testing.T) {
	in := realMap(t, "HQLINK_DEADSEAS")
	out, rep := mustLink(t, in)
	// Team 1 already has everything inside its polygon.
	if r := result(t, rep, "TEAM 1 Nato"); r.Status != UpToDate || len(r.AlreadyLinked) != 13 {
		t.Errorf("Team 1: %s, %d linked", r.Status, len(r.AlreadyLinked))
	}
	// Team 2's polygon also covers 10 capture point B/J spawners, which get added.
	r2 := result(t, rep, "TEAM 2 Pax")
	if r2.Status != Added || len(r2.AlreadyLinked) != 13 || len(r2.Added) != 10 {
		t.Errorf("Team 2: %s, %d linked, %d added", r2.Status, len(r2.AlreadyLinked), len(r2.Added))
	}
	for _, id := range r2.Added {
		if !strings.HasPrefix(id, "Obectives/CapturePoint") {
			t.Errorf("unexpected Team 2 add %s", id)
		}
	}
	sameExceptLists(t, in, out)

	// Remove the boats from both HQs, expect them back.
	var d map[string]any
	json.Unmarshal(in, &d)
	removed := 0
	for _, o := range d["Portal_Dynamic"].([]any) {
		m := o.(map[string]any)
		if m["type"] != "HQ_PlayerSpawner" {
			continue
		}
		var keep []any
		for _, v := range m["VehicleSpawners"].([]any) {
			s := v.(string)
			if strings.Contains(s, "Boat") || strings.Contains(s, "Rhib") {
				removed++
				continue
			}
			keep = append(keep, v)
		}
		m["VehicleSpawners"] = keep
	}
	if removed != 14 {
		t.Fatalf("removed %d boats, want 14", removed)
	}
	stripped, _ := json.MarshalIndent(d, "", "    ")
	_, rep2 := mustLink(t, stripped)
	if n := len(result(t, rep2, "TEAM 1 Nato").Added); n != 7 {
		t.Errorf("Team 1 re-added %d, want 7", n)
	}
	if n := len(result(t, rep2, "TEAM 2 Pax").Added); n != 17 {
		t.Errorf("Team 2 added %d, want 17 (7 boats + 10 capture point)", n)
	}
}

func TestBadlands(t *testing.T) {
	in := realMap(t, "HQLINK_BADLANDS")
	out, rep := mustLink(t, in)
	for _, h := range rep.HQs {
		if h.Status != Added || len(h.Added) != 14 || !h.CreatedList {
			t.Errorf("%s: %s, added %d", h.ID, h.Status, len(h.Added))
		}
	}
	if bytes.Contains(out, []byte("\n")) {
		t.Error("minified map gained newlines")
	}
	sameExceptLists(t, in, out)
}

func TestGulfSharedPolygon(t *testing.T) {
	in := realMap(t, "HQLINK_GULF")
	out, rep := mustLink(t, in)
	a, b := result(t, rep, "aoq/aop"), result(t, rep, "aoq/aos")
	if len(a.Added) == 0 || len(a.Added) != len(b.Added) {
		t.Errorf("shared polygon HQs: %d vs %d", len(a.Added), len(b.Added))
	}
	if len(rep.Overlaps) != 0 {
		t.Errorf("unexpected overlaps: %d", len(rep.Overlaps))
	}
	sameExceptLists(t, in, out)
}
