// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/quickjs/lib"

// Lazy error stacks.
//
// The engine used to format the 'stack' property of an error when it made
// the error, decoding the line number of every frame. This file is the Go
// form of the change in internal/patch/quickjs.diff that makes it lazy:
// making an error only records each frame (the function name, the bytecode
// and the pc), and the text is formatted the first time it is needed, with
// the same bytes.
//
// 'stack' is an ordinary writable, configurable data property, with the
// same shape as an error whose stack was formatted at once, so an error
// never gets a shape of its own and keeps no shape. While the stack is
// pending its value is a marker: an object of a class private to this file,
// JS_CLASS not among the engine's, holding the recorded frames. No path
// that hands a property value on may hand the marker to JavaScript, so
// those paths format the stack first:
//
//   - A read (get, getOwnPropertyDescriptor, a native print through
//     get_prop_string) formats the text, stores it in place of the marker,
//     and reads that. If formatting fails for lack of memory, a get or a
//     descriptor read throws and the stack stays pending, so a later read
//     can try again. A native print reports no stack and restores the
//     exception state it found, so the stack step of a print never leaves
//     an exception (the message step can, as on main).
//   - Redefining a non-writable, non-configurable 'stack' compares the new
//     value with the text, so it formats the stack first.
//   - Operations on the error that look only at the attributes of 'stack'
//     (Object.keys, freeze, isFrozen, __lookupGetter__, the has, delete and
//     ownKeys checks of a proxy whose target is the error, ...) leave it
//     pending. Any other lookup through a proxy formats it, as a lookup of
//     the descriptor does.
//   - Assigning, defining or deleting 'stack' frees the marker like any
//     value, which drops the pending stack without formatting it. Assigning
//     through an object that has the error as its prototype makes an own
//     property on that object and does not touch the marker.
//   - A raw dump (JSPrintValueOptions.raw_dump) prints a placeholder and
//     does not format.
//
// The marker's class frees the record and marks the bytecode it holds, so
// the generic free and mark paths need no change. internal/fixlib wires the
// rest into every ccgo_*.go file: _build_backtrace calls _js_backtrace_build,
// JS_GetPropertyInternal, JS_GetOwnPropertyInternal and the SameValue test
// of JS_DefineProperty call _js_backtrace_settle, every inline property
// read of the interpreter sends a 'stack' holding an object to the slow
// path, the callers of JS_GetOwnPropertyInternal that only read attributes
// call _js_backtrace_own_flags, _get_prop_string calls
// _js_backtrace_prop_string, js_print_error skips the stack of a raw dump,
// js_print_object prints a placeholder for a marker, and
// JS_ComputeMemoryUsage counts the record.
// It also audits every use of a property slot in the engine (see slots.go
// and slotsites.go there). fixlib also adds _js_backtrace_has_debug,
// _js_backtrace_uncatchable, _js_backtrace_prop_flags and
// _js_backtrace_raw_dump, copied from code ccgo generated, as the bit fields
// they touch are laid out differently on some targets.

import (
	"strconv"
	"unsafe"

	"modernc.org/libc"
)

// backtraceFlags are the property flags of 'stack', the same as the string
// build_backtrace stores: writable and configurable.
const backtraceFlags = MJS_PROP_WRITABLE | MJS_PROP_CONFIGURABLE

// backtraceFrame is one recorded frame of a pending stack.
type backtraceFrame struct {
	name uintptr // the JSString of the function name, 0 for <anonymous>
	b    uintptr // the JSFunctionBytecode, 0 for a native function
	pc   uint32  // the pc_value build_backtrace gives to find_line_num
}

// backtrace is the opaque of a marker object: the recorded frames.
type backtrace struct {
	count  int32
	frames [0]backtraceFrame
}

func backtraceSize(n int32) uintptr {
	var hdr backtrace
	return unsafe.Offsetof(hdr.frames) + uintptr(n)*unsafe.Sizeof(backtraceFrame{})
}

func backtraceFrames(bt uintptr) []backtraceFrame {
	p := (*backtrace)(unsafe.Pointer(bt))
	return unsafe.Slice((*backtraceFrame)(unsafe.Add(unsafe.Pointer(bt), unsafe.Offsetof(p.frames))), p.count)
}

