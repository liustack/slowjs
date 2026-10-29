// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The archive keeps the raw results of a run. Reading one and writing it
// back must change nothing but the layout, so numbers keep the kind they
// have in the file (an integer stays an integer, a float stays a float), and
// the writer prints floats and strings as Python's json module does, which
// the archives of earlier runs were made with.

// jobj is a JSON object that remembers the order of its keys.
type jobj struct {
	keys []string
	m    map[string]any
}

func newObj() *jobj { return &jobj{m: map[string]any{}} }

func (o *jobj) set(k string, v any) {
	if _, ok := o.m[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.m[k] = v
}

func (o *jobj) clone() *jobj {
	c := newObj()
	for _, k := range o.keys {
		c.set(k, o.m[k])
	}
	return c
}

// jint is a JSON integer, in decimal without a plus sign or leading zeros.
// The values are the types nil, bool, string, jint, float64, []any, *jobj.
type jint string

type parser struct {
	s string
	i int
}

// parseJSON reads one JSON value, as Python's json.loads does: NaN, Infinity
// and -Infinity are numbers, a control character in a string is an error, and
// so is anything after the value.
func parseJSON(s string) (any, error) {
	if !utf8.ValidString(s) {
		return nil, fmt.Errorf("not UTF-8")
	}
	p := &parser{s: s}
	p.space()
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	p.space()
	if p.i != len(p.s) {
		return nil, fmt.Errorf("extra data at %d", p.i)
	}
	return v, nil
}

func (p *parser) space() {
	for p.i < len(p.s) && strings.IndexByte(" \t\n\r", p.s[p.i]) >= 0 {
		p.i++
	}
}

func (p *parser) errf(format string, a ...any) error {
	return fmt.Errorf("%s at %d", fmt.Sprintf(format, a...), p.i)
}

func (p *parser) literal(word string, v any) (any, bool) {
	if strings.HasPrefix(p.s[p.i:], word) {
		p.i += len(word)
		return v, true
	}
	return nil, false
}

func (p *parser) value() (any, error) {
	if p.i >= len(p.s) {
		return nil, p.errf("expecting a value")
	}
	switch c := p.s[p.i]; {
	case c == '{':
		return p.object()
	case c == '[':
		return p.array()
	case c == '"':
		return p.str()
	case c == '-' || (c >= '0' && c <= '9'):
		if c == '-' {
			if v, ok := p.literal("-Infinity", math.Inf(-1)); ok {
				return v, nil
			}
		}
		return p.number()
	}
	for _, l := range []struct {
		w string
		v any
	}{{"null", nil}, {"true", true}, {"false", false}, {"NaN", math.NaN()}, {"Infinity", math.Inf(1)}} {
		if v, ok := p.literal(l.w, l.v); ok {
			return v, nil
		}
	}
	return nil, p.errf("expecting a value")
}

func (p *parser) number() (any, error) {
	start := p.i
	if p.s[p.i] == '-' {
		p.i++
	}
	digits := func() int {
		n := 0
		for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
			p.i++
			n++
		}
		return n
	}
	if p.i < len(p.s) && p.s[p.i] == '0' {
		p.i++
	} else if digits() == 0 {
		return nil, p.errf("expecting a value")
	}
	float := false
	if p.i+1 < len(p.s) && p.s[p.i] == '.' && p.s[p.i+1] >= '0' && p.s[p.i+1] <= '9' {
		p.i++
		digits()
		float = true
	}
	if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
		j := p.i + 1
		if j < len(p.s) && (p.s[j] == '+' || p.s[j] == '-') {
			j++
		}
		if j < len(p.s) && p.s[j] >= '0' && p.s[j] <= '9' {
			p.i = j
			digits()
			float = true
		}
	}
	text := p.s[start:p.i]
	if float {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil && !math.IsInf(f, 0) {
			return nil, err
		}
		return f, nil
	}
	n, ok := new(big.Int).SetString(text, 10)
	if !ok {
		return nil, p.errf("bad number %q", text)
	}
	return jint(n.String()), nil
}

func (p *parser) str() (any, error) {
	p.i++ // the quote
	var b strings.Builder
	for {
		if p.i >= len(p.s) {
			return nil, p.errf("unterminated string")
		}
		c := p.s[p.i]
		switch {
		case c == '"':
			p.i++
			return b.String(), nil
		case c < 0x20:
			return nil, p.errf("control character in string")
		case c != '\\':
			b.WriteByte(c)
			p.i++
			continue
		}
		p.i++
		if p.i >= len(p.s) {
			return nil, p.errf("unterminated string")
		}
		e := p.s[p.i]
		p.i++
		if e != 'u' {
			i := strings.IndexByte(`"\/bfnrt`, e)
			if i < 0 {
				return nil, p.errf("invalid escape")
			}
			b.WriteByte("\"\\/\b\f\n\r\t"[i])
			continue
		}
		r, err := p.hex4()
		if err != nil {
			return nil, err
		}
		if r >= 0xD800 && r < 0xDC00 && strings.HasPrefix(p.s[p.i:], `\u`) {
			save := p.i
			p.i += 2
			lo, err := p.hex4()
			if err != nil {
				return nil, err
			}
			if lo >= 0xDC00 && lo < 0xE000 {
				r = 0x10000 + (r-0xD800)<<10 + (lo - 0xDC00)
			} else {
				p.i = save
			}
		}
		// A lone surrogate cannot be a Go string: it becomes U+FFFD.
		b.WriteRune(rune(r))
	}
}

