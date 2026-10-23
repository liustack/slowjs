// Copyright (c) 2026 The slowjs Authors.
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

// Phases of gcEphemeron, kept in forkRuntime.ephemeron.
const (
	ephemeronCount       = 1 // counting the references to symbol keys
	ephemeronScanSymbols = 2 // live scan, symbol keys reported to the callback
	ephemeronScan        = 3 // live scan without symbol keys
)

// Symbols are not GC list nodes. Only the symbol accounting and live scan
// ask JS_MarkValue to report them. Ordinary marking keeps its original path.
func gcEphemeronSymbol(tls *libc.TLS, rt, p, mark uintptr) {
	if phase := forkOf(rt).ephemeron; phase == ephemeronCount || phase == ephemeronScanSymbols {
		(*(*func(*libc.TLS, uintptr, uintptr))(unsafe.Pointer(&struct{ uintptr }{mark})))(tls, rt, p|1)
	}
}

// gcEphemeronScan is the state of the live scan of gcEphemeron.
type gcEphemeronScan struct {
	tls     *libc.TLS
	rt      uintptr
	mark    uintptr // the ccgo function the scan marks children with
	symbols map[uintptr]*gcEphemeronSymbolState
	records int // entries of all weak maps and sets, to size pending
	// Entries whose key is not known to be reachable yet, indexed by key:
	// pending gives the last of a key's entries in wait, and each entry
	// links to the one before of the same key. Neither holds a Go pointer,
	// so the Go collector does not scan them. A key is reached once at most
	// (an object is scanned once, a symbol is queued once), so nothing is
	// removed from the index.
	pending gcEphemeronIndex
	wait    []gcEphemeronWait
	queue   []uintptr // symbol keys reached and not activated yet
}

// object is the rest of the scan of p, a weak map or set, or an object
// with weak references.
func (st *gcEphemeronScan) object(p uintptr) {
	obj := (*TJSObject)(unsafe.Pointer(p))
	if obj.Fweakref_count != 0 {
		st.activate(p)
	}
	if obj.Fclass_id != EJS_CLASS_WEAKMAP && obj.Fclass_id != EJS_CLASS_WEAKSET {
		return
	}
	s := obj.Fu.Fopaque
	if s == 0 {
		return
	}
	list := s + unsafe.Offsetof(TJSMapState{}.Frecords)
	// Marking a value cannot add entries: wait is only appended to here.
	wait := st.wait
	for el := (*Tlist_head)(unsafe.Pointer(list)).Fnext; el != list; el = (*Tlist_head)(unsafe.Pointer(el)).Fnext {
		mr := el - unsafe.Offsetof(TJSMapRecord{}.Flink)
		rec := (*TJSMapRecord)(unsafe.Pointer(mr))
		key := jsvPtr(rec.Fkey)
		var live bool
		if jsvTag(rec.Fkey) == EJS_TAG_SYMBOL {
			// Every symbol key of a weak collection is in symbols.
			live = st.symbols[key].live
		} else {
			live = gcEphemeronCount(st.tls, key) > 0
		}
		if live {
			XJS_MarkValue(st.tls, st.rt, rec.Fvalue, st.mark)
			continue
		}
		if wait == nil {
			st.pending.init(st.records)
			wait = make([]gcEphemeronWait, 0, st.records)
		}
		slot := st.pending.find(key)
		next := int32(-1)
		if slot.key != 0 {
			next = slot.last
		} else {
			slot.key = key
			st.pending.n++
		}
		slot.last = int32(len(wait))
		wait = append(wait, gcEphemeronWait{record: mr, state: s, next: next})
	}
	st.wait = wait
}

// activate marks the values of the entries waiting for key.
func (st *gcEphemeronScan) activate(key uintptr) {
	if st.pending.n == 0 {
		return
	}
	slot := st.pending.find(key)
	if slot.key == 0 {
		return
	}
	wait := st.wait
	for i := slot.last; i >= 0; {
		w := &wait[i]
		w.done = true
		i = w.next
		XJS_MarkValue(st.tls, st.rt, (*TJSMapRecord)(unsafe.Pointer(w.record)).Fvalue, st.mark)
	}
}

// symbol notes that the scan reached p, a symbol.
func (st *gcEphemeronScan) symbol(p uintptr) {
	if sym := st.symbols[p]; sym != nil && !sym.live {
		sym.live = true
		st.queue = append(st.queue, p)
	}
}

// drain activates the entries of the symbol keys reached.
func (st *gcEphemeronScan) drain() {
	for len(st.queue) != 0 {
		key := st.queue[len(st.queue)-1]
		st.queue = st.queue[:len(st.queue)-1]
		st.activate(key)
	}
}

// gcEphemeronIndex finds the waiting entries of a key: an open addressed
// table with linear probing, keyed by the address of the key (never 0).
type gcEphemeronIndex struct {
	slots []gcEphemeronSlot
	shift uint
	n     int
}

