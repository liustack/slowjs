// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

// The own keys of an object that is not exotic are the keys of its shape.
// JS_GetOwnPropertyNamesInternal lists them for for in, Object.keys and its
// kin, spread, Object.assign and JSON.stringify, in two passes that ask the
// atom table of each key whether it is a string, a symbol or a private
// name, and whether it is an array index, with a call and a TLS frame per
// question. Then JS_CopyDataProperties, behind object spread and the rest of
// a destructuring, and JS_GetOwnPropertyNames2, behind Object.keys, values
// and entries, look each key up again to read its value. internal/fixlib
// makes the three call ownKeys, copyData and ownKeysArray first (see
// fixOwnKeys), which list the keys of such an object in Go, from its shape,
// and read the values of plain data properties straight from the object.
// Each does nothing and reports so for what it does not handle, and the
// engine goes on as before.
//
// The kind of a key depends only on its atom, which stays the same while a
// shape holds it. The key cache of a runtime keeps the kinds of the keys of
// recently listed shapes, so that the next listing of an object of such a
// shape asks the atom table nothing. It keeps them only for shapes in the
// shape hash table, and drops them when the shape leaves the table:
// js_shape_hash_unlink calls ownKeyCacheDrop. A shape in the table does not
// change in place: before a key is added to it it leaves the table
// (add_shape_property), before a key is deleted or the flags of a key
// change it leaves the table for good or is copied
// (js_shape_prepare_update), and it leaves the table before it is freed
// (js_free_shape0). So the kinds the cache has for a shape are those of its
// keys, and the memory of the shape has not gone to another shape. The
// cache holds no shape, atom or object, so it keeps nothing alive and does
// not change when the cycle collector runs: the flags of the keys are read
// from the shape at each listing.

// The kinds of a key, as the key cache keeps them.
const (
	ownKindStr     = EJS_ATOM_KIND_STRING  // a string that is no array index
	ownKindSym     = EJS_ATOM_KIND_SYMBOL  // a symbol
	ownKindPrivate = EJS_ATOM_KIND_PRIVATE // a private name
	ownKindIndex   = 3                     // an array index
	ownKindNone    = 4                     // a deleted key
)

// ownKeyCacheProps is the most keys of a shape the key cache keeps the
// kinds of. Larger shapes are listed without it.
const ownKeyCacheProps = 24

// ownKeyCacheSize is the number of shapes the key cache keeps the kinds of,
// in a table indexed by the address of the shape.
const ownKeyCacheSize = 256

// TownKeyEntry holds the kinds of the keys of the shape sh.
type TownKeyEntry struct {
	sh    uintptr
	kinds [ownKeyCacheProps]uint8
}

// TownKeyCache is the key cache of a runtime.
type TownKeyCache [ownKeyCacheSize]TownKeyEntry

// ownKeySlot returns the entry of the key cache of the runtime at rt for the
// shape sh.
func ownKeySlot(rt, sh uintptr) *TownKeyEntry {
	return &forkOf(rt).keys[(sh>>4^sh>>12)&(ownKeyCacheSize-1)]
}

// ownKeyCacheDrop drops the kinds the key cache of the runtime at rt keeps
// for the shape sh, which leaves the shape hash table.
func ownKeyCacheDrop(rt, sh uintptr) {
	if e := ownKeySlot(rt, sh); e.sh == sh {
		e.sh = 0
	}
}

// ownKeyKind returns the kind of atom, as JS_AtomGetKind and
// JS_AtomIsArrayIndex tell it, with the value of an array index.
func ownKeyKind(tls *libc.TLS, atoms uintptr, atom TJSAtom) (kind uint8, n uint32) {
	if atom == MJS_ATOM_NULL {
		return ownKindNone, 0
	}
	if atom&MJS_ATOM_TAG_INT != 0 {
		return ownKindIndex, atom &^ MJS_ATOM_TAG_INT
	}
	p := *(*uintptr)(unsafe.Pointer(atoms + uintptr(atom)*unsafe.Sizeof(uintptr(0))))
	h := *(*uint32)(unsafe.Pointer(p + atomKeyTypeOff))
	switch h & atomKeyTypeMask >> atomKeyTypeShift {
	case EJS_ATOM_TYPE_STRING:
		// An index that is no tagged int is a string of digits.
		if *(*uint32)(unsafe.Pointer(p))&atomKeyLenMask>>atomKeyLenShift != 0 {
			if c := _string_get(tls, p, 0); c >= '0' && c <= '9' {
				// What JS_AtomIsArrayIndex does for a string.
				var v Tuint32_t
				if _is_num_string_lazy(tls, &v, p) != 0 && v != 0xffffffff {
					return ownKindIndex, v
				}
			}
		}
		return ownKindStr, 0
	case EJS_ATOM_TYPE_GLOBAL_SYMBOL:
		return ownKindSym, 0
	case EJS_ATOM_TYPE_SYMBOL:
		if h&^atomKeyTypeMask == 1<<30-1 {
			return ownKindPrivate, 0
		}
		return ownKindSym, 0
	}
	libc.Xabort(tls)
	return
}

