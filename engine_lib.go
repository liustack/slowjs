// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

// The engine of lib, as the rest of the package sees it: the JSValue, the
// state of a VM in the engine, and one Go function for each function of the
// engine the package calls, with the memory each needs. The other files of the
// package call only these, so that another engine can give its own forms of
// them (entry_lib.go and hostcall_lib.go have the rest).

import (
	"fmt"
	"time"
	"unsafe"

	lib "github.com/liustack/slowjs/lib"
	"modernc.org/libc"
)

// jsValue is a JSValue of the engine.
type jsValue = lib.TJSValue

const sizeofJsValue = unsafe.Sizeof(jsValue{})

// The tags of a JSValue.
const (
	tagBigInt      = lib.EJS_TAG_BIG_INT
	tagString      = lib.EJS_TAG_STRING
	tagStringRope  = lib.EJS_TAG_STRING_ROPE
	tagModule      = lib.EJS_TAG_MODULE
	tagObject      = lib.EJS_TAG_OBJECT
	tagInt         = lib.EJS_TAG_INT
	tagBool        = lib.EJS_TAG_BOOL
	tagNull        = lib.EJS_TAG_NULL
	tagUndefined   = lib.EJS_TAG_UNDEFINED
	tagException   = lib.EJS_TAG_EXCEPTION
	tagShortBigInt = lib.EJS_TAG_SHORT_BIG_INT
	tagFloat64     = lib.EJS_TAG_FLOAT64
)

// The atoms the package reads properties by, which the engine has built in.
const (
	atomNull         = lib.MJS_ATOM_NULL
	atomFileName     = lib.EJS_ATOM_fileName
	atomLineNumber   = lib.EJS_ATOM_lineNumber
	atomColumnNumber = lib.EJS_ATOM_columnNumber
	atomMessage      = lib.EJS_ATOM_message
	atomStack        = lib.EJS_ATOM_stack
	atomName         = lib.EJS_ATOM_name
	atomToString     = lib.EJS_ATOM_toString
	atomThen         = lib.EJS_ATOM_then
)

// Flags of the engine's functions.
const (
	evalFlagCompileOnly = lib.MJS_EVAL_FLAG_COMPILE_ONLY
	readObjBytecode     = lib.MJS_READ_OBJ_BYTECODE
	readObjSharedDebug  = lib.ReadObjSharedDebug
	propWritable        = lib.MJS_PROP_WRITABLE
	propConfigurable    = lib.MJS_PROP_CONFIGURABLE
	propCWE             = lib.MJS_PROP_C_W_E
	propThrow           = lib.MJS_PROP_THROW
	promisePending      = lib.EJS_PROMISE_PENDING
	promiseFulfilled    = lib.EJS_PROMISE_FULFILLED
	promiseRejected     = lib.EJS_PROMISE_REJECTED
)

// stringLenMax is the most characters a string of the engine holds.
const stringLenMax = lib.MJS_STRING_LEN_MAX

// engineVersion is the release of QuickJS the engine is.
const engineVersion = lib.MCONFIG_VERSION

var (
	null      = jsValue{Ftag: lib.EJS_TAG_NULL}
	undefined = jsValue{Ftag: lib.EJS_TAG_UNDEFINED}
	exception = jsValue{Ftag: lib.EJS_TAG_EXCEPTION}
)

func newInt32(n int32) (r jsValue) {
	return lib.XNewInt32(nil, 0, n)
}

func newFloat(n float64) (r jsValue) {
	*(*float64)(unsafe.Pointer(&r)) = n
	r.Ftag = lib.EJS_TAG_FLOAT64
	return r
}

func newBool(n bool) (r jsValue) {
	switch {
	case n:
		jsvSetInt32(&r, 1)
	default:
		jsvSetInt32(&r, 0)
	}
	r.Ftag = lib.EJS_TAG_BOOL
	return r
}

func isException(v jsValue) bool {
	return v.Ftag == lib.EJS_TAG_EXCEPTION
}

// #define JS_VALUE_GET_TAG(v) ((int32_t)(v).tag)

func tag(v jsValue) int32 {
	return int32(v.Ftag)
}