func jsUndefined() TJSValue  { return jsvMkPtr(EJS_TAG_UNDEFINED, 0) }
func jsvException() TJSValue { return jsvMkPtr(EJS_TAG_EXCEPTION, 0) }

const (
	jsModeBacktraceBarrier        = 1 << 3 // JS_MODE_BACKTRACE_BARRIER
	jsBacktraceFlagSkipFirstLevel = 1 << 0 // JS_BACKTRACE_FLAG_SKIP_FIRST_LEVEL
)

// backtraceClassID is the class of a marker. It is one number for the whole
// process, set before any runtime exists so that reading it never races
// with setting it, and registered in each runtime that records a pending
// stack.
var backtraceClassID TJSClassID

func init() {
	tls := libc.NewTLS()
	XJS_NewClassID(tls, uintptr(unsafe.Pointer(&backtraceClassID)))
	tls.Close()
}

// backtracePlaceholder is printed in place of a marker in a raw dump.
var backtracePlaceholder = [...]byte{'[', 's', 't', 'a', 'c', 'k', ']'}

// backtraceEnsureClass registers the marker class in the runtime of ctx and
// returns its id, or 0 if there is no memory to register it.
func backtraceEnsureClass(tls *libc.TLS, ctx uintptr) TJSClassID {
	rt := (*TJSContext)(unsafe.Pointer(ctx)).Frt
	r := (*TJSRuntime)(unsafe.Pointer(rt))
	if backtraceClassID < uint32(r.Fclass_count) {
		cl := r.Fclass_array + uintptr(backtraceClassID)*unsafe.Sizeof(TJSClass{})
		if (*TJSClass)(unsafe.Pointer(cl)).Fclass_id == backtraceClassID {
			return backtraceClassID
		}
	}
	// JS_NewClass copies the name into an atom, so a C string that lives for
	// the call is enough. Its definition goes in C memory, as JS_NewClass
	// reads it through C pointer arithmetic, and starts zeroed, as TLS
	// memory holds whatever was there before: the class has no call or
	// exotic handler.
	name, err := libc.CString("QuickJSBacktrace")
	if err != nil {
		return 0
	}
	defer libc.Xfree(tls, name)
	defSize := int(unsafe.Sizeof(TJSClassDef{}))
	def := tls.Alloc(defSize)
	d := (*TJSClassDef)(unsafe.Pointer(def))
	*d = TJSClassDef{}
	d.Fclass_name = name
	d.Ffinalizer = __ccgo_fp(_js_backtrace_finalizer)
	d.Fgc_mark = __ccgo_fp(_js_backtrace_gc_mark)
	failed := XJS_NewClass(tls, rt, backtraceClassID, def) != 0
	tls.Free(defSize)
	if failed {
		return 0
	}
	return backtraceClassID
}

// backtraceMarker reports whether val is a pending-stack marker.
func backtraceMarker(val TJSValue) bool {
	return jsvTag(val) == EJS_TAG_OBJECT &&
		uint32((*TJSObject)(unsafe.Pointer(jsvPtr(val))).Fclass_id) == backtraceClassID
}

// backtracePending reports whether the property at prs and pr holds a
// pending stack: a plain data property whose value is a marker. The kind
// is tested first, as only a plain data property holds a JSValue: an
// accessor holds two object pointers, a global variable a pointer to its
// reference followed by whatever bytes were there before.
func backtracePending(tls *libc.TLS, prs uintptr, pr uintptr) bool {
	return _js_backtrace_prop_flags(tls, prs)&MJS_PROP_TMASK == MJS_PROP_NORMAL &&
		backtraceMarker(*(*TJSValue)(unsafe.Pointer(pr)))
}

