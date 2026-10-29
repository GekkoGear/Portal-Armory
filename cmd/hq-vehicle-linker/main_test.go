package main

import (
	"bytes"
	"os"
	"path/filepath"
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
	if code := run([]string{in}, desk, &out); code != 0 {
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
	if !strings.Contains(out.String(), "HQ_B: +3 added (new vehicle list created)") {
		t.Errorf("summary: %s", out.String())
	}

	// Dropping the output file again must not overwrite it, and needs no changes.
	out.Reset()
	again := filepath.Join(dir, "Small Map.spatial.json")
	if code := run([]string{again}, desk, &out); code != 0 || !strings.Contains(out.String(), "No changes were needed") {
		t.Errorf("second run: %d %s", code, out.String())
	}
}

func TestRunBadInput(t *testing.T) {
	desk := t.TempDir()
	var out bytes.Buffer
	if code := run(nil, desk, &out); code != 2 || !strings.Contains(out.String(), "Drag a .spatial.json") {
		t.Errorf("no args: %d %s", code, out.String())
	}
	out.Reset()
	if code := run([]string{"a", "b"}, desk, &out); code != 2 {
		t.Errorf("two args: %d", code)
	}
	out.Reset()
	txt := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(txt, []byte("hello"), 0o644)
	if code := run([]string{txt}, desk, &out); code != 1 || !strings.Contains(out.String(), "not a spatial") {
		t.Errorf("text file: %d %s", code, out.String())
	}
	entries, _ := os.ReadDir(desk)
	if len(entries) != 0 {
		t.Error("bad input wrote something")
	}
}
