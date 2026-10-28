// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && ((linux && amd64) || (darwin && arm64))

package slowjs // import "github.com/liustack/slowjs"

// The C engine of internal/cengine, as the rest of the package sees it: the
// same names engine_lib.go gives, over the engine's generated functions. A
// call into the engine takes the shadow pointer, the first free byte of the
// VM's shadow stack: m.sp, which a Go function that javascript called moves
// above the frames of the call (hostcall_c.go). What a call needs passed by
// address goes on the shadow stack at m.sp, and the call takes the pointer
// past it (tmp). Outside a call into javascript, a function first enters the
// engine (outer in entry_c.go).

import (
	"fmt"
	"math"
	"time"
	"unsafe"

	"github.com/liustack/slowjs/internal/cengine"
	"github.com/liustack/slowjs/internal/gohost"
)

// jsValue is a JSValue of the engine: its union and its tag, a word each.
type jsValue struct{ u, tag uintptr }

const sizeofJsValue = unsafe.Sizeof(jsValue{})

// val makes a jsValue of the two words a function of the engine returns.
func val(u, tag uintptr) jsValue { return jsValue{u, tag} }

// The tags of a JSValue (engine/quickjs.h).
const (
	tagFirst       = -9
	tagBigInt      = -9
	tagString      = -7
	tagStringRope  = -6
	tagModule      = -3
	tagObject      = -1
	tagInt         = 0
	tagBool        = 1
	tagNull        = 2
	tagUndefined   = 3
	tagException   = 6
	tagShortBigInt = 7
	tagFloat64     = 8
)

// The atoms the package reads properties by.
const (
	atomNull         = 0
	atomFileName     = cengine.AtomFileName
	atomLineNumber   = cengine.AtomLineNumber
	atomColumnNumber = cengine.AtomColumnNumber
	atomMessage      = cengine.AtomMessage
	atomStack        = cengine.AtomStack
	atomName         = cengine.AtomName
	atomToString     = cengine.AtomToString
	atomThen         = cengine.AtomThen
)

// Flags of the engine's functions (engine/quickjs.h).
const (
	evalFlagCompileOnly = 1 << 5
	readObjBytecode     = 1 << 0
	readObjSharedDebug  = 1 << 4
	writeObjBytecode    = 1 << 0
	propConfigurable    = 1 << 0
	propWritable        = 1 << 1
	propEnumerable      = 1 << 2
	propCWE             = propConfigurable | propWritable | propEnumerable
	propThrow           = 1 << 14
	promisePending      = 0
	promiseFulfilled    = 1
	promiseRejected     = 2
)

// stringLenMax is the most characters a string of the engine holds.
const stringLenMax = cengine.StringLenMax

// engineVersion is the release of QuickJS the engine is (CONFIG_VERSION of
// the generator).
const engineVersion = "2026-06-04"

var (
	null      = jsValue{tag: tagNull}
	undefined = jsValue{tag: tagUndefined}
	exception = jsValue{tag: tagException}
)

func newInt32(n int32) jsValue { return jsValue{u: uintptr(uint32(n)), tag: tagInt} }

func newFloat(n float64) jsValue { return jsValue{u: uintptr(math.Float64bits(n)), tag: tagFloat64} }

func newBool(n bool) jsValue {
	if n {
		return jsValue{u: 1, tag: tagBool}
	}
	return jsValue{tag: tagBool}
}

func isException(v jsValue) bool { return int32(v.tag) == tagException }

func tag(v jsValue) int32 { return int32(v.tag) }

func jsvToFloat64(v jsValue) float64 { return math.Float64frombits(uint64(v.u)) }

// jsvToInt32 is the int32 payload of a JS_TAG_INT and the boolean of a
// JS_TAG_BOOL: JS_MKVAL zero extends it over the word.
func jsvToInt32(v jsValue) int32 { return int32(v.u) }

// hasRefCount is JS_VALUE_HAS_REF_COUNT: the tags from JS_TAG_FIRST to -1,
// compared as unsigned.
func hasRefCount(v jsValue) bool { return uint32(v.tag) >= 1<<32+tagFirst }

// refCount is the reference count of v, which hasRefCount.
func refCount(v jsValue) *int32 { return (*int32)(addr(uintptr(int(v.u) + cengine.RefCountOffset))) }

// valueAt reads the JSValue at p, in the engine's memory.
func valueAt(p uintptr) jsValue { return *(*jsValue)(addr(p)) }

// dupValue is JS_DupValue, which needs no call of the engine.
func dupValue(v jsValue) jsValue {
	if hasRefCount(v) {
		*refCount(v)++
	}
	return v
}

