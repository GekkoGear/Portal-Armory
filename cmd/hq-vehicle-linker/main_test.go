package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
	if code := run([]string{in}, desk, input(""), &out); code != 0 {
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
	if code := run([]string{again}, desk, input(""), &out); code != 0 || !strings.Contains(out.String(), "No changes were needed") {
		t.Errorf("second run: %d %s", code, out.String())
	}
}

func TestRunBadInput(t *testing.T) {
	desk := t.TempDir()
	var out bytes.Buffer
	if code := run(nil, desk, input(""), &out); code != 2 || !strings.Contains(out.String(), "Drag a .spatial.json") {
		t.Errorf("no args: %d %s", code, out.String())
	}
	out.Reset()
	if code := run([]string{"a", "b"}, desk, input(""), &out); code != 2 {
		t.Errorf("two args: %d", code)
	}
	out.Reset()
	txt := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(txt, []byte("hello"), 0o644)
	if code := run([]string{txt}, desk, input(""), &out); code != 1 || !strings.Contains(out.String(), "not a spatial") {
		t.Errorf("text file: %d %s", code, out.String())
	}
	entries, _ := os.ReadDir(desk)
	if len(entries) != 0 {
		t.Error("bad input wrote something")
	}
}

func input(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }

func TestRunChecklist(t *testing.T) {
	src, desk := t.TempDir(), t.TempDir()
	in := copyFixture(t, src)
	var out bytes.Buffer
	// HQ_A: Enter (keep VA2). HQ_B: untick 2 (VB2), Enter. HQ_Share: N, Enter. HQ_EmptyList: Enter.
	if code := run([]string{in}, desk, input("\n2\n\nN\n\n\n"), &out); code != 0 {
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
	s := out.String()
	for _, w := range []string{"[ ]   2  Abrams", "2 of 3 ticked", "HQ_B: +2 added, new vehicle list created, 1 not picked", "HQ_Share: " + "vehicles found but none picked"} {
		if !strings.Contains(s, w) {
			t.Errorf("output missing %q", w)
		}
	}
}

func TestToggle(t *testing.T) {
	tk := []bool{true, true, true, true, true, true, true}
	if bad := toggle("2 5-7", tk); bad != "" || !reflect.DeepEqual(tk, []bool{true, false, true, true, false, false, false}) {
		t.Errorf("2 5-7 -> %v %q", tk, bad)
	}
	toggle("n", tk)
	if !reflect.DeepEqual(tk, make([]bool, 7)) {
		t.Errorf("n -> %v", tk)
	}
	toggle("A", tk)
	toggle("3-1,", tk)
	if !reflect.DeepEqual(tk, []bool{false, false, false, true, true, true, true}) {
		t.Errorf("3-1 -> %v", tk)
	}
	for _, w := range []string{"x", "9", "0", "2-9", "1-"} {
		if bad := toggle(w, make([]bool, 7)); bad != w {
			t.Errorf("%q should be rejected, got %q", w, bad)
		}
	}
}

func TestChecklistBadInputThenEnter(t *testing.T) {
	var out bytes.Buffer
	rep := hqlinkFind(t)
	pick := askPicks(rep, input("zzz\n\n\n\n\n"), &out)
	if !strings.Contains(out.String(), `Didn't understand "zzz"`) {
		t.Error("no message for bad input")
	}
	if len(pick["HQ_A"]) != 1 || len(pick["HQ_B"]) != 3 {
		t.Errorf("bad input should keep everything ticked: %v", pick)
	}
}
