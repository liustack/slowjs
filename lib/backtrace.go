// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

// Lazy error stacks.
//
// The engine used to format the 'stack' property of an error when it made
// the error, decoding the line number of every frame. This file is the Go
// form of the change in internal/patch/quickjs.diff that makes it lazy:
// making an error only records each frame (the function name, the bytecode
// and the pc), and 'stack' is an autoinit property whose value is formatted
// the first time it is read, with the same text.
//
// internal/fixlib wires it into every ccgo_*.go file: _build_backtrace calls
// _js_backtrace_lazy, the autoinit table gets _js_backtrace_autoinit, the
// autoinit free and mark functions call the ones here, and _get_prop_string
// formats a pending stack when an error is printed. fixlib also adds
// _js_backtrace_prop_flags and _js_backtrace_has_debug to each file, copied
// from code ccgo generated there, as the bit fields they read are laid out
// differently on some targets.

import (
	"strconv"
	"unsafe"

	"modernc.org/libc"
)

// jsAutoInitIDBacktrace is JS_AUTOINIT_ID_BACKTRACE, the autoinit ID of a
// pending stack.
const jsAutoInitIDBacktrace = 3

// backtraceFrame is a JSBacktraceFrame: one frame of a pending stack.
type backtraceFrame struct {
	name uintptr // the JSString of the function name, 0 for <anonymous>
	b    uintptr // the JSFunctionBytecode, 0 for a native function
	pc   uint32  // the pc_value build_backtrace gives to find_line_num
}

// backtrace is a JSBacktrace, the opaque value of the autoinit property.
type backtrace struct {
	count  int32
	frames [0]backtraceFrame
}

func backtraceFrames(bt uintptr) []backtraceFrame {
	p := (*backtrace)(unsafe.Pointer(bt))
	return unsafe.Slice((*backtraceFrame)(unsafe.Add(unsafe.Pointer(bt), unsafe.Offsetof(p.frames))), p.count)
}

