// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs

import (
	"runtime"
	"unsafe"

	"modernc.org/libc"
)

// Symbols are not GC list nodes. Only the symbol accounting and live scan
// ask JS_MarkValue to report them. Ordinary marking keeps its original path.
func gcEphemeronSymbol(tls *libc.TLS, rt, p, mark uintptr) {
	if forkOf(rt).ephemeron != 0 {
		(*(*func(*libc.TLS, uintptr, uintptr))(unsafe.Pointer(&struct{ uintptr }{mark})))(tls, rt, p|1)
	}
}

type gcEphemeronSymbolState struct {
	refs int32
	live bool
}

// A shape owns one reference per property name, independently of how many
// objects share it. Other symbol-valued fields use JS_MarkValue already.
func gcEphemeronAtoms(tls *libc.TLS, rt, p, mark uintptr) {
	if *(*byte)(unsafe.Pointer(_js_rc(tls, p) + 3))&ephemeronTypeMask>>ephemeronTypeShift != EJS_GC_OBJ_TYPE_SHAPE {
		return
	}
	sh := (*TJSShape)(unsafe.Pointer(p))
	prs := _get_shape_prop(tls, p)
	for i := int32(0); i < sh.Fprop_count; i++ {
		atom := (*TJSShapeProperty)(unsafe.Pointer(prs + uintptr(i)*unsafe.Sizeof(TJSShapeProperty{}))).Fatom
		if atom == 0 || atom&0x80000000 != 0 {
			continue
		}
		ptr := *(*uintptr)(unsafe.Pointer((*TJSRuntime)(unsafe.Pointer(rt)).Fatom_array + uintptr(atom)*unsafe.Sizeof(uintptr(0))))
		// The callback ignores atoms that are not a key in a weak collection.
		(*(*func(*libc.TLS, uintptr, uintptr))(unsafe.Pointer(&struct{ uintptr }{mark})))(tls, rt, ptr|1)
	}
}

func gcEphemeronMap(tls *libc.TLS, rt uintptr, val TJSValue, mark uintptr) bool {
	if forkOf(rt).ephemeron != 2 {
		return false
	}
	s := (*TJSObject)(unsafe.Pointer(jsvPtr(val))).Fu.Fopaque
	return s != 0 && (*TJSMapState)(unsafe.Pointer(s)).Fis_weak != 0
}

func gcEphemeronCount(tls *libc.TLS, p uintptr) int32 {
	return (*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(tls, p))).Fref_count
}
func gcEphemeronDead(tls *libc.TLS, key TJSValue, symbols map[uintptr]*gcEphemeronSymbolState) bool {
	if sym := symbols[jsvPtr(key)]; sym != nil {
		return !sym.live
	}
	if jsvTag(key) == EJS_TAG_OBJECT {
		// The preliminary weak pass can drop a target after an earlier map
		// was visited. Such a dead shell is outside the GC lists and has no
		// trial-deletion mark, so its zero strong count matters as well.
		return gcEphemeronCount(tls, jsvPtr(key)) == 0 || *(*byte)(unsafe.Pointer(_js_rc(tls, jsvPtr(key)) + 3))&ephemeronMarkMask != 0
	}
	return _js_weakref_is_live(tls, key) == 0
}