// backtraceCount returns the number of frames build_backtrace prints, and
// whether top-level code (a script, an eval or a module body) is among them.
func backtraceCount(tls *libc.TLS, rt uintptr, backtrace_flags int32) (n int32, topLevel bool) {
	flags := backtrace_flags
	for sf := (*TJSRuntime)(unsafe.Pointer(rt)).Fcurrent_stack_frame; sf != 0; sf = (*TJSStackFrame)(unsafe.Pointer(sf)).Fprev_frame {
		if (*TJSStackFrame)(unsafe.Pointer(sf)).Fjs_mode&jsModeBacktraceBarrier != 0 {
			break
		}
		if flags&jsBacktraceFlagSkipFirstLevel != 0 {
			flags &^= jsBacktraceFlagSkipFirstLevel
			continue
		}
		if b := _JS_GetFunctionBytecode(tls, (*TJSStackFrame)(unsafe.Pointer(sf)).Fcur_func); b != 0 && (*TJSFunctionBytecode)(unsafe.Pointer(b)).Ffunc_name == uint32(EJS_ATOM__eval_) {
			topLevel = true
		}
		n++
	}
	return n, topLevel
}

// backtraceRecord fills frames with the frames backtraceCount counted. With
// hold set, each frame holds its bytecode, so the record outlives the call.
func backtraceRecord(tls *libc.TLS, ctx uintptr, frames []backtraceFrame, backtrace_flags int32, hold bool) {
	ppr := tls.Alloc(int(unsafe.Sizeof(uintptr(0))))
	flags := backtrace_flags
	sf := (*TJSRuntime)(unsafe.Pointer((*TJSContext)(unsafe.Pointer(ctx)).Frt)).Fcurrent_stack_frame
	for i := 0; i < len(frames); sf = (*TJSStackFrame)(unsafe.Pointer(sf)).Fprev_frame {
		if flags&jsBacktraceFlagSkipFirstLevel != 0 {
			flags &^= jsBacktraceFlagSkipFirstLevel
			continue
		}
		f := &frames[i]
		cur := (*TJSStackFrame)(unsafe.Pointer(sf))
		f.name = backtraceFuncName(tls, ctx, ppr, cur.Fcur_func)
		if b := _JS_GetFunctionBytecode(tls, cur.Fcur_func); b != 0 {
			if hold {
				XDupValue(tls, ctx, jsvMkPtr(EJS_TAG_FUNCTION_BYTECODE, b))
			}
			f.b = b
			f.pc = uint32(cur.Fcur_pc - (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fbyte_code_buf - 1)
		} else {
			f.b = 0
			f.pc = 0
		}
		i++
	}
	tls.Free(int(unsafe.Sizeof(uintptr(0))))
}

// backtraceCanAdd reports whether JS_DefineProperty would add 'stack' to
// error_obj with add_property alone: error_obj is an Error, which has no
// exotic behavior, it has no 'stack' yet, and it takes new properties.
func backtraceCanAdd(tls *libc.TLS, ctx uintptr, error_obj TJSValue) bool {
	p := jsvPtr(error_obj)
	return (*TJSObject)(unsafe.Pointer(p)).Fclass_id == EJS_CLASS_ERROR &&
		_find_own_property1(tls, p, uint32(EJS_ATOM_stack)) == 0 &&
		XJS_IsExtensible(tls, ctx, error_obj) != 0
}

// _js_backtrace_build is build_backtrace for a stack without a file name. It
// records the stack of error_obj as a marker, to be formatted when 'stack'
// is read. It formats the stack now when top-level code is on it, which runs
// once, so the error does not keep its bytecode, and the functions it made,
// alive. Libraries keep errors made when they load. It also formats it now
// when 'stack' cannot simply be added to the error, or when there is no
// memory for the record.
func _js_backtrace_build(tls *libc.TLS, ctx uintptr, error_obj TJSValue, backtrace_flags int32) TBOOL {
	// error_obj may be the current exception, and running out of memory
	// throws a new one, which frees it. Keep it until the stack is in place,
	// and the exception, to put it back: like build_backtrace, this does
	// not throw.
	rt := (*TJSContext)(unsafe.Pointer(ctx)).Frt
	XDupValue(tls, ctx, error_obj)
	defer XFreeValue(tls, ctx, error_obj)
	exception := XDupValue(tls, ctx, (*TJSRuntime)(unsafe.Pointer(rt)).Fcurrent_exception)
	uncatchable := _js_backtrace_uncatchable(tls, ctx)

	ok := true
	n, topLevel := backtraceCount(tls, rt, backtrace_flags)
	if topLevel || !backtraceCanAdd(tls, ctx, error_obj) {
		ok = backtraceNow(tls, ctx, error_obj, backtrace_flags, n)
	} else if !backtraceLazy(tls, ctx, error_obj, backtrace_flags, n) {
		backtraceNow(tls, ctx, error_obj, backtrace_flags, n)
		ok = false
	}

	if ok {
		XFreeValue(tls, ctx, exception)
	} else {
		XFreeValue(tls, ctx, XJS_GetException(tls, ctx))
		XJS_Throw(tls, ctx, exception)
		XJS_SetUncatchableException(tls, ctx, uncatchable)
	}
	return 1
}

// backtraceLazy records the n frames of the stack of error_obj, for which
// backtraceCanAdd holds, as a marker stored in a new 'stack' property. It
// returns false if there is no memory for that, leaving error_obj unchanged.
func backtraceLazy(tls *libc.TLS, ctx uintptr, error_obj TJSValue, backtrace_flags int32, n int32) bool {
	rt := (*TJSContext)(unsafe.Pointer(ctx)).Frt
	id := backtraceEnsureClass(tls, ctx)
	if id == 0 {
		return false
	}
	marker := XJS_NewObjectProtoClass(tls, ctx, jsvMkPtr(EJS_TAG_NULL, 0), id)
	if jsvTag(marker) == EJS_TAG_EXCEPTION {
		return false
	}
	bt := Xjs_malloc_rt(tls, rt, Tsize_t(backtraceSize(n)))
	if bt == 0 {
		XFreeValue(tls, ctx, marker)
		return false
	}
	(*backtrace)(unsafe.Pointer(bt)).count = n
	backtraceRecord(tls, ctx, backtraceFrames(bt), backtrace_flags, true)
	XJS_SetOpaque(tls, marker, bt)

	// add_property gives the object the shape it has with a formatted stack,
	// as build_backtrace does, and the marker is the property value.
	pr := _add_property(tls, ctx, jsvPtr(error_obj), uint32(EJS_ATOM_stack), backtraceFlags)
	if pr == 0 {
		XFreeValue(tls, ctx, marker)
		return false
	}
	*(*TJSValue)(unsafe.Pointer(pr)) = marker
	return true
}

// backtraceNow formats the stack of error_obj, which has n frames, and
// stores it as build_backtrace does. Like build_backtrace, it stores null if
// there is no memory for the text. It returns false if it ran out of memory.
func backtraceNow(tls *libc.TLS, ctx uintptr, error_obj TJSValue, backtrace_flags int32, n int32) bool {
	size := int(backtraceSize(n))
	bt := tls.Alloc(size)
	(*backtrace)(unsafe.Pointer(bt)).count = n
	frames := backtraceFrames(bt)
	backtraceRecord(tls, ctx, frames, backtrace_flags, false)
	str := backtraceFormat(tls, ctx, bt)
	for _, f := range frames {
		if f.name != 0 {
			XFreeValue(tls, ctx, jsvMkPtr(EJS_TAG_STRING, f.name))
		}
	}
	tls.Free(size)
	ok := jsvTag(str) != EJS_TAG_EXCEPTION
	if !ok {
		str = jsvMkPtr(EJS_TAG_NULL, 0)
	}
	return XJS_DefinePropertyValue(tls, ctx, error_obj, uint32(EJS_ATOM_stack), str, backtraceFlags) >= 0 && ok
}

// backtraceFuncName returns the string get_prop_string reads for the name of
// fun, as a new reference, or 0 where get_prop_string returns NULL. It runs
// no JS code. ppr is room for a pointer.
func backtraceFuncName(tls *libc.TLS, ctx uintptr, ppr uintptr, fun TJSValue) uintptr {
	if jsvTag(fun) != EJS_TAG_OBJECT {
		return 0
	}
	p := jsvPtr(fun)
	prs := _find_own_property(tls, ppr, p, uint32(EJS_ATOM_name))
	if prs == 0 {
		if p = (*TJSShape)(unsafe.Pointer((*TJSObject)(unsafe.Pointer(p)).Fshape)).Fproto; p == 0 {
			return 0
		}
		if prs = _find_own_property(tls, ppr, p, uint32(EJS_ATOM_name)); prs == 0 {
			return 0
		}
	}
	if _js_backtrace_prop_flags(tls, prs)&MJS_PROP_TMASK != MJS_PROP_NORMAL {
		return 0
	}
	val := *(*TJSValue)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(ppr))))
	if jsvTag(val) != EJS_TAG_STRING {
		return 0
	}
	return jsvPtr(XDupValue(tls, ctx, val))
}

