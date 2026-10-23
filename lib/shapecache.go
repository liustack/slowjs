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

// The shape cache of a runtime keeps the shapes of plain objects alive after
// their last object is gone, so that the next object that gets the same keys
// in the same order finds each of its shapes in the shape hash table, instead
// of copying a shape and growing the copy one key at a time. It is off until
// JS_SetShapeCache turns it on. internal/fixlib wires it in (see
// fixShapeCache):
//
//   - add_property, about to add a key to a shape that only its object
//     uses, asks shapeKeepable, and when it says yes copies the shape as it
//     does a shared one. So the shape without the key stays as it is, in the
//     shape hash table, and its count drops to zero when the object moves on
//     to the copy.
//   - add_property calls shapeFound when it finds the shape with the new
//     key in the shape hash table.
//   - js_free_shape0 calls shapeKeep first, which keeps such a shape in the
//     cache, with a count of one, instead of freeing it.
//   - JS_FreeRuntime calls shapeCacheOff first, which lets every kept shape
//     go.
//
// A copy costs an allocation and the copying of every key so far: an
// object that gets keys no other object has, one at a time, pays it for
// each key and finds no shape again. So the copies are paid for by shapes
// found: each shape found earns one copy, up to shapeCreditMax, and
// shapeKeepable says no while there is none. Without copies the shapes in
// between are still kept, only later: an object grows its shape in place
// until it finds the next one kept, and the shape it leaves then is kept.
//
// Only shapes whose prototype is the Object.prototype of the context given
// to JS_SetShapeCache, with at most shapeMaxProps keys, none of them a
// symbol that is not registered, are kept. So a kept shape holds alive
// nothing but that prototype, which its context holds anyway, and the atoms
// of its keys. The cache keeps the last n shapes, in a ring: when the ring
// is full, the oldest shape is let go.
//
// Kept shapes count against the memory limit of the runtime, with the
// atoms they hold: a shape of 32 keys of about ten characters that no
// other object has takes about 3 KiB with its atoms, and the atom table of the runtime stays as large as
// it grew after they are let go. Unlike the front cache, the shape cache
// does not let go of its shapes when an allocation fails: add_property
// reallocates the properties of an object before it takes the shape it
// found, which may be one only the cache holds, so letting the shapes go
// there would free the shape it is about to take.
//
// The state of the cache is in the state of the runtime (see runtime.go), so
// each runtime has its own, used only by the goroutine that runs the
// runtime.

// shapeMaxProps is the number of keys of the largest shape the cache keeps.
const shapeMaxProps = 32

// shapeCreditMax is the number of copies shapes found can pay ahead for.
const shapeCreditMax = 64

// gcPhaseOffset is the offset in JSRuntime of the byte that holds gc_phase,
// right after tmp_obj_list. ccgo names the field by its offset, which
// differs between targets: fixShapeCache checks that it is this one.
const gcPhaseOffset = unsafe.Offsetof(TJSRuntime{}.Ftmp_obj_list) + unsafe.Sizeof(Tlist_head{})

// TshapeCache is the shape cache of a runtime.
type TshapeCache struct {
	ring    uintptr // n pointers to kept shapes or 0, from js_malloc_rt, or 0 while the cache is off
	n       int32   // the length of the ring
	next    int32   // the slot of the ring for the next kept shape
	count   int32   // the number of shapes in the ring
	freeing int32   // nonzero while the cache lets a shape go
	credit  int32   // the copies add_property may make to keep shapes
	proto   uintptr // the Object.prototype whose shapes are kept
}

// shapeCacheOf returns the shape cache of the runtime at rt.
func shapeCacheOf(rt uintptr) *TshapeCache {
	return &forkOf(rt).shape
}

// slot returns the address of slot i of the ring of sc.
func (sc *TshapeCache) slot(i int32) *uintptr {
	return (*uintptr)(unsafe.Pointer(sc.ring + uintptr(i)*unsafe.Sizeof(uintptr(0))))
}

// XJS_SetShapeCache sets the shape cache of the runtime of ctx to keep the
// last n shapes of plain objects of ctx, and lets go of the shapes it kept
// before. A value of 0 turns the cache off. It returns 0, or -1 when there is
// no memory for the ring, which leaves the cache off.
//
// The kept shapes hold the Object.prototype of ctx, and with it ctx. So
// before ctx is freed while the runtime lives on, the cache must be turned
// off, or set to another context: JS_FreeRuntime turns it off.
func XJS_SetShapeCache(tls *libc.TLS, ctx uintptr, n int32) int32 {
	c := (*TJSContext)(unsafe.Pointer(ctx))
	rt := c.Frt
	shapeCacheOff(tls, rt)
	if n <= 0 {
		return 0
	}
	// On a 32-bit target, the size of a ring of 2^30 shapes or more does
	// not fit in a size_t.
	if uint64(n)*uint64(unsafe.Sizeof(uintptr(0))) > uint64(^Tsize_t(0))/2 {
		return -1
	}
	ring := Xjs_mallocz_rt(tls, rt, Tsize_t(n)*Tsize_t(unsafe.Sizeof(uintptr(0))))
	if ring == 0 {
		return -1
	}
	// TJSValue is a struct on 64-bit targets and an integer on 32-bit ones.
	var proto TJSValue
	proto = *(*TJSValue)(unsafe.Pointer(c.Fclass_proto + uintptr(EJS_CLASS_OBJECT)*unsafe.Sizeof(proto)))
	*shapeCacheOf(rt) = TshapeCache{ring: ring, n: n, proto: jsvPtr(proto), credit: shapeCreditMax}
	return 0
}