type gcEphemeronSlot struct {
	key  uintptr
	last int32 // the last entry of the key in wait
}

// init sizes the table for up to n keys, at most half full.
func (x *gcEphemeronIndex) init(n int) {
	bits := uint(4)
	for 1<<bits < 2*n {
		bits++
	}
	x.slots = make([]gcEphemeronSlot, 1<<bits)
	x.shift = 64 - bits
}

// find returns the slot of key, or the empty slot where it would go.
func (x *gcEphemeronIndex) find(key uintptr) *gcEphemeronSlot {
	mask := uintptr(len(x.slots) - 1)
	i := uintptr(uint64(key)*0x9e3779b97f4a7c15>>x.shift) & mask
	for {
		s := &x.slots[i]
		if s.key == key || s.key == 0 {
			return s
		}
		i = (i + 1) & mask
	}
}

// gcEphemeronWait is a weak map entry whose value waits for its key.
type gcEphemeronWait struct {
	record uintptr // the JSMapRecord
	state  uintptr // the JSMapState of its map
	next   int32   // the entry before of the same key, or -1
	done   bool    // the key was reached and the value marked
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
	if forkOf(rt).ephemeron < ephemeronScanSymbols {
		return false
	}
	s := (*TJSObject)(unsafe.Pointer(jsvPtr(val))).Fu.Fopaque
	return s != 0 && (*TJSMapState)(unsafe.Pointer(s)).Fis_weak != 0
}