// stringChars returns where the characters of v are, how many and whether
// they are 16-bit units, when v is a string the engine holds in one piece.
func stringChars(v jsValue) (p uintptr, n int, wide bool, ok bool) {
	if tag(v) != tagString {
		return 0, 0, false, false
	}
	h := *(*uint32)(addr(v.u))
	return v.u + cengine.StringChars, int(h & cengine.StringLenMask), h&cengine.StringWideBit != 0, true
}

// addr and bytesAt reach the engine's memory, outside the Go heap: the
// garbage collector ignores pointers into it, and reading the address through
// memory keeps go vet from taking it for a Go pointer kept in a uintptr.
func addr(a uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&a)) }

func bytesAt(a, n uintptr) []byte { return unsafe.Slice((*byte)(addr(a)), n) }

// engineState is the state of a VM in the engine, which Close clears.
type engineState struct {
	cvm     *cengine.VM
	h       *gohost.VM
	rt, ctx uintptr
	// sp is the shadow pointer of the next call into the engine: start
	// outside a call, and above the frames of the call while javascript
	// calls Go (hostcall_c.go). end is the end of the shadow stack.
	sp, start, end uintptr
	// entered counts the entries into the engine in progress, each a
	// gohost.VM.Enter or EnterScript: a function of the engine called when
	// it is 0 enters first (outer).
	entered int
	clock   func() time.Time
}

// tmpMax is the most bytes tmp takes from the shadow stack: more go to the
// VM's heap.
const tmpMax = 64 << 10

// tmp returns n bytes for a call into the engine and the shadow pointer the
// call takes: on the shadow stack at m.sp, past which the call starts, or,
// for more than tmpMax bytes or more than the stack has left, in the VM's
// heap, which then counts them against the memory limit until tmpFree. p is
// 0 when the heap has no room, and the engine's out of memory error is
// thrown then, as when the engine itself runs out: the caller returns it
// (tmpError), or the exception.
func (m *VM) tmp(n uintptr) (p, sp uintptr, heap bool) {
	if n <= tmpMax {
		if end := (m.sp + n + 15) &^ 15; end+tmpMax <= m.end {
			return m.sp, end, false
		}
	}
	p = cengine.Malloc(m.sp, m.rt, max(n, 1))
	if p == 0 {
		cengine.JS_ThrowOutOfMemory(m.sp, m.ctx)
	}
	return p, m.sp, true
}

// tmpError returns the out of memory error tmp threw as the error of the call
// it could not pass what it needed: uncatchable, and, inside a Go function
// called from javascript, still pending, so that it ends that javascript too
// (takeException).
func (m *VM) tmpError() error {
	err, _ := m.takeException()
	return err
}

// tmpFree gives back what tmp took from the heap.
func (m *VM) tmpFree(p uintptr, heap bool) {
	if heap && p != 0 {
		cengine.FreeRT(m.sp, m.rt, p)
	}
}

// tmpString is tmp of the bytes of s and a 0 byte after them when nul is set,
// written there.
func (m *VM) tmpString(s string, nul bool) (p, sp uintptr, heap bool) {
	n := uintptr(len(s))
	if nul {
		n++
	}
	p, sp, heap = m.tmp(n)
	if p != 0 {
		b := bytesAt(p, n)
		copy(b, s)
		if nul {
			b[len(s)] = 0
		}
	}
	return p, sp, heap
}

// newEngineVM makes the runtime and the context of a new VM, and gives it
// its id: the id of its host, which the engine passes to the callbacks.
func newEngineVM() (*VM, error) {
	seed := uint64(time.Now().UnixMicro())
	if seed == 0 {
		seed = 1
	}
	cv, err := cengine.New(cengine.Options{Seed: seed})
	if err != nil {
		return nil, err
	}
	// Unless the VM is handed back, nothing else closes the engine's VM.
	handed := false
	defer func() {
		if !handed {
			cv.Close()
		}
	}()
	h := cv.Host()
	sp := cv.SP()
	base, _ := h.Stack()
	m := &VM{
		intr: &interrupter{h: h},
		vmState: vmState{
			engineState: engineState{
				cvm:   cv,
				h:     h,
				rt:    cv.Runtime(),
				ctx:   cv.Context(),
				sp:    sp,
				start: sp,
				end:   base + cengine.ShadowSize,
			},
		},
	}
	m.id = h.ID()
	// The id is free in vms: closeEngine takes it out before the host
	// hands it out again.
	vms.Put(m.id, m)
	handed = true
	return m, nil
}

// forgetVM has nothing to do: closeEngine took the id out of vms already.
func forgetVM(id uint32) {}

