package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GekkoGear/portal-armory/hqlink"
)

func TestOutputDir(t *testing.T) {
	cases := map[string]string{
		"DeadSeas Overcast.spatial.json":                     "DeadSeas Overcast - HQ Vehicles",
		"Operation Fortunate Sun 2026.spatial.minified.json": "Operation Fortunate Sun 2026 - HQ Vehicles",
		"plain.json": "plain - HQ Vehicles",
	}
	for in, want := range cases {
		if got := outputDir("D", filepath.Join("x", in)); got != filepath.Join("D", want) {
			t.Errorf("%s -> %s", in, got)
		}
	}
}

func TestOutputDirFallback(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := desktopDir(); got != filepath.Join(home, "Desktop") {
		t.Errorf("desktopDir() = %s", got)
	}
}

func copyFixture(t *testing.T, dir string) string {
	t.Helper()
	src, err := os.ReadFile("../../hqlink/testdata/small_pretty.json")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "Small Map.spatial.json")
	os.WriteFile(p, src, 0o644)
	return p
}

func TestRunWritesOutput(t *testing.T) {
	src, desk := t.TempDir(), t.TempDir()
	in := copyFixture(t, src)
	before, _ := os.ReadFile(in)
	var out bytes.Buffer
	if code := run([]string{in}, desk, pickAll, &out); code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	dir := filepath.Join(desk, "Small Map - HQ Vehicles")
	if _, err := os.Stat(filepath.Join(dir, "Small Map.spatial.json")); err != nil {
		t.Error("output spatial missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "HQ Vehicle Linker report.txt")); err != nil {
		t.Error("report missing")
	}
	after, _ := os.ReadFile(in)
	if !bytes.Equal(before, after) {
		t.Error("original file changed")
	}
	if !strings.Contains(out.String(), "HQ_B: +3 added, new vehicle list created") {
		t.Errorf("summary: %s", out.String())
	}

	// Dropping the output file again must not overwrite it, and needs no changes.
	out.Reset()
	again := filepath.Join(dir, "Small Map.spatial.json")
	if code := run([]string{again}, desk, pickAll, &out); code != 0 || !strings.Contains(out.String(), "No changes were needed") {
		t.Errorf("second run: %d %s", code, out.String())
	}
}

func TestRunBadInput(t *testing.T) {
	desk := t.TempDir()
	var out bytes.Buffer
	if code := run(nil, desk, pickAll, &out); code != 2 || !strings.Contains(out.String(), "Drag a .spatial.json") {
		t.Errorf("no args: %d %s", code, out.String())
	}
	out.Reset()
	if code := run([]string{"a", "b"}, desk, pickAll, &out); code != 2 {
		t.Errorf("two args: %d", code)
	}
	out.Reset()
	txt := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(txt, []byte("hello"), 0o644)
	if code := run([]string{txt}, desk, pickAll, &out); code != 1 || !strings.Contains(out.String(), "not a spatial") {
		t.Errorf("text file: %d %s", code, out.String())
	}
	entries, _ := os.ReadDir(desk)
	if len(entries) != 0 {
		t.Error("bad input wrote something")
	}
}

func pickAll(rep hqlink.Report, _ string) (map[string][]string, bool) { return rep.PickAll(), true }

func TestRunChecked(t *testing.T) {
	src, desk := t.TempDir(), t.TempDir()
	in := copyFixture(t, src)
	var out bytes.Buffer
	// Checked: VA2 on HQ_A, VB1 + boat on HQ_B, VC1 on HQ_EmptyList. Nothing on HQ_Share.
	choose := func(rep hqlink.Report, name string) (map[string][]string, bool) {
		if name != "Small Map.spatial.json" {
			t.Errorf("map name %q", name)
		}
		return map[string][]string{
			"HQ_A":         {"VA2"},
			"HQ_B":         {"VB1", "Veh & \"Boat\" é"},
			"HQ_EmptyList": {"VC1"},
		}, true
	}
	if code := run([]string{in}, desk, choose, &out); code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	b, err := os.ReadFile(filepath.Join(desk, "Small Map - HQ Vehicles", "Small Map.spatial.json"))
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Portal_Dynamic []struct {
			ID              string
			VehicleSpawners []string
		}
	}
	json.Unmarshal(b, &d)
	got := map[string][]string{}
	for _, o := range d.Portal_Dynamic {
		got[o.ID] = o.VehicleSpawners
	}
	want := map[string][]string{
		"HQ_A":         {"VA1", "VA2"},
		"HQ_B":         {"VB1", "Veh & \"Boat\" é"},
		"HQ_Share":     nil,
		"HQ_EmptyList": {"VC1"},
	}
	for id, w := range want {
		if !reflect.DeepEqual(got[id], w) {
			t.Errorf("%s = %v, want %v", id, got[id], w)
		}
	}
	for _, w := range []string{"HQ_B: +2 added, new vehicle list created, 1 left unchecked", "HQ_Share: vehicles found but none checked"} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("output missing %q", w)
		}
	}
}

func TestRunCancelled(t *testing.T) {
	src, desk := t.TempDir(), t.TempDir()
	in := copyFixture(t, src)
	var out bytes.Buffer
	cancel := func(hqlink.Report, string) (map[string][]string, bool) { return nil, false }
	if code := run([]string{in}, desk, cancel, &out); code != 0 || !strings.Contains(out.String(), "Cancelled") {
		t.Errorf("%d %s", code, out.String())
	}
	if entries, _ := os.ReadDir(desk); len(entries) != 0 {
		t.Error("cancel wrote something")
	}
}

func TestRunNothingToPickSkipsPicker(t *testing.T) {
	src, desk := t.TempDir(), t.TempDir()
	in := copyFixture(t, src)
	var out bytes.Buffer
	run([]string{in}, desk, pickAll, &out)
	again := filepath.Join(desk, "Small Map - HQ Vehicles", "Small Map.spatial.json")
	called := false
	never := func(hqlink.Report, string) (map[string][]string, bool) { called = true; return nil, false }
	out.Reset()
	if code := run([]string{again}, desk, never, &out); code != 0 || called {
		t.Errorf("picker should not open when nothing is new: %d called=%v", code, called)
	}
}