// ownKeyKinds puts the kinds of the keys of the shape sh into kinds, from
// the key cache or from the atoms, and reports whether it did: it does not
// for a shape of more than ownKeyCacheProps keys.
func ownKeyKinds(tls *libc.TLS, ctx, sh uintptr, kinds *[ownKeyCacheProps]uint8) bool {
	s := (*TJSShape)(unsafe.Pointer(sh))
	count := s.Fprop_count
	if count > ownKeyCacheProps {
		return false
	}
	rt := (*TJSContext)(unsafe.Pointer(ctx)).Frt
	var e *TownKeyEntry
	if s.Fis_hashed != 0 {
		e = ownKeySlot(rt, sh)
		if e.sh == sh {
			*kinds = e.kinds
			return true
		}
	}
	atoms := (*TJSRuntime)(unsafe.Pointer(rt)).Fatom_array
	prs := _get_shape_prop(tls, sh)
	for i := range count {
		kinds[i], _ = ownKeyKind(tls, atoms, (*TJSShapeProperty)(unsafe.Pointer(prs+uintptr(i)*unsafe.Sizeof(TJSShapeProperty{}))).Fatom)
	}
	if e != nil {
		e.sh, e.kinds = sh, *kinds
	}
	return true
}

// ownKeyNumMax is the most array indexes ownKeyList sorts. An object with
// more takes the generic path.
const ownKeyNumMax = 16

// ownKeyList is the listing of the own keys of an object that is not
// exotic, in the order JS_GetOwnPropertyNamesInternal gives them: array
// indexes by value, then strings, then symbols, each in the order of the
// shape.
type ownKeyList struct {
	sh     uintptr // the shape of the object
	flags  int32   // the JS_GPN_ flags of the listing
	nums   int32   // the count of array indexes
	n      int32   // the count of keys listed
	plain  bool    // whether every key listed is a plain data property
	cached bool    // whether kinds holds the kinds of the keys of sh
	kinds  [ownKeyCacheProps]uint8
	num    [ownKeyNumMax]struct {
		i int32  // its place in the shape
		v uint32 // its value
	}
}

// prop returns the shape property i of the listing.
func (l *ownKeyList) prop(tls *libc.TLS, i int32) *TJSShapeProperty {
	return (*TJSShapeProperty)(unsafe.Pointer(_get_shape_prop(tls, l.sh) + uintptr(i)*unsafe.Sizeof(TJSShapeProperty{})))
}

// kind returns the kind of the key i, whose shape property is pr.
func (l *ownKeyList) kind(tls *libc.TLS, atoms uintptr, i int32, pr *TJSShapeProperty) uint8 {
	if l.cached {
		return l.kinds[i]
	}
	k, _ := ownKeyKind(tls, atoms, pr.Fatom)
	return k
}

// listed reports whether the key of kind k whose shape property is pr is
// listed with the flags of l.
func (l *ownKeyList) listed(k uint8, pr *TJSShapeProperty) bool {
	if k == ownKindNone {
		return false
	}
	if l.flags&MJS_GPN_ENUM_ONLY != 0 && pr.F__ccgo0&forInFlagsMask>>forInFlagsShift&MJS_PROP_ENUMERABLE == 0 {
		return false
	}
	if k == ownKindIndex {
		k = ownKindStr
	}
	return l.flags>>k&1 != 0
}

