// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// matchTranscript evaluates testdata/<name>.js and compares the result with
// testdata/<name>.txt line by line.
func matchTranscript(t *testing.T, name string) {
	t.Helper()
	js, err := os.ReadFile(filepath.Join("testdata", name+".js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", name+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := eval(t, name+".js", string(js))
	gl, wl := strings.Split(got, "\n"), strings.Split(strings.TrimSuffix(string(want), "\n"), "\n")
	for i := 0; i < len(gl) || i < len(wl); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Errorf("line %d:\n got %s\nwant %s", i+1, g, w)
		}
	}
}

// testdata/forin.js runs for in over objects of every kind, with
// prototypes that have enumerable properties and prototypes that do not,
// and changes the chain while loops run. testdata/forin.txt is the
// transcript of the engine before fixForIn, so it must stay as it is. Node
// gives the same, but for the order of proxy traps and the properties a
// loop meets once its chain changed, which the specification leaves open.
func TestForInSemantics(t *testing.T) {
	matchTranscript(t, "forin")
}

// forInNoEnum answers for the prototypes it can read from their shape, and
// leaves the rest to JS_GetOwnPropertyNamesInternal.
func TestForInNoEnum(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	for _, c := range []struct {
		name, js string
		quiet    bool
	}{
		{"Object.prototype", "Object.prototype", true},
		{"empty", "({})", true},
		{"class prototype", "(class { m() {} get g() { return 1 } }).prototype", true},
		{"hidden", "Object.defineProperty({}, 'x', {value: 1})", true},
		{"deleted", "var d = {a: 1, b: 2}; delete d.a; delete d.b; d", true},
		{"function", "(function () {})", true},
		{"enumerable", "({a: 1})", false},
		{"enumerable after hidden", "Object.defineProperty({b: 1}, 'x', {value: 1})", false},
		{"enumerable getter", "({get g() { return 1 }})", false},
		{"integer key", "({1: 1})", false},
		// Symbols are not listed, but forInNoEnum does not tell them apart.
		{"enumerable symbol", "({[Symbol.iterator]: 1})", false},
		{"empty array", "[]", false},
		{"Array.prototype", "Array.prototype", false},
		{"string object", "new String('')", false},
		{"proxy", "new Proxy({}, {})", false},
	} {
		obj := eval0(t, tls, ctx, c.js)
		if tag(obj) != EJS_TAG_OBJECT {
			t.Fatalf("%s: %s is no object", c.name, c.js)
		}
		// The answer goes to C memory, as it does for the engine.
		ptab, plen := tls.Alloc(16), tls.Alloc(16)
		*(*uintptr)(unsafe.Pointer(ptab)) = 1
		*(*uint32)(unsafe.Pointer(plen)) = 1
		got := forInNoEnum(tls, jsvPtr(obj), ptab, plen)
		tab, n := *(*uintptr)(unsafe.Pointer(ptab)), *(*uint32)(unsafe.Pointer(plen))
		tls.Free(16)
		tls.Free(16)
		if got != c.quiet {
			t.Errorf("%s: forInNoEnum = %v, want %v", c.name, got, c.quiet)
		}
		if got && (tab != 0 || n != 0) {
			t.Errorf("%s: the names are %#x, %d, want none", c.name, tab, n)
		}
		if !got && (tab != 1 || n != 1) {
			t.Errorf("%s: forInNoEnum wrote the names without answering", c.name)
		}
		XFreeValue(tls, ctx, obj)
	}
}
