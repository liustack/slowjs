// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"fmt"
	"slices"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// ownKeysObjects are objects of every kind of key, with the shapes
// lib/ownkeys.go lists, keeps in the key cache, and leaves to the engine.
var ownKeysObjects = []struct{ name, js string }{
	{"empty", "({})"},
	{"strings", "({a: 1, b: 2, c: 3})"},
	{"indexes", "({b: 1, 10: 2, 2: 3, a: 4, 4294967294: 5, 4294967295: 6, '01': 7})"},
	{"symbols", "({[Symbol.iterator]: 1, a: 2, [Symbol('s')]: 3, 1: 4})"},
	{"hidden", "Object.defineProperties({a: 1}, {h: {value: 2}, [Symbol('h')]: {value: 3}, 3: {value: 4}})"},
	{"accessors", "({get a() { return 1 }, set b(v) {}, c: 3})"},
	{"deleted", "(() => { const o = {a: 1, b: 2, 1: 3, c: 4}; delete o.b; delete o[1]; return o })()"},
	{"private", "(() => { const s = Symbol('s'); class C { #p = 1; x = 2; #q = 3; constructor() { this[s] = 4; this[0] = 5 } }; return new C })()"},
	{"null proto", "Object.assign(Object.create(null), {z: 1, 0: 2, y: 3})"},
	{"frozen", "Object.freeze({a: 1, 2: 2})"},
	{"wide", "({'\\u00e9': 1, '\\u4e2d': 2, '1\\u4e2d': 3})"},
	{"many strings", "Object.fromEntries(Array.from({length: 40}, (_, i) => ['k' + i, i]))"},
	{"many indexes", "Object.fromEntries(Array.from({length: 20}, (_, i) => [String(19 - i), i]))"},
	{"function", "(function f(a, b) {})"},
	{"class", "(class K { static s = 1; static m() {} })"},
	{"error", "new Error('m')"},
	{"array", "Object.assign([1, 2], {x: 3})"},
	{"arguments", "(function () { return arguments })(1, 2)"},
	{"string object", "Object.assign(new String('ab'), {x: 1})"},
	{"typed array", "new Uint8Array(3)"},
	{"proxy", "new Proxy({a: 1}, {})"},
}

// The flags of JS_GetOwnPropertyNames.
const ownKeysAllFlags = MJS_GPN_STRING_MASK | MJS_GPN_SYMBOL_MASK | MJS_GPN_PRIVATE_MASK | MJS_GPN_ENUM_ONLY | MJS_GPN_SET_ENUM

// ownKeysWant lists the own keys of the ordinary object p as
// JS_GetOwnPropertyNamesInternal does with flags, from its shape, telling
// the kinds of the keys with the engine's JS_AtomGetKind and
// JS_AtomIsArrayIndex.
func ownKeysWant(tls *libc.TLS, ctx, p uintptr, flags int32) []string {
	sh := (*TJSObject)(unsafe.Pointer(p)).Fshape
	prs := _get_shape_prop(tls, sh)
	type key struct {
		atom TJSAtom
		enum bool
		n    uint32
	}
	var nums, strs, syms []key
	for i := range (*TJSShape)(unsafe.Pointer(sh)).Fprop_count {
		pr := (*TJSShapeProperty)(unsafe.Pointer(prs + uintptr(i)*unsafe.Sizeof(TJSShapeProperty{})))
		enum := pr.F__ccgo0&forInFlagsMask>>forInFlagsShift&MJS_PROP_ENUMERABLE != 0
		if pr.Fatom == MJS_ATOM_NULL || flags&MJS_GPN_ENUM_ONLY != 0 && !enum {
			continue
		}
		kind := _JS_AtomGetKind(tls, ctx, pr.Fatom)
		if flags>>kind&1 == 0 {
			continue
		}
		pn := tls.Alloc(8)
		index := _JS_AtomIsArrayIndex(tls, ctx, pn, pr.Fatom) != 0
		n := *(*Tuint32_t)(unsafe.Pointer(pn))
		tls.Free(8)
		switch {
		case index:
			nums = append(nums, key{pr.Fatom, enum, n})
		case kind == EJS_ATOM_KIND_STRING:
			strs = append(strs, key{pr.Fatom, enum, 0})
		default:
			syms = append(syms, key{pr.Fatom, enum, 0})
		}
	}
	slices.SortStableFunc(nums, func(a, b key) int { return int(int64(a.n) - int64(b.n)) })
	var r []string
	for _, k := range slices.Concat(nums, strs, syms) {
		r = append(r, fmt.Sprintf("%s/%v", ownKeyName(tls, ctx, k.atom), k.enum))
	}
	return r
}