// gcEphemeron runs only for a runtime with weak observers. The ordinary
// collector stays unchanged for runtimes without them. A pending entry is
// indexed by its key, and is activated once when that key becomes reachable.
// Entries are added only from reachable maps, so a dead map never retains a
// value merely because its key lives elsewhere.
func gcEphemeron(tls *libc.TLS, rt uintptr, remove TBOOL) bool {
	weakHead := rt + unsafe.Offsetof(TJSRuntime{}.Fweakref_list)
	if remove == 0 || (*Tlist_head)(unsafe.Pointer(weakHead)).Fnext == weakHead {
		return false
	}
	_gc_remove_weak_objects(tls, rt)
	hasEntries := false
	for el := (*Tlist_head)(unsafe.Pointer(weakHead)).Fnext; el != weakHead; el = (*Tlist_head)(unsafe.Pointer(el)).Fnext {
		if (*TJSWeakRefHeader)(unsafe.Pointer(el)).Fweakref_type == EJS_WEAKREF_TYPE_MAP {
			state := (*TJSMapState)(unsafe.Pointer(el - unsafe.Offsetof(TJSMapState{}.Fweakref_header)))
			if state.Frecord_count != 0 {
				hasEntries = true
				break
			}
		}
	}
	if !hasEntries {
		_gc_decref(tls, rt)
		_gc_scan(tls, rt)
		_gc_free_cycles(tls, rt)
		_gc_remove_weak_objects(tls, rt)
		return true
	}
	head := rt + unsafe.Offsetof(TJSRuntime{}.Fgc_obj_list)
	deadHead := rt + unsafe.Offsetof(TJSRuntime{}.Ftmp_obj_list)
	// Keep function values in pinned heap cells. Their ccgo uintptr form
	// must remain valid if a marking call grows the goroutine stack.
	var callbacks runtime.Pinner
	defer callbacks.Unpin()
	pending := make(map[uintptr][]uintptr)
	var maps []uintptr
	symbols := make(map[uintptr]*gcEphemeronSymbolState)
	for el := (*Tlist_head)(unsafe.Pointer(weakHead)).Fnext; el != weakHead; el = (*Tlist_head)(unsafe.Pointer(el)).Fnext {
		if (*TJSWeakRefHeader)(unsafe.Pointer(el)).Fweakref_type != EJS_WEAKREF_TYPE_MAP {
			continue
		}
		state := el - unsafe.Offsetof(TJSMapState{}.Fweakref_header)
		records := state + unsafe.Offsetof(TJSMapState{}.Frecords)
		for r := (*Tlist_head)(unsafe.Pointer(records)).Fnext; r != records; r = (*Tlist_head)(unsafe.Pointer(r)).Fnext {
			rec := (*TJSMapRecord)(unsafe.Pointer(r - unsafe.Offsetof(TJSMapRecord{}.Flink)))
			if jsvTag(rec.Fkey) == EJS_TAG_SYMBOL {
				p := jsvPtr(rec.Fkey)
				symbols[p] = &gcEphemeronSymbolState{refs: gcEphemeronCount(tls, p)}
			}
		}
	}
	if len(symbols) != 0 {
		count := func(_ *libc.TLS, _, p uintptr) {
			if sym := symbols[p&^1]; sym != nil {
				sym.refs--
			}
		}
		callbacks.Pin(&count)
		fp := __ccgo_fp(count)
		forkOf(rt).ephemeron = 1
		for p := (*Tlist_head)(unsafe.Pointer(head)).Fnext; p != head; p = (*Tlist_head)(unsafe.Pointer(p)).Fnext {
			_mark_children(tls, rt, p, fp)
			gcEphemeronAtoms(tls, rt, p, fp)
		}
		forkOf(rt).ephemeron = 0
		runtime.KeepAlive(count)
		for _, sym := range symbols {
			sym.live = sym.refs > 0
		}
	}
	_gc_decref(tls, rt)
	var symbolQueue []uintptr
	scan := func(tls *libc.TLS, rt, p uintptr) {
		if p&1 != 0 {
			sym := symbols[p&^1]
			if sym == nil {
				return
			}
			if !sym.live {
				sym.live = true
				symbolQueue = append(symbolQueue, p&^1)
			}
			return
		}
		_gc_scan_incref_child(tls, rt, p)
	}
	callbacks.Pin(&scan)
	mark := __ccgo_fp(scan)
	forkOf(rt).ephemeron = 2
	scanObject := func(p uintptr) {
		*(*byte)(unsafe.Pointer(_js_rc(tls, p) + 3)) &^= ephemeronMarkMask
		_mark_children(tls, rt, p, mark)
		if len(symbols) != 0 {
			gcEphemeronAtoms(tls, rt, p, mark)
		}
		if records := pending[p]; len(records) != 0 {
			for _, mr := range records {
				XJS_MarkValue(tls, rt, (*TJSMapRecord)(unsafe.Pointer(mr)).Fvalue, mark)
			}
			delete(pending, p)
		}
		if *(*byte)(unsafe.Pointer(_js_rc(tls, p) + 3))&ephemeronTypeMask>>ephemeronTypeShift != EJS_GC_OBJ_TYPE_JS_OBJECT {
			return
		}
		obj := (*TJSObject)(unsafe.Pointer(p))
		if obj.Fclass_id != EJS_CLASS_WEAKMAP && obj.Fclass_id != EJS_CLASS_WEAKSET {
			return
		}
		s := obj.Fu.Fopaque
		if s == 0 {
			return
		}
		maps = append(maps, s)
		records := s + unsafe.Offsetof(TJSMapState{}.Frecords)
		for el := (*Tlist_head)(unsafe.Pointer(records)).Fnext; el != records; el = (*Tlist_head)(unsafe.Pointer(el)).Fnext {
			mr := el - unsafe.Offsetof(TJSMapRecord{}.Flink)
			rec := (*TJSMapRecord)(unsafe.Pointer(mr))
			key := jsvPtr(rec.Fkey)
			live := gcEphemeronCount(tls, key) > 0
			if sym := symbols[key]; sym != nil {
				live = sym.live
			}
			if live {
				XJS_MarkValue(tls, rt, rec.Fvalue, mark)
			} else {
				pending[key] = append(pending[key], mr)
			}
		}
	}
	for p := (*Tlist_head)(unsafe.Pointer(head)).Fnext; p != head; p = (*Tlist_head)(unsafe.Pointer(p)).Fnext {
		scanObject(p)
		for len(symbolQueue) != 0 {
			key := symbolQueue[len(symbolQueue)-1]
			symbolQueue = symbolQueue[:len(symbolQueue)-1]
			entries := pending[key]
			delete(pending, key)
			for _, mr := range entries {
				XJS_MarkValue(tls, rt, (*TJSMapRecord)(unsafe.Pointer(mr)).Fvalue, mark)
			}
		}
	}
	forkOf(rt).ephemeron = 0
	runtime.KeepAlive(scan)
	// Restore every withheld edge before anything can be freed.
	for _, records := range pending {
		for _, mr := range records {
			XJS_MarkValue(tls, rt, (*TJSMapRecord)(unsafe.Pointer(mr)).Fvalue, __ccgo_fp(_gc_scan_incref_child2))
		}
	}
	for p := (*Tlist_head)(unsafe.Pointer(deadHead)).Fnext; p != deadHead; p = (*Tlist_head)(unsafe.Pointer(p)).Fnext {
		_mark_children(tls, rt, p, __ccgo_fp(_gc_scan_incref_child2))
	}
	// Dead keys still carry the trial-deletion mark. Removing their entries
	// in the sweep phase cannot recursively free a node on either GC list.
	*(*byte)(unsafe.Pointer(rt + gcPhaseOffset)) = EJS_GC_PHASE_REMOVE_CYCLES
	for _, s := range maps {
		gcEphemeronPrune(tls, rt, s, symbols)
	}
	_gc_free_cycles(tls, rt)
	// Clear WeakRefs and enqueue registry jobs only after all dead targets
	// have been swept. This also releases the weak target shells in this GC.
	_gc_remove_weak_objects(tls, rt)
	return true
}