// scan lists the own keys of p with flags, and reports whether it could: p
// is not exotic, has no binding of a module namespace among the keys listed
// with JS_GPN_SET_ENUM or JS_GPN_ENUM_ONLY, where
// JS_GetOwnPropertyNamesInternal may throw, and has at most ownKeyNumMax
// array indexes among them.
func (l *ownKeyList) scan(tls *libc.TLS, ctx, p uintptr, flags int32) bool {
	if *(*uint8)(unsafe.Pointer(p + forInExoticOff))&forInExotic != 0 {
		return false
	}
	atoms := (*TJSRuntime)(unsafe.Pointer((*TJSContext)(unsafe.Pointer(ctx)).Frt)).Fatom_array
	l.sh, l.flags, l.nums, l.n, l.plain = (*TJSObject)(unsafe.Pointer(p)).Fshape, flags, 0, 0, true
	l.cached = ownKeyKinds(tls, ctx, l.sh, &l.kinds)
	for i := range (*TJSShape)(unsafe.Pointer(l.sh)).Fprop_count {
		pr := l.prop(tls, i)
		k := l.kind(tls, atoms, i, pr)
		if !l.listed(k, pr) {
			continue
		}
		t := pr.F__ccgo0 & forInFlagsMask >> forInFlagsShift & MJS_PROP_TMASK
		if t == MJS_PROP_VARREF && flags&(MJS_GPN_SET_ENUM|MJS_GPN_ENUM_ONLY) != 0 {
			return false
		}
		// A pending stack is a marker that must not be handed on: values
		// of 'stack' are read through the engine.
		if t != 0 || pr.Fatom == EJS_ATOM_stack {
			l.plain = false
		}
		if k == ownKindIndex {
			if l.nums == ownKeyNumMax {
				return false
			}
			_, v := ownKeyKind(tls, atoms, pr.Fatom)
			// Sorted as they come: indexes are few, and mostly in order.
			j := l.nums
			for j > 0 && l.num[j-1].v > v {
				l.num[j] = l.num[j-1]
				j--
			}
			l.num[j].i, l.num[j].v = i, v
			l.nums++
		}
		l.n++
	}
	return true
}

// each calls f with the place in the shape of each key listed, in order.
func (l *ownKeyList) each(tls *libc.TLS, ctx uintptr, f func(i int32, pr *TJSShapeProperty)) {
	for _, x := range l.num[:l.nums] {
		f(x.i, l.prop(tls, x.i))
	}
	if l.n == l.nums {
		return
	}
	atoms := (*TJSRuntime)(unsafe.Pointer((*TJSContext)(unsafe.Pointer(ctx)).Frt)).Fatom_array
	count := (*TJSShape)(unsafe.Pointer(l.sh)).Fprop_count
	syms := false
	for i := range count {
		pr := l.prop(tls, i)
		switch k := l.kind(tls, atoms, i, pr); k {
		case ownKindStr:
			if l.listed(k, pr) {
				f(i, pr)
			}
		case ownKindSym, ownKindPrivate:
			syms = true
		}
	}
	if !syms {
		return
	}
	for i := range count {
		pr := l.prop(tls, i)
		if k := l.kind(tls, atoms, i, pr); (k == ownKindSym || k == ownKindPrivate) && l.listed(k, pr) {
			f(i, pr)
		}
	}
}

// ownKeys lists the own keys of p as JS_GetOwnPropertyNamesInternal does
// with flags, into *ptab and *plen, and reports whether it did: when it did
// not, nothing was done (see ownKeyList.scan).
func ownKeys(tls *libc.TLS, ctx, ptab, plen, p uintptr, flags int32) (r int32, ok bool) {
	var l ownKeyList
	if !l.scan(tls, ctx, p, flags) {
		return 0, false
	}
	*(*uintptr)(unsafe.Pointer(ptab)) = 0
	*(*Tuint32_t)(unsafe.Pointer(plen)) = 0
	tab := Xjs_malloc(tls, ctx, Tsize_t(unsafe.Sizeof(TJSPropertyEnum{}))*Tsize_t(max(l.n, 1)))
	if tab == 0 {
		return -1, true
	}
	list := unsafe.Slice((*TJSPropertyEnum)(unsafe.Pointer(tab)), l.n)
	k := 0
	l.each(tls, ctx, func(i int32, pr *TJSShapeProperty) {
		list[k].Fatom = XJS_DupAtom(tls, ctx, pr.Fatom)
		list[k].Fis_enumerable = int32(pr.F__ccgo0 & forInFlagsMask >> forInFlagsShift & MJS_PROP_ENUMERABLE >> 2)
		k++
	})
	*(*uintptr)(unsafe.Pointer(ptab)) = tab
	*(*Tuint32_t)(unsafe.Pointer(plen)) = Tuint32_t(l.n)
	return 0, true
}