// backtraceStore replaces the marker at pr, a property of error_obj, with
// the formatted stack, freeing the marker. It returns -1, with the stack
// left pending and an exception set, if there is no memory for the text.
func backtraceStore(tls *libc.TLS, ctx uintptr, pr uintptr) int32 {
	val := *(*TJSValue)(unsafe.Pointer(pr))
	str := backtraceFormat(tls, ctx, XJS_GetOpaque(tls, val, backtraceClassID))
	if jsvTag(str) == EJS_TAG_EXCEPTION {
		return -1
	}
	XFreeValue(tls, ctx, val)
	*(*TJSValue)(unsafe.Pointer(pr)) = str
	return 0
}

// _js_backtrace_object reports whether the property value at pr is an
// object, as a pending stack is. The interpreter's inline reads send a
// 'stack' whose value is an object to the slow path, which formats a pending
// stack. It is small enough for the compiler to inline, so the inline reads
// of other properties cost what they did.
func _js_backtrace_object(pr uintptr) bool {
	return jsvTag(*(*TJSValue)(unsafe.Pointer(pr))) == EJS_TAG_OBJECT
}

// _js_backtrace_settle formats a pending stack at prs and pr, a property of
// p, and stores it, so the caller reads the text. It returns -1, with an exception
// set, if the stack cannot be formatted. It does nothing if pr is not a
// pending stack. JS_GetPropertyInternal and JS_GetOwnPropertyInternal call
// it before they read the value of 'stack', and JS_DefineProperty before it
// compares that value with a new one.
func _js_backtrace_settle(tls *libc.TLS, ctx uintptr, p uintptr, prs uintptr, pr uintptr) int32 {
	if !backtracePending(tls, prs, pr) {
		return 0
	}
	return backtraceStore(tls, ctx, pr)
}