// closeEngine frees the context and the runtime of m, gives its memory back
// and clears m.vmState. The id of m leaves vms before the host takes it back
// (cvm.Close), also when a panic leaves first: another goroutine may get it
// for a new VM from then on.
func (m *VM) closeEngine() error {
	func() {
		defer vms.Delete(m.id)
		if !m.h.Broken() && m.entered == 0 {
			// The async context may hold what the context holds.
			m.jsSetAsyncContext(undefined)
		}
	}()
	_, err := m.cvm.Close()
	// Interrupt keeps the host: it must not keep what the clock holds.
	m.h.SetClock(nil)
	m.vmState = vmState{}
	return err
}

// The functions of the engine. Each is the C function of the name, JS_ for
// js, on the runtime or the context of the VM. Outside a call into the
// engine, each enters it first and calls itself again inside (outer).

func (m *VM) jsFreeValue(v jsValue) {
	if !hasRefCount(v) {
		return
	}
	if *refCount(v)--; *refCount(v) > 0 {
		return
	}
	if m.entered == 0 {
		// A broken VM is not entered again: Close gives its memory back.
		if !m.h.Broken() {
			m.outer(func() { cengine.FreeValue(m.sp, m.ctx, v.u, v.tag) })
		}
		return
	}
	cengine.FreeValue(m.sp, m.ctx, v.u, v.tag)
}

func (m *VM) jsSetShapeCache(n int32) (r int32) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsSetShapeCache(n) })
		return r
	}
	return int32(cengine.JS_SetShapeCache(m.sp, m.ctx, uintptr(n)))
}

func (m *VM) jsSetRegExpCache(n int32, bytes uint64) {
	if m.entered == 0 {
		m.outer(func() { m.jsSetRegExpCache(n, bytes) })
		return
	}
	cengine.JS_SetRegExpCache(m.sp, m.rt, uintptr(n), uintptr(bytes))
}

func (m *VM) jsSetMemoryLimit(limit uintptr) {
	if m.entered == 0 {
		m.outer(func() { m.jsSetMemoryLimit(limit) })
		return
	}
	cengine.JS_SetMemoryLimit(m.sp, m.rt, limit)
}

func (m *VM) jsSetGCThreshold(threshold uintptr) {
	if m.entered == 0 {
		m.outer(func() { m.jsSetGCThreshold(threshold) })
		return
	}
	cengine.JS_SetGCThreshold(m.sp, m.rt, threshold)
}

// killedReads reports whether m is a VM a kill left good only for Close and no
// call is in progress. It is for the functions of the engine that only read a
// counter of the runtime (jsGetGCThreshold, jsGetMemorySize): a kill ends the
// script at a poll, which sits between two allocations, so a counter is still
// whole, and reading it needs no entry into the engine, which a broken VM
// refuses (outer). It reads on the shadow stack at start, as the frames above
// are dead. Every other function of the engine is left to panic (outer).
func (m *VM) killedReads() bool {
	return m.entered == 0 && m.h.Killed()
}

func (m *VM) jsGetGCThreshold() (r uintptr) {
	if m.killedReads() {
		return cengine.JS_GetGCThreshold(m.start, m.rt)
	}
	if m.entered == 0 {
		m.outer(func() { r = m.jsGetGCThreshold() })
		return r
	}
	return cengine.JS_GetGCThreshold(m.sp, m.rt)
}

func (m *VM) jsRunGC() {
	if m.entered == 0 {
		m.outer(func() { m.jsRunGC() })
		return
	}
	cengine.JS_RunGC(m.sp, m.rt)
}

func (m *VM) jsRunYoungGC() {
	if m.entered == 0 {
		m.outer(func() { m.jsRunYoungGC() })
		return
	}
	cengine.JS_RunYoungGC(m.sp, m.rt)
}

func (m *VM) jsGetMemorySize() (r uintptr) {
	if m.killedReads() {
		return cengine.JS_GetMemorySize(m.start, m.rt)
	}
	if m.entered == 0 {
		m.outer(func() { r = m.jsGetMemorySize() })
		return r
	}
	return cengine.JS_GetMemorySize(m.sp, m.rt)
}

// The call depths of SetMaxStackSize (JS_DEFAULT_MAX_CALL_DEPTH and
// JS_MAX_CALL_DEPTH_LIMIT of engine/quickjs.h).
const (
	defaultMaxCallDepth = 5000
	maxCallDepthLimit   = 10000
)

