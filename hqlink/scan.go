package hqlink

import (
	"encoding/json"
	"errors"
	"fmt"
)

// A tiny JSON scanner that records byte offsets, so the tool can edit the
// original text in place instead of re-saving (which would reorder keys and
// reformat the whole file).

var errSyntax = errors.New("invalid JSON")

func isWS(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func skipWS(b []byte, i int) int {
	for i < len(b) && isWS(b[i]) {
		i++
	}
	return i
}

// scanString expects b[i] == '"' and returns the index just after the closing quote.
func scanString(b []byte, i int) (int, error) {
	if i >= len(b) || b[i] != '"' {
		return 0, fmt.Errorf("%w: expected string at byte %d", errSyntax, i)
	}
	for j := i + 1; j < len(b); j++ {
		switch b[j] {
		case '\\':
			j++
		case '"':
			return j + 1, nil
		}
	}
	return 0, fmt.Errorf("%w: unterminated string at byte %d", errSyntax, i)
}

// skipValue returns the index just after the JSON value starting at b[i].
func skipValue(b []byte, i int) (int, error) {
	i = skipWS(b, i)
	if i >= len(b) {
		return 0, fmt.Errorf("%w: unexpected end", errSyntax)
	}
	switch b[i] {
	case '"':
		return scanString(b, i)
	case '{', '[':
		depth := 0
		for j := i; j < len(b); j++ {
			switch b[j] {
			case '"':
				end, err := scanString(b, j)
				if err != nil {
					return 0, err
				}
				j = end - 1
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return j + 1, nil
				}
			}
		}
		return 0, fmt.Errorf("%w: unclosed value at byte %d", errSyntax, i)
	default:
		j := i
		for j < len(b) && !isWS(b[j]) && b[j] != ',' && b[j] != '}' && b[j] != ']' {
			j++
		}
		if j == i {
			return 0, fmt.Errorf("%w: unexpected %q at byte %d", errSyntax, b[i], i)
		}
		return j, nil
	}
}

// span is a half-open byte range [Start, End).
type span struct{ Start, End int }

// field is one key/value pair of an object.
type field struct {
	KeyStart int  // index of the key's opening quote
	KeyEnd   int  // index just after the key's closing quote
	Val      span // the value's bytes
}

// object is one element of Portal_Dynamic.
type object struct {
	Open, Close int // indexes of '{' and '}'; Open == -1 for non-object elements
	Fields      map[string]field
}

// eachMember walks an object or array starting at b[i] (which must be '{' or '[')
// and calls fn for every member. For objects key is the decoded key; for arrays key is "".
// It returns the index of the closing bracket.
func eachMember(b []byte, i int, fn func(key string, f field) error) (int, error) {
	open := b[i]
	isObj := open == '{'
	closeCh := byte(']')
	if isObj {
		closeCh = '}'
	}
	j := skipWS(b, i+1)
	if j < len(b) && b[j] == closeCh {
		return j, nil
	}
	for {
		var f field
		key := ""
		if isObj {
			ke, err := scanString(b, j)
			if err != nil {
				return 0, err
			}
			if err := json.Unmarshal(b[j:ke], &key); err != nil {
				return 0, fmt.Errorf("%w: bad key at byte %d", errSyntax, j)
			}
			f.KeyStart, f.KeyEnd = j, ke
			j = skipWS(b, ke)
			if j >= len(b) || b[j] != ':' {
				return 0, fmt.Errorf("%w: expected ':' at byte %d", errSyntax, j)
			}
			j = skipWS(b, j+1)
		}
		end, err := skipValue(b, j)
		if err != nil {
			return 0, err
		}
		f.Val = span{j, end}
		if err := fn(key, f); err != nil {
			return 0, err
		}
		j = skipWS(b, end)
		if j >= len(b) {
			return 0, fmt.Errorf("%w: unexpected end", errSyntax)
		}
		if b[j] == closeCh {
			return j, nil
		}
		if b[j] != ',' {
			return 0, fmt.Errorf("%w: expected ',' at byte %d", errSyntax, j)
		}
		j = skipWS(b, j+1)
	}
}

// locateDynamic returns one object per Portal_Dynamic element, in file order.
func locateDynamic(b []byte) ([]object, error) {
	i := skipWS(b, 0)
	if i >= len(b) || b[i] != '{' {
		return nil, ErrNotSpatial
	}
	var objs []object
	found := false
	_, err := eachMember(b, i, func(key string, f field) error {
		if key != "Portal_Dynamic" || b[f.Val.Start] != '[' {
			return nil
		}
		found = true
		_, err := eachMember(b, f.Val.Start, func(_ string, el field) error {
			o := object{Open: -1}
			if b[el.Val.Start] == '{' {
				o.Open = el.Val.Start
				o.Close = el.Val.End - 1
				o.Fields = map[string]field{}
				_, err := eachMember(b, el.Val.Start, func(k string, ff field) error {
					o.Fields[k] = ff
					return nil
				})
				if err != nil {
					return err
				}
			}
			objs = append(objs, o)
			return nil
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotSpatial
	}
	return objs, nil
}

// arrayElems returns the spans of the elements of the array at sp.
func arrayElems(b []byte, sp span) []span {
	var out []span
	eachMember(b, sp.Start, func(_ string, f field) error {
		out = append(out, f.Val)
		return nil
	})
	return out
}