// ownKeyName returns the name of atom, with the kind of a symbol or a
// private name.
func ownKeyName(tls *libc.TLS, ctx uintptr, atom TJSAtom) string {
	p := XJS_AtomToCStringLen(tls, ctx, 0, atom)
	defer XJS_FreeCString(tls, ctx, p)
	return fmt.Sprintf("%s(%d)", libc.GoString(p), _JS_AtomGetKind(tls, ctx, atom))
}

// ownKeysGot lists the own keys of obj with JS_GetOwnPropertyNames.
func ownKeysGot(t *testing.T, tls *libc.TLS, ctx uintptr, obj TJSValue, flags int32) []string {
	t.Helper()
	// The out parameters are in C memory, as the engine's are.
	out := tls.Alloc(16)
	defer tls.Free(16)
	if XJS_GetOwnPropertyNames(tls, ctx, out, out+8, obj, flags) != 0 {
		t.Fatalf("flags %#x: JS_GetOwnPropertyNames throws", flags)
	}
	tab, n := *(*uintptr)(unsafe.Pointer(out)), *(*uint32)(unsafe.Pointer(out + 8))
	defer XJS_FreePropertyEnum(tls, ctx, tab, n)
	var r []string
	for _, e := range unsafe.Slice((*TJSPropertyEnum)(unsafe.Pointer(tab)), n) {
		r = append(r, fmt.Sprintf("%s/%v", ownKeyName(tls, ctx, e.Fatom), e.Fis_enumerable != 0))
	}
	return r
}

// For every combination of flags, JS_GetOwnPropertyNames lists the own
// keys of an ordinary object as the engine does, from the key cache too,
// and lists those of exotic objects as before.
func TestOwnKeysFlags(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	for _, c := range ownKeysObjects {
		obj := eval0(t, tls, ctx, c.js)
		if tag(obj) != EJS_TAG_OBJECT {
			t.Fatalf("%s: %s is no object", c.name, c.js)
		}
		p := jsvPtr(obj)
		exotic := *(*uint8)(unsafe.Pointer(p + forInExoticOff))&forInExotic != 0
		for flags := int32(0); flags <= ownKeysAllFlags; flags++ {
			if flags&^ownKeysAllFlags != 0 {
				continue
			}
			// Twice: the second listing takes the kinds from the cache.
			for range 2 {
				got := ownKeysGot(t, tls, ctx, obj, flags)
				if exotic {
					continue
				}
				if want := ownKeysWant(tls, ctx, p, flags); !slices.Equal(got, want) {
					t.Errorf("%s, flags %#x:\ngot  %q\nwant %q", c.name, flags, got, want)
				}
			}
		}
		XFreeValue(tls, ctx, obj)
	}
}

// ownKeyCached reports whether the key cache of the runtime holds the
// kinds of the shape sh.
func ownKeyCached(rt, sh uintptr) bool {
	return ownKeySlot(rt, sh).sh == sh
}

// The key cache keeps the kinds of the keys of a shape in the shape hash
// table, and drops them when the shape leaves the table: when the shape
// gets a key, when a key is deleted or changes, and when the shape is
// freed. A shape out of the table is listed without the cache.
func TestOwnKeysCache(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	keys := func(js string) string { return eval1(t, tls, ctx, "JSON.stringify("+js+")") }
	shapeOf := func(js string) (uintptr, bool) {
		v := eval0(t, tls, ctx, js)
		defer XFreeValue(tls, ctx, v)
		sh := (*TJSObject)(unsafe.Pointer(jsvPtr(v))).Fshape
		return sh, (*TJSShape)(unsafe.Pointer(sh)).Fis_hashed != 0
	}

	eval1(t, tls, ctx, "globalThis.o = {a: 1, 2: 2}; globalThis.s = Symbol('s'); 0")
	if got := keys("Object.keys(o)"); got != `["2","a"]` {
		t.Fatalf("keys %s", got)
	}
	sh, hashed := shapeOf("o")
	if !hashed || !ownKeyCached(rt, sh) {
		t.Fatalf("the shape of o is not in the cache (hashed %v)", hashed)
	}

	// A new key: the shape leaves the table, and changes in place or is
	// copied.
	eval1(t, tls, ctx, "o[s] = 3; o[1] = 4; 0")
	if ownKeyCached(rt, sh) {
		t.Errorf("the cache keeps the kinds of a shape that got a key")
	}
	if got := keys("Reflect.ownKeys(o).map(String)"); got != `["1","2","a","Symbol(s)"]` {
		t.Errorf("after new keys: %s", got)
	}
	if got := keys("Object.values(o)"); got != `[4,2,1]` {
		t.Errorf("values after new keys: %s", got)
	}

	// A deleted key and a changed key take the shape out of the table.
	for _, js := range []string{"delete o.a", "Object.defineProperty(o, 2, {enumerable: false})"} {
		eval1(t, tls, ctx, "Object.keys(o); 0")
		sh, hashed = shapeOf("o")
		if hashed && !ownKeyCached(rt, sh) {
			t.Fatalf("before %s: the shape of o is not in the cache", js)
		}
		eval1(t, tls, ctx, js+"; 0")
		if sh2, hashed2 := shapeOf("o"); hashed2 || ownKeyCached(rt, sh2) || ownKeyCached(rt, sh) {
			t.Errorf("after %s: the shape is hashed %v, cached %v", js, hashed2, ownKeyCached(rt, sh2))
		}
	}
	if got := keys("Reflect.ownKeys(o).map(String)"); got != `["1","2","Symbol(s)"]` {
		t.Errorf("after delete: %s", got)
	}
	if got := keys("Object.keys(o)"); got != `["1"]` {
		t.Errorf("enumerable after delete: %s", got)
	}

	// A freed shape leaves the cache.
	eval1(t, tls, ctx, "globalThis.f = {fresh1: 1, fresh2: 2}; Object.keys(f); 0")
	sh, _ = shapeOf("f")
	if !ownKeyCached(rt, sh) {
		t.Fatalf("the shape of f is not in the cache")
	}
	eval1(t, tls, ctx, "delete globalThis.f; 0")
	XJS_RunGC(tls, rt)
	if ownKeyCached(rt, sh) {
		t.Errorf("the cache keeps the kinds of a freed shape")
	}
}