// _js_backtrace_prop_string is the part of get_prop_string for a pending
// stack, which prints an error: it formats the stack and stores it, like a
// read, then returns the text as a C string. If either step runs out of
// memory, it restores the exception state from before the call and reports
// no stack, so printing never leaves an exception. ok is false if pr is not
// a pending stack.
func _js_backtrace_prop_string(tls *libc.TLS, ctx uintptr, p uintptr, prs uintptr, pr uintptr) (s uintptr, ok bool) {
	if !backtracePending(tls, prs, pr) {
		return 0, false
	}
	rt := (*TJSContext)(unsafe.Pointer(ctx)).Frt
	exception := XDupValue(tls, ctx, (*TJSRuntime)(unsafe.Pointer(rt)).Fcurrent_exception)
	uncatchable := _js_backtrace_uncatchable(tls, ctx)
	if backtraceStore(tls, ctx, pr) == 0 {
		if s = XJS_ToCStringLen2(tls, ctx, 0, *(*TJSValue)(unsafe.Pointer(pr)), 0); s != 0 {
			XFreeValue(tls, ctx, exception)
			return s, true
		}
	}
	XFreeValue(tls, ctx, XJS_GetException(tls, ctx))
	XJS_Throw(tls, ctx, exception)
	XJS_SetUncatchableException(tls, ctx, uncatchable)
	return 0, true
}

// _js_backtrace_own_flags is JS_GetOwnPropertyInternal for callers that only
// look at the attributes of the property in desc, never at its value: for a
// pending stack it fills in the attributes and an undefined value without
// formatting the stack, which leaves it pending and cannot run out of
// memory. internal/fixlib makes those callers call it instead.
func _js_backtrace_own_flags(tls *libc.TLS, ctx uintptr, desc uintptr, p uintptr, prop TJSAtom) int32 {
	if desc != 0 && prop == uint32(EJS_ATOM_stack) {
		ppr := tls.Alloc(int(unsafe.Sizeof(uintptr(0))))
		prs := _find_own_property(tls, ppr, p, prop)
		pending := prs != 0 && backtracePending(tls, prs, *(*uintptr)(unsafe.Pointer(ppr)))
		tls.Free(int(unsafe.Sizeof(uintptr(0))))
		if pending {
			d := (*TJSPropertyDescriptor)(unsafe.Pointer(desc))
			d.Fflags = _js_backtrace_prop_flags(tls, prs) & MJS_PROP_C_W_E
			d.Fvalue = jsUndefined()
			d.Fgetter = jsUndefined()
			d.Fsetter = jsUndefined()
			return 1
		}
	}
	return _JS_GetOwnPropertyInternal(tls, ctx, desc, p, prop)
}

