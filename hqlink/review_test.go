package hqlink

import (
	"reflect"
	"strings"
	"testing"
)

// Regression tests for issues found in code review.

const sq = `"points":[{"x":0,"y":0,"z":0},{"x":10,"y":0,"z":0},{"x":10,"y":0,"z":10},{"x":0,"y":0,"z":10}]`

func spatialOf(objs ...string) []byte {
	return []byte(`{"Portal_Dynamic":[` + strings.Join(objs, ",") + `],"Static":[]}`)
}

func TestNullVehicleSpawnersSkipped(t *testing.T) {
	in := spatialOf(
		`{"type":"HQ_PlayerSpawner","id":"H","HQArea":"P","InfantrySpawns":["a"],"VehicleSpawners":null,"linked":["HQArea"]}`,
		`{"type":"PolygonVolume","id":"P",`+sq+`}`,
		`{"type":"VehicleSpawner","id":"v1","position":{"x":5,"y":0,"z":5}}`,
	)
	out, rep, err := Link(in)
	if err != nil {
		t.Fatal(err)
	}
	if r := result(t, rep, "H"); r.Status != BadList || rep.Changed || string(out) != string(in) {
		t.Errorf("null list: %+v changed=%v", r, rep.Changed)
	}
}

func TestDuplicateSpawnerIDsOfferedOnce(t *testing.T) {
	in := spatialOf(
		`{"type":"HQ_PlayerSpawner","id":"H","HQArea":"P","InfantrySpawns":["a"],"linked":["HQArea"]}`,
		`{"type":"PolygonVolume","id":"P",`+sq+`}`,
		`{"type":"VehicleSpawner","id":"v1","position":{"x":5,"y":0,"z":5}}`,
		`{"type":"VehicleSpawner","id":"v1","position":{"x":6,"y":0,"z":5}}`,
	)
	out, rep, _ := Link(in)
	v, _ := vehicles(t, out)
	if !reflect.DeepEqual(v["H"], []string{"v1"}) || len(result(t, rep, "H").Found) != 1 {
		t.Errorf("duplicate ids: %v", v["H"])
	}
}

func TestAllOverlapStatus(t *testing.T) {
	in := spatialOf(
		`{"type":"HQ_PlayerSpawner","id":"H1","HQArea":"P1","InfantrySpawns":["a"],"linked":["HQArea"]}`,
		`{"type":"HQ_PlayerSpawner","id":"H2","HQArea":"P2","InfantrySpawns":["b"],"linked":["HQArea"]}`,
		`{"type":"PolygonVolume","id":"P1",`+sq+`}`,
		`{"type":"PolygonVolume","id":"P2",`+sq+`}`,
		`{"type":"VehicleSpawner","id":"v1","position":{"x":5,"y":0,"z":5}}`,
	)
	_, rep, _ := Link(in)
	for _, id := range []string{"H1", "H2"} {
		if r := result(t, rep, id); r.Status != AllOverlap {
			t.Errorf("%s = %s", id, r.Status)
		}
	}
	if len(rep.Overlaps) != 1 || len(rep.Overlaps[0].HQIDs) != 2 {
		t.Errorf("overlaps = %+v", rep.Overlaps)
	}
}

func TestOddFieldOnOtherObjectIgnored(t *testing.T) {
	in := spatialOf(
		`{"type":"SomethingNew","id":5,"position":[1,2,3]}`,
		`{"type":"HQ_PlayerSpawner","id":"H","HQArea":"P","InfantrySpawns":["a"],"linked":["HQArea"]}`,
		`{"type":"PolygonVolume","id":"P",`+sq+`}`,
		`{"type":"VehicleSpawner","id":"v1","position":{"x":5,"y":0,"z":5}}`,
	)
	_, rep, err := Link(in)
	if err != nil || rep.AddedCount() != 1 {
		t.Errorf("err=%v added=%d", err, rep.AddedCount())
	}
}

func TestUnreadableHQSkipped(t *testing.T) {
	in := spatialOf(
		`{"type":"HQ_PlayerSpawner","id":"H","HQArea":7,"InfantrySpawns":["a"]}`,
		`{"type":"VehicleSpawner","id":"v1","position":{"x":5,"y":0,"z":5}}`,
	)
	_, rep, err := Link(in)
	if err != nil {
		t.Fatal(err)
	}
	if r := result(t, rep, "H"); r.Status != BadHQ {
		t.Errorf("H = %s", r.Status)
	}
}

func TestAlsoLinkedToAnotherHQ(t *testing.T) {
	in := spatialOf(
		`{"type":"HQ_PlayerSpawner","id":"H1","HQArea":"P","InfantrySpawns":["a"],"VehicleSpawners":["v1"],"linked":["HQArea","VehicleSpawners"]}`,
		`{"type":"HQ_PlayerSpawner","id":"H2","HQArea":"P","InfantrySpawns":["b"],"linked":["HQArea"]}`,
		`{"type":"PolygonVolume","id":"P",`+sq+`}`,
		`{"type":"VehicleSpawner","id":"v1","position":{"x":5,"y":0,"z":5}}`,
	)
	rep, _ := Find(in)
	f := result(t, rep, "H2").Found
	if len(f) != 1 || !reflect.DeepEqual(f[0].AlsoLinkedTo, []string{"H1"}) {
		t.Errorf("H2 found = %+v", f)
	}
	_, rep2, _ := Link(in)
	if !strings.Contains(rep2.Text("x"), "v1 was already on H1 too") {
		t.Error("report should note the spawner is also on H1")
	}
}
