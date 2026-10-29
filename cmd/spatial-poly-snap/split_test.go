package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSplitPaths(t *testing.T) {
	dir := t.TempDir()
	spaced := filepath.Join(dir, "Dead Seas Snowquest+.spatial.json")
	os.WriteFile(spaced, []byte("{}"), 0o644)
	cases := map[string][]string{
		`"C:\Users\gekko\Desktop\Fortunate Sun.spatial.json"` + "\r\n": {`C:\Users\gekko\Desktop\Fortunate Sun.spatial.json`},
		`C:\maps\a.spatial.json "C:\my maps\b.spatial.json"`:          {`C:\maps\a.spatial.json`, `C:\my maps\b.spatial.json`},
		`& 'C:\my maps\b.spatial.json'`:                                 {`C:\my maps\b.spatial.json`},
		spaced:                                                          {spaced},
		"   \r\n":                                                       nil,
	}
	for in, want := range cases {
		if got := splitPaths(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitPaths(%q) = %q, want %q", in, got, want)
		}
	}
}
