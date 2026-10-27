// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

import (
	"fmt"
	"unsafe"

	lib "github.com/liustack/slowjs/lib"
	"modernc.org/libc"
)

// runtime represents a Javascript runtime.
//
// Note: runtime is not safe for concurrent use by multiple goroutines.
type runtime struct {
	cRuntime          uintptr // lib.TJSRuntime
	tls               *libc.TLS
	callbackClassInit bool
	// base is where the TLS stack is between calls (see tlsMark).
	base tlsMark
}

// newRuntime returns a newly create Runtime
func newRuntime() (*runtime, error) {
	tls := libc.NewTLS()
	warmTLS(tls)
	base := markTLS(tls)
	r := lib.XJS_NewRuntime(tls)
	if r == 0 {
		tls.Close()
		return nil, fmt.Errorf("failed to create a new Javascript runtime")
	}

	return &runtime{
		cRuntime: r,
		tls:      tls,
		base:     base,
	}, nil
}

// newVM returns a newly created VM.
func (r *runtime) newVM() (*VM, error) {
	tls := r.tls
	argv := libc.Xcalloc(tls, 2, libc.Tsize_t(sizeofJsValue))
	if argv == 0 {
		return nil, fmt.Errorf("OOM")
	}

	interruptData := libc.Xcalloc(tls, 1, libc.Tsize_t(unsafe.Sizeof(interruptData{})))
	if interruptData == 0 {
		libc.Xfree(tls, argv)
		return nil, fmt.Errorf("OOM")
	}

	c := lib.XJS_NewContext(tls, r.cRuntime)
	if c == 0 {
		libc.Xfree(tls, interruptData)
		libc.Xfree(tls, argv)
		return nil, fmt.Errorf("failed to create a new Javascript context")
	}

	m := &VM{
		intr: &interrupter{flag: interruptData},
		vmState: vmState{
			cContext:      c,
			int32_16:      lib.XNewInt32(r.tls, c, 16),
			int32_2:       lib.XNewInt32(r.tls, c, 2),
			interruptData: interruptData,
			runtime:       r,
			toStringArgv:  argv,
		},
	}
	id, err := vms.add(m)
	if err != nil {
		lib.XJS_FreeContext(tls, c)
		libc.Xfree(tls, interruptData)
		libc.Xfree(tls, argv)
		return nil, err
	}

	// How a Builtin finds the VM that calls it.
	m.id = id
	lib.XJS_SetContextOpaque(tls, c, uintptr(id))
	return m, nil
}

// close releases the resources held by r.
func (r *runtime) close() error {
	lib.XJS_FreeRuntime(r.tls, r.cRuntime)
	r.tls.Close()
	*r = runtime{}
	return nil
}

func newInt32(n int32) (r lib.TJSValue) {
	return lib.XNewInt32(nil, 0, n)
}

// freeTopLevel frees the function of the module mod, which has run to its
// end: the code of its top level, and the variables of the top level that no
// function of the module captures, which the function holds as long as the
// module does. Nothing calls it again: a module runs once, and the module
// keeps the variables it exports apart from the function. So a module holds
// what a script holds once its top-level code returned. QuickJS keeps the
// function until the module is freed, which for a module in the context is
// when the VM closes.
func freeTopLevel(tls *libc.TLS, ctx uintptr, mod lib.TJSValue) {
	def := (*lib.TJSModuleDef)(unsafe.Pointer(jsvToPtr(mod)))
	f := def.Ffunc_obj
	def.Ffunc_obj = lib.TJSValue{Ftag: lib.EJS_TAG_UNDEFINED}
	lib.XFreeValue(tls, ctx, f)
}

