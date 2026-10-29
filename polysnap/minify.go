package polysnap

// A faithful port of defanz0r's JsonMinifier (v1.0.0, .NET 10) using its
// default settings:
//   - strip "metadata/*" and "color" keys from objects that have both "name" and "type"
//   - replace every name and id path segment with a short letter code (a, b, ... z, aa, ...),
//     skipping "Static" and ids that start with "Static/", and update the
//     references listed in each object's "linked" array
//   - round x/y/z under "position" to 4 decimals, "right"/"up"/"front" to 6,
//     "points" to 3, and "ObjId" to 0 (Math.Round, MidpointRounding.AwayFromZero)
//   - write compact JSON the way System.Text.Json does (default encoder)
// Output file name: <name>.minified<ext>, e.g. "Map.spatial.minified.json".

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type mnode struct {
	kind   int
	keys   []string
	vals   []*mnode
	str    string  // decoded string, or literal text for bool/null
	raw    string  // original number text (written as-is unless rounded)
	num    float64 // number value
	setNum bool    // number was rounded and must be re-formatted
}

func (n *mnode) get(k string) (*mnode, bool) {
	for i, key := range n.keys {
		if key == k {
			return n.vals[i], true
		}
	}
	return nil, false
}

func (n *mnode) set(k string, v *mnode) {
	for i, key := range n.keys {
		if key == k {
			n.vals[i] = v
			return
		}
	}
}

func (n *mnode) remove(k string) {
	for i, key := range n.keys {
		if key == k {
			n.keys = append(n.keys[:i:i], n.keys[i+1:]...)
			n.vals = append(n.vals[:i:i], n.vals[i+1:]...)
			return
		}
	}
}

func toM(j *jnode, src []byte) *mnode {
	m := &mnode{kind: j.kind}
	switch j.kind {
	case kObject:
		// System.Text.Json keeps the last value for a repeated key in place of the first.
		for i, k := range j.keys {
			v := toM(j.vals[i], src)
			if _, dup := m.get(k); dup {
				m.set(k, v)
				continue
			}
			m.keys = append(m.keys, k)
			m.vals = append(m.vals, v)
		}
	case kArray:
		for _, v := range j.vals {
			m.vals = append(m.vals, toM(v, src))
		}
	case kString, kBool, kNull:
		m.str = j.str
	case kNumber:
		m.raw = string(src[j.start:j.end])
		m.num = j.num
	}
	return m
}

func strVal(n *mnode) (string, bool) {
	if n != nil && n.kind == kString {
		return n.str, true
	}
	return "", false
}

func isValue(n *mnode) bool { return n != nil && n.kind != kObject && n.kind != kArray }

type minifier struct {
	nameMap map[string]string
	counter int
}

func minifyJSON(src []byte) ([]byte, int, error) {
	root, err := parseJSON(src)
	if err != nil {
		return nil, 0, err
	}
	m := toM(root, src)
	mf := &minifier{nameMap: map[string]string{}, counter: 1}
	stripEditorOnly(m)
	mf.collect(m)
	mf.replace(m)
	reduceByKey(m, "")
	var b strings.Builder
	b.Grow(len(src) / 2)
	writeM(&b, m)
	return []byte(b.String()), len(mf.nameMap), nil
}

// ---- StripEditorOnlyProperties ----

func stripEditorOnly(n *mnode) {
	switch n.kind {
	case kObject:
		_, hasName := n.get("name")
		_, hasType := n.get("type")
		isEntity := hasName && hasType
		var drop []string
		for i, k := range n.keys {
			meta := strings.HasPrefix(k, "metadata/")
			color := k == "color"
			if isEntity && (meta || color) {
				drop = append(drop, k)
			} else {
				stripEditorOnly(n.vals[i])
			}
		}
		for _, k := range drop {
			n.remove(k)
		}
	case kArray:
		for _, v := range n.vals {
			stripEditorOnly(v)
		}
	}
}

// ---- names and ids ----

func isExcluded(s string) bool {
	return s == "" || strings.HasPrefix(s, "Static/") || s == "Static"
}

func shortName(n int) string {
	s := ""
	for n > 0 {
		n--
		s = string(rune('a'+n%26)) + s
		n /= 26
	}
	return s
}