// _js_backtrace_raw_stack reports whether js_print_error should skip the
// stack of p: a raw dump does not format a pending stack.
func _js_backtrace_raw_stack(tls *libc.TLS, s uintptr, p uintptr) bool {
	if _js_backtrace_raw_dump(tls, s) == 0 {
		return false
	}
	ppr := tls.Alloc(int(unsafe.Sizeof(uintptr(0))))
	prs := _find_own_property(tls, ppr, p, uint32(EJS_ATOM_stack))
	pending := prs != 0 && backtracePending(tls, prs, *(*uintptr)(unsafe.Pointer(ppr)))
	tls.Free(int(unsafe.Sizeof(uintptr(0))))
	return pending
}

// _js_backtrace_print_pending prints a placeholder for a marker at prs and
// pr in a raw dump, where js_print_object would otherwise print the marker
// object. It returns 1 if it printed, 0 if pr is not a pending stack.
func _js_backtrace_print_pending(tls *libc.TLS, s uintptr, prs uintptr, pr uintptr) int32 {
	if !backtracePending(tls, prs, pr) {
		return 0
	}
	write := (*TJSPrintValueState)(unsafe.Pointer(s)).Fwrite_func
	(*(*func(*libc.TLS, uintptr, uintptr, Tsize_t))(unsafe.Pointer(&struct{ uintptr }{write})))(
		tls, (*TJSPrintValueState)(unsafe.Pointer(s)).Fwrite_opaque,
		uintptr(unsafe.Pointer(&backtracePlaceholder[0])), Tsize_t(len(backtracePlaceholder)))
	return 1
}

// _js_backtrace_memory_usage is the part of JS_ComputeMemoryUsage for the
// property at prs and pr: it counts the record of a pending stack and the
// function names it holds. The marker object is counted by the normal walk.
func _js_backtrace_memory_usage(tls *libc.TLS, s uintptr, hp uintptr, prs uintptr, pr uintptr) {
	if (*TJSShapeProperty)(unsafe.Pointer(prs)).Fatom == uint32(MJS_ATOM_NULL) || !backtracePending(tls, prs, pr) {
		return
	}
	bt := XJS_GetOpaque(tls, *(*TJSValue)(unsafe.Pointer(pr)), backtraceClassID)
	u := (*TJSMemoryUsage)(unsafe.Pointer(s))
	u.Fmemory_used_count++
	u.Fmemory_used_size += int64(backtraceSize((*backtrace)(unsafe.Pointer(bt)).count))
	for _, f := range backtraceFrames(bt) {
		if f.name != 0 {
			_compute_jsstring_size(tls, f.name, hp)
		}
	}
}

// backtraceUnref releases a reference the record holds, with only the
// runtime at hand: it decrements the reference count and frees the value
// when it reaches zero, as the JS_FreeValueRT macro does.
func backtraceUnref(tls *libc.TLS, rt uintptr, v TJSValue) {
	if jsvTag(v) < EJS_TAG_FIRST {
		return
	}
	h := (*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(tls, jsvPtr(v))))
	h.Fref_count--
	if h.Fref_count <= 0 {
		x__JS_FreeValueRT(tls, rt, v)
	}
}

// _js_backtrace_finalizer frees the record of a marker.
func _js_backtrace_finalizer(tls *libc.TLS, rt uintptr, val TJSValue) {
	bt := XJS_GetOpaque(tls, val, backtraceClassID)
	if bt == 0 {
		return
	}
	for _, f := range backtraceFrames(bt) {
		if f.name != 0 {
			backtraceUnref(tls, rt, jsvMkPtr(EJS_TAG_STRING, f.name))
		}
		if f.b != 0 {
			backtraceUnref(tls, rt, jsvMkPtr(EJS_TAG_FUNCTION_BYTECODE, f.b))
		}
	}
	Xjs_free_rt(tls, rt, bt)
}