// jsSetMaxStackSize is JS_SetMaxCallDepth: n is how deep calls may nest, 0
// for the default, and at most maxCallDepthLimit.
func (m *VM) jsSetMaxStackSize(n uintptr) {
	if m.entered == 0 {
		m.outer(func() { m.jsSetMaxStackSize(n) })
		return
	}
	switch {
	case n == 0:
		n = defaultMaxCallDepth
	case n > maxCallDepthLimit:
		n = maxCallDepthLimit
	}
	// It refuses while the engine runs javascript, as from a Go function
	// that javascript called: the depth stays as it was.
	cengine.JS_SetMaxCallDepth(m.sp, m.rt, n)
}

func (m *VM) jsSetStripInfo(flags int32) {
	if m.entered == 0 {
		m.outer(func() { m.jsSetStripInfo(flags) })
		return
	}
	cengine.JS_SetStripInfo(m.sp, m.rt, uintptr(flags))
}

func (m *VM) setClock(now func() time.Time) {
	m.clock = now
	m.h.SetClock(now)
}

func (m *VM) now() time.Time {
	if m.clock != nil {
		return m.clock()
	}
	return time.Now()
}

func (m *VM) jsToString(v jsValue) (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsToString(v) })
		return r
	}
	return val(cengine.JS_ToString(m.sp, m.ctx, v.u, v.tag))
}

func (m *VM) jsGetException() (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsGetException() })
		return r
	}
	return val(cengine.JS_GetException(m.sp, m.ctx))
}

func (m *VM) jsThrow(v jsValue) (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsThrow(v) })
		return r
	}
	return val(cengine.JS_Throw(m.sp, m.ctx, v.u, v.tag))
}

func (m *VM) jsThrowOutOfMemory() (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsThrowOutOfMemory() })
		return r
	}
	return val(cengine.JS_ThrowOutOfMemory(m.sp, m.ctx))
}

func (m *VM) jsIsUncatchableException() (r bool) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsIsUncatchableException() })
		return r
	}
	return int32(cengine.JS_IsUncatchableException(m.sp, m.ctx)) != 0
}

func (m *VM) jsSetUncatchableException(on bool) {
	if m.entered == 0 {
		m.outer(func() { m.jsSetUncatchableException(on) })
		return
	}
	n := uintptr(0)
	if on {
		n = 1
	}
	cengine.JS_SetUncatchableException(m.sp, m.ctx, n)
}

func (m *VM) jsNewObject() (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsNewObject() })
		return r
	}
	return val(cengine.JS_NewObject(m.sp, m.ctx))
}

func (m *VM) jsNewArray() (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsNewArray() })
		return r
	}
	return val(cengine.JS_NewArray(m.sp, m.ctx))
}

func (m *VM) jsNewError() (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsNewError() })
		return r
	}
	return val(cengine.JS_NewError(m.sp, m.ctx))
}

func (m *VM) jsGetGlobalObject() (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsGetGlobalObject() })
		return r
	}
	return val(cengine.JS_GetGlobalObject(m.sp, m.ctx))
}

func (m *VM) jsGetPrototype(v jsValue) (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsGetPrototype(v) })
		return r
	}
	return val(cengine.JS_GetPrototype(m.sp, m.ctx, v.u, v.tag))
}

func (m *VM) jsSetPrototype(v, proto jsValue) (r int32) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsSetPrototype(v, proto) })
		return r
	}
	return int32(cengine.JS_SetPrototype(m.sp, m.ctx, v.u, v.tag, proto.u, proto.tag))
}

func (m *VM) jsIsFunction(v jsValue) (r bool) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsIsFunction(v) })
		return r
	}
	return int32(cengine.JS_IsFunction(m.sp, m.ctx, v.u, v.tag)) != 0
}

// jsGetProperty is JS_GetProperty, an inline function of quickjs.h.
func (m *VM) jsGetProperty(this jsValue, prop uint32) jsValue {
	return m.jsGetPropertyInternal(this, prop, this, 0)
}

func (m *VM) jsGetPropertyInternal(obj jsValue, prop uint32, this jsValue, throwRefError int32) (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsGetPropertyInternal(obj, prop, this, throwRefError) })
		return r
	}
	return val(cengine.JS_GetPropertyInternal(m.sp, m.ctx, obj.u, obj.tag, uintptr(prop), this.u, this.tag, uintptr(throwRefError)))
}

// jsSetProperty is JS_SetProperty, an inline function of quickjs.h:
// JS_SetPropertyInternal, whose arguments go in a block on the shadow stack
// (the generator packs them, as there are more than the registers hold, and
// writes the Go function that makes the block, gen.PackedGo).
func (m *VM) jsSetProperty(this jsValue, prop uint32, v jsValue) (r int32) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsSetProperty(this, prop, v) })
		return r
	}
	return int32(cengine.JS_SetPropertyInternal(m.sp, m.ctx, this.u, this.tag, uintptr(prop), v.u, v.tag, this.u, this.tag, propThrow))
}