func gcEphemeronCount(tls *libc.TLS, p uintptr) int32 {
	return (*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(tls, p))).Fref_count
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
		_gc_remove_weak_objects(tls, rt)
		_gc_decref(tls, rt)
		_gc_scan(tls, rt)
		_gc_free_cycles(tls, rt)
		_gc_remove_weak_objects(tls, rt)
		return true
	}
	// The first weak pass leaves the weak maps and sets alone. An entry
	// whose key is a shell, with weak references only, is pending like any
	// key the scan does not reach, and is removed with them below.
	gcEphemeronObservers(tls, rt)
	head := rt + unsafe.Offsetof(TJSRuntime{}.Fgc_obj_list)
	deadHead := rt + unsafe.Offsetof(TJSRuntime{}.Ftmp_obj_list)
	// Keep function values in pinned heap cells. Their ccgo uintptr form
	// must remain valid if a marking call grows the goroutine stack.
	var callbacks runtime.Pinner
	defer callbacks.Unpin()
	symbols := make(map[uintptr]*gcEphemeronSymbolState)
	records := 0
	for el := (*Tlist_head)(unsafe.Pointer(weakHead)).Fnext; el != weakHead; el = (*Tlist_head)(unsafe.Pointer(el)).Fnext {
		if (*TJSWeakRefHeader)(unsafe.Pointer(el)).Fweakref_type != EJS_WEAKREF_TYPE_MAP {
			continue
		}
		state := el - unsafe.Offsetof(TJSMapState{}.Fweakref_header)
		records += int((*TJSMapState)(unsafe.Pointer(state)).Frecord_count)
		list := state + unsafe.Offsetof(TJSMapState{}.Frecords)
		for r := (*Tlist_head)(unsafe.Pointer(list)).Fnext; r != list; r = (*Tlist_head)(unsafe.Pointer(r)).Fnext {
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
		forkOf(rt).ephemeron = ephemeronCount
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
	st := &gcEphemeronScan{tls: tls, rt: rt, symbols: symbols, records: records}
	hasSymbols := len(symbols) != 0
	phase := uint8(ephemeronScan)
	if hasSymbols {
		phase = ephemeronScanSymbols
		scan := func(tls *libc.TLS, rt, p uintptr) {
			if p&1 != 0 {
				st.symbol(p &^ 1)
				return
			}
			_gc_scan_incref_child(tls, rt, p)
		}
		callbacks.Pin(&scan)
		st.mark = __ccgo_fp(scan)
		defer runtime.KeepAlive(scan)
	} else {
		// Without symbol keys the live scan marks as _gc_scan does.
		st.mark = __ccgo_fp(_gc_scan_incref_child)
	}
	mark := st.mark
	forkOf(rt).ephemeron = phase
	// The loop of _gc_scan. Everything else is out of line: only weak maps
	// and sets, and objects with weak references, which may be keys, need
	// more.
	if hasSymbols {
		for p := (*Tlist_head)(unsafe.Pointer(head)).Fnext; p != head; p = (*Tlist_head)(unsafe.Pointer(p)).Fnext {
			hdr := (*byte)(unsafe.Pointer(_js_rc(tls, p) + 3))
			*hdr &^= ephemeronMarkMask
			_mark_children(tls, rt, p, mark)
			gcEphemeronAtoms(tls, rt, p, mark)
			if *hdr&ephemeronTypeMask>>ephemeronTypeShift == EJS_GC_OBJ_TYPE_JS_OBJECT {
				obj := (*TJSObject)(unsafe.Pointer(p))
				if obj.Fweakref_count != 0 || obj.Fclass_id == EJS_CLASS_WEAKMAP || obj.Fclass_id == EJS_CLASS_WEAKSET {
					st.object(p)
				}
			}
			st.drain()
		}
	} else {
		for p := (*Tlist_head)(unsafe.Pointer(head)).Fnext; p != head; p = (*Tlist_head)(unsafe.Pointer(p)).Fnext {
			hdr := (*byte)(unsafe.Pointer(_js_rc(tls, p) + 3))
			*hdr &^= ephemeronMarkMask
			_mark_children(tls, rt, p, mark)
			if *hdr&ephemeronTypeMask>>ephemeronTypeShift == EJS_GC_OBJ_TYPE_JS_OBJECT {
				obj := (*TJSObject)(unsafe.Pointer(p))
				if obj.Fweakref_count != 0 || obj.Fclass_id == EJS_CLASS_WEAKMAP || obj.Fclass_id == EJS_CLASS_WEAKSET {
					st.object(p)
				}
			}
		}
	}
	forkOf(rt).ephemeron = 0
	wait := st.wait
	// Restore every withheld edge before anything can be freed.
	for i := range wait {
		if !wait[i].done {
			XJS_MarkValue(tls, rt, (*TJSMapRecord)(unsafe.Pointer(wait[i].record)).Fvalue, __ccgo_fp(_gc_scan_incref_child2))
		}
	}
	for p := (*Tlist_head)(unsafe.Pointer(deadHead)).Fnext; p != deadHead; p = (*Tlist_head)(unsafe.Pointer(p)).Fnext {
		_mark_children(tls, rt, p, __ccgo_fp(_gc_scan_incref_child2))
	}
	// Dead keys still carry the trial-deletion mark. Removing their entries
	// in the sweep phase cannot recursively free a node on either GC list.
	*(*byte)(unsafe.Pointer(rt + gcPhaseOffset)) = EJS_GC_PHASE_REMOVE_CYCLES
	// The keys of the entries still waiting were never reached: they are
	// the dead keys, whether they carry the trial-deletion mark, are a
	// shell with no strong reference left, or are symbols nothing reached.
	// The entries of each map are together in wait, as each map is
	// scanned once.
	last := uintptr(0)
	for i := range wait {
		if wait[i].done {
			continue
		}
		_map_delete_record_internal(tls, rt, wait[i].state, wait[i].record)
		if wait[i].state != last && last != 0 {
			gcEphemeronRehash(tls, last)
		}
		last = wait[i].state
	}
	if last != 0 {
		gcEphemeronRehash(tls, last)
	}
	_gc_free_cycles(tls, rt)
	// Clear WeakRefs and enqueue registry jobs only after all dead targets
	// have been swept. This also releases the weak target shells in this GC.
	gcEphemeronObservers(tls, rt)
	return true
}

// gcEphemeronObservers is _gc_remove_weak_objects for WeakRefs and
// FinalizationRegistry records only. gcEphemeron removes the entries of
// weak maps and sets with dead keys itself, after its scan. Before the
// scan they wait for it, and after the sweep they are gone.
func gcEphemeronObservers(tls *libc.TLS, rt uintptr) {
	weakHead := rt + unsafe.Offsetof(TJSRuntime{}.Fweakref_list)
	*(*byte)(unsafe.Pointer(rt + gcPhaseOffset)) = EJS_GC_PHASE_DECREF
	for el := (*Tlist_head)(unsafe.Pointer(weakHead)).Fnext; el != weakHead; el = (*Tlist_head)(unsafe.Pointer(el)).Fnext {
		switch (*TJSWeakRefHeader)(unsafe.Pointer(el)).Fweakref_type {
		case EJS_WEAKREF_TYPE_MAP:
		case EJS_WEAKREF_TYPE_WEAKREF:
			_weakref_delete_weakref(tls, rt, el)
		case EJS_WEAKREF_TYPE_FINREC:
			_finrec_delete_weakref(tls, rt, el)
		default:
			libc.Xabort(tls)
		}
	}
	*(*byte)(unsafe.Pointer(rt + gcPhaseOffset)) = EJS_GC_PHASE_NONE
	_free_zero_refcount(tls, rt)
}

// gcEphemeronRehash rebuilds the hash table of a map after its entries
// with dead keys were deleted, once rather than searching each collision
// chain per deletion. A weak map has no iterators or forEach, so none of
// its records is a zombie kept for one.
func gcEphemeronRehash(tls *libc.TLS, s uintptr) {
	state := (*TJSMapState)(unsafe.Pointer(s))
	head := s + unsafe.Offsetof(TJSMapState{}.Frecords)
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
