// Copyright (c) 2026 The liustack/quickjs Authors.
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

// A young collection (JS_RunYoungGC) frees the cycles of garbage among the
// GC objects made since the last collection, and leaves the older ones
// alone. internal/fixlib wires it in (see fixYoungGC):
//
//   - add_gc_object puts a new GC object on the young list of the runtime,
//     kept in its state (see runtime.go), which JS_NewRuntime2 sets up.
//   - JS_RunGC, the GC the engine runs by itself, JS_FreeRuntime,
//     JS_ComputeMemoryUsage and JS_DumpMemoryUsage first move the young
//     list to the end of gc_obj_list, so they see every GC object in the
//     order add_gc_object made them, as before.
//   - __JS_FreeValueRT and async_func_free call youngDefer when the count
//     of an object gets to 0 while a young collection frees its garbage.
//
// JS_RunYoungGC runs the trial deletion of JS_RunGC over the young list
// only. It takes away the references young objects hold to each other, so
// what is left of a young object's count are the references from outside
// the young list: old objects, the stack, Go, and anything else that is not
// a GC object. Those objects and what they reach stay, and the rest are
// cycles of garbage that gc_free_cycles frees as JS_RunGC does. The
// survivors become old. An old object is never visited, so its count never
// changes during the trial deletion, and no write barrier is needed.
//
// It is conservative where the full collection is not:
//
//   - A cycle that runs through an old object stays until the next full
//     collection.
//   - A weak map value is a strong child of its map here (the ephemeron
//     phase of gcEphemeron is off), so an entry of a reachable or old weak
//     map keeps its value, and through it a key the value points back to,
//     until the next full collection. A key that nothing else holds is
//     freed: its entry goes in the weak pass at the end.
//
// So what a full collection frees today, it still frees after any number of
// young collections, and a young collection frees nothing a full one would
// keep.
//
// Young objects are told from old ones by the mark bit of their header,
// which is 0 on every GC object outside a collection: the young collection
// sets it on the young list first, clears it on the survivors before it
// frees anything, and the garbage keeps it, as in JS_RunGC.

// Tyoung is the young collection state of a runtime.
type Tyoung struct {
	list Tlist_head // the young GC objects, in the order they were made
	// dead holds the objects outside the garbage whose count got to 0 while
	// the garbage was freed, until they are freed after it.
	dead  Tlist_head
	sweep bool // a young collection is freeing its garbage
}

// youngOffset is the offset of the young list in JSRuntime.
const youngOffset = unsafe.Offsetof(TJSRuntime{}.Ffork) + unsafe.Offsetof(TforkRuntime{}.young) + unsafe.Offsetof(Tyoung{}.list)

// youngHead returns the young list of the runtime at rt, where
// add_gc_object puts a new GC object.
func youngHead(rt uintptr) uintptr { return rt + youngOffset }

func youngListInit(head uintptr) {
	(*Tlist_head)(unsafe.Pointer(head)).Fprev = head
	(*Tlist_head)(unsafe.Pointer(head)).Fnext = head
}

func youngDel(el uintptr) {
	h := (*Tlist_head)(unsafe.Pointer(el))
	(*Tlist_head)(unsafe.Pointer(h.Fprev)).Fnext = h.Fnext
	(*Tlist_head)(unsafe.Pointer(h.Fnext)).Fprev = h.Fprev
	h.Fprev, h.Fnext = 0, 0
}

func youngAddTail(el, head uintptr) {
	hd := (*Tlist_head)(unsafe.Pointer(head))
	prev := hd.Fprev
	(*Tlist_head)(unsafe.Pointer(prev)).Fnext = el
	(*Tlist_head)(unsafe.Pointer(el)).Fprev = prev
	(*Tlist_head)(unsafe.Pointer(el)).Fnext = head
	hd.Fprev = el
}

// youngMove moves the objects of the list at from to the end of the list at
// to, and leaves from empty.
func youngMove(from, to uintptr) {
	first := (*Tlist_head)(unsafe.Pointer(from)).Fnext
	if first == from {
		return
	}
	last := (*Tlist_head)(unsafe.Pointer(from)).Fprev
	toLast := (*Tlist_head)(unsafe.Pointer(to)).Fprev
	(*Tlist_head)(unsafe.Pointer(toLast)).Fnext = first
	(*Tlist_head)(unsafe.Pointer(first)).Fprev = toLast
	(*Tlist_head)(unsafe.Pointer(last)).Fnext = to
	(*Tlist_head)(unsafe.Pointer(to)).Fprev = last
	youngListInit(from)
}

// youngInit sets up the young list of a new runtime, before it makes its
// first GC object.
func youngInit(rt uintptr) {
	youngListInit(youngHead(rt))
}

// youngSplice makes every young object old: it moves the young list to the
// end of gc_obj_list.
func youngSplice(rt uintptr) {
	youngMove(youngHead(rt), rt+unsafe.Offsetof(TJSRuntime{}.Fgc_obj_list))
}

func youngMarked(tls *libc.TLS, p uintptr) bool {
	return *(*byte)(unsafe.Pointer(_js_rc(tls, p) + 3))&ephemeronMarkMask != 0
}