// jsGetOwnProperty is JS_GetOwnProperty without a descriptor.
func (m *VM) jsGetOwnProperty(v jsValue, prop uint32) (r int32) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsGetOwnProperty(v, prop) })
		return r
	}
	return int32(cengine.JS_GetOwnProperty(m.sp, m.ctx, 0, v.u, v.tag, uintptr(prop)))
}

func (m *VM) jsDefinePropertyValue(obj jsValue, prop uint32, v jsValue, flags int32) (r int32) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsDefinePropertyValue(obj, prop, v, flags) })
		return r
	}
	return int32(cengine.JS_DefinePropertyValue(m.sp, m.ctx, obj.u, obj.tag, uintptr(prop), v.u, v.tag, uintptr(flags)))
}

func (m *VM) jsDefinePropertyValueUint32(obj jsValue, i uint32, v jsValue, flags int32) (r int32) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsDefinePropertyValueUint32(obj, i, v, flags) })
		return r
	}
	return int32(cengine.JS_DefinePropertyValueUint32(m.sp, m.ctx, obj.u, obj.tag, uintptr(i), v.u, v.tag, uintptr(flags)))
}

// jsSetPropertyStr is JS_SetPropertyStr of the property name.
func (m *VM) jsSetPropertyStr(obj jsValue, name string, v jsValue) (r int32, err error) {
	if m.entered == 0 {
		m.outer(func() { r, err = m.jsSetPropertyStr(obj, name, v) })
		return r, err
	}
	p, sp, heap := m.tmpString(name, true)
	if p == 0 {
		m.jsFreeValue(v)
		return 0, m.tmpError()
	}
	// JS_SetPropertyStr takes over the reference to v, not the name.
	r = int32(cengine.JS_SetPropertyStr(sp, m.ctx, obj.u, obj.tag, p, v.u, v.tag))
	m.tmpFree(p, heap)
	return r, nil
}

// jsNewAtom is JS_NewAtomLen of s. It returns atomNull and an error when
// there is no memory to pass s.
func (m *VM) jsNewAtom(s string) (r uint32, err error) {
	if m.entered == 0 {
		m.outer(func() { r, err = m.jsNewAtom(s) })
		return r, err
	}
	p, sp, heap := m.tmpString(s, false)
	if p == 0 {
		return atomNull, m.tmpError()
	}
	r = uint32(cengine.JS_NewAtomLen(sp, m.ctx, p, uintptr(len(s))))
	m.tmpFree(p, heap)
	return r, nil
}

func (m *VM) jsFreeAtom(a uint32) {
	if m.entered == 0 {
		if !m.h.Broken() {
			m.outer(func() { m.jsFreeAtom(a) })
		}
		return
	}
	cengine.JS_FreeAtom(m.sp, m.ctx, uintptr(a))
}

func (m *VM) jsGetAsyncContext() (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsGetAsyncContext() })
		return r
	}
	return val(cengine.JS_GetAsyncContext(m.sp, m.ctx))
}

func (m *VM) jsSetAsyncContext(v jsValue) {
	if m.entered == 0 {
		m.outer(func() { m.jsSetAsyncContext(v) })
		return
	}
	cengine.JS_SetAsyncContext(m.sp, m.ctx, v.u, v.tag)
}

// jsJSONStringify is JS_JSONStringify without a replacer and a space, whose
// arguments go in a block (see jsSetProperty).
func (m *VM) jsJSONStringify(v jsValue) (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsJSONStringify(v) })
		return r
	}
	return val(cengine.JS_JSONStringify(m.sp, m.ctx, v.u, v.tag, undefined.u, undefined.tag, undefined.u, undefined.tag))
}

// The file names of syntax errors.
const (
	evalFN = "<eval>\x00"
	jsonFN = "JSON\x00"
)

// jsParseJSON2 is JS_ParseJSON2 of s, with the file name of syntax errors
// "<eval>", or "JSON" when json is set. err is not nil when there is no
// memory to pass s.
func (m *VM) jsParseJSON2(s string, json bool, flags int32) (r jsValue, err error) {
	if m.entered == 0 {
		m.outer(func() { r, err = m.jsParseJSON2(s, json, flags) })
		return r, err
	}
	fn := evalFN
	if json {
		fn = jsonFN
	}
	// The file name first, then s and the 0 byte the parser reads up to.
	p, sp, heap := m.tmp(uintptr(len(fn) + len(s) + 1))
	if p == 0 {
		return jsValue{}, m.tmpError()
	}
	b := bytesAt(p, uintptr(len(fn)+len(s)+1))
	copy(b, fn)
	copy(b[len(fn):], s)
	b[len(b)-1] = 0
	r = val(cengine.JS_ParseJSON2(sp, m.ctx, p+uintptr(len(fn)), uintptr(len(s)), p, uintptr(flags)))
	m.tmpFree(p, heap)
	return r, nil
}

