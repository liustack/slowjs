// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// atomVM is a runtime and a context for the tests of atomKey.
type atomVM struct {
	t   *testing.T
	tls *libc.TLS
	rt  uintptr
	ctx uintptr
}

func newAtomVM(t *testing.T) *atomVM {
	tls := libc.NewTLS()
	rt := XJS_NewRuntime(tls)
	v := &atomVM{t, tls, rt, XJS_NewContext(tls, rt)}
	t.Cleanup(func() {
		XJS_FreeContext(tls, v.ctx)
		XJS_FreeRuntime(tls, rt)
		tls.Close()
	})
	return v
}

// atom returns the atom of s, with a reference for the caller.
func (v *atomVM) atom(s string) TJSAtom {
	cs, err := libc.CString(s)
	if err != nil {
		v.t.Fatal(err)
	}
	defer libc.Xfree(v.tls, cs)
	return XJS_NewAtomLen(v.tls, v.ctx, cs, Tsize_t(len(s)))
}

// str returns a new string s, which is not an atom.
func (v *atomVM) str(s string) TJSValue {
	cs, err := libc.CString(s)
	if err != nil {
		v.t.Fatal(err)
	}
	defer libc.Xfree(v.tls, cs)
	return XJS_NewStringLen(v.tls, v.ctx, cs, Tsize_t(len(s)))
}

// count returns the reference count of the string or symbol val.
func (v *atomVM) count(val TJSValue) int32 {
	return (*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(v.tls, jsvPtr(val)))).Fref_count
}

// generic converts val as JS_ValueToAtom does without atomKey.
func (v *atomVM) generic(val TJSValue) TJSAtom {
	key := XJS_ToPropertyKey(v.tls, v.ctx, val)
	if jsvTag(key) != EJS_TAG_STRING {
		v.t.Fatalf("JS_ToPropertyKey gave tag %d", jsvTag(key))
	}
	return _JS_NewAtomStr(v.tls, v.ctx, jsvPtr(key))
}

// A string that is an atom and does not start with a digit takes atomKey:
// the atom and the counts are those of the generic path, one more
// reference to the atom unless it is a constant.
func TestAtomKeyCounts(t *testing.T) {
	v := newAtomVM(t)
	for _, c := range []struct {
		key      string
		constant bool
	}{
		{"abc", false},
		{"-0", true},
		{"-1", false},
		{" 1", false},
		{"NaN", true},
		{"ключ", false},
		{"length", true},
		{"constructor", true},
	} {
		a := v.atom(c.key)
		val := XJS_AtomToString(v.tls, v.ctx, a)
		n, step := v.count(val), int32(1)
		if c.constant {
			step = 0
		}
		if ___JS_AtomIsConst(v.tls, a) != 0 != c.constant {
			t.Fatalf("%q: constant %v", c.key, !c.constant)
		}
		got, ok := atomKey(v.tls, v.ctx, jsvPtr(val))
		if !ok || got != a {
			t.Fatalf("%q: atomKey gave %d %v, want %d", c.key, got, ok, a)
		}
		if v.count(val) != n+step {
			t.Errorf("%q: count %d after atomKey, want %d", c.key, v.count(val), n+step)
		}
		if g := v.generic(val); g != a || v.count(val) != n+2*step {
			t.Errorf("%q: the generic path gave %d with count %d, want %d with %d", c.key, g, v.count(val), a, n+2*step)
		}
		if x := XJS_ValueToAtom(v.tls, v.ctx, val); x != a || v.count(val) != n+3*step {
			t.Errorf("%q: JS_ValueToAtom gave %d with count %d, want %d with %d", c.key, x, v.count(val), a, n+3*step)
		}
		for range 3 {
			XJS_FreeAtom(v.tls, v.ctx, a)
		}
		if v.count(val) != n {
			t.Errorf("%q: count %d after the atoms are freed, want %d", c.key, v.count(val), n)
		}
		XFreeValue(v.tls, v.ctx, val)
		XJS_FreeAtom(v.tls, v.ctx, a)
	}
}