// copyData copies the own properties of source into target as
// JS_CopyDataProperties does for a spread in an object literal or the rest
// of a destructuring, which define them on target, a new object, and
// reports whether it did: when it did not, nothing was done. It does it
// when every key listed of source is a plain data property, so that no
// getter runs during the copy, which could change source: its values are
// then read from its properties. When target has no property yet, there is
// no key to leave out, and every property of source is enumerable, writable
// and configurable, listed in the order of its shape, target takes the
// shape of source, which defining the keys one by one would have led it to.
func copyData(tls *libc.TLS, ctx uintptr, target, source, excluded TJSValue, setprop TBOOL) (r int32, ok bool) {
	if setprop != 0 || jsvTag(source) != EJS_TAG_OBJECT || jsvTag(target) != EJS_TAG_OBJECT {
		return 0, false
	}
	ps := jsvPtr(source)
	var pexcl uintptr
	if jsvTag(excluded) == EJS_TAG_OBJECT {
		pexcl = jsvPtr(excluded)
	} else if r, ok := copyShape(tls, ctx, jsvPtr(target), ps); ok {
		return r, true
	}
	var l ownKeyList
	if !l.scan(tls, ctx, ps, MJS_GPN_STRING_MASK|MJS_GPN_SYMBOL_MASK|MJS_GPN_ENUM_ONLY) || !l.plain {
		return 0, false
	}
	failed := false
	l.each(tls, ctx, func(i int32, pr *TJSShapeProperty) {
		if failed {
			return
		}
		if pexcl != 0 {
			switch _js_backtrace_own_flags(tls, ctx, 0, pexcl, pr.Fatom) {
			case 0:
			case -1:
				failed = true
				return
			default:
				return
			}
		}
		v := ownKeyValue((*TJSObject)(unsafe.Pointer(ps)).Fprop, i)
		if XJS_DefinePropertyValue(tls, ctx, target, pr.Fatom, v, MJS_PROP_C_W_E) < 0 {
			failed = true
		}
	})
	if failed {
		return -1, true
	}
	return 0, true
}

// copyShape gives pt, an ordinary object with no property, the shape and
// the values of ps, and reports whether it did: when it did not, nothing
// was done. It does it when ps is not exotic, has the prototype of pt, and
// every property of ps is a plain data property, enumerable, writable and
// configurable, with no array index, no private name and no 'stack' key
// among them and no string after a symbol, so that they are listed in the
// order of the shape, and the shape of ps is in the shape hash table, as a
// shape two objects share must be.
func copyShape(tls *libc.TLS, ctx, pt, ps uintptr) (r int32, ok bool) {
	t := (*TJSObject)(unsafe.Pointer(pt))
	if *(*uint8)(unsafe.Pointer(ps + forInExoticOff))&forInExotic != 0 ||
		*(*uint8)(unsafe.Pointer(pt + objlitFlagsOff))&(objlitExtensible|objlitExotic|objlitFastArray) != objlitExtensible ||
		t.Fclass_id != EJS_CLASS_OBJECT {
		return 0, false
	}
	ssh, tsh := (*TJSObject)(unsafe.Pointer(ps)).Fshape, t.Fshape
	s, ts := (*TJSShape)(unsafe.Pointer(ssh)), (*TJSShape)(unsafe.Pointer(tsh))
	if ts.Fprop_count != 0 || ts.Fis_hashed == 0 || s.Fis_hashed == 0 || s.Fproto != ts.Fproto || s.Fprop_count == 0 {
		return 0, false
	}
	var kinds [ownKeyCacheProps]uint8
	if !ownKeyKinds(tls, ctx, ssh, &kinds) {
		return 0, false
	}
	prs := _get_shape_prop(tls, ssh)
	sym := false
	for i := range s.Fprop_count {
		pr := (*TJSShapeProperty)(unsafe.Pointer(prs + uintptr(i)*unsafe.Sizeof(TJSShapeProperty{})))
		if pr.F__ccgo0&forInFlagsMask>>forInFlagsShift != MJS_PROP_C_W_E || pr.Fatom == EJS_ATOM_stack {
			return 0, false
		}
		switch kinds[i] {
		case ownKindStr:
			if sym {
				return 0, false
			}
		case ownKindSym:
			sym = true
		default:
			return 0, false
		}
	}
	// What add_property does when it finds the next shape.
	if s.Fprop_size != ts.Fprop_size {
		prop := Xjs_realloc(tls, ctx, t.Fprop, Tsize_t(unsafe.Sizeof(TJSProperty{}))*Tsize_t(s.Fprop_size))
		if prop == 0 {
			return -1, true
		}
		t.Fprop = prop
	}
	t.Fshape = _js_dup_shape(tls, ssh)
	_js_free_shape(tls, (*TJSContext)(unsafe.Pointer(ctx)).Frt, tsh)
	src := (*TJSObject)(unsafe.Pointer(ps)).Fprop
	for i := range s.Fprop_count {
		*(*TJSValue)(unsafe.Pointer(t.Fprop + uintptr(i)*unsafe.Sizeof(TJSProperty{}))) = ownKeyValue(src, i)
	}
	return 0, true
}