// evalSource is tmp of src with the 0 byte JS_Eval reads up to, and of the
// file name "<eval>" before it. src is at p+len(evalFN).
func (m *VM) evalSource(src string) (p, sp uintptr, heap bool) {
	n := uintptr(len(evalFN) + len(src) + 1)
	p, sp, heap = m.tmp(n)
	if p != 0 {
		b := bytesAt(p, n)
		copy(b, evalFN)
		copy(b[len(evalFN):], src)
		b[n-1] = 0
	}
	return p, sp, heap
}

// jsEval is JS_Eval of src, with the file name "<eval>". err is not nil when
// there is no memory for the source.
func (m *VM) jsEval(src string, flags int32) (r jsValue, err error) {
	if m.entered == 0 {
		m.outer(func() { r, err = m.jsEval(src, flags) })
		return r, err
	}
	p, sp, heap := m.evalSource(src)
	if p == 0 {
		return jsValue{}, m.tmpError()
	}
	r = val(cengine.JS_Eval(sp, m.ctx, p+uintptr(len(evalFN)), uintptr(len(src)), p, uintptr(flags)))
	m.tmpFree(p, heap)
	return r, nil
}

// jsEvalThis is JS_EvalThis, as jsEval.
func (m *VM) jsEvalThis(this jsValue, src string, flags int32) (r jsValue, err error) {
	if m.entered == 0 {
		m.outer(func() { r, err = m.jsEvalThis(this, src, flags) })
		return r, err
	}
	p, sp, heap := m.evalSource(src)
	if p == 0 {
		return jsValue{}, m.tmpError()
	}
	r = val(cengine.JS_EvalThis(sp, m.ctx, this.u, this.tag, p+uintptr(len(evalFN)), uintptr(len(src)), p, uintptr(flags)))
	m.tmpFree(p, heap)
	return r, nil
}

func (m *VM) jsEvalFunction(v jsValue) (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsEvalFunction(v) })
		return r
	}
	return val(cengine.JS_EvalFunction(m.sp, m.ctx, v.u, v.tag))
}

func (m *VM) jsResolveModule(v jsValue) (r int32) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsResolveModule(v) })
		return r
	}
	return int32(cengine.JS_ResolveModule(m.sp, m.ctx, v.u, v.tag))
}

// setImportMeta is js_module_set_import_meta of quickjs-libc.c, which the
// engine does not have, with no URL and the module as main: import.meta.url
// is "file://" and the module's name, and import.meta.main is true. As there,
// what fails is left pending.
func (m *VM) setImportMeta(v jsValue) {
	if m.entered == 0 {
		m.outer(func() { m.setImportMeta(v) })
		return
	}
	name := cengine.JS_GetModuleName(m.sp, m.ctx, v.u)
	s := val(cengine.JS_AtomToString(m.sp, m.ctx, name))
	cengine.JS_FreeAtom(m.sp, m.ctx, name)
	if isException(s) {
		return
	}
	url, ok := m.toGoString(s, true)
	m.jsFreeValue(s)
	if !ok {
		return
	}
	if !containsByte(url, ':') {
		url = "file://" + url
	}
	meta := val(cengine.JS_GetImportMeta(m.sp, m.ctx, v.u))
	if isException(meta) {
		return
	}
	defer m.jsFreeValue(meta)
	u, err := m.jsNewStringLen(url)
	if err != nil {
		return
	}
	m.defineStr(meta, "url", u)
	m.defineStr(meta, "main", newBool(true))
}

// defineStr is JS_DefinePropertyValueStr of the property name, which takes v.
func (m *VM) defineStr(obj jsValue, name string, v jsValue) {
	p, sp, heap := m.tmpString(name, true)
	if p == 0 {
		m.jsFreeValue(v)
		return
	}
	cengine.JS_DefinePropertyValueStr(sp, m.ctx, obj.u, obj.tag, p, v.u, v.tag, propCWE)
	m.tmpFree(p, heap)
}

func containsByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

func (m *VM) jsPromiseState(v jsValue) (r int32) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsPromiseState(v) })
		return r
	}
	return int32(cengine.JS_PromiseState(m.sp, m.ctx, v.u, v.tag))
}