// shapeCacheOff lets go of the shapes the cache of the runtime at rt keeps,
// and turns it off.
func shapeCacheOff(tls *libc.TLS, rt uintptr) {
	sc := shapeCacheOf(rt)
	if sc.ring == 0 {
		return
	}
	// With the cache off, js_free_shape0 frees the shapes let go here.
	off := *sc
	*sc = TshapeCache{}
	for i := range off.n {
		if sh := *off.slot(i); sh != 0 {
			_js_free_shape(tls, rt, sh)
		}
	}
	Xjs_free_rt(tls, rt, off.ring)
}

// shapeKeepable reports whether add_property, adding a key to the shape sh
// of an object of ctx that only the object uses, should copy sh rather than
// grow it, so that the cache keeps sh once the object moved on to the copy.
// Each copy takes one of the credit shapeFound earns.
func shapeKeepable(tls *libc.TLS, ctx, sh uintptr) bool {
	rt := (*TJSContext)(unsafe.Pointer(ctx)).Frt
	sc := shapeCacheOf(rt)
	s := (*TJSShape)(unsafe.Pointer(sh))
	if sc.ring == 0 || sc.credit <= 0 || s.Fproto != sc.proto || s.Fprop_count >= shapeMaxProps {
		return false
	}
	sc.credit--
	// add_property links the copy into the shape hash table, which only
	// js_new_shape2 grows: grow it as js_new_shape2 does, as sh stays in
	// it.
	r := (*TJSRuntime)(unsafe.Pointer(rt))
	if 2*(r.Fshape_hash_count+1) > r.Fshape_hash_size {
		_resize_shape_hash(tls, rt, r.Fshape_hash_bits+1)
	}
	return true
}

// shapeKeep is called by js_free_shape0 when the count of the shape sh
// dropped to zero. It keeps sh in the cache with a count of one, letting go
// of the oldest kept shape if the ring is full, and reports whether it did.
func shapeKeep(tls *libc.TLS, rt, sh uintptr) bool {
	sc := shapeCacheOf(rt)
	// The cycle collector frees the memory of the shapes of a cycle itself,
	// once their objects are gone: a shape freed while it removes cycles
	// cannot be kept. Objects whose count dropped to zero are freed in the
	// phase DECREF, and their shapes can.
	if sc.ring == 0 || sc.freeing != 0 || *(*uint8)(unsafe.Pointer(rt + gcPhaseOffset)) == EJS_GC_PHASE_REMOVE_CYCLES {
		return false
	}
	s := (*TJSShape)(unsafe.Pointer(sh))
	if s.Fis_hashed == 0 || s.Fproto != sc.proto || s.Fprop_count > shapeMaxProps || shapeHasSymbol(tls, rt, sh) {
		return false
	}
	(*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(tls, sh))).Fref_count = 1
	slot := sc.slot(sc.next)
	if old := *slot; old != 0 {
		sc.freeing = 1
		_js_free_shape(tls, rt, old)
		sc.freeing = 0
		sc.count--
	}
	*slot = sh
	sc.count++
	if sc.next++; sc.next == sc.n {
		sc.next = 0
	}
	return true
}

// shapeHasSymbol reports whether a key of the shape sh is a symbol that is
// not registered (or a private name). Such a symbol can be the target of a
// WeakRef or a FinalizationRegistry: a kept shape would hold it alive after
// the last object with the key is gone. A symbol made by Symbol.for cannot
// be a target, and the symbols of the engine (Symbol.iterator and the
// others) are never freed: shapes with those keys are kept.
//
// __JS_NewAtom links every atom but a symbol into the atom hash table
// through hash_next, and puts a symbol's own index there instead, as
// js_get_atom_index and JS_FreeAtomStruct read it, and JS_ResizeAtomHash
// relinks only the atoms of the table: so an atom is such a symbol when
// its hash_next is its own index.
func shapeHasSymbol(tls *libc.TLS, rt, sh uintptr) bool {
	atoms := (*TJSRuntime)(unsafe.Pointer(rt)).Fatom_array
	prop := _get_shape_prop(tls, sh)
	for i := range (*TJSShape)(unsafe.Pointer(sh)).Fprop_count {
		a := (*TJSShapeProperty)(unsafe.Pointer(prop + uintptr(i)*unsafe.Sizeof(TJSShapeProperty{}))).Fatom
		// A deleted key, an integer key or an atom of the engine.
		if ___JS_AtomIsConst(tls, a) != 0 {
			continue
		}
		p := *(*uintptr)(unsafe.Pointer(atoms + uintptr(a)*unsafe.Sizeof(uintptr(0))))
		if (*TJSAtomStruct)(unsafe.Pointer(p)).Fhash_next == a {
			return true
		}
	}
	return false
}

// shapeFound is called by add_property when it found the next shape of an
// object of ctx in the shape hash table: the shapes of the keys of that
// object are found again, and the copies that keep them pay. It earns one
// copy, up to shapeCreditMax.
func shapeFound(tls *libc.TLS, ctx uintptr) {
	sc := shapeCacheOf((*TJSContext)(unsafe.Pointer(ctx)).Frt)
	if sc.credit < shapeCreditMax {
		sc.credit++
	}
}