// _js_backtrace_lazy records the stack of error_obj, to be formatted when
// 'stack' is read. It returns 0 if build_backtrace must format it now: when
// the error already has a stack, when top-level code is on the stack, or when
// there is no memory for the record.
func _js_backtrace_lazy(tls *libc.TLS, ctx uintptr, error_obj TJSValue, backtrace_flags int32) TBOOL {
	const barrier = 1 << 3   // JS_MODE_BACKTRACE_BARRIER
	const skipFirst = 1 << 0 // JS_BACKTRACE_FLAG_SKIP_FIRST_LEVEL

	if _find_own_property1(tls, jsvPtr(error_obj), uint32(EJS_ATOM_stack)) != 0 {
		return 0
	}

	rt := (*TJSContext)(unsafe.Pointer(ctx)).Frt
	n := int32(0)
	flags := backtrace_flags
	for sf := (*TJSRuntime)(unsafe.Pointer(rt)).Fcurrent_stack_frame; sf != 0; sf = (*TJSStackFrame)(unsafe.Pointer(sf)).Fprev_frame {
		if (*TJSStackFrame)(unsafe.Pointer(sf)).Fjs_mode&barrier != 0 {
			break
		}
		if flags&skipFirst != 0 {
			flags &^= skipFirst
			continue
		}
		// Top-level code (a script, an eval or a module body) runs once, so
		// recording its bytecode would keep it, and the functions it made,
		// alive for as long as the error. Libraries keep errors made when
		// they load, so format these now.
		if b := _JS_GetFunctionBytecode(tls, (*TJSStackFrame)(unsafe.Pointer(sf)).Fcur_func); b != 0 && (*TJSFunctionBytecode)(unsafe.Pointer(b)).Ffunc_name == uint32(EJS_ATOM__eval_) {
			return 0
		}
		n++
	}

	var hdr backtrace
	bt := Xjs_malloc_rt(tls, rt, Tsize_t(unsafe.Offsetof(hdr.frames)+uintptr(n)*unsafe.Sizeof(backtraceFrame{})))
	if bt == 0 {
		return 0
	}

	(*backtrace)(unsafe.Pointer(bt)).count = n
	frames := backtraceFrames(bt)
	ppr := tls.Alloc(int(unsafe.Sizeof(uintptr(0)))) // the JSProperty *pr of find_own_property
	defer tls.Free(int(unsafe.Sizeof(uintptr(0))))
	flags = backtrace_flags
	sf := (*TJSRuntime)(unsafe.Pointer(rt)).Fcurrent_stack_frame
	for i := 0; i < len(frames); sf = (*TJSStackFrame)(unsafe.Pointer(sf)).Fprev_frame {
		if flags&skipFirst != 0 {
			flags &^= skipFirst
			continue
		}
		f := &frames[i]
		cur := (*TJSStackFrame)(unsafe.Pointer(sf))
		f.name = backtraceFuncName(tls, ctx, ppr, cur.Fcur_func)
		if b := _JS_GetFunctionBytecode(tls, cur.Fcur_func); b != 0 {
			XDupValue(tls, ctx, jsvMkPtr(EJS_TAG_FUNCTION_BYTECODE, b))
			f.b = b
			f.pc = uint32(cur.Fcur_pc - (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fbyte_code_buf - 1)
		} else {
			f.b = 0
			f.pc = 0
		}
		i++
	}
	if _JS_DefineAutoInitProperty(tls, ctx, error_obj, uint32(EJS_ATOM_stack), jsAutoInitIDBacktrace, bt, 1<<0|1<<1) < 0 { // JS_PROP_CONFIGURABLE | JS_PROP_WRITABLE
		_js_backtrace_free(tls, ctx, bt)
	}
	return 1
}

// backtraceFuncName returns the string get_prop_string reads for the name of
// fun, as a new reference, or 0 where get_prop_string returns NULL. It runs
// no JS code. ppr is room for a pointer.
func backtraceFuncName(tls *libc.TLS, ctx uintptr, ppr uintptr, fun TJSValue) uintptr {
	const tmask = 3 << 4 // JS_PROP_TMASK, JS_PROP_NORMAL is 0

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
	if _js_backtrace_prop_flags(tls, prs)&tmask != 0 {
		return 0
	}
	val := *(*TJSValue)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(ppr))))
	if jsvTag(val) != EJS_TAG_STRING {
		return 0
	}
	return jsvPtr(XDupValue(tls, ctx, val))
}

// _js_backtrace_free releases a pending stack.
func _js_backtrace_free(tls *libc.TLS, ctx uintptr, bt uintptr) {
	for _, f := range backtraceFrames(bt) {
		if f.name != 0 {
			XFreeValue(tls, ctx, jsvMkPtr(EJS_TAG_STRING, f.name))
		}
		if f.b != 0 {
			XFreeValue(tls, ctx, jsvMkPtr(EJS_TAG_FUNCTION_BYTECODE, f.b))
		}
	}
	Xjs_free(tls, ctx, bt)
}

// _js_backtrace_mark marks the bytecode a pending stack holds.
func _js_backtrace_mark(tls *libc.TLS, rt uintptr, bt uintptr, mark_func uintptr) {
	for _, f := range backtraceFrames(bt) {
		if f.b != 0 {
			XJS_MarkValue(tls, rt, jsvMkPtr(EJS_TAG_FUNCTION_BYTECODE, f.b), mark_func)
		}
	}
}

func autoinitOpaque(pr uintptr) uintptr {
	// JSProperty.u.init is {uintptr_t realm_and_id; void *opaque;}
	return *(*uintptr)(unsafe.Pointer(pr + unsafe.Sizeof(uintptr(0))))
}

// _js_backtrace_autoinit_free is the part of js_autoinit_free for a pending
// stack.
func _js_backtrace_autoinit_free(tls *libc.TLS, pr uintptr) {
	if _js_autoinit_get_id(tls, pr) == jsAutoInitIDBacktrace {
		_js_backtrace_free(tls, _js_autoinit_get_realm(tls, pr), autoinitOpaque(pr))
	}
}