func (m *VM) jsPromiseResult(v jsValue) (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsPromiseResult(v) })
		return r
	}
	return val(cengine.JS_PromiseResult(m.sp, m.ctx, v.u, v.tag))
}

// jsExecutePendingJob is JS_ExecutePendingJob, which does not report the
// context of the job: a VM has one.
func (m *VM) jsExecutePendingJob() (r int32) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsExecutePendingJob() })
		return r
	}
	return int32(cengine.JS_ExecutePendingJob(m.sp, m.rt, 0))
}

// tmpValues is tmp of the values a, copied there, or p 0 for no values.
func (m *VM) tmpValues(a []jsValue) (p, sp uintptr, heap bool, err error) {
	if len(a) == 0 {
		return 0, m.sp, false, nil
	}
	p, sp, heap = m.tmp(sizeofJsValue * uintptr(len(a)))
	if p == 0 {
		return 0, 0, false, m.tmpError()
	}
	copy(unsafe.Slice((*jsValue)(addr(p)), len(a)), a)
	return p, sp, heap, nil
}

// jsCall is JS_Call with the arguments args, which it borrows. err is not
// nil when there is no memory to pass the arguments.
func (m *VM) jsCall(f, this jsValue, args []jsValue) (r jsValue, err error) {
	if m.entered == 0 {
		m.outer(func() { r, err = m.jsCall(f, this, args) })
		return r, err
	}
	argv, sp, heap, err := m.tmpValues(args)
	if err != nil {
		return jsValue{}, err
	}
	r = val(cengine.JS_Call(sp, m.ctx, f.u, f.tag, this.u, this.tag, uintptr(len(args)), argv))
	m.tmpFree(argv, heap)
	return r, nil
}

// jsArrayOfValues is js_array_of (Array.of) of the values a, which it
// borrows. err is not nil when there is no memory to pass the values.
func (m *VM) jsArrayOfValues(a []jsValue) (r jsValue, err error) {
	if m.entered == 0 {
		m.outer(func() { r, err = m.jsArrayOfValues(a) })
		return r, err
	}
	argv, sp, heap, err := m.tmpValues(a)
	if err != nil {
		return jsValue{}, err
	}
	r = val(cengine.ArrayOf(sp, m.ctx, uintptr(len(a)), argv))
	m.tmpFree(argv, heap)
	return r, nil
}

// jsReadObject is JS_ReadObject of bytecode, with the flags flags, which
// copies what it keeps. err is not nil when there is no memory to pass the
// bytecode.
func (m *VM) jsReadObject(bytecode []byte, flags int32) (r jsValue, err error) {
	if m.entered == 0 {
		m.outer(func() { r, err = m.jsReadObject(bytecode, flags) })
		return r, err
	}
	p, sp, heap := m.tmp(uintptr(len(bytecode)))
	if p == 0 {
		return jsValue{}, m.tmpError()
	}
	copy(bytesAt(p, uintptr(len(bytecode))), bytecode)
	r = val(cengine.JS_ReadObject(sp, m.ctx, p, uintptr(len(bytecode)), uintptr(flags)))
	m.tmpFree(p, heap)
	return r, nil
}

// jsReadShared is JS_ReadObject of the bytes of b, which stay where they
// are: with JS_READ_OBJ_SHARED_DEBUG the functions point into them.
func (m *VM) jsReadShared(b *SharedBytecode, flags int32) (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsReadShared(b, flags) })
		return r
	}
	return val(cengine.JS_ReadObject(m.sp, m.ctx, b.p, uintptr(b.n), uintptr(flags)))
}

// jsWriteObject returns the bytecode of the compiled script v, and false
// when JS_WriteObject threw.
func (m *VM) jsWriteObject(v jsValue) (b []byte, ok bool) {
	if m.entered == 0 {
		m.outer(func() { b, ok = m.jsWriteObject(v) })
		return b, ok
	}
	psize := m.sp
	p := cengine.JS_WriteObject(m.sp+16, m.ctx, psize, v.u, v.tag, writeObjBytecode)
	if p == 0 {
		return nil, false
	}
	b = append([]byte(nil), bytesAt(p, *(*uintptr)(addr(psize)))...)
	cengine.Free(m.sp, m.ctx, p)
	return b, true
}

// freeTopLevel frees the function of the module mod, which has run to its
// end (see engine_lib.go), with JS_FreeModuleTopLevel.
func (m *VM) freeTopLevel(mod jsValue) {
	if m.entered == 0 {
		m.outer(func() { m.freeTopLevel(mod) })
		return
	}
	cengine.JS_FreeModuleTopLevel(m.sp, m.ctx, mod.u)
}