// writeBytecode returns the bytecode of the compiled script v.
func (m *VM) writeBytecode(v lib.TJSValue) ([]byte, error) {
	tls := m.runtime.tls
	ctx := m.cContext
	// https://github.com/bellard/quickjs/blob/eb2c89087def1829ed99630cb14b549d7a98408c/qjsc.c#L193
	flags2 := int32(lib.MJS_WRITE_OBJ_BYTECODE)
	usize := int(unsafe.Sizeof(libc.Tsize_t(0)))
	// Freed before it returns, not in a defer (see tlsMark).
	plen := tls.Alloc(usize)
	p := lib.XJS_WriteObject(tls, ctx, plen, v, flags2)
	plen2 := int(*(*libc.Tsize_t)(unsafe.Pointer(plen)))
	tls.Free(usize)
	if p == 0 {
		return nil, m.errFromException()
	}

	b := libc.GoBytes(p, plen2)
	bcopy := make([]byte, len(b))
	copy(bcopy, b)

	lib.Xjs_free(tls, ctx, p)
	return bcopy, nil
}

// goString converts the JS value 'v' to a Go string; the JS -> Go half of the
// string bridge, JS_NewStringLen being the other.
//
// It is length-based on purpose. JS_ToCString hands out a NUL-terminated
// buffer, but a JavaScript string is a sequence of UTF-16 code units and U+0000
// is an ordinary one - a binary string built with String.fromCharCode, btoa's
// argument, pixel data - so libc.GoString over that buffer cut every such string
// at its first NUL, and a host function could not tell a truncated argument
// from a short one. JS_ToCStringLen2 reports the length instead, and the bytes
// are copied out before the buffer is released.
//
// The conversion throws when there is no memory for the UTF-8 copy, which a
// string with non-ASCII characters needs, or when 'v' is not a string and its
// toString throws. ok is then false and the exception is left pending: use
// [VM.goString], or [errString] on the paths that read an exception.
func cString(tls *libc.TLS, ctx uintptr, v lib.TJSValue) (s string, ok bool) {
	return toCString(tls, ctx, v, false)
}

// cWTF8 is cString without wellFormed: a lone surrogate stays the three bytes
// of its code point.
func cWTF8(tls *libc.TLS, ctx uintptr, v lib.TJSValue) (s string, ok bool) {
	return toCString(tls, ctx, v, true)
}

func toCString(tls *libc.TLS, ctx uintptr, v lib.TJSValue, wtf8 bool) (s string, ok bool) {
	// The out-parameter lives on the emulated C stack, never the Go stack: the
	// transpiled callee holds it as a uintptr, which a Go stack move would leave
	// dangling. It is freed before cString returns, not in a defer (see
	// tlsMark).
	usize := int(unsafe.Sizeof(libc.Tsize_t(0)))
	plen := tls.Alloc(usize)
	p := lib.XJS_ToCStringLen2(tls, ctx, plen, v, 0)
	n := *(*libc.Tsize_t)(unsafe.Pointer(plen))
	tls.Free(usize)
	if p == 0 {
		return "", false
	}

	defer lib.XJS_FreeCString(tls, ctx, p)

	b := unsafe.Slice((*byte)(unsafe.Pointer(p)), n)
	if wtf8 {
		return string(b), true
	}
	return wellFormed(b), true
}

// cGoString returns the NUL-terminated C string p, which the engine wrote
// from a JS string, as a Go string with U+FFFD for each lone surrogate, like
// cString.
func cGoString(tls *libc.TLS, p uintptr) string {
	return wellFormed(unsafe.Slice((*byte)(unsafe.Pointer(p)), libc.Xstrlen(tls, p)))
}

var (
	// The C format string used for all messages produced on the Go side. The
	// message must never be passed as the format itself, it is not a literal
	// and any '%' in it would be taken for a conversion specification.
	percentSC = [...]byte{'%', 's', 0}
	percentS  = uintptr(unsafe.Pointer(&percentSC[0]))
)

func throwTypeError(tls *libc.TLS, ctx uintptr, msg string) (r lib.TJSValue) {
	return throwf(tls, ctx, lib.XJS_ThrowTypeError, msg)
}

