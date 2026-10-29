package polysnap

// A small JSON parser that remembers where every value sits in the original
// bytes. That lets the tool rewrite only the exact number tokens it changes
// and leave all other bytes (spacing, key order, number formatting) as they were.

import (
	"encoding/json"
	"fmt"
	"strconv"
)

const (
	kObject = iota
	kArray
	kString
	kNumber
	kBool
	kNull
)

type jnode struct {
	kind       int
	start, end int // byte range of this value in the source
	keys       []string
	vals       []*jnode
	num        float64
	str        string
}

func (n *jnode) get(key string) *jnode {
	if n == nil || n.kind != kObject {
		return nil
	}
	for i, k := range n.keys {
		if k == key {
			return n.vals[i]
		}
	}
	return nil
}

func (n *jnode) getString(key string) string {
	v := n.get(key)
	if v == nil || v.kind != kString {
		return ""
	}
	return v.str
}

type parser struct {
	b     []byte
	i     int
	depth int
}

func parseJSON(b []byte) (*jnode, error) {
	p := &parser{b: b}
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		p.i = 3 // UTF-8 BOM
	}
	p.ws()
	n, err := p.value()
	if err != nil {
		return nil, err
	}
	p.ws()
	if p.i != len(p.b) {
		return nil, p.errf("unexpected extra content")
	}
	return n, nil
}

func (p *parser) errf(f string, a ...any) error {
	line, col := 1, 1
	for k := 0; k < p.i && k < len(p.b); k++ {
		if p.b[k] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return fmt.Errorf("line %d, column %d: %s", line, col, fmt.Sprintf(f, a...))
}

func (p *parser) ws() {
	for p.i < len(p.b) {
		switch p.b[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *parser) value() (*jnode, error) {
	if p.i >= len(p.b) {
		return nil, p.errf("unexpected end of file")
	}
	switch c := p.b[p.i]; {
	case c == '{':
		return p.object()
	case c == '[':
		return p.array()
	case c == '"':
		s, start, err := p.stringTok()
		if err != nil {
			return nil, err
		}
		return &jnode{kind: kString, start: start, end: p.i, str: s}, nil
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	case c == 't':
		return p.lit("true", kBool)
	case c == 'f':
		return p.lit("false", kBool)
	case c == 'n':
		return p.lit("null", kNull)
	default:
		return nil, p.errf("unexpected character %q", c)
	}
}

func (p *parser) lit(word string, kind int) (*jnode, error) {
	if p.i+len(word) > len(p.b) || string(p.b[p.i:p.i+len(word)]) != word {
		return nil, p.errf("invalid literal")
	}
	n := &jnode{kind: kind, start: p.i, end: p.i + len(word), str: word}
	p.i += len(word)
	return n, nil
}

func (p *parser) enter() error {
	p.depth++
	if p.depth > 512 {
		return p.errf("nesting too deep")
	}
	return nil
}

func (p *parser) object() (*jnode, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer func() { p.depth-- }()
	n := &jnode{kind: kObject, start: p.i}
	p.i++
	p.ws()
	if p.i < len(p.b) && p.b[p.i] == '}' {
		p.i++
		n.end = p.i
		return n, nil
	}
	for {
		p.ws()
		if p.i >= len(p.b) || p.b[p.i] != '"' {
			return nil, p.errf("expected a key in quotes")
		}
		k, _, err := p.stringTok()
		if err != nil {
			return nil, err
		}
		p.ws()
		if p.i >= len(p.b) || p.b[p.i] != ':' {
			return nil, p.errf("expected ':' after key %q", k)
		}
		p.i++
		p.ws()
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		n.keys = append(n.keys, k)
		n.vals = append(n.vals, v)
		p.ws()
		if p.i >= len(p.b) {
			return nil, p.errf("unexpected end of file inside an object")
		}
		if p.b[p.i] == ',' {
			p.i++
			continue
		}
		if p.b[p.i] == '}' {
			p.i++
			n.end = p.i
			return n, nil
		}
		return nil, p.errf("expected ',' or '}'")
	}
}

func (p *parser) array() (*jnode, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer func() { p.depth-- }()
	n := &jnode{kind: kArray, start: p.i}
	p.i++
	p.ws()
	if p.i < len(p.b) && p.b[p.i] == ']' {
		p.i++
		n.end = p.i
		return n, nil
	}
	for {
		p.ws()
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		n.vals = append(n.vals, v)
		p.ws()
		if p.i >= len(p.b) {
			return nil, p.errf("unexpected end of file inside a list")
		}
		if p.b[p.i] == ',' {
			p.i++
			continue
		}
		if p.b[p.i] == ']' {
			p.i++
			n.end = p.i
			return n, nil
		}
		return nil, p.errf("expected ',' or ']'")
	}
}

func (p *parser) stringTok() (string, int, error) {
	start := p.i
	p.i++
	escaped := false
	for p.i < len(p.b) {
		c := p.b[p.i]
		if c == '\\' {
			escaped = true
			p.i += 2
			continue
		}
		if c == '"' {
			p.i++
			raw := p.b[start:p.i]
			if !escaped {
				return string(raw[1 : len(raw)-1]), start, nil
			}
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				return "", start, p.errf("bad string: %v", err)
			}
			return s, start, nil
		}
		if c < 0x20 {
			return "", start, p.errf("control character inside a string")
		}
		p.i++
	}
	return "", start, p.errf("unterminated string")
}

func (p *parser) number() (*jnode, error) {
	start := p.i
	if p.b[p.i] == '-' {
		p.i++
	}
	for p.i < len(p.b) {
		c := p.b[p.i]
		if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
			p.i++
			continue
		}
		break
	}
	v, err := strconv.ParseFloat(string(p.b[start:p.i]), 64)
	if err != nil {
		return nil, p.errf("bad number %q", string(p.b[start:p.i]))
	}
	return &jnode{kind: kNumber, start: start, end: p.i, num: v}, nil
}