func (m *VM) jsNewPromiseCapability() (promise, resolve, reject jsValue, err error) {
	if m.entered == 0 {
		m.outer(func() { promise, resolve, reject, err = m.jsNewPromiseCapability() })
		return promise, resolve, reject, err
	}
	funcs := m.sp
	promise = val(cengine.JS_NewPromiseCapability(m.sp+2*sizeofJsValue, m.ctx, funcs))
	if isException(promise) {
		return promise, undefined, undefined, nil
	}
	return promise, valueAt(funcs), valueAt(funcs + sizeofJsValue), nil
}

// newStringChars returns a new string of n characters, 16-bit units when wide
// is true, and where its characters are, for the caller to write all of them
// before the string is used (JS_NewStringChars). An empty string has no
// characters to write, and p is 0, as it is for the exception when there is
// no memory.
func (m *VM) newStringChars(n int, wide bool) (r jsValue, p uintptr) {
	if m.entered == 0 {
		m.outer(func() { r, p = m.newStringChars(n, wide) })
		return r, p
	}
	if n < 0 || n > stringLenMax {
		panic("slowjs: newStringChars of a length the engine cannot hold")
	}
	w := uintptr(0)
	if wide {
		w = 1
	}
	r = val(cengine.JS_NewStringChars(m.sp, m.ctx, uintptr(n), w))
	if n == 0 || isException(r) {
		return r, 0
	}
	return r, r.u + cengine.StringChars
}

// jsNewStringLen is JS_NewStringLen of the UTF-8 s. err is not nil when
// there is no memory to pass s.
func (m *VM) jsNewStringLen(s string) (r jsValue, err error) {
	if m.entered == 0 {
		m.outer(func() { r, err = m.jsNewStringLen(s) })
		return r, err
	}
	p, sp, heap := m.tmpString(s, false)
	if p == 0 {
		return jsValue{}, m.tmpError()
	}
	r = val(cengine.JS_NewStringLen(sp, m.ctx, p, uintptr(len(s))))
	m.tmpFree(p, heap)
	return r, nil
}

// toGoString converts the JS value v to a Go string, by JS_ToCStringLen2 as
// the engine of lib does (see engine_lib.go). ok is false, and the exception
// pending, when the conversion threw.
func (m *VM) toGoString(v jsValue, wtf8 bool) (s string, ok bool) {
	if m.entered == 0 {
		m.outer(func() { s, ok = m.toGoString(v, wtf8) })
		return s, ok
	}
	plen := m.sp
	p := cengine.JS_ToCStringLen2(m.sp+16, m.ctx, plen, v.u, v.tag, 0)
	if p == 0 {
		return "", false
	}
	b := bytesAt(p, *(*uintptr)(addr(plen)))
	if wtf8 {
		s = string(b)
	} else {
		s = wellFormed(b)
	}
	cengine.JS_FreeCString(m.sp, m.ctx, p)
	return s, true
}

// The kinds of error jsThrowError throws.
const (
	errorKindType = iota
	errorKindRange
	errorKindSyntax
	errorKindReference
	errorKindInternal
)

// errorKinds are the JSErrorKind of each kind (engine/quickjs.h).
var errorKinds = [...]uintptr{
	errorKindType:      4, // JS_ERROR_KIND_TYPE
	errorKindRange:     1, // JS_ERROR_KIND_RANGE
	errorKindSyntax:    3, // JS_ERROR_KIND_SYNTAX
	errorKindReference: 2, // JS_ERROR_KIND_REFERENCE
	errorKindInternal:  6, // JS_ERROR_KIND_INTERNAL
}

// maxErrorMessage is the most bytes of a message JS_ThrowErrorKind keeps.
const maxErrorMessage = 255

// jsThrowError throws a new error of the kind with the message msg, which
// the engine cuts to 255 bytes, and returns the exception.
func (m *VM) jsThrowError(kind int, msg string) (r jsValue) {
	if m.entered == 0 {
		m.outer(func() { r = m.jsThrowError(kind, msg) })
		return r
	}
	if kind < 0 || kind >= len(errorKinds) {
		panic(fmt.Sprintf("slowjs: an error of kind %d", kind))
	}
	msg = validUTF8(msg)
	if len(msg) > maxErrorMessage {
		msg = msg[:maxErrorMessage]
	}
	p, sp, heap := m.tmpString(msg, false)
	if p == 0 {
		return exception
	}
	r = val(cengine.JS_ThrowErrorKind(sp, m.ctx, errorKinds[kind], p, uintptr(len(msg))))
	m.tmpFree(p, heap)
	return r
}