// copyShape gives a new object the shape of its source when every key of
// the source is a string or a symbol after the strings, enumerable,
// writable and configurable, and both have the same prototype. Other
// spreads copy the keys one by one, as the engine does.
func TestOwnKeysCopyShape(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	for _, c := range []struct {
		name, src, spread string
		same              bool
	}{
		{"strings", "({a: 1, b: 'x', c: {}})", "({...src})", true},
		{"symbol last", "({a: 1, [Symbol.iterator]: 2})", "({...src})", true},
		{"symbol first", "({[Symbol.iterator]: 2, a: 1})", "({...src})", false},
		{"index", "({a: 1, 0: 2})", "({...src})", false},
		{"hidden", "Object.defineProperty({a: 1}, 'h', {value: 2})", "({...src})", false},
		{"read-only", "Object.freeze({a: 1})", "({...src})", false},
		{"getter", "({get a() { return 1 }})", "({...src})", false},
		{"key before", "({a: 1})", "({x: 0, ...src})", false},
		{"key after", "({a: 1})", "({...src, x: 0})", false},
		{"null proto", "Object.assign(Object.create(null), {a: 1})", "({...src})", false},
		{"rest", "({a: 1, b: 2})", "(({...r}) => r)(src)", true},
		{"rest excluded", "({a: 1, b: 2})", "(({a, ...r}) => r)(src)", false},
		{"stack", "Object.assign(Object.create(null), {stack: 1})", "Object.setPrototypeOf({...src}, null)", false},
	} {
		eval1(t, tls, ctx, "globalThis.src = "+c.src+"; 0")
		v := eval0(t, tls, ctx, c.spread)
		if tag(v) != EJS_TAG_OBJECT {
			t.Fatalf("%s: %s", c.name, toString(t, tls, ctx, v))
		}
		s := eval0(t, tls, ctx, "src")
		same := (*TJSObject)(unsafe.Pointer(jsvPtr(v))).Fshape == (*TJSObject)(unsafe.Pointer(jsvPtr(s))).Fshape
		if same != c.same {
			t.Errorf("%s: the copy has the shape of the source: %v, want %v", c.name, same, c.same)
		}
		XFreeValue(tls, ctx, s)
		XFreeValue(tls, ctx, v)
		got := eval1(t, tls, ctx, "JSON.stringify(Reflect.ownKeys("+c.spread+").map(String))")
		want := eval1(t, tls, ctx, "JSON.stringify(Reflect.ownKeys(src).filter((k) => Object.getOwnPropertyDescriptor(src, k).enumerable).map(String))")
		if c.spread == "({x: 0, ...src})" {
			want = `["x",` + want[1:]
		} else if c.spread == "({...src, x: 0})" {
			want = want[:len(want)-1] + `,"x"]`
		} else if c.spread == "(({a, ...r}) => r)(src)" {
			want = `["b"]`
		}
		if got != want {
			t.Errorf("%s: keys %s, want %s", c.name, got, want)
		}
	}
}