func youngSetMark(tls *libc.TLS, p uintptr) {
	*(*byte)(unsafe.Pointer(_js_rc(tls, p) + 3)) |= ephemeronMarkMask
}

func youngClearMark(tls *libc.TLS, p uintptr) {
	*(*byte)(unsafe.Pointer(_js_rc(tls, p) + 3)) &^= ephemeronMarkMask
}

func youngCount(tls *libc.TLS, p uintptr) *int32 {
	return &(*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(tls, p))).Fref_count
}

// youngDefer is called by __JS_FreeValueRT and async_func_free when the count
// of the object at p gets to 0 in the phase REMOVE_CYCLES, which leaves it
// where it is: in JS_RunGC every such object is garbage, on tmp_obj_list and
// about to be freed. In a young collection it may be an old object that only
// the garbage held. Left on gc_obj_list with a count of 0 and no mark, it
// would be freed by js_weakref_free, when the weak pass removes an entry of
// which it is the key, while still on the list. So it goes on the dead list,
// with the mark that says it is about to be freed, as __JS_FreeValueRT does
// outside a collection, and JS_RunYoungGC frees it after the garbage. The
// garbage itself keeps its mark from the trial deletion, and is left alone.
func youngDefer(tls *libc.TLS, rt, p uintptr) {
	y := &forkOf(rt).young
	if !y.sweep || youngMarked(tls, p) {
		return
	}
	youngDel(p)
	youngAddTail(p, uintptr(unsafe.Pointer(&y.dead)))
	youngSetMark(tls, p)
}

// youngDecref takes away a reference between two young objects, as
// gc_decref_child does.
func youngDecref(tls *libc.TLS, rt, p uintptr) {
	if youngMarked(tls, p) {
		*youngCount(tls, p)--
	}
}

// youngIncref gives back a reference from a young object that stays, and
// brings an object that had no count left back to the young list, as
// gc_scan_incref_child does. It keeps the mark, which tells the young
// objects until the scan is over.
func youngIncref(tls *libc.TLS, rt, p uintptr) {
	if !youngMarked(tls, p) {
		return
	}
	c := youngCount(tls, p)
	*c++
	if *c == 1 {
		youngDel(p)
		youngAddTail(p, youngHead(rt))
	}
}

// youngIncref2 gives back a reference from a young object about to be
// freed, as gc_scan_incref_child2 does.
func youngIncref2(tls *libc.TLS, rt, p uintptr) {
	if youngMarked(tls, p) {
		*youngCount(tls, p)++
	}
}

// XJS_RunYoungGC frees the cycles of garbage among the GC objects of rt made
// since its last collection, young or full, and makes the others old. Like
// JS_RunGC, it then removes the weak references to what is gone and gives the
// front cache back to the arenas. It runs no javascript.
func XJS_RunYoungGC(tls *libc.TLS, rt uintptr) {
	y := &forkOf(rt).young
	head := youngHead(rt)
	tmp := rt + unsafe.Offsetof(TJSRuntime{}.Ftmp_obj_list)
	next := func(el uintptr) uintptr { return (*Tlist_head)(unsafe.Pointer(el)).Fnext }

	for el := next(head); el != head; el = next(el) {
		youngSetMark(tls, el)
	}
	// Trial deletion: take away the references among young objects, and
	// move the objects with no count left to tmp_obj_list.
	for el := next(head); el != head; el = next(el) {
		_mark_children(tls, rt, el, __ccgo_fp(youngDecref))
	}
	youngListInit(tmp)
	for el := next(head); el != head; {
		n := next(el)
		if *youngCount(tls, el) == 0 {
			youngDel(el)
			youngAddTail(el, tmp)
		}
		el = n
	}
	// Scan: what stays gives its references back and brings what it reaches
	// back to the young list, which this loop goes on to.
	for el := next(head); el != head; el = next(el) {
		_mark_children(tls, rt, el, __ccgo_fp(youngIncref))
	}
	for el := next(tmp); el != tmp; el = next(el) {
		_mark_children(tls, rt, el, __ccgo_fp(youngIncref2))
	}
	for el := next(head); el != head; el = next(el) {
		youngClearMark(tls, el)
	}
	youngSplice(rt)

	// Free the garbage. An object outside it whose count gets to 0 waits on
	// the dead list (see youngDefer), and goes afterwards, in the phase
	// DECREF, as it would outside a collection.
	dead := uintptr(unsafe.Pointer(&y.dead))
	youngListInit(dead)
	y.sweep = true
	_gc_free_cycles(tls, rt)
	y.sweep = false
	youngMove(dead, rt+unsafe.Offsetof(TJSRuntime{}.Fgc_zero_ref_count_list))
	_free_zero_refcount(tls, rt)

	// Remove the entries of weak maps and the WeakRefs whose target is gone,
	// and queue the FinalizationRegistry jobs, as JS_RunGC does before its
	// trial deletion. This also frees the shells of what the garbage left
	// with weak references.
	_gc_remove_weak_objects(tls, rt)
	frontDrain(tls, rt)
}
