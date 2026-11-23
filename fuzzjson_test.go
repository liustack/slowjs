// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs // import "github.com/liustack/tifajs"

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
	"unicode/utf8"
)

// fuzzJSONScript parses T and, when it parses, writes it again every way a
// script can and checks that each text parses to what wrote it. It gives
// the text JSON.stringify wrote, or undefined when T is not JSON.
const fuzzJSONScript = `(() => {
	const fail = (what) => { const e = new Error(what); e.name = '` + fuzzContract + `'; throw e }
	let v
	try { v = JSON.parse(T) } catch (e) { if (e instanceof SyntaxError) return undefined; throw e }
	const s = JSON.stringify(v)
	if (JSON.stringify(JSON.parse(s)) !== s) fail('the text JSON.stringify wrote parses to another value')
	const pretty = JSON.stringify(v, null, '\t')
	if (JSON.stringify(JSON.parse(pretty)) !== s) fail('the indented text parses to another value')
	const keys = []
	const revived = JSON.parse(T, function (k, x) { if (keys.length < 4096) keys.push(k); return x })
	if (JSON.stringify(revived) !== s) fail('a reviver that changes nothing changed the value')
	const filtered = JSON.stringify(v, (k, x) => typeof x === 'number' ? String(x) : x)
	JSON.parse(filtered)
	V = v
	return s
})()`

// FuzzJSON parses a text with JSON.parse, writes it again with
// JSON.stringify (plain, indented, with a replacer) and with
// Value.MarshalJSON, and holds the engine to encoding/json: a text that is
// valid UTF-8 parses in javascript when Go's decoder takes it, and to the
// same value.
func FuzzJSON(f *testing.F) {
	for _, s := range []string{
		`{"a":[1,2.5,-0,1e400,"xé😀"],"b":{"c":null,"d":true}}`,
		`[]`, `{}`, `""`, `0`, `-0.0e-0`, `123456789012345678901234567890`,
		`"\ud800"`, `"\udc00\ud800"`, `{"__proto__":1,"a":1,"a":2}`, `{"1":1,"0":0,"b":2}`,
		` [ 1 , [ [ [ ] ] ] ] `, `[1,]`, `{"a" 1}`, `01`, `1.`, `"\x"`, "\"\t\"", `nul`,
		`[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[1]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]`,
		`{"toJSON":1,"constructor":{"prototype":2}}`, "\xef\xbb\xbf{}", "\"\xff\"",
	} {
		f.Add(s, byte(0))
	}
	fuzzNativeSeeds(3, func(limit byte, rest []byte) { f.Add(string(rest), limit) })
	f.Fuzz(func(t *testing.T, text string, limit byte) {
		if len(text) > fuzzMaxInput {
			return
		}
		m := newFuzzVM(t, int(limit))
		defer m.close()
		m.set("T", text)
		var out Value
		var evalErr error
		m.call("the JSON script", func() error {
			out, evalErr = m.EvalValue(fuzzJSONScript, EvalGlobal)
			return evalErr
		})
		if evalErr != nil {
			return // too deep, or out of memory or time
		}
		defer out.Free()
		parsed := !out.IsUndefined()
		var s string
		if parsed {
			var err error
			if s, err = m.goString(out.v); err != nil {
				t.Fatal(err)
			}
		}

		if parsed {
			var mj []byte
			var mjErr error
			m.call("MarshalJSON", func() error {
				var v Value
				if v, mjErr = m.EvalValue(`V`, EvalGlobal); mjErr != nil {
					return mjErr
				}
				defer v.Free()
				mj, mjErr = v.MarshalJSON()
				return mjErr
			})
			if mjErr != nil {
				return
			}
			if string(mj) != s {
				t.Fatalf("MarshalJSON gives %q, JSON.stringify %q", mj, s)
			}
			if !json.Valid(mj) {
				t.Fatalf("MarshalJSON gives a text Go's decoder refuses: %q", mj)
			}
		}

		// Go's decoder replaces bytes that are not UTF-8 with U+FFFD and
		// stops at a depth of 10000, where the engine has limits of its
		// own: compare only what both read the same way.
		if !utf8.ValidString(text) || fuzzJSONDepth(text) > 512 {
			return
		}
		var want any
		goErr := json.Unmarshal([]byte(text), &want)
		var syntax *json.SyntaxError
		if goErr != nil && !errors.As(goErr, &syntax) {
			return
		}
		switch {
		case parsed && goErr != nil:
			t.Fatalf("JSON.parse takes %q, Go refuses it: %v", text, goErr)
		case !parsed && goErr == nil:
			t.Fatalf("JSON.parse refuses %q, Go takes it", text)
		case parsed:
			var got any
			if err := json.Unmarshal([]byte(s), &got); err != nil {
				t.Fatalf("Go refuses %q, what JSON.stringify wrote: %v", s, err)
			}
			if !fuzzJSONSame(got, want) {
				t.Fatalf("JSON.parse of %q gives %q, Go reads %#v", text, s, want)
			}
		}
	})
}

// fuzzJSONDepth is the deepest nesting of brackets in s, strings counted
// too: a bound.
func fuzzJSONDepth(s string) int {
	d, most := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[', '{':
			d++
			most = max(most, d)
		case ']', '}':
			d--
		}
	}
	return most
}

// fuzzJSONSame reports whether two values of encoding/json are the same,
// numbers compared as JSON.stringify writes them: a number that does not fit
// a float64 is null.
func fuzzJSONSame(got, want any) bool {
	switch w := want.(type) {
	case float64:
		if math.IsInf(w, 0) {
			return got == nil
		}
		g, ok := got.(float64)
		return ok && g == w
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for i := range w {
			if !fuzzJSONSame(g[i], w[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for k, x := range w {
			y, ok := g[k]
			if !ok || !fuzzJSONSame(y, x) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(got, want)
}
