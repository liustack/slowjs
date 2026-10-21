// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// fixColdFrames leaves a fixed file as it is on every target, and makes the
// change in the file as ccgo writes it, as fixlib runs it.
func TestColdFramesIdempotent(t *testing.T) {
	hw, err := handWritten(filepath.Join("..", "..", "lib"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if n := bytes.Count(b, []byte(coldMark)); n != 2*len(coldSites) {
			t.Errorf("%s: %d marks of the cold paths, want %d", name, n, 2*len(coldSites))
		}
		got, err := fixColdFrames(bytes.Clone(b), hw, name == "ccgo_darwin_arm64.go")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: a fixed file changed", name)
		}
	}
}

// coldInput returns lib/ccgo_darwin_arm64.go as fixColdFrames gets it from
// a file taken from upstream, after fixBeforeUp and fixUp: the fixed file
// with the change of fixColdFrames undone.
func coldInput(t *testing.T) ([]byte, []*ast.File) {
	t.Helper()
	hw, err := handWritten(filepath.Join("..", "..", "lib"))
	if err != nil {
		t.Fatal(err)
	}
	return uncold(t, engineFile(t)), hw
}

var reColdCell = regexp.MustCompile(`\(\*([^()]+)\)\(unsafe\.Add\(unsafe\.Pointer\(&localbp\), (\d+)\)\)|\(\*([^()]+)\)\(unsafe\.Pointer\(&localbp\)\)`)

// uncold undoes fixColdFrames in b, the text of the 64-bit file it was
// made for.
func uncold(t *testing.T, b []byte) []byte {
	t.Helper()
	for name, site := range coldSites {
		shape := coldShapes[name][0]
		vals := map[string]string{}
		cold, err := funcBody(b, funcHead(site.cold), site.cold)
		if err != nil {
			t.Fatal(err)
		}
		fn := append([]byte("\n"+coldMark), cold[1:]...)
		if _, _, err := matchHoles(fn, shape.fn, site.cold, vals); err != nil {
			t.Fatal(err)
		}
		b = bytes.Replace(b, fn, nil, 1)
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			t.Fatal(err)
		}
		orig := bytes.Replace(body, []byte(fill(shape.call, vals)), []byte(fill(shape.path, vals)), 1)
		head := framesInGo + localPrefix + strconv.Itoa(site.frame/8) + "]uint64\n"
		orig = bytes.Replace(orig, []byte(head), []byte(fmt.Sprintf("bp := tls.Alloc(%d)\n\tdefer tls.Free(%d)\n", site.frame, site.frame)), 1)
		orig = reColdCell.ReplaceAllFunc(orig, func(m []byte) []byte {
			g := reColdCell.FindSubmatch(m)
			if g[1] != nil {
				return []byte(fmt.Sprintf("(*%s)(unsafe.Pointer(bp + %s))", g[1], g[2]))
			}
			return []byte(fmt.Sprintf("(*%s)(unsafe.Pointer(bp))", g[3]))
		})
		b = bytes.Replace(b, body, orig, 1)
	}
	return b
}

// From the file as ccgo writes it, fixColdFrames gives the functions and
// the cold functions of the fixed file, and the rest of the file as it
// was.
func TestColdFramesFromUpstream(t *testing.T) {
	b, hw := coldInput(t)
	got, err := fixColdFrames(b, hw, true)
	if err != nil {
		t.Fatal(err)
	}
	want := engineFile(t)
	if !bytes.Equal(got, want) {
		t.Error("the change differs from the fixed file")
	}
	for name, site := range coldSites {
		for _, fn := range []string{name, site.cold} {
			g, err := funcBody(got, funcHead(fn), fn)
			if err != nil {
				t.Fatal(err)
			}
			w, err := funcBody(want, funcHead(fn), fn)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(g, w) {
				t.Errorf("%s differs from the fixed file", fn)
			}
		}
	}
}

// A change to a cold path, or to what fixColdFrames relies on in the rest
// of the function, stops it.
func TestColdFramesRejects(t *testing.T) {
	b, hw := coldInput(t)
	fixed := engineFile(t)
	for _, c := range []struct {
		name string
		in   []byte
		fn   string // the function to change
		old  string
		new  string
		err  string
	}{
		{"another call in the cold path", b, "___JS_AtomToValue", "\t\tlen1 = Xu32toa(tls, bp+16, ___JS_AtomToUInt32(tls, atom))\n", "\t\tlen1 = Xu32toa(tls, bp+16, ___JS_AtomToUInt32(tls, atom)+1)\n", "no shape of coldShapes fits"},
		{"a second BigInt", b, "_js_strict_eq2", "\tres = libc.BoolInt32(_js_bigint_cmp(tls, ctx1, p11, p21) == libc.Int32FromInt32(0))\n", "\tres = libc.BoolInt32(_js_bigint_cmp(tls, ctx1, p11, p11) == libc.Int32FromInt32(0))\n", "no shape of coldShapes fits"},
		{"a parameter cell written", b, "_js_strict_eq2", "\ttag2 = int32((*(*TJSValue)(unsafe.Pointer(bp + 32))).Ftag)\n", "\ttag2 = int32((*(*TJSValue)(unsafe.Pointer(bp + 32))).Ftag)\n\t*(*TJSValue)(unsafe.Pointer(bp + 16)) = _op2\n", "the cell of a parameter, bp+16, written"},
		{"a BigInt buffer used outside", b, "_js_strict_eq2", "\ttag2 = int32((*(*TJSValue)(unsafe.Pointer(bp + 32))).Ftag)\n", "\ttag2 = int32((*(*TJSValue)(unsafe.Pointer(bp + 32))).Ftag)\n\t*(*int32)(unsafe.Pointer(bp + 64)) = 0\n", "bp+64 used outside the cold path"},
		{"an address passed outside", b, "___JS_AtomToValue", "\t\trt = (*TJSContext)(unsafe.Pointer(ctx1)).Frt\n", "\t\trt = (*TJSContext)(unsafe.Pointer(ctx1)).Frt\n\t\t_ = Xu32toa(tls, bp, 0)\n", "bp+0 in Xu32toa(tls, bp, 0)"},
		{"a stack check", b, "___JS_AtomToValue", "\t\trt = (*TJSContext)(unsafe.Pointer(ctx1)).Frt\n", "\t\trt = (*TJSContext)(unsafe.Pointer(ctx1)).Frt\n\t\t_ = _js_check_stack_overflow(rt, 0)\n", "can reach a stack check other than through ___JS_AtomToValue_cold"},
		{"a changed cold function", fixed, "___JS_AtomToValue_cold", "\tlen1 = Xu32toa(tls, bp+16, ___JS_AtomToUInt32(tls, atom))\n\treturn", "\tlen1 = Xu32toa(tls, bp+24, ___JS_AtomToUInt32(tls, atom))\n\treturn", "___JS_AtomToValue_cold: {BUF} is"},
		{"a cold function elsewhere", fixed, "", "\nfunc ___JS_AtomToValue_cold(", "\nfunc ___JS_AtomToValue_cold2(", "want 1 function ___JS_AtomToValue_cold, found 0"},
	} {
		in := string(c.in)
		start, end := 0, len(in)
		if c.fn != "" {
			body, err := funcBody(c.in, funcHead(c.fn), c.fn)
			if err != nil {
				t.Fatal(err)
			}
			start = strings.Index(in, string(body))
			end = start + len(body)
		}
		i := strings.Index(in[start:end], c.old)
		if i < 0 {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		i += start
		changed := in[:i] + c.new + in[i+len(c.old):]
		if _, err := fixColdFrames([]byte(changed), hw, true); err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