// ownKeysArray makes the array JS_GetOwnPropertyNames2 makes for obj with
// flags and kind: of the own keys of obj, of their values, or of pairs of
// both, and reports whether it did: when it did not, nothing was done. It
// does it for an object ownKeyList can list, and for values only when every
// key listed is a plain data property: then no getter runs between the
// listing and the reads, which could change the object.
func ownKeysArray(tls *libc.TLS, ctx uintptr, obj TJSValue, flags, kind int32) (TJSValue, bool) {
	if jsvTag(obj) != EJS_TAG_OBJECT {
		return TJSValue{}, false
	}
	p := jsvPtr(obj)
	var l ownKeyList
	if !l.scan(tls, ctx, p, flags) || kind != EJS_ITERATOR_KIND_KEY && !l.plain {
		return TJSValue{}, false
	}
	r := XJS_NewArray(tls, ctx)
	if jsvTag(r) == EJS_TAG_EXCEPTION {
		return r, true
	}
	if l.n > 0 && _expand_fast_array(tls, ctx, jsvPtr(r), Tuint32_t(l.n)) < 0 {
		ownKeyFree(tls, ctx, r)
		return jsvException(), true
	}
	props := (*TJSObject)(unsafe.Pointer(p)).Fprop
	failed := false
	l.each(tls, ctx, func(i int32, pr *TJSShapeProperty) {
		if failed {
			return
		}
		var v TJSValue
		switch kind {
		case EJS_ITERATOR_KIND_KEY:
			v = XJS_AtomToValue(tls, ctx, pr.Fatom)
		case EJS_ITERATOR_KIND_VALUE:
			v = ownKeyValue(props, i)
		default:
			v = ownKeyPair(tls, ctx, pr.Fatom, ownKeyValue(props, i))
		}
		if jsvTag(v) == EJS_TAG_EXCEPTION || _add_fast_array_element(tls, ctx, jsvPtr(r), v, 0) < 0 {
			failed = true
		}
	})
	if failed {
		ownKeyFree(tls, ctx, r)
		return jsvException(), true
	}
	return r, true
}

// ownKeyValue returns the value of the plain data property i of the
// properties props, with one more reference.
func ownKeyValue(props uintptr, i int32) TJSValue {
	v := *(*TJSValue)(unsafe.Pointer(props + uintptr(i)*unsafe.Sizeof(TJSProperty{})))
	if t := jsvTag(v); t >= EJS_TAG_FIRST && t < 0 { // JS_VALUE_HAS_REF_COUNT
		*(*int32)(unsafe.Pointer(jsvPtr(v) - 4))++
	}
	return v
}

// ownKeyPair returns the array [key, value] of Object.entries, taking
// value.
func ownKeyPair(tls *libc.TLS, ctx uintptr, atom TJSAtom, value TJSValue) TJSValue {
	a := XJS_NewArray(tls, ctx)
	if jsvTag(a) == EJS_TAG_EXCEPTION {
		ownKeyFree(tls, ctx, value)
		return a
	}
	key := XJS_AtomToValue(tls, ctx, atom)
	if jsvTag(key) == EJS_TAG_EXCEPTION {
		ownKeyFree(tls, ctx, value)
		ownKeyFree(tls, ctx, a)
		return key
	}
	if _add_fast_array_element(tls, ctx, jsvPtr(a), key, 0) < 0 {
		ownKeyFree(tls, ctx, value)
		ownKeyFree(tls, ctx, a)
		return jsvException()
	}
	if _add_fast_array_element(tls, ctx, jsvPtr(a), value, 0) < 0 {
		ownKeyFree(tls, ctx, a)
		return jsvException()
	}
	return a
}

// ownKeyFree drops a reference to v, as JS_FreeValue does.
func ownKeyFree(tls *libc.TLS, ctx uintptr, v TJSValue) {
	if t := jsvTag(v); t >= EJS_TAG_FIRST && t < 0 { // JS_VALUE_HAS_REF_COUNT
		rc := (*int32)(unsafe.Pointer(jsvPtr(v) - 4))
		if *rc--; *rc <= 0 {
			x__JS_FreeValue(tls, ctx, v)
		}
	}
}