func throwInternalError(tls *libc.TLS, ctx uintptr, msg string) (r lib.TJSValue) {
	return throwf(tls, ctx, lib.XJS_ThrowInternalError, msg)
}

// throwf throws msg with throw, one of the engine's JS_Throw...Error
// functions, which cut the message to 255 bytes.
func throwf(tls *libc.TLS, ctx uintptr, throw func(tls *libc.TLS, ctx uintptr, fmt uintptr, va uintptr) lib.TJSValue, msg string) (r lib.TJSValue) {
	p, err := libc.CString(validUTF8(msg))
	if err != nil {
		return throw(tls, ctx, 0, 0)
	}

	defer libc.Xfree(tls, p)

	// Freed before it returns, not in a defer (see tlsMark).
	bp := tls.Alloc(16)
	r = throw(tls, ctx, percentS, libc.VaList(bp+8, p))
	tls.Free(16)
	return r
}

func fp(f interface{}) uintptr {
	type iface [2]uintptr
	return (*iface)(unsafe.Pointer(&f))[1]
}

const sizeofJsValue = unsafe.Sizeof(lib.TJSValue{})

var (
	null      = lib.TJSValue{Ftag: lib.EJS_TAG_NULL}
	undefined = lib.TJSValue{Ftag: lib.EJS_TAG_UNDEFINED}
	exception = lib.TJSValue{Ftag: lib.EJS_TAG_EXCEPTION}
)

func newFloat(n float64) (r lib.TJSValue) {
	*(*float64)(unsafe.Pointer(&r)) = n
	r.Ftag = lib.EJS_TAG_FLOAT64
	return r
}

func newBool(n bool) (r lib.TJSValue) {
	switch {
	case n:
		jsvSetInt32(&r, 1)
	default:
		jsvSetInt32(&r, 0)
	}
	r.Ftag = lib.EJS_TAG_BOOL
	return r
}

func isException(v lib.TJSValue) bool {
	return v.Ftag == lib.EJS_TAG_EXCEPTION
}

// #define JS_VALUE_GET_TAG(v) ((int32_t)(v).tag)

func tag(v lib.TJSValue) int32 {
	return int32(v.Ftag)
}

func jsvToFloat64(v lib.TJSValue) (r float64) {
	return *(*float64)(unsafe.Pointer(&v))
}

// #define JS_VALUE_GET_INT(v) ((v).u.int32)
//
// The JSValue payload is the eight bytes wide C union JSValueUnion, which
// the lib package (ccgo) represents as its widest member, an uint64. The
// narrower members of that union are accessed in the value domain, not by
// address: JS_MKVAL(tag, val) zero extends 'val' into the low 32 bits of the
// uint64 and JS_VALUE_GET_INT truncates the uint64 back to 32 bits. Reading or
// writing the payload as the first four bytes of the union instead is
// equivalent on a little endian target only - on s390x those are the high, for
// an int32 or a boolean always zero, bits.
//
// jsvToInt32 must be used for the int32 payload of a JS_TAG_INT and for the
// boolean payload of a JS_TAG_BOOL. The float64 and pointer payloads are as
// wide as the union itself, so for those the two domains coincide and no
// conversion is needed.
func jsvToInt32(v lib.TJSValue) int32 {
	return int32(*(*uint64)(unsafe.Pointer(&v)))
}

// jsvSetInt32 sets the int32 payload of 'v', zero extended over the whole
// union the way JS_MKVAL does. See jsvToInt32 for why the payload is not
// written as the first four bytes of the union.
func jsvSetInt32(v *lib.TJSValue, n int32) {
	*(*uint64)(unsafe.Pointer(v)) = uint64(uint32(n))
}

// jsvToPtr extracts the JSModuleDef pointer from a JSValue.
func jsvToPtr(v lib.TJSValue) uintptr {
	return *(*uintptr)(unsafe.Pointer(&v))
}