// _js_backtrace_gc_mark marks the bytecode a marker holds.
func _js_backtrace_gc_mark(tls *libc.TLS, rt uintptr, val TJSValue, mark_func uintptr) {
	bt := XJS_GetOpaque(tls, val, backtraceClassID)
	if bt == 0 {
		return
	}
	for _, f := range backtraceFrames(bt) {
		if f.b != 0 {
			XJS_MarkValue(tls, rt, jsvMkPtr(EJS_TAG_FUNCTION_BYTECODE, f.b), mark_func)
		}
	}
}

// backtraceFormat formats a pending stack as build_backtrace does. It
// returns an exception if there is no memory for the text.
//
// Like the engine, the Go code under lib/ frees its TLS frames before it
// returns and not in a defer: a Go panic that goes up through the engine
// leaves the engine's frames on the TLS stack, which a defer would free in
// place of its own (see tlsMark in the quickjs package).
func backtraceFormat(tls *libc.TLS, ctx uintptr, bt uintptr) TJSValue {
	dbufSize := int(unsafe.Sizeof(TDynBuf{}))
	dbuf := tls.Alloc(dbufSize)
	scratch := tls.Alloc(backtraceScratchSize)
	str := backtraceFormatIn(tls, ctx, bt, dbuf, scratch)
	tls.Free(backtraceScratchSize)
	tls.Free(dbufSize)
	return str
}

// backtraceScratchSize is the size of the room backtraceFormatIn writes
// short strings and the column number to.
const backtraceScratchSize = 64

// backtraceFormatIn is backtraceFormat with dbuf, room for a DynBuf, and
// scratch, backtraceScratchSize bytes, in its TLS frame.
func backtraceFormatIn(tls *libc.TLS, ctx uintptr, bt, dbuf, scratch uintptr) TJSValue {
	const scratchSize = backtraceScratchSize

	put := func(s string) {
		copy(unsafe.Slice((*byte)(unsafe.Pointer(scratch)), scratchSize), s)
		Xdbuf_put(tls, dbuf, scratch, Tsize_t(len(s)))
	}
	putCString := func(str uintptr) bool {
		if str == 0 {
			return false
		}
		Xdbuf_putstr(tls, dbuf, str)
		XJS_FreeCString(tls, ctx, str)
		return true
	}
	fail := func() TJSValue {
		Xdbuf_free(tls, dbuf)
		return jsvMkPtr(EJS_TAG_EXCEPTION, 0)
	}

	_js_dbuf_init(tls, ctx, dbuf)
	for _, f := range backtraceFrames(bt) {
		put("    at ")
		if f.name == 0 {
			put("<anonymous>")
		} else {
			name := XJS_ToCStringLen2(tls, ctx, 0, jsvMkPtr(EJS_TAG_STRING, f.name), 0)
			if name != 0 && *(*byte)(unsafe.Pointer(name)) == 0 {
				XJS_FreeCString(tls, ctx, name)
				put("<anonymous>")
			} else if !putCString(name) {
				return fail()
			}
		}

		if b := f.b; b != 0 {
			if _js_backtrace_has_debug(tls, b) != 0 {
				pcol := scratch
				line := _find_line_num(tls, ctx, b, f.pc, pcol)
				col := *(*int32)(unsafe.Pointer(pcol))
				put(" (")
				if !putCString(XJS_AtomToCStringLen(tls, ctx, 0, (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Ffilename)) {
					return fail()
				}
				if line != 0 {
					put(":" + strconv.Itoa(int(line)) + ":" + strconv.Itoa(int(col)))
				}
				put(")")
			}
		} else {
			put(" (native)")
		}
		put("\n")
	}
	put("\x00")
	d := (*TDynBuf)(unsafe.Pointer(dbuf))
	if d.Ferror1 != 0 {
		Xdbuf_free(tls, dbuf)
		return XJS_ThrowOutOfMemory(tls, ctx)
	}

	str := XJS_NewStringLen(tls, ctx, d.Fbuf, Tsize_t(libc.Xstrlen(tls, d.Fbuf)))
	Xdbuf_free(tls, dbuf)
	return str
}