func jsvToFloat64(v jsValue) (r float64) {
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
func jsvToInt32(v jsValue) int32 {
	return int32(*(*uint64)(unsafe.Pointer(&v)))
}

// jsvSetInt32 sets the int32 payload of 'v', zero extended over the whole
// union the way JS_MKVAL does. See jsvToInt32 for why the payload is not
// written as the first four bytes of the union.
func jsvSetInt32(v *jsValue, n int32) {
	*(*uint64)(unsafe.Pointer(v)) = uint64(uint32(n))
}

// jsvToPtr extracts the JSModuleDef pointer from a JSValue.
func jsvToPtr(v jsValue) uintptr {
	return *(*uintptr)(unsafe.Pointer(&v))
}

// valueAt reads the JSValue at p, in the engine's memory.
func valueAt(p uintptr) jsValue {
	return *(*jsValue)(unsafe.Pointer(p))
}

// dupValue is JS_DupValue, which needs no call of the engine.
func dupValue(v jsValue) jsValue {
	return lib.DupValue(v)
}

// stringChars returns where the characters of v are, how many and whether
// they are 16-bit units, when v is a string the engine holds in one piece.
func stringChars(v jsValue) (p uintptr, n int, wide bool, ok bool) {
	return lib.StringChars(v)
}

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

// engineState is the state of a VM in the engine, which Close clears.
type engineState struct {
	runtime       *runtime
	cContext      uintptr // lib.TJSContext
	interruptData uintptr
	// entry is where the TLS stack was when the outermost call into
	// javascript started (see tlsMark).
	entry tlsMark
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

// newEngineVM makes the runtime and the context of a new VM, with its
// interrupt handler, and gives it its id.
func newEngineVM() (*VM, error) {
	r, err := newRuntime()
	if err != nil {
		return nil, err
	}

	m, err := r.newVM()
	if err != nil {
		r.close()
		return nil, err
	}
	return m, nil
}

// newVM returns a VM of a new context of r. When it fails, it frees what it
// allocated, but not r.
func (r *runtime) newVM() (*VM, error) {
	tls := r.tls
	interruptData := libc.Xcalloc(tls, 1, libc.Tsize_t(unsafe.Sizeof(interruptData{})))
	if interruptData == 0 {
		return nil, fmt.Errorf("OOM")
	}

	c := lib.XJS_NewContext(tls, r.cRuntime)
	if c == 0 {
		libc.Xfree(tls, interruptData)
		return nil, fmt.Errorf("failed to create a new Javascript context")
	}

	m := &VM{
		intr: &interrupter{flag: interruptData},
		vmState: vmState{
			engineState: engineState{
				cContext:      c,
				interruptData: interruptData,
				runtime:       r,
			},
		},
	}
	id, err := vms.add(m)
	if err != nil {
		lib.XJS_FreeContext(tls, c)
		libc.Xfree(tls, interruptData)
		return nil, err
	}

	// How a Builtin finds the VM that calls it.
	m.id = id
	lib.XJS_SetContextOpaque(tls, c, uintptr(id))
	lib.XJS_SetInterruptHandler(tls, r.cRuntime, fp(interruptHandler), interruptData)
	return m, nil
}

// close releases the resources held by r.
func (r *runtime) close() error {
	lib.XJS_FreeRuntime(r.tls, r.cRuntime)
	r.tls.Close()
	*r = runtime{}
	return nil
}

// closeEngine frees the context and the runtime of m and clears m.vmState.
func (m *VM) closeEngine() error {
	tls := m.runtime.tls
	ctx := m.cContext
	// A Go panic in the engine outside a call into javascript (see
	// tlsMark) left its frames on the TLS stack.
	m.runtime.base.restore(tls)

	// Interrupt can run concurrently: let it see the flag gone before it
	// is freed.
	m.intr.mu.Lock()
	m.intr.flag = 0
	m.intr.mu.Unlock()
	libc.Xfree(tls, m.interruptData)
	// The kept shapes hold the context, and so may the async context.
	lib.XJS_SetShapeCache(tls, ctx, 0)
	lib.XJS_SetAsyncContext(tls, ctx, undefined)
	lib.SetClock(m.runtime.cRuntime, nil)
	lib.XJS_FreeContext(tls, ctx)
	r := m.runtime
	m.vmState = vmState{}
	return r.close()
}

// The functions of the engine. Each is the C function of the name, JS_ for
// js, on the runtime or the context of the VM.

func (e *engineState) jsFreeValue(v jsValue) {
	lib.XFreeValue(e.runtime.tls, e.cContext, v)
}

func (e *engineState) jsSetShapeCache(n int32) int32 {
	return lib.XJS_SetShapeCache(e.runtime.tls, e.cContext, n)
}

func (e *engineState) jsSetRegExpCache(n int32, bytes uint64) {
	lib.XJS_SetRegExpCache(e.runtime.tls, e.runtime.cRuntime, n, lib.Tsize_t(bytes))
}

func (e *engineState) jsSetMemoryLimit(limit uintptr) {
	lib.XJS_SetMemoryLimit(e.runtime.tls, e.runtime.cRuntime, libc.Tsize_t(limit))
}

func (e *engineState) jsSetGCThreshold(threshold uintptr) {
	lib.XJS_SetGCThreshold(e.runtime.tls, e.runtime.cRuntime, libc.Tsize_t(threshold))
}

func (e *engineState) jsGetGCThreshold() uintptr {
	return uintptr((*lib.TJSRuntime)(unsafe.Pointer(e.runtime.cRuntime)).Fmalloc_gc_threshold)
}

func (e *engineState) jsRunGC() {
	lib.XJS_RunGC(e.runtime.tls, e.runtime.cRuntime)
}

func (e *engineState) jsRunYoungGC() {
	lib.XJS_RunYoungGC(e.runtime.tls, e.runtime.cRuntime)
}

func (e *engineState) jsGetMemorySize() uintptr {
	return uintptr((*lib.TJSRuntime)(unsafe.Pointer(e.runtime.cRuntime)).Fmalloc_ctx.Fmalloc_state.Fmalloc_size)
}

func (e *engineState) jsSetMaxStackSize(n uintptr) {
	lib.XJS_SetMaxStackSize(e.runtime.tls, e.runtime.cRuntime, libc.Tsize_t(n))
}

func (e *engineState) jsSetStripInfo(flags int32) {
	lib.XJS_SetStripInfo(e.runtime.tls, e.runtime.cRuntime, flags)
}

func (e *engineState) setClock(now func() time.Time) {
	lib.SetClock(e.runtime.cRuntime, now)
}

func (e *engineState) now() time.Time {
	return lib.Now(e.runtime.cRuntime)
}

func (e *engineState) jsGetException() jsValue {
	return lib.XJS_GetException(e.runtime.tls, e.cContext)
}

func (e *engineState) jsThrow(v jsValue) jsValue {
	return lib.XJS_Throw(e.runtime.tls, e.cContext, v)
}

func (e *engineState) jsThrowOutOfMemory() jsValue {
	return lib.XJS_ThrowOutOfMemory(e.runtime.tls, e.cContext)
}

func (e *engineState) jsIsUncatchableException() bool {
	return lib.XJS_IsUncatchableException(e.runtime.tls, e.cContext) != 0
}

func (e *engineState) jsSetUncatchableException(on bool) {
	n := int32(0)
	if on {
		n = 1
	}
	lib.XJS_SetUncatchableException(e.runtime.tls, e.cContext, n)
}

func (e *engineState) jsNewObject() jsValue {
	return lib.XJS_NewObject(e.runtime.tls, e.cContext)
}

func (e *engineState) jsNewArray() jsValue {
	return lib.XJS_NewArray(e.runtime.tls, e.cContext)
}

func (e *engineState) jsNewError() jsValue {
	return lib.XJS_NewError(e.runtime.tls, e.cContext)
}

func (e *engineState) jsGetGlobalObject() jsValue {
	return lib.XJS_GetGlobalObject(e.runtime.tls, e.cContext)
}

func (e *engineState) jsGetPrototype(v jsValue) jsValue {
	return lib.XJS_GetPrototype(e.runtime.tls, e.cContext, v)
}

func (e *engineState) jsSetPrototype(v, proto jsValue) int32 {
	return lib.XJS_SetPrototype(e.runtime.tls, e.cContext, v, proto)
}

func (e *engineState) jsIsFunction(v jsValue) bool {
	return lib.XJS_IsFunction(e.runtime.tls, e.cContext, v) != 0
}

// jsGetProperty is JS_GetProperty, an inline function of quickjs.h.
func (e *engineState) jsGetProperty(this jsValue, prop uint32) jsValue {
	return lib.XGetProperty(e.runtime.tls, e.cContext, this, prop)
}

func (e *engineState) jsGetPropertyInternal(obj jsValue, prop uint32, this jsValue, throwRefError int32) jsValue {
	return lib.XJS_GetPropertyInternal(e.runtime.tls, e.cContext, obj, prop, this, throwRefError)
}

// jsSetProperty is JS_SetProperty, an inline function of quickjs.h.
func (e *engineState) jsSetProperty(this jsValue, prop uint32, v jsValue) int32 {
	return lib.XSetProperty(e.runtime.tls, e.cContext, this, prop, v)
}

// jsGetOwnProperty is JS_GetOwnProperty without a descriptor.
func (e *engineState) jsGetOwnProperty(v jsValue, prop uint32) int32 {
	return lib.XJS_GetOwnProperty(e.runtime.tls, e.cContext, 0, v, prop)
}

func (e *engineState) jsDefinePropertyValue(obj jsValue, prop uint32, v jsValue, flags int32) int32 {
	return lib.XJS_DefinePropertyValue(e.runtime.tls, e.cContext, obj, prop, v, flags)
}

func (e *engineState) jsDefinePropertyValueUint32(obj jsValue, i uint32, v jsValue, flags int32) int32 {
	return lib.XJS_DefinePropertyValueUint32(e.runtime.tls, e.cContext, obj, i, v, flags)
}

// jsSetPropertyStr is JS_SetPropertyStr of the property name.
func (e *engineState) jsSetPropertyStr(obj jsValue, name string, v jsValue) (int32, error) {
	tls := e.runtime.tls
	p, err := libc.CString(name)
	if err != nil {
		e.jsFreeValue(v)
		return 0, err
	}

	// JS_SetPropertyStr takes over the reference to v, it does not take over
	// the property name.
	rc := lib.XJS_SetPropertyStr(tls, e.cContext, obj, p, v)
	libc.Xfree(tls, p)
	return rc, nil
}

// jsNewAtom is JS_NewAtom of s, which must not hold a NUL byte. It returns
// atomNull and an error when there is no memory for the C string.
func (e *engineState) jsNewAtom(s string) (uint32, error) {
	tls := e.runtime.tls
	p, err := libc.CString(s)
	if err != nil {
		return atomNull, err
	}

	defer libc.Xfree(tls, p)

	return lib.XJS_NewAtom(tls, e.cContext, p), nil
}

func (e *engineState) jsFreeAtom(a uint32) {
	lib.XJS_FreeAtom(e.runtime.tls, e.cContext, a)
}

func (e *engineState) jsGetAsyncContext() jsValue {
	return lib.XJS_GetAsyncContext(e.runtime.tls, e.cContext)
}

func (e *engineState) jsSetAsyncContext(v jsValue) {
	lib.XJS_SetAsyncContext(e.runtime.tls, e.cContext, v)
}

func (e *engineState) jsJSONStringify(v jsValue) jsValue {
	return lib.XJS_JSONStringify(e.runtime.tls, e.cContext, v, undefined, undefined)
}

var (
	evalFN  = [...]byte{'<', 'e', 'v', 'a', 'l', '>', 0}
	jsonFNC = [...]byte{'J', 'S', 'O', 'N', 0}
)

// jsParseJSON2 is JS_ParseJSON2 of s, with the file name of syntax errors
// "<eval>", or "JSON" when json is set. err is not nil when there is no
// memory for the C string.
func (e *engineState) jsParseJSON2(s string, json bool, flags int32) (jsValue, error) {
	tls := e.runtime.tls
	cs, err := libc.CString(s)
	if err != nil {
		return jsValue{}, err
	}

	defer libc.Xfree(tls, cs)

	fn := uintptr(unsafe.Pointer(&evalFN))
	if json {
		fn = uintptr(unsafe.Pointer(&jsonFNC[0]))
	}
	return lib.XJS_ParseJSON2(tls, e.cContext, cs, lib.Tsize_t(len(s)), fn, flags), nil
}

// jsEval is JS_Eval of src, with the file name "<eval>". err is not nil when
// there is no memory for the source.
func (e *engineState) jsEval(src string, flags int32) (jsValue, error) {
	tls := e.runtime.tls
	ps, err := libc.CString(src)
	if err != nil {
		return jsValue{}, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, ps)

	return lib.XJS_Eval(tls, e.cContext, ps, libc.Tsize_t(len(src)), uintptr(unsafe.Pointer(&evalFN)), flags), nil
}

// jsEvalThis is JS_EvalThis, as jsEval.
func (e *engineState) jsEvalThis(this jsValue, src string, flags int32) (jsValue, error) {
	tls := e.runtime.tls
	ps, err := libc.CString(src)
	if err != nil {
		return jsValue{}, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, ps)

	return lib.XJS_EvalThis(tls, e.cContext, this, ps, libc.Tsize_t(len(src)), uintptr(unsafe.Pointer(&evalFN)), flags), nil
}

func (e *engineState) jsEvalFunction(v jsValue) jsValue {
	return lib.XJS_EvalFunction(e.runtime.tls, e.cContext, v)
}

func (e *engineState) jsResolveModule(v jsValue) int32 {
	return lib.XJS_ResolveModule(e.runtime.tls, e.cContext, v)
}

// setImportMeta is js_module_set_import_meta with no URL and the module as
// main.
func (e *engineState) setImportMeta(v jsValue) {
	lib.Xjs_module_set_import_meta(e.runtime.tls, e.cContext, v, 0, 1)
}

func (e *engineState) jsPromiseState(v jsValue) int32 {
	return lib.XJS_PromiseState(e.runtime.tls, e.cContext, v)
}

func (e *engineState) jsPromiseResult(v jsValue) jsValue {
	return lib.XJS_PromiseResult(e.runtime.tls, e.cContext, v)
}

// jsExecutePendingJob is JS_ExecutePendingJob, which does not report the
// context of the job: a VM has one.
func (e *engineState) jsExecutePendingJob() int32 {
	return lib.XJS_ExecutePendingJob(e.runtime.tls, e.runtime.cRuntime, 0)
}

// jsCall is JS_Call with the arguments args, which it borrows.
func (e *engineState) jsCall(f, this jsValue, args []jsValue) jsValue {
	tls := e.runtime.tls
	var argv uintptr
	if len(args) != 0 {
		sz := libc.Tsize_t(sizeofJsValue * uintptr(len(args)))
		argv = libc.Xmalloc(tls, sz)

		defer libc.Xfree(tls, argv)

		copy(unsafe.Slice((*jsValue)(unsafe.Pointer(argv)), len(args)), args)
	}
	return lib.XJS_Call(tls, e.cContext, f, this, int32(len(args)), argv)
}

// jsArrayOfValues is js_array_of (Array.of) of the values a, which it
// borrows.
func (e *engineState) jsArrayOfValues(a []jsValue) jsValue {
	tls := e.runtime.tls
	argv := libc.Xmalloc(tls, lib.Tsize_t(len(a))*lib.Tsize_t(sizeofJsValue))

	defer libc.Xfree(tls, argv)

	p := argv
	for _, v := range a {
		*(*jsValue)(unsafe.Pointer(p)) = v
		p += sizeofJsValue
	}
	return lib.Xjs_array_of(tls, e.cContext, undefined, int32(len(a)), argv)
}

// jsReadObject is JS_ReadObject of bytecode, with the flags flags. err is
// not nil when there is no memory to copy the bytecode to C.
func (e *engineState) jsReadObject(bytecode []byte, flags int32) (jsValue, error) {
	ps, err := libc.CString(string(bytecode))
	if err != nil {
		return jsValue{}, fmt.Errorf("OOM")
	}

	defer libc.Xfree(e.runtime.tls, ps)

	return lib.XJS_ReadObject(e.runtime.tls, e.cContext, ps, lib.Tsize_t(len(bytecode)), flags), nil
}

// jsReadShared is JS_ReadObject of the bytes of b, which stay where they are.
func (e *engineState) jsReadShared(b *SharedBytecode, flags int32) jsValue {
	return lib.XJS_ReadObject(e.runtime.tls, e.cContext, b.p, lib.Tsize_t(b.n), flags)
}

// jsWriteObject returns the bytecode of the compiled script v, and false
// when JS_WriteObject threw.
func (e *engineState) jsWriteObject(v jsValue) ([]byte, bool) {
	tls := e.runtime.tls
	ctx := e.cContext
	// https://github.com/bellard/quickjs/blob/eb2c89087def1829ed99630cb14b549d7a98408c/qjsc.c#L193
	flags2 := int32(lib.MJS_WRITE_OBJ_BYTECODE)
	usize := int(unsafe.Sizeof(libc.Tsize_t(0)))
	// Freed before it returns, not in a defer (see tlsMark).
	plen := tls.Alloc(usize)
	p := lib.XJS_WriteObject(tls, ctx, plen, v, flags2)
	plen2 := int(*(*libc.Tsize_t)(unsafe.Pointer(plen)))
	tls.Free(usize)
	if p == 0 {
		return nil, false
	}

	b := libc.GoBytes(p, plen2)
	bcopy := make([]byte, len(b))
	copy(bcopy, b)

	lib.Xjs_free(tls, ctx, p)
	return bcopy, true
}

// freeTopLevel frees the function of the module mod, which has run to its
// end: the code of its top level, and the variables of the top level that no
// function of the module captures, which the function holds as long as the
// module does. Nothing calls it again: a module runs once, and the module
// keeps the variables it exports apart from the function. So a module holds
// what a script holds once its top-level code returned. QuickJS keeps the
// function until the module is freed, which for a module in the context is
// when the VM closes.
func (e *engineState) freeTopLevel(mod jsValue) {
	def := (*lib.TJSModuleDef)(unsafe.Pointer(jsvToPtr(mod)))
	f := def.Ffunc_obj
	def.Ffunc_obj = jsValue{Ftag: lib.EJS_TAG_UNDEFINED}
	e.jsFreeValue(f)
}

func (e *engineState) jsNewPromiseCapability() (promise, resolve, reject jsValue, err error) {
	tls := e.runtime.tls
	// Allocate on the C heap so the pointer stays valid if Go's stack grows.
	resolvingFuncsSize := 2 * int(sizeofJsValue)
	resolvingFuncsPtr := libc.Xcalloc(tls, 1, libc.Tsize_t(resolvingFuncsSize))
	if resolvingFuncsPtr == 0 {
		return promise, resolve, reject, fmt.Errorf("OOM")
	}
	defer libc.Xfree(tls, resolvingFuncsPtr)

	promise = lib.XJS_NewPromiseCapability(tls, e.cContext, resolvingFuncsPtr)
	resolvingFuncs := unsafe.Slice((*jsValue)(unsafe.Pointer(resolvingFuncsPtr)), 2)
	return promise, resolvingFuncs[0], resolvingFuncs[1], nil
}

// newStringChars returns a new string of n characters, 16-bit units when wide
// is true, and where its characters are, for the caller to write all of them
// before the string is used (lib.NewStringChars).
func (e *engineState) newStringChars(n int, wide bool) (jsValue, uintptr) {
	return lib.NewStringChars(e.runtime.tls, e.cContext, n, wide)
}

// jsNewStringLen is JS_NewStringLen of the UTF-8 s. err is not nil when
// there is no memory for the C string.
func (e *engineState) jsNewStringLen(s string) (jsValue, error) {
	tls := e.runtime.tls
	p, err := libc.CString(s)
	if err != nil {
		return jsValue{}, err
	}

	defer libc.Xfree(tls, p)

	return lib.XJS_NewStringLen(tls, e.cContext, p, lib.Tsize_t(len(s))), nil
}

// toGoString converts the JS value 'v' to a Go string; the JS -> Go half of
// the string bridge, JS_NewStringLen being the other. With wtf8 a lone
// surrogate stays the three bytes of its code point, and without it is
// U+FFFD (see wellFormed).
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
// [VM.goString], or [VM.errString] on the paths that read an exception.
func (e *engineState) toGoString(v jsValue, wtf8 bool) (s string, ok bool) {
	tls := e.runtime.tls
	ctx := e.cContext
	// The out-parameter lives on the emulated C stack, never the Go stack: the
	// transpiled callee holds it as a uintptr, which a Go stack move would leave
	// dangling. It is freed before toGoString returns, not in a defer (see
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

// The kinds of error jsThrowError throws.
const (
	errorKindType = iota
	errorKindRange
	errorKindSyntax
	errorKindReference
	errorKindInternal
)

var (
	// The C format string used for all messages produced on the Go side. The
	// message must never be passed as the format itself, it is not a literal
	// and any '%' in it would be taken for a conversion specification.
	percentSC = [...]byte{'%', 's', 0}
	percentS  = uintptr(unsafe.Pointer(&percentSC[0]))
)

// jsThrowError throws a new error of the kind with the message msg, which
// the engine cuts to 255 bytes, and returns the exception.
func (e *engineState) jsThrowError(kind int, msg string) jsValue {
	return throwError(e.runtime.tls, e.cContext, kind, msg)
}

// throwError is jsThrowError in ctx, which no VM may have.
func throwError(tls *libc.TLS, ctx uintptr, kind int, msg string) (r jsValue) {
	var throw func(tls *libc.TLS, ctx uintptr, fmt uintptr, va uintptr) jsValue
	switch kind {
	case errorKindType:
		throw = lib.XJS_ThrowTypeError
	case errorKindRange:
		throw = lib.XJS_ThrowRangeError
	case errorKindSyntax:
		throw = lib.XJS_ThrowSyntaxError
	case errorKindReference:
		throw = lib.XJS_ThrowReferenceError
	case errorKindInternal:
		throw = lib.XJS_ThrowInternalError
	default:
		panic(fmt.Sprintf("slowjs: an error of kind %d", kind))
	}
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