// _js_backtrace_autoinit_mark is the part of js_autoinit_mark for a pending
// stack.
func _js_backtrace_autoinit_mark(tls *libc.TLS, rt uintptr, pr uintptr, mark_func uintptr) {
	if _js_autoinit_get_id(tls, pr) == jsAutoInitIDBacktrace {
		_js_backtrace_mark(tls, rt, autoinitOpaque(pr), mark_func)
	}
}

// _js_backtrace_prop_string is the part of get_prop_string for a pending
// stack: it formats the stack without changing the object. It returns 0 if
// prs is not a pending stack or the stack cannot be formatted.
func _js_backtrace_prop_string(tls *libc.TLS, ctx uintptr, prs uintptr, pr uintptr) uintptr {
	const tmask, autoinit = 3 << 4, 3 << 4 // JS_PROP_TMASK, JS_PROP_AUTOINIT

	if _js_backtrace_prop_flags(tls, prs)&tmask != autoinit || _js_autoinit_get_id(tls, pr) != jsAutoInitIDBacktrace {
		return 0
	}

	val := _js_backtrace_autoinit(tls, ctx, 0, 0, autoinitOpaque(pr))
	if jsvTag(val) == EJS_TAG_EXCEPTION {
		return 0
	}

	str := XJS_ToCStringLen2(tls, ctx, 0, val, 0)
	XFreeValue(tls, ctx, val)
	return str
}

// _js_backtrace_autoinit formats a pending stack as build_backtrace does. It
// is the JSAutoInitFunc of JS_AUTOINIT_ID_BACKTRACE.
func _js_backtrace_autoinit(tls *libc.TLS, ctx uintptr, p uintptr, atom TJSAtom, bt uintptr) TJSValue {
	const scratchSize = 64

	dbufSize := int(unsafe.Sizeof(TDynBuf{}))
	dbuf := tls.Alloc(dbufSize)
	defer tls.Free(dbufSize)
	scratch := tls.Alloc(scratchSize)
	defer tls.Free(scratchSize)

	// put appends s, which is at most scratchSize bytes.
	put := func(s string) {
		copy(unsafe.Slice((*byte)(unsafe.Pointer(scratch)), scratchSize), s)
		Xdbuf_put(tls, dbuf, scratch, Tsize_t(len(s)))
	}
	_js_dbuf_init(tls, ctx, dbuf)
	for _, f := range backtraceFrames(bt) {
		var name uintptr
		if f.name != 0 {
			name = XJS_ToCStringLen2(tls, ctx, 0, jsvMkPtr(EJS_TAG_STRING, f.name), 0)
		}
		put("    at ")
		if name == 0 || *(*byte)(unsafe.Pointer(name)) == 0 {
			put("<anonymous>")
		} else {
			Xdbuf_putstr(tls, dbuf, name)
		}
		XJS_FreeCString(tls, ctx, name)

		if b := f.b; b != 0 {
			if _js_backtrace_has_debug(tls, b) != 0 {
				pcol := scratch // find_line_num writes the column here
				line := _find_line_num(tls, ctx, b, f.pc, pcol)
				col := *(*int32)(unsafe.Pointer(pcol))
				file := XJS_AtomToCStringLen(tls, ctx, 0, (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Ffilename)
				put(" (")
				if file != 0 {
					Xdbuf_putstr(tls, dbuf, file)
				} else {
					put("<null>")
				}
				XJS_FreeCString(tls, ctx, file)
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

	// Like JS_NewString, the text ends at the first NUL, which a function
	// name may hold.
	str := XJS_NewStringLen(tls, ctx, d.Fbuf, Tsize_t(libc.Xstrlen(tls, d.Fbuf)))
	Xdbuf_free(tls, dbuf)
	return str
}
