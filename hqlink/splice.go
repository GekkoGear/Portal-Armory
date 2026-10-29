package hqlink

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// edit replaces b[At:End] with Text (End == At means a pure insert).
type edit struct {
	At, End int
	Text    string
}

func applyEdits(b []byte, edits []edit) []byte {
	sort.Slice(edits, func(i, j int) bool { return edits[i].At > edits[j].At })
	out := append([]byte(nil), b...)
	for _, e := range edits {
		out = append(out[:e.At], append([]byte(e.Text), out[e.End:]...)...)
	}
	return out
}

// encodeString writes s the way Godot does: JSON-escaped, but '&', '<', '>' left alone.
func encodeString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return strings.TrimRight(buf.String(), "\n")
}

// layout is the whitespace an array uses before each element and before ']'.
type layout struct{ Elem, Close string }

// arrayLayout reads the layout of a non-empty array; ok is false for empty arrays.
func arrayLayout(b []byte, sp span) (layout, bool) {
	els := arrayElems(b, sp)
	if len(els) == 0 {
		return layout{}, false
	}
	last := els[len(els)-1]
	return layout{
		Elem:  string(b[sp.Start+1 : els[0].Start]),
		Close: string(b[last.End : sp.End-1]),
	}, true
}

func renderArray(l layout, ids []string) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, id := range ids {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(l.Elem)
		sb.WriteString(encodeString(id))
	}
	sb.WriteString(l.Close)
	sb.WriteByte(']')
	return sb.String()
}

// wsBefore returns the whitespace immediately before index i.
func wsBefore(b []byte, i int) string {
	j := i
	for j > 0 && isWS(b[j-1]) {
		j--
	}
	return string(b[j:i])
}

// appendEdit adds ids to the end of the array at sp. fallback is used when the array is empty.
func appendEdit(b []byte, sp span, ids []string, fallback layout) edit {
	els := arrayElems(b, sp)
	if len(els) == 0 {
		return edit{At: sp.Start, End: sp.End, Text: renderArray(fallback, ids)}
	}
	l, _ := arrayLayout(b, sp)
	var sb strings.Builder
	for _, id := range ids {
		sb.WriteByte(',')
		sb.WriteString(l.Elem)
		sb.WriteString(encodeString(id))
	}
	last := els[len(els)-1]
	return edit{At: last.End, End: last.End, Text: sb.String()}
}

// templateLayout picks the layout a new or empty array should copy, from the
// HQ's other arrays. keyWS is the whitespace in front of the HQ's keys.
func templateLayout(b []byte, o object, keyWS string) layout {
	for _, k := range []string{"InfantrySpawns", "linked", "VehicleSpawners"} {
		if f, ok := o.Fields[k]; ok && b[f.Val.Start] == '[' {
			if l, ok := arrayLayout(b, f.Val); ok {
				return l
			}
		}
	}
	if strings.ContainsAny(keyWS, "\n") {
		return layout{Elem: keyWS + "    ", Close: keyWS}
	}
	return layout{}
}

// hqEdits builds the edits that add ids to one HQ object. ok is false when
// there is nowhere sensible to put a new VehicleSpawners key.
func hqEdits(b []byte, o object, ids []string) (edits []edit, ok bool) {
	anchorKey := ""
	for _, k := range []string{"InfantrySpawns", "HQArea"} {
		if _, has := o.Fields[k]; has {
			anchorKey = k
			break
		}
	}
	keyWS := ""
	colon := ":"
	if anchorKey != "" {
		a := o.Fields[anchorKey]
		keyWS = wsBefore(b, a.KeyStart)
		colon = string(b[a.KeyEnd:a.Val.Start])
	} else if f, has := o.Fields["type"]; has {
		keyWS = wsBefore(b, f.KeyStart)
	}
	tmpl := templateLayout(b, o, keyWS)

	if vf, has := o.Fields["VehicleSpawners"]; has {
		edits = append(edits, appendEdit(b, vf.Val, ids, tmpl))
	} else {
		if anchorKey == "" {
			return nil, false
		}
		a := o.Fields[anchorKey]
		text := "," + keyWS + encodeString("VehicleSpawners") + colon + renderArray(tmpl, ids)
		edits = append(edits, edit{At: a.Val.End, End: a.Val.End, Text: text})
	}

	if lf, has := o.Fields["linked"]; has && b[lf.Val.Start] == '[' {
		var linked []string
		json.Unmarshal(b[lf.Val.Start:lf.Val.End], &linked)
		present := false
		for _, s := range linked {
			if s == "VehicleSpawners" {
				present = true
			}
		}
		if !present {
			edits = append(edits, appendEdit(b, lf.Val, []string{"VehicleSpawners"}, tmpl))
		}
	}
	return edits, true
}