func (mf *minifier) shortForName(name string) string {
	if isExcluded(name) {
		return name
	}
	if v, ok := mf.nameMap[name]; ok {
		return v
	}
	s := shortName(mf.counter)
	mf.counter++
	mf.nameMap[name] = s
	return s
}

func (mf *minifier) shortForID(id string) string {
	if id == "" || isExcluded(id) {
		return id
	}
	if v, ok := mf.nameMap[id]; ok {
		return v
	}
	if strings.Contains(id, "/") {
		parts := strings.Split(id, "/")
		out := make([]string, len(parts))
		for i, p := range parts {
			switch {
			case isExcluded(p):
				out[i] = p
			case mf.nameMap[p] != "":
				out[i] = mf.nameMap[p]
			default:
				out[i] = mf.shortForName(p)
			}
		}
		joined := strings.Join(out, "/")
		mf.nameMap[id] = joined
		return joined
	}
	return mf.shortForName(id)
}

func (mf *minifier) collect(n *mnode) {
	switch n.kind {
	case kObject:
		for i, k := range n.keys {
			v := n.vals[i]
			if k == "name" && isValue(v) {
				if s, ok := strVal(v); ok {
					mf.shortForName(s)
				}
			} else if k == "id" && isValue(v) {
				if s, ok := strVal(v); ok {
					mf.shortForID(s)
				}
			}
			mf.collect(v)
		}
	case kArray:
		for _, v := range n.vals {
			mf.collect(v)
		}
	}
}

func newStr(s string) *mnode { return &mnode{kind: kString, str: s} }

func (mf *minifier) replace(n *mnode) {
	switch n.kind {
	case kArray:
		for _, v := range n.vals {
			mf.replace(v)
		}
		return
	case kObject:
	default:
		return
	}
	var linked map[string]bool
	if l, ok := n.get("linked"); ok && l.kind == kArray {
		linked = map[string]bool{}
		for _, e := range l.vals {
			if s, ok := strVal(e); ok {
				linked[s] = true
			}
		}
	}
	for i := 0; i < len(n.keys); i++ {
		k, v := n.keys[i], n.vals[i]
		switch {
		case k == "name" && isValue(v):
			s, ok := strVal(v)
			if !ok {
				continue
			}
			static := false
			if idv, ok := n.get("id"); ok && isValue(idv) {
				if ids, ok := strVal(idv); ok {
					static = strings.HasPrefix(ids, "Static/")
				}
			}
			if !static {
				if r, ok := mf.nameMap[s]; ok {
					n.vals[i] = newStr(r)
				}
			}
		case k == "id" && isValue(v):
			if s, ok := strVal(v); ok {
				if r, ok := mf.nameMap[s]; ok {
					n.vals[i] = newStr(r)
				}
			}
		case linked != nil && linked[k]:
			if isValue(v) {
				if s, ok := strVal(v); ok {
					if r, ok := mf.nameMap[s]; ok {
						n.vals[i] = newStr(r)
					}
				}
			} else if v.kind == kArray {
				for j, e := range v.vals {
					if s, ok := strVal(e); ok {
						if r, ok := mf.nameMap[s]; ok {
							v.vals[j] = newStr(r)
						}
					}
				}
			}
		case v.kind == kArray:
			for _, e := range v.vals {
				mf.replace(e)
			}
		default:
			mf.replace(v)
		}
	}
}

// ---- precision ----

const (
	positionDigits = 4
	basisDigits    = 6
	pointsDigits   = 3
)

func reduceByKey(n *mnode, key string) {
	switch n.kind {
	case kObject:
		applyKeyed(n, key)
		for i, k := range n.keys {
			reduceByKey(n.vals[i], k)
		}
	case kArray:
		for _, v := range n.vals {
			reduceByKey(v, key)
		}
	}
}

func applyKeyed(obj *mnode, key string) {
	digits := -1
	switch key {
	case "position":
		digits = positionDigits
	case "right", "up", "front":
		digits = basisDigits
	case "points":
		digits = pointsDigits
	}
	if digits >= 0 {
		roundProp(obj, "x", digits)
		roundProp(obj, "y", digits)
		roundProp(obj, "z", digits)
	}
	roundProp(obj, "ObjId", 0)
}