// Every other key goes the generic path: a string that is not an atom, an
// atom string that starts with a digit (is_num_string reads it), the empty
// string, a number and a symbol. JS_ValueToAtom gives what it gave.
func TestAtomKeyLeaves(t *testing.T) {
	v := newAtomVM(t)
	names := []string{"not an atom yet", "0", "4294967294"}
	var vals []TJSValue
	for _, k := range names {
		vals = append(vals, v.str(k))
	}
	for _, k := range []string{"1a", "1e3", "01", "4294967295", "1ключ"} {
		a := v.atom(k)
		names, vals = append(names, k), append(vals, XJS_AtomToString(v.tls, v.ctx, a))
		XJS_FreeAtom(v.tls, v.ctx, a)
	}
	names, vals = append(names, ""), append(vals, XJS_AtomToString(v.tls, v.ctx, EJS_ATOM_empty_string))
	for i, val := range vals {
		n := v.count(val)
		if _, ok := atomKey(v.tls, v.ctx, jsvPtr(val)); ok {
			t.Errorf("%q: atomKey took it", names[i])
		}
		if v.count(val) != n {
			t.Errorf("atomKey changed the count")
		}
		x := XJS_ValueToAtom(v.tls, v.ctx, val)
		g := v.generic(val)
		if x != g {
			t.Errorf("%q: JS_ValueToAtom gave %d, the generic path %d", names[i], x, g)
		}
		XJS_FreeAtom(v.tls, v.ctx, x)
		XJS_FreeAtom(v.tls, v.ctx, g)
		XFreeValue(v.tls, v.ctx, val)
	}
	num := eval0(t, v.tls, v.ctx, "7")
	if x := XJS_ValueToAtom(v.tls, v.ctx, num); x != ___JS_AtomFromUInt32(v.tls, 7) {
		t.Errorf("7 gave atom %d", x)
	}
	sym := eval0(t, v.tls, v.ctx, "Symbol('s')")
	n := v.count(sym)
	x := XJS_ValueToAtom(v.tls, v.ctx, sym)
	back := XJS_AtomToValue(v.tls, v.ctx, x)
	if jsvTag(back) != EJS_TAG_SYMBOL || jsvPtr(back) != jsvPtr(sym) {
		t.Errorf("the atom of a symbol is not that symbol")
	}
	XFreeValue(v.tls, v.ctx, back)
	XJS_FreeAtom(v.tls, v.ctx, x)
	if v.count(sym) != n {
		t.Errorf("symbol: count %d, want %d", v.count(sym), n)
	}
	XFreeValue(v.tls, v.ctx, sym)
}

// An atom freed and made again from the same text takes atomKey with its
// new index.
func TestAtomKeyMadeAgain(t *testing.T) {
	v := newAtomVM(t)
	for range 3 {
		a := v.atom("made again")
		val := XJS_AtomToString(v.tls, v.ctx, a)
		XJS_FreeAtom(v.tls, v.ctx, a)
		n := v.count(val)
		x := XJS_ValueToAtom(v.tls, v.ctx, val)
		if v.count(val) != n+1 {
			t.Errorf("count %d, want %d", v.count(val), n+1)
		}
		if got := toString(t, v.tls, v.ctx, XJS_AtomToString(v.tls, v.ctx, x)); got != "made again" {
			t.Errorf("the atom is %q", got)
		}
		XJS_FreeAtom(v.tls, v.ctx, x)
		// The string was the last reference: the atom is freed here.
		XFreeValue(v.tls, v.ctx, val)
	}
}

// The description of a symbol and the key of a registered symbol are
// strings whose JSString is the symbol's own atom, with the atom type of a
// symbol. atomKey leaves them, and JS_ValueToAtom gives the string atom of
// their text, as the generic path does, not the symbol.
func TestAtomKeySymbolText(t *testing.T) {
	v := newAtomVM(t)
	for _, c := range []struct{ sym, text, want string }{
		{"globalThis.ds = Symbol('desc'), ds", "ds.description", "desc"},
		{"Symbol.for('glob')", "Symbol.keyFor(Symbol.for('glob'))", "glob"},
	} {
		sym := eval0(t, v.tls, v.ctx, c.sym)
		val := eval0(t, v.tls, v.ctx, c.text)
		if jsvTag(val) != EJS_TAG_STRING || jsvPtr(val) != jsvPtr(sym) {
			t.Fatalf("%s: not the JSString of the symbol", c.text)
		}
		n := v.count(val)
		if _, ok := atomKey(v.tls, v.ctx, jsvPtr(val)); ok || v.count(val) != n {
			t.Errorf("%s: atomKey took it (count %d, was %d)", c.text, v.count(val), n)
		}
		x := XJS_ValueToAtom(v.tls, v.ctx, val)
		g := v.generic(val)
		if x != g {
			t.Errorf("%s: JS_ValueToAtom gave %d, the generic path %d", c.text, x, g)
		}
		back := XJS_AtomToValue(v.tls, v.ctx, x)
		if jsvTag(back) != EJS_TAG_STRING || jsvPtr(back) == jsvPtr(sym) {
			t.Errorf("%s: the atom is not a string atom", c.text)
		}
		if got := toString(t, v.tls, v.ctx, back); got != c.want {
			t.Errorf("%s: the atom is %q, want %q", c.text, got, c.want)
		}
		XJS_FreeAtom(v.tls, v.ctx, x)
		XJS_FreeAtom(v.tls, v.ctx, g)
		XFreeValue(v.tls, v.ctx, val)
		XFreeValue(v.tls, v.ctx, sym)
	}
}
