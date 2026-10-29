package polysnap

import (
	"bytes"
	"os"
	"testing"
)

// Checks Minify byte-for-byte against real JsonMinifier output. Set MINI_PAIRS to
// "normal.spatial.json|reference.minified.json|normal2...|reference2..." and run go test.
func TestMatchReference(t *testing.T) {
	files := os.Getenv("MINI_PAIRS")
	if files == "" {
		t.Skip("set MINI_PAIRS")
	}
	parts := bytes.Split([]byte(files), []byte("|"))
	for i := 0; i+1 < len(parts); i += 2 {
		src, _ := os.ReadFile(string(parts[i]))
		ref, _ := os.ReadFile(string(parts[i+1]))
		out, _, err := minifyJSON(src)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out, ref) {
			k := 0
			for k < len(out) && k < len(ref) && out[k] == ref[k] {
				k++
			}
			lo := k - 80
			if lo < 0 {
				lo = 0
			}
			hi := func(b []byte) int {
				if k+80 > len(b) {
					return len(b)
				}
				return k + 80
			}
			t.Errorf("%s: differs at byte %d (len %d vs ref %d)\nours: %s\nref:  %s", parts[i], k, len(out), len(ref), out[lo:hi(out)], ref[lo:hi(ref)])
		} else {
			t.Logf("%s: byte-identical (%d bytes)", parts[i], len(out))
		}
	}
}