func (p *parser) hex4() (int, error) {
	if p.i+4 > len(p.s) {
		return 0, p.errf("invalid \\u escape")
	}
	n, err := strconv.ParseUint(p.s[p.i:p.i+4], 16, 32)
	if err != nil || strings.ContainsAny(p.s[p.i:p.i+4], "+-_") {
		return 0, p.errf("invalid \\u escape")
	}
	p.i += 4
	return int(n), nil
}

func (p *parser) array() (any, error) {
	p.i++
	out := []any{}
	p.space()
	if p.i < len(p.s) && p.s[p.i] == ']' {
		p.i++
		return out, nil
	}
	for {
		p.space()
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		p.space()
		if p.i >= len(p.s) {
			return nil, p.errf("expecting , or ]")
		}
		switch p.s[p.i] {
		case ',':
			p.i++
		case ']':
			p.i++
			return out, nil
		default:
			return nil, p.errf("expecting , or ]")
		}
	}
}

func (p *parser) object() (any, error) {
	p.i++
	o := newObj()
	p.space()
	if p.i < len(p.s) && p.s[p.i] == '}' {
		p.i++
		return o, nil
	}
	for {
		p.space()
		if p.i >= len(p.s) || p.s[p.i] != '"' {
			return nil, p.errf("expecting a property name")
		}
		k, err := p.str()
		if err != nil {
			return nil, err
		}
		p.space()
		if p.i >= len(p.s) || p.s[p.i] != ':' {
			return nil, p.errf("expecting :")
		}
		p.i++
		p.space()
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		o.set(k.(string), v)
		p.space()
		if p.i >= len(p.s) {
			return nil, p.errf("expecting , or }")
		}
		switch p.s[p.i] {
		case ',':
			p.i++
		case '}':
			p.i++
			return o, nil
		default:
			return nil, p.errf("expecting , or }")
		}
	}
}

// pyFloat formats x as Python's repr does: the shortest digits that read back
// as x, in fixed notation from 1e-4 up to 1e16 and in exponent notation
// outside, always with a point or an exponent.
func pyFloat(x float64) string {
	switch {
	case math.IsNaN(x):
		return "nan"
	case math.IsInf(x, 1):
		return "inf"
	case math.IsInf(x, -1):
		return "-inf"
	}
	e := strconv.FormatFloat(x, 'e', -1, 64) // d.ddde+XX
	sign := ""
	if e[0] == '-' {
		sign, e = "-", e[1:]
	}
	mant, exp, _ := strings.Cut(e, "e")
	digits := strings.Replace(mant, ".", "", 1)
	decpt, _ := strconv.Atoi(exp)
	decpt++ // the value is 0.DIGITS * 10^decpt
	if decpt <= -4 || decpt > 16 {
		out := digits[:1]
		if len(digits) > 1 {
			out += "." + digits[1:]
		}
		x := decpt - 1
		es := "+"
		if x < 0 {
			es, x = "-", -x
		}
		return fmt.Sprintf("%s%se%s%02d", sign, out, es, x)
	}
	switch {
	case decpt <= 0:
		return sign + "0." + strings.Repeat("0", -decpt) + digits
	case decpt >= len(digits):
		return sign + digits + strings.Repeat("0", decpt-len(digits)) + ".0"
	}
	return sign + digits[:decpt] + "." + digits[decpt:]
}

// pyG is Python's format(x, ".<prec>g").
func pyG(x float64, prec int) string {
	switch {
	case math.IsNaN(x):
		return "nan"
	case math.IsInf(x, 1):
		return "inf"
	case math.IsInf(x, -1):
		return "-inf"
	}
	return strconv.FormatFloat(x, 'g', prec, 64)
}

// pyStr is Python's str() of a number.
func pyStr(v any) string {
	switch x := v.(type) {
	case jint:
		return string(x)
	case float64:
		return pyFloat(x)
	case string:
		return x
	}
	panic(fatal{fmt.Sprintf("cannot print %v as text", v)})
}

// writeJSON writes v as json.dump(v, indent=2, sort_keys=True) does, with
// ensure_ascii. A value is one of the parsed types, int, or map[string]any.
func writeJSON(b *strings.Builder, v any, depth int) {
	nl := "\n" + strings.Repeat("  ", depth+1)
	end := "\n" + strings.Repeat("  ", depth)
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case string:
		writeString(b, x)
	case jint:
		b.WriteString(string(x))
	case int:
		b.WriteString(strconv.Itoa(x))
	case float64:
		switch {
		case math.IsNaN(x):
			b.WriteString("NaN")
		case math.IsInf(x, 1):
			b.WriteString("Infinity")
		case math.IsInf(x, -1):
			b.WriteString("-Infinity")
		default:
			b.WriteString(pyFloat(x))
		}
	case []any:
		if len(x) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[")
		for i, e := range x {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(nl)
			writeJSON(b, e, depth+1)
		}
		b.WriteString(end + "]")
	case *jobj:
		writeMap(b, x.m, depth)
	case map[string]any:
		writeMap(b, x, depth)
	default:
		panic(fatal{fmt.Sprintf("cannot write %T as JSON", v)})
	}
}

func writeMap(b *strings.Builder, m map[string]any, depth int) {
	if len(m) == 0 {
		b.WriteString("{}")
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n" + strings.Repeat("  ", depth+1))
		writeString(b, k)
		b.WriteString(": ")
		writeJSON(b, m[k], depth+1)
	}
	b.WriteString("\n" + strings.Repeat("  ", depth) + "}")
}

// writeString writes s as a JSON string with every character outside the
// printable ASCII escaped, as Python's json does.
func writeString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
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
		case r >= ' ' && r <= '~':
			b.WriteRune(r)
		case r < 0x10000:
			fmt.Fprintf(b, `\u%04x`, r)
		default:
			r -= 0x10000
			fmt.Fprintf(b, `\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
		}
	}
	b.WriteByte('"')
}