func roundProp(obj *mnode, k string, digits int) {
	v, ok := obj.get(k)
	if !ok || v == nil || v.kind != kNumber {
		return
	}
	if math.IsInf(v.num, 0) || math.IsNaN(v.num) {
		return
	}
	r := dotnetRoundAway(v.num, digits)
	if r == 0 {
		r = 0 // drop negative zero
	}
	obj.set(k, &mnode{kind: kNumber, num: r, setNum: true})
}

var pow10 = [...]float64{1, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10, 1e11, 1e12, 1e13, 1e14, 1e15}

// Math.Round(value, digits, MidpointRounding.AwayFromZero) as implemented in .NET 7+.
func dotnetRoundAway(value float64, digits int) float64 {
	if math.Abs(value) < 1e16 {
		p := pow10[digits]
		value *= p
		value = math.Trunc(value + math.Copysign(math.Nextafter(0.5, 0), value))
		value /= p
	}
	return value
}

// Double formatting as System.Text.Json writes it (shortest round-trip, "G" rules).
func formatDotnet(v float64) string {
	if v == 0 {
		if math.Signbit(v) {
			return "-0"
		}
		return "0"
	}
	e := strconv.FormatFloat(v, 'e', -1, 64) // like -1.2345e+03
	neg := e[0] == '-'
	if neg {
		e = e[1:]
	}
	ei := strings.IndexByte(e, 'e')
	mant, exp := e[:ei], e[ei+1:]
	digits := strings.Replace(mant, ".", "", 1)
	x, _ := strconv.Atoi(exp)
	var out string
	scale := x + 1
	if scale > 15 || scale < -3 {
		m := digits[:1]
		if len(digits) > 1 {
			m += "." + digits[1:]
		}
		sign := "+"
		if x < 0 {
			sign = "-"
			x = -x
		}
		es := strconv.Itoa(x)
		if len(es) < 2 {
			es = "0" + es
		}
		out = m + "E" + sign + es
	} else if scale <= 0 {
		out = "0." + strings.Repeat("0", -scale) + digits
	} else if scale >= len(digits) {
		out = digits + strings.Repeat("0", scale-len(digits))
	} else {
		out = digits[:scale] + "." + digits[scale:]
	}
	if neg {
		out = "-" + out
	}
	return out
}

// ---- writer (System.Text.Json, JavaScriptEncoder.Default) ----

func writeM(b *strings.Builder, n *mnode) {
	switch n.kind {
	case kObject:
		b.WriteByte('{')
		for i, k := range n.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			writeStr(b, k)
			b.WriteByte(':')
			writeM(b, n.vals[i])
		}
		b.WriteByte('}')
	case kArray:
		b.WriteByte('[')
		for i, v := range n.vals {
			if i > 0 {
				b.WriteByte(',')
			}
			writeM(b, v)
		}
		b.WriteByte(']')
	case kString:
		writeStr(b, n.str)
	case kNumber:
		if n.setNum {
			b.WriteString(formatDotnet(n.num))
		} else {
			b.WriteString(n.raw)
		}
	default:
		b.WriteString(n.str)
	}
}

const hexd = "0123456789ABCDEF"

func writeU(b *strings.Builder, c uint16) {
	b.WriteString(`\u`)
	b.WriteByte(hexd[c>>12&15])
	b.WriteByte(hexd[c>>8&15])
	b.WriteByte(hexd[c>>4&15])
	b.WriteByte(hexd[c&15])
}

func writeStr(b *strings.Builder, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch {
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r == '\\':
			b.WriteString(`\\`)
		case r < 0x20 || r == '"' || r == '&' || r == '\'' || r == '+' || r == '<' || r == '>' || r == '`' || r == 0x7f:
			writeU(b, uint16(r))
		case r < 0x80:
			b.WriteByte(byte(r))
		default:
			if r > 0xffff {
				a, c := utf16.EncodeRune(r)
				writeU(b, uint16(a))
				writeU(b, uint16(c))
			} else {
				writeU(b, uint16(r))
			}
		}
	}
	b.WriteByte('"')
}