func gcEphemeronPrune(tls *libc.TLS, rt, s uintptr, symbols map[uintptr]*gcEphemeronSymbolState) {
	state := (*TJSMapState)(unsafe.Pointer(s))
	head := s + unsafe.Offsetof(TJSMapState{}.Frecords)
	changed := false
	for el := (*Tlist_head)(unsafe.Pointer(head)).Fnext; el != head; {
		next := (*Tlist_head)(unsafe.Pointer(el)).Fnext
		mr := el - unsafe.Offsetof(TJSMapRecord{}.Flink)
		if gcEphemeronDead(tls, (*TJSMapRecord)(unsafe.Pointer(mr)).Fkey, symbols) {
			_map_delete_record_internal(tls, rt, s, mr)
			changed = true
		}
		el = next
	}
	if !changed {
		return
	}
	// Rebuild once, rather than searching each collision chain per deletion.
	table := unsafe.Slice((*uintptr)(unsafe.Pointer(state.Fhash_table)), int(state.Fhash_size))
	clear(table)
	for el := (*Tlist_head)(unsafe.Pointer(head)).Fnext; el != head; el = (*Tlist_head)(unsafe.Pointer(el)).Fnext {
		mr := el - unsafe.Offsetof(TJSMapRecord{}.Flink)
		rec := (*TJSMapRecord)(unsafe.Pointer(mr))
		h := _map_hash_key(tls, rec.Fkey, state.Fhash_bits)
		rec.Fhash_next = table[h]
		table[h] = mr
	}
}
