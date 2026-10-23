// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// libFiles returns the generated files of lib/.
func libFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "lib", "ccgo_*.go"))
	if err != nil || len(files) == 0 {
		t.Fatal(files, err)
	}
	return files
}

var (
	reLocalHead = regexp.MustCompile(`\t` + regexp.QuoteMeta(framesInGo) + `\tvar localbp \[(\d+)\]uint64\n`)
	reLocalCell = regexp.MustCompile(`unsafe\.Add\(unsafe\.Pointer\(&localbp\), (\d+)\)`)
)

// unfixFrames gives the functions fixFrames changed their TLS frame back,
// with every cell written as (*T)(unsafe.Pointer(bp + K)).
func unfixFrames(t *testing.T, b []byte) []byte {
	t.Helper()
	b = reLocalHead.ReplaceAllFunc(b, func(m []byte) []byte {
		var words int
		fmt.Sscan(string(reLocalHead.FindSubmatch(m)[1]), &words)
		return fmt.Appendf(nil, "\tbp := tls.Alloc(%d)\n\tdefer tls.Free(%d)\n", 8*words, 8*words)
	})
	b = reLocalCell.ReplaceAll(b, []byte("unsafe.Pointer(bp + $1)"))
	b = bytes.ReplaceAll(b, []byte("unsafe.Pointer(&localbp)"), []byte("unsafe.Pointer(bp)"))
	if bytes.Contains(b, []byte("localbp")) {
		t.Fatal("localbp left after undoing fixFrames")
	}
	return b
}

// Every generated file has the functions of frameSites changed (on 32-bit
// targets, the three that have a frame there), fixFrames leaves a fixed file
// as it is and makes it again from the file with the frames given back
// (with fixColdFrames for the functions of coldSites), and the property
// slot audit sees the same uses in both.
func TestFramesEveryFile(t *testing.T) {
	hw, err := handWritten(filepath.Join("..", "..", "lib"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		pin := strings.HasSuffix(fn, "ccgo_darwin_arm64.go")
		out, err := fixFrames(b, hw, frameSites, pin)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(out, b) {
			t.Fatalf("%s: fixFrames changed a fixed file", fn)
		}
		// The functions of coldSites have the rest of their frame changed
		// too.
		want := len(frameSites) + len(coldSites)
		if n := bytes.Count(b, []byte(framesInGo)); n != want {
			t.Errorf("%s: %d frames changed, want %d", fn, n, want)
		}

		before := unfixFrames(t, b)
		if bytes.Equal(before, b) {
			t.Fatalf("%s: nothing to undo", fn)
		}
		again, err := fixFrames(before, hw, frameSites, pin)
		if err == nil {
			again, err = fixColdFrames(again, hw, pin)
		}
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(again, b) {
			t.Errorf("%s: fixFrames does not make the file again from its frames given back", fn)
		}
		slotsBefore, err := auditSlotUses(before)
		if err != nil {
			t.Fatal(err)
		}
		slotsAfter, err := auditSlotUses(b)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(slotsBefore, slotsAfter) {
			t.Errorf("%s: the slot audit differs after fixFrames", fn)
		}
	}
}

// A synthetic file: _leaf qualifies, _caller calls it, _check calls the
// stack check.
const (
	framesHead = `package lib

import (
	"unsafe"

	"modernc.org/libc"
)

func _caller(tls *libc.TLS, v TJSValue) uintptr {
	return _leaf(tls, v)
}

func _check(tls *libc.TLS) int32 {
	return _js_check_stack_overflow(tls, 0, 0)
}
`
	framesLeaf = `
func _leaf(tls *libc.TLS, _v TJSValue) (r uintptr) {
	bp := tls.Alloc(32)
	defer tls.Free(32)
	*(*TJSValue)(unsafe.Pointer(bp + 16)) = _v
	var d float64
	if int32((**(**TJSValue)(__ccgo_up(bp + 16))).Ftag) != 0 {
		return *(*uintptr)(unsafe.Pointer(bp + 16))
	}
	*(*float64)(unsafe.Pointer(bp)) = d
	(*(*[2]uint32)(unsafe.Pointer(bp)))[1] = 0
	return *(*uintptr)(unsafe.Pointer(bp))
}
`
	framesLeafFixed = `
func _leaf(tls *libc.TLS, _v TJSValue) (r uintptr) {
	// Changed by internal/fixlib: the TLS frame is a Go array (see fixFrames).
	var localbp [4]uint64
	*(*TJSValue)(unsafe.Add(unsafe.Pointer(&localbp), 16)) = _v
	var d float64
	if int32((*(*TJSValue)(unsafe.Add(unsafe.Pointer(&localbp), 16))).Ftag) != 0 {
		return *(*uintptr)(unsafe.Add(unsafe.Pointer(&localbp), 16))
	}
	*(*float64)(unsafe.Pointer(&localbp)) = d
	(*(*[2]uint32)(unsafe.Pointer(&localbp)))[1] = 0
	return *(*uintptr)(unsafe.Pointer(&localbp))
}
`
	framesWide   = "\ntype TJSValue = struct {\n\tFu   uint64\n\tFtag int64\n}\n"
	framesNarrow = "\ntype TJSValue = uint64\n"
)

var leafSite = map[string]frameSite{"_leaf": {32, 6, "a test"}}

func TestFramesShape(t *testing.T) {
	in := []byte(framesHead + framesWide + framesLeaf)
	want := framesHead + framesWide + framesLeafFixed
	out, err := fixFrames(in, nil, leafSite, true)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != want {
		t.Fatalf("got\n%s\nwant\n%s", out, want)
	}
	again, err := fixFrames(out, nil, leafSite, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, out) {
		t.Fatal("fixFrames changed a fixed file")
	}

	// On a 32-bit target, a function of frameSites may have no frame.
	noFrame := framesHead + framesNarrow + "\nfunc _leaf(tls *libc.TLS, _v TJSValue) (r uintptr) {\n\treturn uintptr(_v)\n}\n"
	if out, err := fixFrames([]byte(noFrame), nil, leafSite, false); err != nil || string(out) != noFrame {
		t.Fatalf("32-bit without a frame: %v\n%s", err, out)
	}
}

func TestFramesRejects(t *testing.T) {
	for _, c := range []struct {
		name, old, new, want string
		narrow, nopin        bool
	}{
		{name: "pinned size", old: "tls.Alloc(32)\n\tdefer tls.Free(32)", new: "tls.Alloc(48)\n\tdefer tls.Free(48)", want: "a frame of 48 bytes with 6 cell uses, frameSites has 32 and 6"},
		{name: "pinned cells", old: "\treturn *(*uintptr)(unsafe.Pointer(bp))\n", new: "\treturn 0\n", want: "a frame of 32 bytes with 5 cell uses"},
		{name: "not pinned", old: "\treturn *(*uintptr)(unsafe.Pointer(bp))\n", new: "\treturn 0\n", nopin: true},
		{name: "stack check", old: "\tvar d float64\n", new: "\tvar d float64\n\t_js_check_stack_overflow(tls, 0, 0)\n", want: "_leaf can reach a stack check, through _js_check_stack_overflow"},
		{name: "through a call", old: "\tvar d float64\n", new: "\tvar d float64\n\t_check(tls)\n", want: "_leaf can reach a stack check, through _check"},
		{name: "indirect call", old: "\tvar d float64\n", new: "\tvar d float64\n\t(*(*func(*libc.TLS))(unsafe.Pointer(&struct{ uintptr }{r})))(tls)\n", want: "through an indirect call"},
		{name: "finalizer", old: "\tvar d float64\n", new: "\tvar d float64\n\t(*(*func(*libc.TLS))(unsafe.Pointer(&struct{ uintptr }{(*TJSClass)(unsafe.Pointer(r)).Ffinalizer})))(tls)\n", nopin: true},
		{name: "bp passed", old: "\tvar d float64\n", new: "\tvar d float64\n\t_caller(tls, bp)\n", want: "bp+0 in _caller(tls, bp)"},
		{name: "cell passed", old: "\tvar d float64\n", new: "\tvar d float64\n\t_caller(tls, bp+16)\n", want: "bp+16 in _caller(tls, bp+16)"},
		{name: "bp stored", old: "\tvar d float64\n", new: "\tvar d float64\n\tr = bp\n", want: "bp+0 in r = bp"},
		{name: "address", old: "\tvar d float64\n", new: "\tvar d float64\n\tr = uintptr(unsafe.Pointer(&(*TJSValue)(unsafe.Pointer(bp + 16)).Fu))\n", want: "its address taken"},
		{name: "pointer", old: "\tvar d float64\n", new: "\tvar d float64\n\tr = uintptr(unsafe.Pointer((*TJSValue)(unsafe.Pointer(bp + 16))))\n", want: "the pointer used in"},
		{name: "variable index", old: "[1] = 0", new: "[r] = 0", want: "a variable index"},
		{name: "slice", old: "(*(*[2]uint32)(unsafe.Pointer(bp)))[1] = 0", new: "_ = (*(*[2]uint32)(unsafe.Pointer(bp)))[:]", want: "sliced"},
		{name: "variable offset", old: "unsafe.Pointer(bp)) = d", new: "unsafe.Pointer(bp + r)) = d", want: "bp in unsafe.Pointer(bp + r)"},
		{name: "late frame", old: "\tbp := tls.Alloc(32)\n", new: "\tr = 1\n\tbp := tls.Alloc(32)\n", want: "the frame is not allocated at the entry"},
		{name: "name taken", old: "\tvar d float64\n", new: "\tvar d float64\n\tvar localbp int\n\t_ = localbp\n", want: "already has a variable localbp"},
		{name: "no frame", old: "\tbp := tls.Alloc(32)\n\tdefer tls.Free(32)\n", new: "\tvar bp uintptr\n", want: "_leaf: no TLS frame"},
		{name: "no frame on 32-bit", old: "\tbp := tls.Alloc(32)\n\tdefer tls.Free(32)\n", new: "\tvar bp uintptr\n", narrow: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if strings.Count(framesLeaf, c.old) != 1 {
				t.Fatalf("%q is not once in the function", c.old)
			}
			typ := framesWide
			if c.narrow {
				typ = framesNarrow
			}
			in := framesHead + typ + strings.Replace(framesLeaf, c.old, c.new, 1)
			_, err := fixFrames([]byte(in), nil, leafSite, !c.nopin)
			switch {
			case c.want == "" && err != nil:
				t.Fatal(err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("got %v, want an error with %q", err, c.want)
			}
		})
	}

	// A function fixFrames changed must not use bp beside the array.
	fixed := strings.Replace(framesLeafFixed, "\treturn *(*uintptr)(unsafe.Pointer(&localbp))\n", "\treturn bp\n", 1)
	if _, err := fixFrames([]byte(framesHead+framesWide+fixed), nil, leafSite, false); err == nil || !strings.Contains(err.Error(), "bp is used beside the Go array") {
		t.Fatalf("got %v", err)
	}
}

// The step reaching gives for a function with several ways to a stack
// check is the same in every run.
func TestReachingOrder(t *testing.T) {
	for range 20 {
		g := map[string]*callNode{
			"a": {callees: map[string]bool{"d": true, "c": true, "b": true}},
			"b": {callees: map[string]bool{"e": true}},
			"c": {checks: true},
			"d": {checks: true},
			"e": {callees: map[string]bool{"f": true, "c": true}},
			"f": {indirect: []string{"x"}},
		}
		why := reaching(g)
		if why["a"] != "c" || why["b"] != "e" || why["e"] != "c" || why["f"] != "an indirect call" {
			t.Fatalf("got %v", why)
		}
	}
}
