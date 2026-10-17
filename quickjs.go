// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate go run generator.go

// Package quickjs is a pure Go embeddable Javascript engine. It supports the
// ECMA script 14 ([ES2023]) specification including modules, asynchronous
// generators, proxies and BigInt.
//
// See also the original [C QuickJS] library.
//
// # Internationalization
//
// The engine has no Intl. ECMA-402 is not implemented by the C library this
// package wraps, so a page or bundle reaching for Intl.DateTimeFormat gets a
// ReferenceError, and the toLocale* methods it ships ignore both their locale
// and their timeZone argument. Package [github.com/liustack/quickjs/intl] provides
// Intl.DateTimeFormat and Intl.NumberFormat, en-US over Go's time zone
// database, for a VM that opts in by calling its Register.
//
// # Performance
//
// Geomeans of time/op over a set of benchmarks, relative to CCGO, lower number
// is better. Detailed results available in the testdata/benchmarks directory.
//
//	 CCGO: github.com/liustack/quickjs/lib
//	 GOJA: github.com/dop251/goja@v0.0.0-20260723142020-b4aef50fa347
//	  QJS: github.com/fastschema/qjs@v0.0.6
//
//		                        CCGO     GOJA     QJS
//		-----------------------------------------------
//		        darwin/amd64    1.000    1.575    1.300
//		        darwin/arm64    1.000    1.069    1.080
//		       freebsd/amd64    1.000    1.943    1.472    (qemu)
//		       freebsd/arm64    1.000    1.803    1.030    (qemu)
//		           linux/386    1.000    3.082   95.312    (qemu)
//		         linux/amd64    1.000    1.419    1.213
//		           linux/arm    1.000    3.859  139.339
//		         linux/arm64    1.000    1.611    1.267
//		       linux/loong64    1.000    1.555   85.539
//		       linux/ppc64le    1.000    2.286   75.183
//		       linux/riscv64    1.000    1.656   76.609
//		         linux/s390x    1.000    2.016   68.871    (qemu)
//		       windows/amd64    1.000    2.254    1.438
//		       windows/arm64    1.000    2.002    1.610
//		-----------------------------------------------
//		                        CCGO     GOJA     QJS
//
// # Notes
//
// Parts of the documentation were copied from the quickjs documentation, see
// LICENSE-QUICKJS for details.
//
// # Supported platforms and architectures
//
// These combinations of GOOS and GOARCH are currently supported
//
//	OS      Arch
//	-------------
//	darwin  amd64
//	darwin  arm64
//	freebsd amd64
//	freebsd arm64
//	linux   386
//	linux   amd64
//	linux   arm
//	linux   arm64
//	linux   loong64
//	linux   ppc64le
//	linux   riscv64
//	linux   s390x
//	windows amd64
//	windows arm64
//
// # Builders
//
// Builder results are available [here].
//
// # Change Log
//
// 2026-07-27: Upgrade to QuickJS release 2026-06-04. The bytecode format
// changed, code produced by [VM.Compile] using an older release will no longer
// load.
//
// 2025-10-10: Upgrade to QuickJS release 2025-09-13.
//
// [C QuickJS]: https://bellard.org/quickjs
// [ES2023]: https://tc39.es/ecma262/2023/
// [here]: https://modern-c.appspot.com/-/builder/?importpath=modernc.org%2fquickjs
package quickjs // import "github.com/liustack/quickjs"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
	"unsafe"

	lib "github.com/liustack/quickjs/lib"
	"modernc.org/libc"
)

var (
	_ json.Marshaler = (*Object)(nil)
	_ json.Marshaler = (*Value)(nil)

	staticMagic atomic.Int32

	goFuncsMu sync.Mutex
	goFuncs   = map[int32]*goFunc{}
	freeMagic []int32

	customLoadersMu sync.Mutex
	customLoaders   = map[uintptr]moduleLoaderState{}

	// UndefinedValue is a Value representing Javascript value 'undefined'. It is
	// not associated with any particular VM.
	UndefinedValue = Value{v: undefined}

	// callbackClassID is a global JS class ID for callback objects whose
	// lifetime is tied to a JS object's refcount. Each VM registers this
	// class ID once with its runtime. The class carries both an Fcall
	// handler (so the object is callable from JS) and an Ffinalizer (so the
	// goFuncs entry is released when the last JS reference drops).
	callbackClassID     lib.TJSClassID
	initCallbackClassID sync.Once
)

// runtime represents a Javascript runtime.
//
// Note: runtime is not safe for concurrent use by multiple goroutines.
type runtime struct {
	cRuntime          uintptr // lib.TJSRuntime
	tls               *libc.TLS
	callbackClassInit bool
}

// newRuntime returns a newly create Runtime
func newRuntime() (*runtime, error) {
	tls := libc.NewTLS()
	r := lib.XJS_NewRuntime(tls)
	if r == 0 {
		tls.Close()
		return nil, fmt.Errorf("failed to create a new Javascript runtime")
	}

	return &runtime{
		cRuntime: r,
		tls:      tls,
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
		return nil, fmt.Errorf("failed to create a new Javascript context")
	}

	return &VM{
		intr: &interrupter{flag: interruptData},
		vmState: vmState{
			cContext:      c,
			int32_16:      lib.XNewInt32(r.tls, c, 16),
			int32_2:       lib.XNewInt32(r.tls, c, 2),
			interruptData: interruptData,
			runtime:       r,
			toStringArgv:  argv,
		},
	}, nil
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

// NewInt returns a new Value from 'n'.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) NewInt(n int) Value {
	if n >= math.MinInt32 && n <= math.MaxInt32 {
		return Value{vm: m, v: newInt32(int32(n))}
	}

	return m.NewFloat64(float64(n))
}

// NewFloat64 returns a new Value from 'n'.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) NewFloat64(n float64) Value {
	return Value{vm: m, v: newFloat(n)}
}

// NewString returns a new Value from 's'.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) NewString(s string) (r Value, err error) {
	v, err := m.newString(s)
	if err != nil {
		return r, err
	}

	return Value{vm: m, v: v}, nil
}

// NewObjectValue returns a new Value representing '{}'.
func (m *VM) NewObjectValue() (r Value, err error) {
	return Value{vm: m, v: lib.XJS_NewObject(m.runtime.tls, m.cContext)}, nil
}

func (m *VM) newString(s string) (r lib.TJSValue, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	s = validUTF8(s)
	p, err := libc.CString(s)
	if err != nil {
		return r, err
	}

	defer libc.Xfree(tls, p)

	return lib.XJS_NewStringLen(tls, ctx, p, lib.Tsize_t(len(s))), nil
}

func jsInt(n int64) lib.TJSValue {
	switch {
	case n >= math.MinInt32 && n <= math.MaxInt32:
		return newInt32(int32(n))
	default:
		return newFloat(float64(n))
	}
}

func jsUint(n uint64) lib.TJSValue {
	switch {
	case n <= math.MaxInt32:
		return newInt32(int32(n))
	default:
		return newFloat(float64(n))
	}
}

var (
	jsonFNC = [...]byte{'J', 'S', 'O', 'N', 0}
	jsonFN  = uintptr(unsafe.Pointer(&jsonFNC[0]))
)

func (m *VM) parseJSON(s string, flags int32) (r lib.TJSValue, err error) {
	tls := m.runtime.tls
	ctx := m.cContext

	cs, err := libc.CString(s)
	if err != nil {
		return r, err
	}

	defer libc.Xfree(tls, cs)

	r = lib.XJS_ParseJSON2(tls, ctx, cs, lib.Tsize_t(len(s)), jsonFN, flags)
	if isException(r) {
		err = m.errFromException()
	}
	return r, err
}

// Unsupported represents an unsupported Javascript value.
type Unsupported struct{}

// String implements fmt.Stringer.
func (u Unsupported) String() string {
	return "unsupported Javascript type"
}

// Undefined represents the Javascript value "undefined".
type Undefined struct{}

// String implements fmt.Stringer.
func (u Undefined) String() string {
	return "undefined"
}

// Error holds structured information from a JavaScript exception. The error
// returned by [VM.Eval], [VM.Call] and the other eval-family methods is an
// *Error; recover the fields with [errors.As]:
//
//	var e *quickjs.Error
//	if errors.As(err, &e) {
//		fmt.Println(e.Stack)
//	}
//
// Its Error method reports the same message string the engine has always
// returned, so existing code comparing the error text keeps working.
//
// Only parse/syntax errors carry the FileName, LineNumber and ColumnNumber
// properties. For runtime errors those fields are zero and the source location
// is available only from Stack.
//
// Uncatchable, OutOfMemory and Interrupted tell an error the engine ended the
// call with from one the script threw, without comparing names or messages,
// which a script can copy:
//
//	if errors.As(err, &e) && e.OutOfMemory {
//		// the script needs a higher SetMemoryLimit
//	}
type Error struct {
	raw string

	Name         string // e.g. "ReferenceError", "TypeError"
	Message      string // the error message
	Stack        string // stack trace, one "    at ...\n" frame per line
	LineNumber   int    // line where the error occurred (parse errors only)
	ColumnNumber int    // column where the error occurred (parse errors only)
	FileName     string // file name where the error occurred (parse errors only)

	// Uncatchable reports that the call ended with an exception no catch
	// or finally block of the script ran for: the VM ran out of memory
	// (OutOfMemory), Interrupt or the eval timeout stopped it
	// (Interrupted), or a Go function that javascript called panicked
	// (neither of the two). Whatever a script throws itself is never
	// uncatchable, whatever its name and message.
	Uncatchable bool
	OutOfMemory bool // the VM ran out of memory (Uncatchable is true)
	Interrupted bool // Interrupt or the eval timeout stopped the script (Uncatchable is true)
}

// Error implements the error interface. It returns the same string the engine
// produces for the exception, unchanged from earlier releases.
func (e *Error) Error() string { return e.raw }

// Object represents the value of a Javascript object, but not the javascript
// object instance itself. Do not compare instances of Object.
type Object struct {
	json string
}

// String implements fmt.Stringer.
func (o *Object) String() string {
	return o.json
}

// MarshalJSON implements encoding/json.Marshaler.
func (o *Object) MarshalJSON() (r []byte, err error) {
	return []byte(o.json), nil
}

// Into helps you quickly deserialize an Object into a native Go type.
// This uses encoding/json.Unmarshal internally, so the target type must be
// compatible with that package.
func (o *Object) Into(v any) error {
	return json.Unmarshal([]byte(o.json), v)
}

// return != 0 if the JS code needs to be interrupted
func interruptHandler(tls *libc.TLS, rt, opaque uintptr) int32 {
	if opaque == 0 {
		return 0
	}

	if atomic.LoadInt32(&(*interruptData)(unsafe.Pointer(opaque)).interrupt) != 0 {
		return 1
	}

	if timeLimit := (*interruptData)(unsafe.Pointer(opaque)).timeLimit; timeLimit != 0 && monoNow() >= timeLimit {
		return 1
	}

	return 0
}

// The eval timeout runs on the monotonic clock, which only goes forward. The
// wall clock can be set back or forward while a script runs (NTP, a person, a
// machine resumed from sleep): set back, a deadline on it fired that much
// later or never, and set forward, at once.
var (
	monoStart = time.Now()
	timeNow   = time.Now // tests replace it
)

// monoNow returns the time since monoStart in nanoseconds, from the
// monotonic readings of time.Now.
func monoNow() int64 {
	return int64(timeNow().Sub(monoStart))
}

// monoDeadline returns the time d from now on the clock of monoNow. A d
// further away than that clock reaches gives its end, which comes after
// 292 years: without the cap the deadline wrapped around to the past.
func monoDeadline(d time.Duration) int64 {
	now := monoNow()
	if int64(d) > math.MaxInt64-now {
		return math.MaxInt64
	}
	return now + int64(d)
}

type interruptData struct {
	interrupt int32 // Interrupt() pending
	timeLimit int64 // stop when monoNow() >= timeLimit, 0 for no limit
}

// VM represents a Javascript context (or Realm). Each VM has its
// own global objects and system objects.
//
// Note: VM is not safe for concurrent use by multiple goroutines, except
// for [VM.Interrupt].
type VM struct {
	// intr is set when the VM is made and never written again, Close
	// included, so that Interrupt can read it from any goroutine.
	intr *interrupter
	vmState
}

// interrupter is how Interrupt reaches the interrupt flag, which lives in C
// memory that Close frees. It lives in Go memory, so it stays valid after
// Close, and its mutex orders Interrupt and Close: the race detector does not
// see the atomic operations on the flag itself, which is outside Go's heap.
type interrupter struct {
	mu   sync.Mutex
	flag uintptr // the interruptData, 0 once Close has started
}

// vmState is the state of a VM that Close clears.
type vmState struct {
	cContext uintptr // lib.TJSContext
	goFuncs  map[string]int32
	// Safe to share, not reference counted
	int32_16      lib.TJSValue
	int32_2       lib.TJSValue
	runtime       *runtime
	timeout       time.Duration
	interruptData uintptr
	toStringArgv  uintptr
	// depth counts the calls into javascript in progress: a host function
	// that evaluates more javascript is one level down.
	depth int
	// host counts the calls of Go functions from javascript in progress.
	host int
	// panicked holds what a Go function called from javascript panicked
	// with, until the panic goes on in the Go code that called javascript
	// (see hostPanicked).
	panicked any
}

// NewVM returns a newly created VM.
func NewVM() (*VM, error) {
	rt, err := newRuntime()
	if err != nil {
		return nil, err
	}

	vm, err := rt.newVM()
	if err != nil {
		rt.close()
		return nil, err
	}

	lib.XJS_SetInterruptHandler(rt.tls, rt.cRuntime, fp(interruptHandler), vm.interruptData)
	return vm, nil
}

// SetEvalTimeout sets the timeout for the various eval functions. Passing
// zero 'd' disables timeouts. The timeout starts with the outermost call into
// javascript: a host function that evaluates more javascript on m runs inside
// the time left to the call that got it there. The timeout runs on the
// monotonic clock, so setting the system clock does not move it. A timeout
// too long for that clock (about 292 years) never fires.
func (m *VM) SetEvalTimeout(d time.Duration) error {
	m.timeout = d
	return nil
}

// Interrupt requests 'm' to interrupt Javascript evaluation. Interrupt
// can be called asynchronously, from any goroutine, at any time m is
// evaluating a script. The request stays pending until the outermost call
// into javascript returns, so javascript a host function evaluates on m
// meanwhile does not clear it. Interrupt is also safe while m closes and after
// Close, where it does nothing, so a watchdog need not know whether the
// script it guards has finished.
func (m *VM) Interrupt() {
	m.intr.mu.Lock()
	if m.intr.flag != 0 {
		atomic.StoreInt32(&(*interruptData)(unsafe.Pointer(m.intr.flag)).interrupt, 1)
	}
	m.intr.mu.Unlock()
}

// enter marks the start of a call into javascript, leave its end. Only the
// outermost call arms the eval timeout and clears a pending Interrupt. A host
// function evaluating more javascript runs inside the budget of the call that
// got it there, and an Interrupt requested meanwhile stays pending.
func (m *VM) enter() {
	if m.depth == 0 {
		m.configureInterrupt()
	}
	m.depth++
}

func (m *VM) leave() {
	m.depth--
	m.repanic()
}

func (m *VM) configureInterrupt() {
	if m.interruptData != 0 {
		atomic.StoreInt32(&(*interruptData)(unsafe.Pointer(m.interruptData)).interrupt, 0)
		if m.timeout != 0 {
			(*interruptData)(unsafe.Pointer(m.interruptData)).timeLimit = monoDeadline(m.timeout)
		} else {
			(*interruptData)(unsafe.Pointer(m.interruptData)).timeLimit = 0
		}
	}
}

// SetMemoryLimit limits m's maxmimum memory usage, in bytes. Small values of
// 'limit' are not honored because the VM needs to allocate memory also for the
// exception object itself.  The particular value of "small" is unspecified and
// subject to change without notice.
func (m *VM) SetMemoryLimit(limit uintptr) {
	lib.XJS_SetMemoryLimit(m.runtime.tls, m.runtime.cRuntime, max(libc.Tsize_t(limit), 128*1024))
}

// SetGCThreshold sets the memory threshold at which a GC will be performed, in bytes.
func (m *VM) SetGCThreshold(threshold uintptr) {
	lib.XJS_SetGCThreshold(m.runtime.tls, m.runtime.cRuntime, libc.Tsize_t(threshold))
}

// SetMaxStackSize limits the JS call depth, in libc.TLS stack slots (~one per JS
// call frame in the CGo-free port). Deep recursion then throws a catchable
// RangeError instead of overflowing the Go goroutine stack. 0 restores the
// MaxStackSlots default.
func (m *VM) SetMaxStackSize(threshold uintptr) {
	lib.XJS_SetMaxStackSize(m.runtime.tls, m.runtime.cRuntime, libc.Tsize_t(threshold))
}

// SetCanBlock configures m's blocking mode.
func (m *VM) SetCanBlock(value bool) {
	n := int32(0)
	if value {
		n = 1
	}
	lib.XJS_SetCanBlock(m.runtime.tls, m.runtime.cRuntime, n)
}

// Close releases the resources held by 'm'. That includes the module loader
// and normalizer set by [VM.SetModuleLoader]: Close removes them, so a closed
// VM keeps nothing they close over reachable.
func (m *VM) Close() error {
	tls := m.runtime.tls
	ctx := m.cContext

	// SetModuleLoader files the loader process-wide under the runtime
	// pointer, which is freed below and may be reused by a later runtime.
	// Drop the entry first, so the closures - and whatever host they
	// capture - do not outlive the VM. Nothing in the teardown loads a module.
	customLoadersMu.Lock()
	delete(customLoaders, m.runtime.cRuntime)
	customLoadersMu.Unlock()

	var freed []*goFunc
	goFuncsMu.Lock()
	for _, magic := range m.goFuncs {
		if gf := goFuncs[magic]; gf != nil {
			delete(goFuncs, magic)
			freeMagic = append(freeMagic, magic)
			freed = append(freed, gf)
		}
	}
	goFuncsMu.Unlock()

	// Free outside the lock: goFuncsMu is process-global and taken by callGo on
	// every JS->Go call, and callbackFinalizer re-enters it, so keep libc calls
	// and anything that can run JS out of its critical section.
	for _, gf := range freed {
		libc.Xfree(tls, gf.cName)
		libc.Xfree(tls, gf.tab)
	}

	// Interrupt can run concurrently: let it see the flag gone before it
	// is freed.
	m.intr.mu.Lock()
	m.intr.flag = 0
	m.intr.mu.Unlock()
	libc.Xfree(tls, m.interruptData)
	libc.Xfree(tls, m.toStringArgv)
	lib.XJS_FreeContext(tls, ctx)
	r := m.runtime
	m.vmState = vmState{}
	return r.close()
}

// Eval flags.
const (
	EvalGlobal = lib.MJS_EVAL_TYPE_GLOBAL // global code
	EvalModule = lib.MJS_EVAL_TYPE_MODULE // module code
)

var evalFN = [...]byte{'<', 'e', 'v', 'a', 'l', '>', 0}

// Eval evaluates a script or module source in 'javascript'.
//
//	Javascript result type  Go result type                          Go result error
//	-------------------------------------------------------------------------------
//	exception               nil                                     non-nil
//	null                    nil                                     nil
//	undefined               Undefined                               nil
//	string                  string                                  nil
//	int                     int                                     nil
//	bool                    bool                                    nil
//	float64                 float64                                 nil
//	BigInt                  *math/big.Int                           nil
//	object                  *Object                                 nil
//	any other type          Unsupported                             nil
func (m *VM) Eval(javascript string, flags int) (r any, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	ps, err := libc.CString(javascript)
	if err != nil {
		return nil, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, ps)

	m.enter()
	defer m.leave()
	return m.value(m.settled(lib.XJS_Eval(tls, ctx, ps, libc.Tsize_t(len(javascript)), uintptr(unsafe.Pointer(&evalFN)), int32(flags))), true)
}

// ExecutePendingJobs runs queued Javascript jobs — Promise reactions (the
// .then/.catch/.finally callbacks) and async/await continuations — until the
// queue is empty, returning the number of jobs executed. The runtime does not run
// these itself: an embedding event loop calls ExecutePendingJobs at the end of
// each turn to drain the microtask queue (so a resolved Promise's .then runs, a
// pending await resumes, etc.). An exception thrown by a job is returned as an
// error — the first one — while the remaining jobs still run; a self-feeding job
// queue is bounded so it cannot hang the host.
func (m *VM) ExecutePendingJobs() (n int, err error) {
	tls := m.runtime.tls
	rt := m.runtime.cRuntime

	// pctx receives the context of a throwing job; with a single context per VM its
	// value is not otherwise needed. It is a pointer to one JSContext*.
	pctx := libc.Xcalloc(tls, 1, libc.Tsize_t(unsafe.Sizeof(uintptr(0))))
	if pctx == 0 {
		return 0, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, pctx)

	// Re-arm the eval-timeout deadline, as the Eval* functions do. Draining runs JS
	// (Promise reactions and async continuations), so without this the whole drain
	// inherits a stale deadline from the last Eval — one that has kept
	// ticking through idle time — and is spuriously interrupted.
	m.enter()
	defer m.leave()

	const maxJobs = 1 << 20 // bound a self-feeding microtask loop so it cannot hang the host
	for n < maxJobs {
		r := lib.XJS_ExecutePendingJob(tls, rt, pctx)
		if r == 0 {
			return n, err // queue drained
		}

		n++
		if r > 0 && m.uncatchablePending() {
			r = -1 // the job stopped at an uncatchable exception without failing
		}
		if r < 0 {
			e, uncatchable := m.takeException()
			if uncatchable {
				// An interrupt or out of memory ends the drain: the
				// jobs left are page code that must not run now.
				return n, e
			}
			if err == nil {
				err = e // report the first failure, keep draining the rest
			}
		}
	}

	if err == nil {
		err = fmt.Errorf("quickjs: aborted draining after %d pending jobs", maxJobs)
	}
	return n, err
}

// EvalThis is like eval but sets javascript 'this' to 'obj'.
func (m *VM) EvalThis(obj Value, javascript string, flags int) (r any, err error) {
	defer obj.Free()

	tls := m.runtime.tls
	ctx := m.cContext
	ps, err := libc.CString(javascript)
	if err != nil {
		return nil, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, ps)

	m.enter()
	defer m.leave()
	return m.value(m.settled(lib.XJS_EvalThis(tls, ctx, obj.v, ps, libc.Tsize_t(len(javascript)), uintptr(unsafe.Pointer(&evalFN)), int32(flags))), true)
}

// EvalValue evaluates a script or module source in 'javascript' and returns
// the resulting Value, or an error, if any.
//
// Exceptions thrown during evaluation of the script are returned as Go errors.
//
// If no error is returned, the caller must properly handle the returned Value
// using Dup/Free.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) EvalValue(javascript string, flags int) (r Value, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	ps, err := libc.CString(javascript)
	if err != nil {
		return r, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, ps)

	m.enter()
	defer m.leave()
	v := m.settled(lib.XJS_Eval(tls, ctx, ps, libc.Tsize_t(len(javascript)), uintptr(unsafe.Pointer(&evalFN)), int32(flags)))
	if isException(v) {
		lib.XFreeValue(tls, ctx, v)
		return r, m.errFromException()
	}

	return Value{vm: m, v: v}, nil
}

// EvalThisValue is like EvalValue but sets javascript 'this' to obj.
func (m *VM) EvalThisValue(obj Value, javascript string, flags int) (r Value, err error) {
	defer obj.Free()

	tls := m.runtime.tls
	ctx := m.cContext
	ps, err := libc.CString(javascript)
	if err != nil {
		return r, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, ps)

	m.enter()
	defer m.leave()
	v := m.settled(lib.XJS_EvalThis(tls, ctx, obj.v, ps, libc.Tsize_t(len(javascript)), uintptr(unsafe.Pointer(&evalFN)), int32(flags)))
	if isException(v) {
		lib.XFreeValue(tls, ctx, v)
		return r, m.errFromException()
	}

	return Value{vm: m, v: v}, nil
}

// EvalBytecode is like Eval but uses QuickJS bytecode.
//
// Note: The bytecode format is linked to a given QuickJS version. Moreover, no security check is done before
// its execution. Hence the bytecode should not be loaded from untrusted sources.
//
// Note: Javascript 'this' is always set to the global context.
func (m *VM) EvalBytecode(bytecode []byte) (r any, err error) {
	v, err := m.EvalBytecodeValue(bytecode)
	if err != nil {
		return nil, err
	}

	return m.value(v.v, true)
}

// EvalBytecodeValue is like EvalValue but operates on QuickJS bytecode.
//
// Exceptions thrown during evaluation of the script are returned as Go errors.
//
// If no error is returned, the caller must properly handle the returned Value
// using Dup/Free.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) EvalBytecodeValue(bytecode []byte) (r Value, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	ps, err := libc.CString(string(bytecode))
	if err != nil {
		return r, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, ps)

	// https://github.com/bellard/quickjs/blob/eb2c89087def1829ed99630cb14b549d7a98408c/quickjs-libc.c#L4292
	obj := lib.XJS_ReadObject(tls, ctx, ps, lib.Tsize_t(len(bytecode)), lib.MJS_READ_OBJ_BYTECODE)
	if isException(obj) {
		return r, m.errFromException()
	}

	m.enter()
	defer m.leave()

	var v lib.TJSValue
	if tag(obj) == lib.EJS_TAG_MODULE {
		if lib.XJS_ResolveModule(tls, ctx, obj) < 0 {
			lib.XFreeValue(tls, ctx, obj)
			return r, m.errFromException()
		}

		lib.Xjs_module_set_import_meta(tls, ctx, obj, 0, 1)
		// 'this' is set to ctx->global_obj
		// see https://github.com/bellard/quickjs/blob/eb2c89087def1829ed99630cb14b549d7a98408c/quickjs.c#L36045
		v = lib.XJS_EvalFunction(tls, ctx, obj)
		v = lib.Xjs_std_await(tls, ctx, v)
	} else {
		v = lib.XJS_EvalFunction(tls, ctx, obj)
	}
	v = m.settled(v)

	if isException(v) {
		lib.XFreeValue(tls, ctx, v)
		return r, m.errFromException()
	}

	return Value{vm: m, v: v}, nil
}

// Compile compiles the bytecode representation of the passed script.
//
// The returned bytecode can be later executed using EvalBytecode or EvalBytecodeValue.
//
// Note: The bytecode format is linked to a given QuickJS version.
func (m *VM) Compile(javascript string, flags int) ([]byte, error) {
	tls := m.runtime.tls
	ctx := m.cContext
	// https://github.com/bellard/quickjs/blob/eb2c89087def1829ed99630cb14b549d7a98408c/qjsc.c#L346-L360

	flags |= lib.MJS_EVAL_FLAG_COMPILE_ONLY

	v, err := m.eval(javascript, flags)
	if err != nil {
		return nil, err
	}

	defer lib.XFreeValue(tls, ctx, v)

	if isException(v) {
		return nil, m.errFromException()
	}

	return m.writeBytecode(v)
}

// writeBytecode returns the bytecode of the compiled script v.
func (m *VM) writeBytecode(v lib.TJSValue) ([]byte, error) {
	tls := m.runtime.tls
	ctx := m.cContext
	// https://github.com/bellard/quickjs/blob/eb2c89087def1829ed99630cb14b549d7a98408c/qjsc.c#L193
	flags2 := int32(lib.MJS_WRITE_OBJ_BYTECODE)
	usize := int(unsafe.Sizeof(libc.Tsize_t(0)))
	plen := tls.Alloc(usize)
	defer tls.Free(usize)

	p := lib.XJS_WriteObject(tls, ctx, plen, v, flags2)
	if p == 0 {
		return nil, m.errFromException()
	}

	plen2 := int(*(*libc.Tsize_t)(unsafe.Pointer(plen)))
	b := libc.GoBytes(p, plen2)
	bcopy := make([]byte, len(b))
	copy(bcopy, b)

	lib.Xjs_free(tls, ctx, p)
	return bcopy, nil
}

func (m *VM) eval(js string, flags int) (r lib.TJSValue, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	ps, err := libc.CString(js)
	if err != nil {
		return r, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, ps)

	m.enter()
	defer m.leave()
	return m.settled(lib.XJS_Eval(tls, ctx, ps, libc.Tsize_t(len(js)), uintptr(unsafe.Pointer(&evalFN)), int32(flags))), nil
}

// GlobalObject returns m's global object.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) GlobalObject() (r Value) {
	return Value{vm: m, v: m.globalObject()}
}

func (m *VM) globalObject() lib.TJSValue {
	return lib.XJS_GetGlobalObject(m.runtime.tls, m.cContext)
}

// Call evaluates 'function(args...)' and returns the resulting (value, error).
//
// Argument types must be one of:
//
//	Go argument type                        Javascript argument type
//	----------------------------------------------------------------
//	nil                                     null
//	Undefined                               undefined
//	string                                  string
//	int*/uint* (value in int32 range)       int
//	int*/uint* (value out of int32 range)   float
//	bool                                    bool
//	float64                                 float64
//	*math/big.Int                           BigInt
//	*Object                                 object
//	Value                                   native Javascript Value
//	any other type                          object from JSON produced by encoding.json/Marshall(arg)
func (m *VM) Call(function string, args ...any) (r any, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	ps, err := libc.CString(function)
	if err != nil {
		return nil, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, ps)

	// Re-arm the eval-timeout deadline, as the Eval* functions do: Call runs JS (it
	// evaluates the function reference and invokes it), so without this the
	// deadline set by the last Eval keeps ticking through idle time and interrupts a
	// later Call that has not itself run long.
	m.enter()
	defer m.leave()

	f := m.settled(lib.XJS_Eval(tls, ctx, ps, libc.Tsize_t(len(function)), uintptr(unsafe.Pointer(&evalFN)), int32(EvalGlobal)))

	if isException(f) {
		// Evaluating the function expression threw, eg. out of memory or an
		// unresolvable name. Report that exception, not a non-function.
		return nil, m.errFromException()
	}

	defer lib.XFreeValue(tls, ctx, f)

	g := m.globalObject()

	defer lib.XFreeValue(tls, ctx, g)

	return m.call(f, g, args...)
}

// CallValue is like Call but returns (Value, error) like EvalValue
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) CallValue(function string, args ...any) (r Value, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	ps, err := libc.CString(function)
	if err != nil {
		return r, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, ps)

	// Re-arm the eval-timeout deadline, as the Eval* functions do (see Call).
	m.enter()
	defer m.leave()

	f := m.settled(lib.XJS_Eval(tls, ctx, ps, libc.Tsize_t(len(function)), uintptr(unsafe.Pointer(&evalFN)), int32(EvalGlobal)))

	if isException(f) {
		return r, m.errFromException()
	}

	defer lib.XFreeValue(tls, ctx, f)

	g := m.globalObject()

	defer lib.XFreeValue(tls, ctx, g)

	return m.callValue(f, g, args...)
}

func (m *VM) convertArgs(goArgs ...any) (jsArgs, free []lib.TJSValue, err error) {
	tls := m.runtime.tls
	ctx := m.cContext

	defer func() {
		if err != nil && len(free) != 0 {
			for _, v := range free {
				lib.XFreeValue(tls, ctx, v)
			}
			free = nil
		}
	}()

	for _, v := range goArgs {
		switch x := v.(type) {
		case nil:
			jsArgs = append(jsArgs, null)
		case Value:
			if x.vm != m && tag(x.v) < 0 {
				return nil, free, fmt.Errorf("cannot use a Value from a different VM")
			}

			dup := x.Dup().v
			free = append(free, dup)
			jsArgs = append(jsArgs, dup)
		case Undefined:
			jsArgs = append(jsArgs, undefined)
		case int8:
			jsArgs = append(jsArgs, newInt32(int32(x)))
		case uint8:
			jsArgs = append(jsArgs, newInt32(int32(x)))
		case int16:
			jsArgs = append(jsArgs, newInt32(int32(x)))
		case uint16:
			jsArgs = append(jsArgs, newInt32(int32(x)))
		case int32:
			jsArgs = append(jsArgs, newInt32(int32(x)))
		case uint32:
			switch {
			case x <= math.MaxInt32:
				jsArgs = append(jsArgs, newInt32(int32(x)))
			default:
				jsArgs = append(jsArgs, newFloat(float64(x)))
			}
		case int:
			switch {
			case x >= math.MinInt32 && x <= math.MaxInt32:
				jsArgs = append(jsArgs, newInt32(int32(x)))
			default:
				jsArgs = append(jsArgs, newFloat(float64(x)))
			}
		case uint:
			switch {
			case x <= math.MaxInt32:
				jsArgs = append(jsArgs, newInt32(int32(x)))
			default:
				jsArgs = append(jsArgs, newFloat(float64(x)))
			}
		case int64:
			switch {
			case x >= math.MinInt32 && x <= math.MaxInt32:
				jsArgs = append(jsArgs, newInt32(int32(x)))
			default:
				jsArgs = append(jsArgs, newFloat(float64(x)))
			}
		case uint64:
			switch {
			case x <= math.MaxInt32:
				jsArgs = append(jsArgs, newInt32(int32(x)))
			default:
				jsArgs = append(jsArgs, newFloat(float64(x)))
			}
		case bool:
			jsArgs = append(jsArgs, newBool(x))
		case string:
			x = validUTF8(x)
			p, err := libc.CString(x)
			if err != nil {
				return nil, free, err
			}

			defer libc.Xfree(tls, p)

			jv := lib.XJS_NewStringLen(tls, ctx, p, libc.Tsize_t(len(x)))
			free = append(free, jv)
			jsArgs = append(jsArgs, jv)
		case *big.Int:
			s := x.String() + "n"
			jv, err := m.eval(s, EvalGlobal)
			if err != nil {
				return nil, free, err
			}

			free = append(free, jv)
			jsArgs = append(jsArgs, jv)
		default:
			b, err := json.Marshal(x)
			if err != nil {
				return nil, free, err
			}

			p, err := libc.CString(string(b))
			if err != nil {
				return nil, free, err
			}

			defer libc.Xfree(tls, p)

			jv := lib.XJS_ParseJSON(tls, ctx, p, libc.Tsize_t(len(b)), uintptr(unsafe.Pointer(&evalFN)))
			free = append(free, jv)
			jsArgs = append(jsArgs, jv)
		}
	}
	return jsArgs, free, nil
}

// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) callValue(f, this lib.TJSValue, args ...any) (r Value, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	if lib.XJS_IsFunction(tls, ctx, f) == 0 {
		return r, fmt.Errorf("cannot call a non-function")
	}

	jsArgs, free, err := m.convertArgs(args...)
	if err != nil {
		return r, err
	}

	if len(free) != 0 {
		defer func() {
			for _, v := range free {
				lib.XFreeValue(tls, ctx, v)
			}
		}()
	}
	var argv uintptr
	if len(jsArgs) != 0 {
		sz := libc.Tsize_t(sizeofJsValue * uintptr(len(jsArgs)))
		argv = libc.Xmalloc(tls, sz)

		defer libc.Xfree(tls, argv)

		for i, v := range jsArgs {
			unsafe.Slice((*lib.TJSValue)(unsafe.Pointer(argv)), len(jsArgs))[i] = v
		}

	}
	v := m.settled(lib.XJS_Call(tls, ctx, f, this, int32(len(jsArgs)), argv))
	if isException(v) {
		lib.XFreeValue(tls, ctx, v)
		return r, m.errFromException()
	}

	return Value{vm: m, v: v}, nil
}

func (m *VM) call(f, this lib.TJSValue, args ...any) (r any, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	if lib.XJS_IsFunction(tls, ctx, f) == 0 {
		return nil, fmt.Errorf("cannot call a non-function")
	}

	jsArgs, free, err := m.convertArgs(args...)
	if err != nil {
		return nil, err
	}

	if len(free) != 0 {
		defer func() {
			for _, v := range free {
				lib.XFreeValue(tls, ctx, v)
			}
		}()
	}
	var argv uintptr
	if len(jsArgs) != 0 {
		sz := libc.Tsize_t(sizeofJsValue * uintptr(len(jsArgs)))
		argv = libc.Xmalloc(tls, sz)

		defer libc.Xfree(tls, argv)

		for i, v := range jsArgs {
			unsafe.Slice((*lib.TJSValue)(unsafe.Pointer(argv)), len(jsArgs))[i] = v
		}

	}
	return m.value(m.settled(lib.XJS_Call(tls, ctx, f, this, int32(len(jsArgs)), argv)), true)
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
	// The out-parameter lives on the emulated C stack, never the Go stack: the
	// transpiled callee holds it as a uintptr, which a Go stack move would leave
	// dangling.
	usize := int(unsafe.Sizeof(libc.Tsize_t(0)))
	plen := tls.Alloc(usize)

	defer tls.Free(usize)

	p := lib.XJS_ToCStringLen2(tls, ctx, plen, v, 0)
	if p == 0 {
		return "", false
	}

	defer lib.XJS_FreeCString(tls, ctx, p)

	return wellFormed(unsafe.Slice((*byte)(unsafe.Pointer(p)), *(*libc.Tsize_t)(unsafe.Pointer(plen)))), true
}

// wellFormed returns the UTF-8 b as a Go string with U+FFFD for each lone
// surrogate, as Go's utf16.Decode, TextEncoder and Node's Buffer make a string
// of UTF-16 valid. JS_ToCStringLen2 joins the halves of a surrogate pair into
// one character, but writes a lone surrogate as the three bytes of its code
// point, 0xED then 0xA0 to 0xBF, which is not UTF-8. They are the only bytes
// it writes in that form, and U+FFFD takes three bytes too.
func wellFormed(b []byte) string {
	var fixed []byte
	for i := 0; i < len(b); {
		j := bytes.IndexByte(b[i:], 0xED)
		if j < 0 {
			break
		}
		i += j
		if i+2 < len(b) && b[i+1] >= 0xA0 {
			if fixed == nil {
				fixed = bytes.Clone(b)
			}
			copy(fixed[i:], "\uFFFD")
		}
		i++
	}
	if fixed == nil {
		return string(b)
	}
	return unsafe.String(unsafe.SliceData(fixed), len(fixed))
}

// validUTF8 returns s with U+FFFD for each byte that does not start a valid
// UTF-8 sequence, the Go -> JS counterpart of wellFormed. That is how Go
// itself decodes a string (for range, []rune, utf16.Encode) and how
// encoding/json writes one, so a Go string reaches javascript the same way
// whether it is passed as it is or inside a struct, which goes as JSON. The
// engine's JS_NewStringLen skipped the bytes after an invalid one, ASCII
// included ("\x80a" became a lone U+FFFD), and took the three bytes of a
// surrogate (0xED 0xA0 to 0xBF) as that surrogate. A sequence cut short
// ("\xe4\xb8") gives one U+FFFD per byte here, where a browser's TextDecoder
// gives one for the sequence.
func validUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}

	b := make([]byte, 0, len(s)+2*utf8.UTFMax)
	for _, r := range s {
		b = utf8.AppendRune(b, r) // an invalid byte comes as utf8.RuneError
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// cGoString returns the NUL-terminated C string p, which the engine wrote
// from a JS string, as a Go string with U+FFFD for each lone surrogate, like
// cString.
func cGoString(tls *libc.TLS, p uintptr) string {
	return wellFormed(unsafe.Slice((*byte)(unsafe.Pointer(p)), libc.Xstrlen(tls, p)))
}

// goString converts the JS value 'v' to a Go string. When the conversion
// throws, it takes the exception out of the context and returns it as the
// error, for example the engine's out of memory error.
func (m *VM) goString(v lib.TJSValue) (string, error) {
	s, ok := cString(m.runtime.tls, m.cContext, v)
	if !ok {
		return "", m.errFromException()
	}

	return s, nil
}

// errString converts the JS value 'v' to a Go string for ErrorFromValue, which
// has no error result, and which errFromException calls, so it cannot turn a
// failure into an error the way goString does without recursing. When the
// conversion throws, errString drops that new exception and returns "": the
// exception being read is the one to report, and a second one left pending
// would be found by the next JS_GetException of an unrelated call.
func errString(tls *libc.TLS, ctx uintptr, v lib.TJSValue) string {
	s, ok := cString(tls, ctx, v)
	if !ok {
		dropException(tls, ctx)
	}

	return s
}

// dropException frees the exception pending in 'ctx'.
func dropException(tls *libc.TLS, ctx uintptr) {
	lib.XFreeValue(tls, ctx, lib.XJS_GetException(tls, ctx))
}

func (m *VM) newObject(v lib.TJSValue) (r *Object, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	json := lib.XJS_JSONStringify(tls, ctx, v, undefined, undefined)

	defer lib.XFreeValue(tls, ctx, json)

	if isException(json) {
		return nil, m.errFromException()
	}

	s, err := m.goString(json)
	if err != nil {
		return nil, err
	}

	return &Object{json: s}, nil
}

var (
	toStringC = [...]byte{'t', 'o', 'S', 't', 'r', 'i', 'n', 'g', 0}
	toString  = uintptr(unsafe.Pointer(&toStringC[0]))

	// C string property names, read by ErrorFromValue.
	messageC  = [...]byte{'m', 'e', 's', 's', 'a', 'g', 'e', 0}
	nameC     = [...]byte{'n', 'a', 'm', 'e', 0}
	stackC    = [...]byte{'s', 't', 'a', 'c', 'k', 0}
	lineNumC  = [...]byte{'l', 'i', 'n', 'e', 'N', 'u', 'm', 'b', 'e', 'r', 0}
	colNumC   = [...]byte{'c', 'o', 'l', 'u', 'm', 'n', 'N', 'u', 'm', 'b', 'e', 'r', 0}
	fileNameC = [...]byte{'f', 'i', 'l', 'e', 'N', 'a', 'm', 'e', 0}
)

// value "unpacks" 'v'. FreeValue(v) is called before returning, 'v' must not
// be used afterwards.
func (m *VM) value(v lib.TJSValue, free bool) (r any, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	if free && tag(v) < 0 {
		// all tags with a reference count are negative
		defer lib.XFreeValue(tls, ctx, v)
	}

	switch tag(v) {
	case
		lib.EJS_TAG_BIG_INT,       // -9,
		lib.EJS_TAG_SHORT_BIG_INT: // 7,

		ps := lib.XJS_GetPropertyStr(tls, ctx, v, toString)
		if isException(ps) {
			return nil, m.errFromException()
		}

		defer lib.XFreeValue(tls, ctx, ps)

		if tag(ps) != lib.EJS_TAG_OBJECT {
			return nil, fmt.Errorf("failed to get BigInt.toString()")
		}

		*(*lib.TJSValue)(unsafe.Pointer(m.toStringArgv)) = m.int32_16
		jsString := lib.XJS_Call(tls, ctx, ps, v, 1, m.toStringArgv)
		if isException(jsString) {
			return nil, m.errFromException()
		}

		defer lib.XFreeValue(tls, ctx, jsString)

		s, err := m.goString(jsString)
		if err != nil {
			return nil, err
		}

		n := big.NewInt(0)
		if _, ok := n.SetString(s, 16); !ok {
			return nil, fmt.Errorf("big.Int.SetString failed")
		}

		return n, nil
	case
		lib.EJS_TAG_STRING_ROPE, // -6,
		lib.EJS_TAG_STRING:      // -7,

		return m.goString(v)
	case lib.EJS_TAG_OBJECT: // -1
		return m.newObject(v)
	case lib.EJS_TAG_INT: //  0,
		return int(jsvToInt32(v)), nil
	case lib.EJS_TAG_BOOL: //  1,
		return jsvToInt32(v) != 0, nil
	case lib.EJS_TAG_NULL: //  2,
		return nil, nil
	case lib.EJS_TAG_UNDEFINED: //  3,
		return Undefined{}, nil
	case lib.EJS_TAG_EXCEPTION: // 6,
		return nil, m.errFromException()
		panic(todo(""))
	case lib.EJS_TAG_FLOAT64: // 8,
		return jsvToFloat64(v), nil
	}
	return Unsupported{}, nil
}

// errStrProp reads the string-valued property 'name' from error value 'v',
// returning "" when the property is absent or not a string, when reading it
// throws, or when there is no memory to convert it. Like errString it drops
// the exception, see there. 'v' is borrowed.
func errStrProp(tls *libc.TLS, ctx uintptr, v lib.TJSValue, name uintptr) (r string) {
	vr := lib.XJS_GetPropertyStr(tls, ctx, v, name)
	if isException(vr) {
		dropException(tls, ctx)
		return ""
	}

	defer lib.XFreeValue(tls, ctx, vr)

	if tag(vr) == lib.EJS_TAG_STRING || tag(vr) == lib.EJS_TAG_STRING_ROPE {
		return errString(tls, ctx, vr)
	}

	return ""
}

// errIntProp reads the integer-valued property 'name' from error value 'v',
// returning 0 when the property is absent or not numeric, or when reading it
// throws. Like errString it drops the exception, see there. 'v' is borrowed.
func errIntProp(tls *libc.TLS, ctx uintptr, v lib.TJSValue, name uintptr) (r int) {
	vr := lib.XJS_GetPropertyStr(tls, ctx, v, name)
	if isException(vr) {
		dropException(tls, ctx)
		return 0
	}

	defer lib.XFreeValue(tls, ctx, vr)

	switch tag(vr) {
	case lib.EJS_TAG_INT:
		return int(jsvToInt32(vr))
	case lib.EJS_TAG_FLOAT64:
		return int(jsvToFloat64(vr))
	}

	return 0
}

// ErrorFromValue extracts structured error information from JavaScript error
// value 'v', typically an Error instance caught from a JS exception.
//
// 'v' is borrowed: ErrorFromValue does not free it and the caller remains
// responsible for its lifetime. Like all VM operations it must be called from
// the VM's own goroutine. See [Error] for which fields are populated.
//
// A field that cannot be read is left empty: the string form of 'v' (the
// result of Error) when its toString throws, a property whose getter throws,
// and any string field when there is no memory for its UTF-8 copy. The
// exception that reading throws is dropped, so ErrorFromValue leaves no
// exception pending.
func ErrorFromValue(v Value) *Error {
	tls := v.vm.runtime.tls
	ctx := v.vm.cContext

	return &Error{
		raw:          errString(tls, ctx, v.v),
		Name:         errStrProp(tls, ctx, v.v, uintptr(unsafe.Pointer(&nameC[0]))),
		Message:      errStrProp(tls, ctx, v.v, uintptr(unsafe.Pointer(&messageC[0]))),
		Stack:        errStrProp(tls, ctx, v.v, uintptr(unsafe.Pointer(&stackC[0]))),
		LineNumber:   errIntProp(tls, ctx, v.v, uintptr(unsafe.Pointer(&lineNumC[0]))),
		ColumnNumber: errIntProp(tls, ctx, v.v, uintptr(unsafe.Pointer(&colNumC[0]))),
		FileName:     errStrProp(tls, ctx, v.v, uintptr(unsafe.Pointer(&fileNameC[0]))),
	}
}

func (m *VM) errFromException() error {
	err, _ := m.takeException()
	return err
}

// takeException takes the pending exception out of the context and returns
// it as an *Error, and whether it was uncatchable (an interrupt, running out
// of memory, or a panic in a Go function).
func (m *VM) takeException() (err *Error, uncatchable bool) {
	tls := m.runtime.tls
	ctx := m.cContext
	uncatchable = m.uncatchablePending()
	e := lib.XJS_GetException(tls, ctx)
	// The flag outlives the exception. Clear it, so that it does not stay
	// with the next exception the engine fails to throw.
	lib.XJS_SetUncatchableException(tls, ctx, 0)

	defer lib.XFreeValue(tls, ctx, e)

	switch {
	case uncatchable && tag(e) == lib.EJS_TAG_NULL:
		// The engine throws null when it runs out of memory building an
		// error.
		err = outOfMemoryError()
	case uncatchable:
		err = uncatchableError(tls, ctx, e)
	default:
		err = ErrorFromValue(Value{vm: m, v: e})
	}
	if uncatchable {
		// No script can throw an uncatchable exception, and none runs
		// between the engine making it and here (uncatchableError runs
		// none either), so this is one of the errors the engine or the VM
		// ends a call with, with the message it was made with: an interrupt,
		// running out of memory, or a panic in a Go function (hostPanicked).
		err.Uncatchable = true
		switch err.Message {
		case "out of memory":
			err.OutOfMemory = true
		case "interrupted":
			err.Interrupted = true
		}
	}
	defer m.repanic()
	if uncatchable && m.host > 0 {
		// Inside a Go function called from javascript, the exception ends
		// that javascript too: it stays pending, so that every call into
		// javascript fails until callGo returns it to the engine.
		lib.XJS_Throw(tls, ctx, lib.XDupValue(tls, ctx, e))
		lib.XJS_SetUncatchableException(tls, ctx, 1)
	}
	return err, uncatchable
}

// inHost runs f, Go code that javascript called, counted in m.host. A
// panic in f does not go through the engine (see hostPanicked).
func (m *VM) inHost(name string, f func()) {
	m.host++
	defer func() {
		if p := recover(); p != nil {
			m.hostPanicked(name, p)
		}
		m.host--
	}()
	f()
}

// hostPanicked keeps p, what the Go function name called from javascript
// panicked with, and throws an uncatchable error in its place. A panic must
// not unwind through the engine: the engine would keep pointing at stack
// frames that are gone, and the next error it made would walk them, forever
// when the new frame took the old one's place. The javascript ends as for an
// interrupt, and the panic goes on when the Go code that called javascript
// takes the exception or returns (repanic).
func (m *VM) hostPanicked(name string, p any) {
	m.panicked = p
	tls := m.runtime.tls
	ctx := m.cContext
	throwInternalError(tls, ctx, fmt.Sprintf("panic in Go function %s: %v", name, p))
	lib.XJS_SetUncatchableException(tls, ctx, 1)
}

// repanic goes on with the panic hostPanicked kept.
func (m *VM) repanic() {
	if p := m.panicked; p != nil {
		m.panicked = nil
		panic(p)
	}
}

// uncatchableError returns e, an uncatchable error the engine or the VM
// made, without running javascript. Once the exception is out of the
// context, javascript can run again, and ErrorFromValue would run what page
// code installed with e as 'this': Error.prototype.toString for the string
// form, and a getter for the name, which comes from the prototype. Either
// could rewrite the message, which tells the errors apart. So only the own
// data properties message and stack are read, the name is InternalError,
// the class the engine and hostPanicked make all of them with, and the
// string form is the one Error.prototype.toString makes of the two.
func uncatchableError(tls *libc.TLS, ctx uintptr, e lib.TJSValue) *Error {
	message := ownString(tls, ctx, e, lib.EJS_ATOM_message)
	raw := "InternalError"
	if message != "" {
		raw += ": " + message
	}
	return &Error{raw: raw, Name: "InternalError", Message: message, Stack: ownString(tls, ctx, e, lib.EJS_ATOM_stack)}
}

// ownString returns the own property atom of v, an error the engine or the
// VM made, when it is a string, and "" otherwise. It runs no javascript: the
// engine defined message and stack on v as data properties, and nothing ran
// since, so the read finds them on v and calls nothing, and a property v
// lacks is not looked up on the prototype, where page code could have put a
// getter. Reading a stack the engine has not formatted yet formats it, and
// making the Go string of a string built by concatenation needs memory:
// running out of memory there is dropped, uncatchable flag and all.
func ownString(tls *libc.TLS, ctx uintptr, v lib.TJSValue, atom lib.TJSAtom) string {
	switch lib.XJS_GetOwnProperty(tls, ctx, 0, v, atom) {
	case 0:
		return ""
	case 1:
	default:
		dropUncatchable(tls, ctx)
		return ""
	}

	p := lib.XJS_GetPropertyInternal(tls, ctx, v, atom, v, 0)
	if isException(p) {
		dropUncatchable(tls, ctx)
		return ""
	}

	defer lib.XFreeValue(tls, ctx, p)

	if tag(p) != lib.EJS_TAG_STRING && tag(p) != lib.EJS_TAG_STRING_ROPE {
		return ""
	}

	str, ok := cString(tls, ctx, p)
	if !ok {
		dropUncatchable(tls, ctx)
	}
	return str
}

// dropUncatchable drops the exception pending in ctx and clears the
// uncatchable flag running out of memory sets, so that it does not stay
// with the next exception thrown (see takeException).
func dropUncatchable(tls *libc.TLS, ctx uintptr) {
	dropException(tls, ctx)
	lib.XJS_SetUncatchableException(tls, ctx, 0)
}

// outOfMemoryError is the error the engine throws when it runs out of
// memory.
func outOfMemoryError() *Error {
	return &Error{raw: "InternalError: out of memory", Name: "InternalError", Message: "out of memory"}
}

// uncatchablePending reports whether an uncatchable exception (an
// interrupt) is pending in m's context. Every call into javascript fails
// while one is.
func (m *VM) uncatchablePending() bool {
	return lib.XJS_IsUncatchableException(m.runtime.tls, m.cContext) != 0
}

// settled returns v, the result of a call into javascript, or the exception
// value when an uncatchable exception is still pending after the call. A
// built-in that only records an error, such as a promise resolve function
// that rejects its promise when reading 'then' throws, returns as if it had
// not failed, and the code that called it can go on until its next call,
// which fails. The call that ran it fails all the same.
func (m *VM) settled(v lib.TJSValue) lib.TJSValue {
	if isException(v) || !m.uncatchablePending() {
		return v
	}

	lib.XFreeValue(m.runtime.tls, m.cContext, v)
	return exception
}

// StdAddHelpers adds the 'print' and 'console' global objects to 'm'.
func (m *VM) StdAddHelpers() error {
	tls := m.runtime.tls
	ctx := m.cContext
	lib.Xjs_std_add_helpers(tls, ctx, -1, 0)
	return nil
}

var (
	// The C format string used for all messages produced on the Go side. The
	// message must never be passed as the format itself, it is not a literal
	// and any '%' in it would be taken for a conversion specification.
	percentSC = [...]byte{'%', 's', 0}
	percentS  = uintptr(unsafe.Pointer(&percentSC[0]))
)

func throwTypeError(tls *libc.TLS, ctx uintptr, msg string) (r lib.TJSValue) {
	p, err := libc.CString(validUTF8(msg))
	if err != nil {
		return lib.XJS_ThrowTypeError(tls, ctx, 0, 0)
	}

	defer libc.Xfree(tls, p)

	bp := tls.Alloc(16)

	defer tls.Free(16)

	return lib.XJS_ThrowTypeError(tls, ctx, percentS, libc.VaList(bp+8, p))
}

func throwInternalError(tls *libc.TLS, ctx uintptr, msg string) (r lib.TJSValue) {
	p, err := libc.CString(validUTF8(msg))
	if err != nil {
		return lib.XJS_ThrowInternalError(tls, ctx, 0, 0)
	}

	defer libc.Xfree(tls, p)

	bp := tls.Alloc(16)

	defer tls.Free(16)

	return lib.XJS_ThrowInternalError(tls, ctx, percentS, libc.VaList(bp+8, p))
}

type goFunc struct {
	cName        uintptr
	f            reflect.Value
	in           []reflect.Type
	name         string
	out          []reflect.Type
	tab          uintptr
	variadicType reflect.Type
	vm           *VM

	minArgs int
	maxArgs int

	wantThis bool
	hostFunc bool
}

// RegisterFunc registers a Go function 'f' and makes it callable from
// Javascript.
//
// The 'f' argument can be a regular Go function, a closure, a method
// expression or a method value. All of them are called 'Go function' below.
//
// The Go function can have zero or more parameters. If 'wantThis' is true then
// the first parameter of the Go function will get the Javascript value of
// 'this'.  Depending on context, 'this' can be Javascript null or undefined.
//
// Go functions with multiple results return them as an Javascript array.
//
// Go nil errors are converted to Javascript null.
//
// Go non-nil errors are converted to Javascript strings using the Error()
// method.
//
// Any Go <-> Javascript failing type conversion between arguments/return
// values throws a Javascript type error exception.
//
// There is a limit on the total number of currently registered Go
// functions.
//
// Note: The 'name' argument should be a valid Javascript identifier. It is not
// currently enforced but this may change later.
//
// For binding host objects (a DOM, console, fetch, …) prefer RegisterHostFunc,
// which gives the more convenient single-return / error-throws-exception shape.
func (m *VM) RegisterFunc(name string, f any, wantThis bool) error {
	return m.registerFunc(name, f, wantThis, false)
}

// HostFunc is a Go function exposed to JavaScript by RegisterHostFunc. It
// receives the call arguments already converted to Go values and returns a
// single value to convert back to JS, or a non-nil error which is thrown as a
// JavaScript exception.
type HostFunc func(args []any) (any, error)

// RegisterHostFunc exposes fn to JavaScript as a global function named name,
// using the calling convention an embedding host wants: every JS call argument
// arrives in the single []any slice (already converted to Go values), fn returns
// exactly one value to convert back to JS, and a non-nil error is thrown as a
// JavaScript exception.
//
// This differs from RegisterFunc, whose reflection mapping turns a Go
// multiple-return into a JS array and converts a returned error like any other
// value (so a (value, error) pair becomes the array [value, error] rather than a
// value-or-throw). RegisterHostFunc is the better fit for binding host objects;
// RegisterFunc remains for direct, typed Go calls.
func (m *VM) RegisterHostFunc(name string, fn HostFunc) error {
	if fn == nil {
		return fmt.Errorf("host func %q cannot be nil", name)
	}

	// Register a variadic adapter so every JS argument is collected into the one
	// []any; the host flag makes callGo return fn's value directly and throw on a
	// non-nil error, instead of array-wrapping the (value, error) pair.
	return m.registerFunc(name, func(args ...any) (any, error) { return fn(args) }, false, true)
}

func (m *VM) registerFunc(name string, f any, wantThis, host bool) (err error) {
	if name == "" {
		return fmt.Errorf("func name cannot be empty")
	}

	if _, ok := m.goFuncs[name]; ok {
		return fmt.Errorf("func already registered: %s", name)
	}

	tls := m.runtime.tls
	ctx := m.cContext
	val := reflect.ValueOf(f)
	typ := reflect.TypeOf(f)
	if typ.Kind() != reflect.Func {
		return fmt.Errorf("%s is not a function", name)
	}

	if wantThis && typ.NumIn() == 0 {
		return fmt.Errorf("'wantThis' is true but 'f' has zero parameters")
	}

	minArgs := typ.NumIn()
	if wantThis {
		minArgs--
	}
	maxArgs := minArgs
	var vt reflect.Type
	if typ.IsVariadic() {
		maxArgs = -1
		sliceT := typ.In(typ.NumIn() - 1)
		vt = sliceT.Elem()
		minArgs--
	}
	info := &goFunc{
		f:            val,
		maxArgs:      maxArgs,
		minArgs:      minArgs,
		name:         name,
		variadicType: vt,
		vm:           m,
		wantThis:     wantThis,
		hostFunc:     host,
	}
	for i := 0; i < typ.NumIn(); i++ {
		info.in = append(info.in, typ.In(i))
	}
	if maxArgs < 0 {
		info.in = info.in[:len(info.in)-1]
	}
	for i := 0; i < typ.NumOut(); i++ {
		info.out = append(info.out, typ.Out(i))
	}

	magic, err := registerCallback(info)
	if err != nil {
		return err
	}
	if m.goFuncs == nil {
		m.goFuncs = map[string]int32{}
	}
	m.goFuncs[name] = magic

	cName, err := libc.CString(validUTF8(name))
	if err != nil {
		return err
	}

	info.cName = cName
	argc := int32(typ.NumIn()) - 1
	if argc < 0 {
		argc = 0
	}
	tab := libc.Xcalloc(tls, 1, libc.Tsize_t(unsafe.Sizeof(lib.TJSCFunctionListEntry{})))
	info.tab = tab

	g := m.globalObject()

	defer lib.XFreeValue(tls, ctx, g)

	(*lib.TJSCFunctionListEntry)(unsafe.Pointer(tab)).Fname = cName
	(*lib.TJSCFunctionListEntry)(unsafe.Pointer(tab)).Fprop_flags = lib.MJS_PROP_WRITABLE | lib.MJS_PROP_CONFIGURABLE
	(*lib.TJSCFunctionListEntry)(unsafe.Pointer(tab)).Fdef_type = lib.MJS_DEF_CFUNC
	(*lib.TJSCFunctionListEntry)(unsafe.Pointer(tab)).Fmagic = int16(magic)
	(*lib.TJSCFunctionListEntry)(unsafe.Pointer(tab)).Fu.Ffunc1.Flength = uint8(argc)
	(*lib.TJSCFunctionListEntry)(unsafe.Pointer(tab)).Fu.Ffunc1.Fcproto = lib.EJS_CFUNC_generic_magic
	(*lib.TJSCFunctionListEntry)(unsafe.Pointer(tab)).Fu.Ffunc1.Fcfunc.Fgeneric = fp(callGo)
	lib.XJS_SetPropertyFunctionList(tls, ctx, g, tab, 1)
	return nil
}

// registerCallback allocates a magic number for the given goFunc and stores
// it in the global goFuncs map. It returns the allocated magic number or an error.
func registerCallback(info *goFunc) (int32, error) {
	goFuncsMu.Lock()
	defer goFuncsMu.Unlock()
	if n := len(freeMagic); n > 0 {
		k := freeMagic[n-1]
		freeMagic = freeMagic[:n-1]
		goFuncs[k] = info
		return k, nil
	}
	magic := staticMagic.Add(1)
	if magic > math.MaxInt16 {
		staticMagic.Add(-1)
		return -1, fmt.Errorf("too many registered functions")
	}
	goFuncs[magic] = info
	return magic, nil
}

// releaseMagic removes the Go function associated with magic from the global
// tables, returning it to the free pool.
func releaseMagic(magic int32) {
	goFuncsMu.Lock()
	// Append only if the magic was actually registered. freeMagic is a slice,
	// so unlike the old map it does not dedup: a double release would queue the
	// same magic twice and later hand it to two live callbacks at once.
	if _, ok := goFuncs[magic]; ok {
		delete(goFuncs, magic)
		freeMagic = append(freeMagic, magic)
	}
	goFuncsMu.Unlock()
}

var rvt = reflect.TypeOf(Value{})

func isNative(t reflect.Type) bool {
	return t == rvt
}

// canBeNil reports whether nil is a valid value of type 't'. Javascript null
// converts to a Go nil, so it can be passed only to a parameter able to hold
// one. Coercing it to the zero value of, say, string or int would silently
// make f(null) indistinguishable from f("") or f(0).
func canBeNil(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return true
	}

	return false
}

func fp(f interface{}) uintptr {
	type iface [2]uintptr
	return (*iface)(unsafe.Pointer(&f))[1]
}

// typedef JSValue JSCFunctionMagic(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv, int magic);
func callGo(tls *libc.TLS, ctx uintptr, this lib.TJSValue, argc int32, argv uintptr, magic int32) (r lib.TJSValue) {
	goFuncsMu.Lock()
	info := goFuncs[magic]
	goFuncsMu.Unlock()

	if info == nil {
		return throwInternalError(tls, ctx, fmt.Sprintf("callback id=%d not registered", magic))
	}

	m := info.vm
	m.host++
	defer func() {
		if p := recover(); p != nil {
			m.hostPanicked(info.name, p)
		}
		m.host--
		// An interrupt or running out of memory in converting the
		// arguments, in the Go function or in converting its result stays
		// pending (see takeException). It is the result whatever the Go
		// function returned, and no error callGo threw replaced it.
		if m.uncatchablePending() {
			if !isException(r) {
				lib.XFreeValue(tls, ctx, r)
			}
			r = exception
		}
	}()

	haveArgs := int(argc)
	if haveArgs < info.minArgs {
		return throwTypeError(tls, ctx, fmt.Sprintf("not enough arguments in call to %s", info.name))
	}

	if info.maxArgs >= 0 && haveArgs > info.maxArgs {
		return throwTypeError(tls, ctx, fmt.Sprintf("too many arguments in call to %s", info.name))
	}

	var in []reflect.Value
	i := 0
	if info.wantThis {
		switch {
		case isNative(info.in[0]):
			dup := lib.XDupValue(tls, ctx, this)
			defer lib.XFreeValue(tls, ctx, dup)
			in = append(in, reflect.ValueOf(Value{vm: m, v: dup}))
		default:
			v, err := m.value(this, false)
			if err != nil {
				return throwTypeError(tls, ctx, fmt.Sprintf("calling %s: argument conversion error: %v", info.name, err))
			}

			rv := reflect.ValueOf(v)
			switch {
			case !rv.IsValid(): // Javascript null, m.value returns a nil any for it.
				if !canBeNil(info.in[0]) {
					return throwTypeError(tls, ctx, fmt.Sprintf("calling %s: cannot assign null to %s", info.name, info.in[0]))
				}

				rv = reflect.Zero(info.in[0])
			case !rv.Type().AssignableTo(info.in[0]):
				return throwTypeError(tls, ctx, fmt.Sprintf("calling %s: cannot assign %s to %s", info.name, rv.Type(), info.in[0]))
			}

			in = append(in, rv)
		}
		i++
	}
	types := info.in[i:]
	for i := 0; i < int(argc); i++ {
		var typ reflect.Type
		switch {
		case i < len(types):
			typ = types[i]
		default:
			typ = info.variadicType
		}
		switch {
		case isNative(typ):
			dup := lib.XDupValue(tls, ctx, *(*lib.TJSValue)(unsafe.Pointer(argv)))
			defer lib.XFreeValue(tls, ctx, dup)
			in = append(in, reflect.ValueOf(Value{vm: m, v: dup}))
		default:
			v, err := m.value(*(*lib.TJSValue)(unsafe.Pointer(argv)), false)
			if err != nil {
				return throwTypeError(tls, ctx, fmt.Sprintf("calling %s: argument conversion error: %v", info.name, err))
			}

			rv := reflect.ValueOf(v)
			switch {
			case !rv.IsValid(): // Javascript null, m.value returns a nil any for it.
				if !canBeNil(typ) {
					return throwTypeError(tls, ctx, fmt.Sprintf("calling %s: cannot assign null to %s", info.name, typ))
				}

				rv = reflect.Zero(typ)
			case !rv.Type().AssignableTo(typ):
				return throwTypeError(tls, ctx, fmt.Sprintf("calling %s: cannot assign %s to %s", info.name, rv.Type(), typ))
			}

			in = append(in, rv)
		}
		argv += sizeofJsValue
	}

	out := info.f.Call(in)
	var err error

	// A host function (RegisterHostFunc) has the fixed shape func(...any) (any,
	// error): return its single value, or throw a JS exception for a non-nil
	// error — rather than array-wrapping the (value, error) pair.
	if info.hostFunc {
		if e, _ := out[1].Interface().(error); e != nil {
			return throwTypeError(tls, ctx, e.Error())
		}
		r, err = m.jsValue(out[0])
		if err != nil {
			return throwTypeError(tls, ctx, fmt.Sprintf("callback %s: %v", info.name, err))
		}
		return r
	}

	switch len(info.out) {
	case 0:
		return undefined
	case 1:
		switch {
		case isNative(info.out[0]):
			x, ok := out[0].Interface().(Value)
			if !ok {
				return throwTypeError(tls, ctx, fmt.Sprintf("internal error: native Javascript value does not have type 'Value' (%v:)", origin(1)))
			}

			r = x.v
		default:
			r, err = m.jsValue(out[0])
			if err != nil {
				return throwTypeError(tls, ctx, fmt.Sprintf("cannot convert value: %v", err))
			}
		}
	default:
		r, err = m.jsArray(out)
	}
	if err != nil {
		return throwTypeError(tls, ctx, fmt.Sprintf("callback %s: %v", info.name, err))
	}

	return r
}

func (m *VM) jsValue(in reflect.Value) (out lib.TJSValue, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	defer func() {
		if isException(out) && err == nil {
			err = m.errFromException()
		}
	}()

	switch x := in.Interface().(type) {
	case nil:
		return null, nil
	case Undefined:
		return undefined, nil
	case error:
		return m.newString(x.Error())
	}

	switch in.Kind() {
	case reflect.Pointer:
		return m.jsPtrValue(in)
	case reflect.Bool:
		return newBool(in.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return jsInt(in.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return jsUint(in.Uint()), nil
	case reflect.Float64, reflect.Float32:
		return newFloat(in.Float()), nil
	case reflect.String:
		return m.newString(in.String())
	case reflect.Struct:
		b, err := json.Marshal(in.Interface())
		if err != nil {
			return out, err
		}

		p, err := libc.CString(string(b))
		if err != nil {
			return out, err
		}

		defer libc.Xfree(tls, p)

		v := lib.XJS_ParseJSON(tls, ctx, p, libc.Tsize_t(len(b)), uintptr(unsafe.Pointer(&evalFN)))
		if isException(v) {
			return out, m.errFromException()
		}

		return v, nil
	case reflect.Interface:
		return m.jsInterfaceValue(in)
	case reflect.Map:
		return m.jsMap(in)
	case reflect.Slice:
		var a []reflect.Value
		for i := 0; i < in.Len(); i++ {
			a = append(a, in.Index(i))
		}
		return m.jsArray(a)
	default:
		return out, fmt.Errorf("internal error: %v (%v:)", in.Kind(), origin(1))
	}
}

func (m *VM) jsInterfaceValue(in reflect.Value) (out lib.TJSValue, err error) {
	if in.IsNil() {
		return null, nil
	}

	return m.jsValue(in.Elem())
}

func (m *VM) jsPtrValue(in reflect.Value) (out lib.TJSValue, err error) {
	if in.IsNil() {
		return null, nil
	}

	switch x := in.Interface().(type) {
	case *Object:
		return m.parseJSON(x.json, 0)
	case *big.Int:
		s := x.String() + "n"
		return m.eval(s, EvalGlobal)
	default:
		ev := in.Elem()
		if ev.Kind() == reflect.Pointer { // avoid unbound recursion
			return out, fmt.Errorf("type not supported: %T", x)
		}

		return m.jsValue(ev)
	}
}

// jsMap converts a Go map to a Javascript object. Like jsArray, and unlike the
// JSON round trip used for structs, it converts every value with jsValue, so a
// Value stays a live reference and a *big.Int stays a BigInt.
func (m *VM) jsMap(in reflect.Value) (out lib.TJSValue, err error) {
	if in.IsNil() {
		return null, nil
	}

	// Javascript property names are strings. Integer keys stringify the same way
	// they do in JSON and the same way obj[1] and obj["1"] are the same property
	// in Javascript. Collect and validate them before creating the object so a
	// bad key needs no cleanup.
	type kv struct {
		key string
		val reflect.Value
	}
	var a []kv
	for _, k := range in.MapKeys() {
		var key string
		switch k.Kind() {
		case reflect.String:
			key = k.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			key = strconv.FormatInt(k.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			key = strconv.FormatUint(k.Uint(), 10)
		default:
			return out, fmt.Errorf("unsupported map key type: %s", k.Type())
		}

		a = append(a, kv{key, in.MapIndex(k)})
	}

	// Javascript property order is observable, via Object.keys, JSON.stringify or
	// for-in, while Go map iteration order is randomized. Sort so the same map
	// always produces the same object, which is what encoding/json does too.
	sort.Slice(a, func(i, j int) bool { return a[i].key < a[j].key })

	tls := m.runtime.tls
	ctx := m.cContext
	obj := lib.XJS_NewObject(tls, ctx)
	if isException(obj) {
		return out, m.errFromException()
	}

	for _, kv := range a {
		// Like jsArray, a Value is passed on as is rather than converted, and the
		// reference it holds is the one handed over below.
		var jv lib.TJSValue
		switch x := kv.val.Interface().(type) {
		case Value:
			jv = x.v
		default:
			if jv, err = m.jsValue(kv.val); err != nil {
				lib.XFreeValue(tls, ctx, obj)
				return out, err
			}
		}

		p, err2 := libc.CString(validUTF8(kv.key))
		if err2 != nil {
			lib.XFreeValue(tls, ctx, jv)
			lib.XFreeValue(tls, ctx, obj)
			return out, err2
		}

		// JS_SetPropertyStr takes over the reference to jv, it does not take over
		// the property name.
		rc := lib.XJS_SetPropertyStr(tls, ctx, obj, p, jv)
		libc.Xfree(tls, p)
		if rc < 0 {
			lib.XFreeValue(tls, ctx, obj)
			return out, m.errFromException()
		}
	}

	return obj, nil
}

func (m *VM) jsArray(a []reflect.Value) (out lib.TJSValue, err error) {
	var s []lib.TJSValue
	for _, v := range a {
		switch x := v.Interface().(type) {
		case Value:
			s = append(s, x.v)
		default:
			jv, err := m.jsValue(v)
			if err != nil {
				return out, err
			}

			s = append(s, jv)
		}
	}
	return m.jsArrayOf(s)
}

func (m *VM) jsArrayOf(a []lib.TJSValue) (out lib.TJSValue, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	argv := libc.Xmalloc(tls, lib.Tsize_t(len(a))*lib.Tsize_t(sizeofJsValue))

	defer libc.Xfree(tls, argv)

	p := argv
	for _, v := range a {
		*(*lib.TJSValue)(unsafe.Pointer(p)) = v
		p += sizeofJsValue
	}
	out = lib.Xjs_array_of(tls, ctx, undefined, int32(len(a)), argv)
	if isException(out) {
		err = m.errFromException()
	}
	for _, v := range a {
		lib.XFreeValue(tls, ctx, v)
	}
	return out, err
}

// SetDefaultModuleLoader will enable loading module using the default module
// loader.
func (m *VM) SetDefaultModuleLoader() {
	lib.XJS_SetModuleLoaderFunc2(m.runtime.tls, m.runtime.cRuntime, 0, fp(lib.Xjs_module_loader), 0, 0)
}

// Atom is an unique identifier of, for example, a string value. Atom values
// are VM-specific.
type Atom = lib.TJSAtom

// NewAtom returns an unique indentifier of 's' or an error, if any.
func (m *VM) NewAtom(s string) (r Atom, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	p, err := libc.CString(validUTF8(s))
	if err != nil {
		return lib.MJS_ATOM_NULL, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, p)

	if r = lib.XJS_NewAtom(tls, ctx, p); r == lib.MJS_ATOM_NULL {
		err = fmt.Errorf("OOM")
	}

	return r, err
}

// SetPropertyValue sets this.prop = val.
func (m *VM) SetPropertyValue(this Value, prop Atom, val Value) (err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	dup := val.Dup()
	if lib.XSetProperty(tls, ctx, this.v, prop, dup.v) < 0 {
		return fmt.Errorf("failed to set property")
	}

	return nil
}

// SetProperty sets this.prop = val.
func (m *VM) SetProperty(this Value, prop Atom, val any) (err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	jsArgs, free, err := m.convertArgs(val)
	if err != nil {
		return err
	}

	if len(free) != 0 {
		defer func() {
			for _, v := range free {
				lib.XFreeValue(tls, ctx, v)
			}
		}()
	}

	return m.SetPropertyValue(this, prop, Value{vm: m, v: jsArgs[0]})
}

// GetPropertyValue returns this.prop.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) GetPropertyValue(this Value, prop Atom) (r Value, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	v := lib.XGetProperty(tls, ctx, this.v, prop)
	if isException(v) {
		err = m.errFromException()
	}
	return Value{vm: m, v: v}, err
}

// GetProperty returns this.prop.
func (m *VM) GetProperty(this Value, prop Atom) (r any, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	return m.value(lib.XGetProperty(tls, ctx, this.v, prop), true)
}

// Value represents a native Javascript value. Values are reference counted and
// their lifetime is managed by an independent Javascript garbage collector.
// To avoid memory corruption/leaks caused by tripping the Javascript GC, a
// Value must not
//
//   - be copied. Use the Dup method instead.
//   - become unreachable without calling its Free method.
//   - be used after its Free() method was called.
//   - outlive its VM.
//
// It is recommended to use native Go values instead of Value where possible.
//
// When passing a Value down the call stack use Dup. For example in main
//
//	v, _ := EvalValue(someScript)
//	defer v.Free()
//	foo(v.Dup()) // Instead of foo(v)
//
// In 'foo' Free must be used. For example
//
//	func foo(v Value) {
//	        defer v.Free()
//	        ...
//	}
//
// This ensures the/only topmost Free marks 'v' eligible for garbage collection.
//
// Beware that the correct setup/handling becomes more complicated when using
// closures, Values are sent through a channel etc. In particular, if a
// goroutine 1 passes a Dup of 'v' to goroutine 2 and goroutine 1 completes and
// thus frees 'v' before goroutine 2 completes, the reference counting
// mechanism will fail. In other words, every Free must be strictly paired with
// the Dup that preceded obtaining the Value and the Dup/Free calls must
// respect the original nesting. This is correct.
//
//	Dup             // in main
//	        Dup     // in foo
//	        Free    // in foo
//	Free            // in main
//
// This will fail, for example in the above discussed goroutines scenario.
//
//	Dup            // in g1
//	        Dup    // in g2
//	Free           // in g1
//	        Free   // in g2
//
// The fix might be in this case to arrange goroutine 1 to wait for goroutine 2
// to complete before executing Free in goroutine 1.
//
// See TestMemgrind for an example of how to detect memory leaks caused not
// only by improper Value use.
type Value struct {
	vm *VM
	v  lib.TJSValue
}

// VM returns the VM associated with 'v'.
func (v *Value) VM() *VM {
	return v.vm
}

// SetProperty sets v.prop = val.
func (v Value) SetProperty(prop Atom, val any) (err error) {
	return v.vm.SetProperty(v, prop, val)
}

// SetPropertyValue sets v.prop = val.
func (v Value) SetPropertyValue(prop Atom, val Value) (err error) {
	return v.vm.SetPropertyValue(v, prop, val)
}

// GetPropertyValue returns v.prop.
func (v Value) GetPropertyValue(prop Atom) (r Value, err error) {
	return v.vm.GetPropertyValue(v, prop)
}

// GetProperty returns v.prop. The 'this' argument is ignored.
//
// Deprecated: The 'this' argument is vestigial, the property is always looked
// up on 'v'. Use [Value.Property] instead.
func (v Value) GetProperty(this Value, prop Atom) (r any, err error) {
	return v.vm.GetProperty(v, prop)
}

// Property returns v.prop. It is [Value.GetProperty] without the vestigial
// 'this' argument.
func (v Value) Property(prop Atom) (r any, err error) {
	return v.vm.GetProperty(v, prop)
}

// MarshalJSON implements encoding/json.Marshaler.
func (v Value) MarshalJSON() (r []byte, err error) {
	tls := v.vm.runtime.tls
	ctx := v.vm.cContext
	json := lib.XJS_JSONStringify(tls, ctx, v.v, undefined, undefined)

	defer lib.XFreeValue(tls, ctx, json)

	if isException(json) {
		return nil, v.vm.errFromException()
	}

	// A copy: the JS buffer is released before this returns.
	s, err := v.vm.goString(json)
	if err != nil {
		return nil, err
	}

	return []byte(s), nil
}

// Dup returns a copy of 'v' while updating its reference count.
func (v Value) Dup() Value {
	return Value{
		vm: v.vm,
		v:  lib.XDupValue(v.vm.runtime.tls, v.vm.cContext, v.v),
	}
}

// Free marks 'v' as no longer used and updates its reference count. 'v' must
// not be used afterwards.
func (v *Value) Free() {
	lib.XFreeValue(v.vm.runtime.tls, v.vm.cContext, v.v)
	*v = Value{}
}

// Any attemtps to convert 'v' to any using the same rules as there are for
// the return value of VM.Eval.
func (v Value) Any() (r any, err error) {
	return v.vm.value(v.v, false)
}

// IsUndefined reports whether 'v' represents the Javascript value 'undefined'.
func (v Value) IsUndefined() bool {
	return tag(v.v) == lib.EJS_TAG_UNDEFINED
}

// Call evaluates 'v(args...)' with 'this' as the receiver and returns the
// resulting (value, error). 'v' must be a Javascript function. Pass
// [UndefinedValue] as 'this' when the function does not use it.
//
//	resolve.Call(quickjs.UndefinedValue, "done")
//	reject.Call(quickjs.UndefinedValue, err.Error())
//
// Argument types are converted using the same rules as in [VM.Call] and the
// result is unpacked using the same rules as there are for the return value of
// [VM.Eval]. Note that a Go error is not one of the recognized types, so it is
// marshaled to JSON like any other value — and because the standard error
// types have no exported fields that yields '{}'. Pass err.Error() instead.
//
// Note: A Value passed to a Go function registered by [VM.RegisterFunc] is
// valid only for the duration of that call. To call it later, for example to
// settle a promise after the Go function has returned, retain it using
// [Value.Dup] and release it using [Value.Free] when done.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (v Value) Call(this Value, args ...any) (r any, err error) {
	if err = v.checkCall(this); err != nil {
		return nil, err
	}

	// Re-arm the eval-timeout deadline, as the Eval* functions do (see VM.Call).
	v.vm.enter()
	defer v.vm.leave()
	return v.vm.call(v.v, this.v, args...)
}

// newCallback creates a JS function object backed by fn and stores fn in the
// global Go-function table keyed by a fresh magic number. The returned
// TJSValue is a custom-class object whose finalizer calls releaseMagic when
// the last JS reference is dropped, so the goFuncs entry is freed even when
// the callback never fires (e.g. the sibling handler in a .then(f, g)).
//
// Callers must free the returned TJSValue when it is no longer needed.
func (m *VM) newCallback(name string, fn func(Value)) (lib.TJSValue, int32, error) {
	var zero lib.TJSValue
	if err := m.ensureCallbackClass(); err != nil {
		return zero, -1, err
	}

	magic, err := registerCallback(&goFunc{
		f:       reflect.ValueOf(fn),
		maxArgs: 1,
		minArgs: 1,
		name:    name,
		vm:      m,
		in:      []reflect.Type{rvt},
	})
	if err != nil {
		return zero, -1, err
	}

	tls := m.runtime.tls
	ctx := m.cContext

	obj := lib.XJS_NewObjectClass(tls, ctx, int32(callbackClassID))
	if isException(obj) {
		lib.XFreeValue(tls, ctx, obj)
		releaseMagic(magic)
		return zero, -1, m.errFromException()
	}
	lib.XJS_SetOpaque(tls, obj, uintptr(magic))
	return obj, magic, nil
}

// ensureCallbackClass registers the custom callback class on m's runtime if
// not already done. The class is callable from JS (via Fcall) and has a
// finalizer (via Ffinalizer) that releases the magic when the JS object's
// refcount reaches zero — this is what makes Then leak-free.
func (m *VM) ensureCallbackClass() error {
	if m.runtime.callbackClassInit {
		return nil
	}

	var initErr error
	initCallbackClassID.Do(func() {
		if lib.XJS_NewClassID(m.runtime.tls, uintptr(unsafe.Pointer(&callbackClassID))) == 0 {
			initErr = fmt.Errorf("failed to allocate callback class ID")
		}
	})
	if initErr != nil {
		return initErr
	}

	tls := m.runtime.tls
	cName, err := libc.CString("GoCallback")
	if err != nil {
		return err
	}
	defer libc.Xfree(tls, cName)

	// Allocate the class def on the C heap so the uintptr passed to
	// XJS_NewClass stays valid even if Go's stack grows.
	defSize := int(unsafe.Sizeof(lib.TJSClassDef{}))
	defPtr := libc.Xcalloc(tls, 1, libc.Tsize_t(defSize))
	if defPtr == 0 {
		return fmt.Errorf("OOM")
	}
	defer libc.Xfree(tls, defPtr)

	*(*lib.TJSClassDef)(unsafe.Pointer(defPtr)) = lib.TJSClassDef{
		Fclass_name: cName,
		Ffinalizer:  fp(callbackFinalizer),
		Fcall:       fp(callbackCall),
	}

	if lib.XJS_NewClass(tls, m.runtime.cRuntime, callbackClassID, defPtr) < 0 {
		return fmt.Errorf("failed to register callback class")
	}

	m.runtime.callbackClassInit = true
	return nil
}

// callbackCall is the JSClassCall handler for callback-class objects. It reads
// the magic from the object's opaque storage and dispatches through the
// existing callGo path.
func callbackCall(tls *libc.TLS, ctx uintptr, funcObj lib.TJSValue, this lib.TJSValue, argc int32, argv uintptr, flags int32) lib.TJSValue {
	magic := int32(lib.XJS_GetOpaque(tls, funcObj, callbackClassID))
	return callGo(tls, ctx, this, argc, argv, magic)
}

// callbackFinalizer is the JSClassFinalizer for callback-class objects. It
// reads the magic from the object's opaque storage and releases the global
// goFuncs entry so that the magic can be reused.
func callbackFinalizer(tls *libc.TLS, rt uintptr, val lib.TJSValue) {
	magic := int32(lib.XJS_GetOpaque(tls, val, callbackClassID))
	releaseMagic(magic)
}

// Then attaches fulfillment and rejection handlers to the Promise v,
// returning the new promise for chaining. Either onFulfilled or onRejected
// may be nil.
func (v Value) Then(onFulfilled, onRejected func(Value)) (_ Value, err error) {
	if v.vm == nil {
		return Value{}, fmt.Errorf("cannot call Then on a Value not associated with a VM")
	}
	m := v.vm
	tls := m.runtime.tls
	ctx := m.cContext

	fns := [2]func(Value){onFulfilled, onRejected}
	names := [2]string{"onFulfilled", "onRejected"}

	// Allocate on the C heap so the pointer stays valid if Go's stack grows.
	jsArgsSize := 2 * int(sizeofJsValue)
	jsArgsPtr := libc.Xcalloc(tls, 1, libc.Tsize_t(jsArgsSize))
	if jsArgsPtr == 0 {
		return Value{}, fmt.Errorf("OOM")
	}
	defer libc.Xfree(tls, jsArgsPtr)
	jsArgs := unsafe.Slice((*lib.TJSValue)(unsafe.Pointer(jsArgsPtr)), 2)

	// magics[i] >= 0 marks a slot for which we created a callback and so own a
	// reference to jsArgs[i]; -1 means a nil handler or a slot whose callback
	// was never created (e.g. newCallback failed on the other slot).
	magics := [2]int32{-1, -1}

	defer func() {
		for i := range jsArgs {
			if magics[i] < 0 {
				continue
			}
			lib.XFreeValue(tls, ctx, jsArgs[i])
		}
	}()

	for i, fn := range fns {
		if fn == nil {
			jsArgs[i] = undefined
			magics[i] = -1
			continue
		}
		jsFn, magic, err := m.newCallback(names[i], fn)
		if err != nil {
			return Value{}, err
		}
		jsArgs[i] = jsFn
		magics[i] = magic
	}

	// Re-arm the eval-timeout deadline, as the Eval*/Call entry points do:
	// fetching and invoking .then runs Javascript (a then getter or a
	// user-overridden then), so without this the deadline from the last Eval
	// keeps ticking through idle time and could interrupt this call.
	m.enter()
	defer m.leave()
	thenFn := lib.XGetProperty(tls, ctx, v.v, lib.EJS_ATOM_then)
	result := m.settled(lib.XJS_Call(tls, ctx, thenFn, v.v, 2, jsArgsPtr))
	lib.XFreeValue(tls, ctx, thenFn)
	if isException(result) {
		lib.XFreeValue(tls, ctx, result)
		err = m.errFromException()
		return
	}

	return Value{vm: m, v: result}, nil
}

// CallValue is like [Value.Call] but returns (Value, error) like
// [VM.EvalValue].
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (v Value) CallValue(this Value, args ...any) (r Value, err error) {
	if err = v.checkCall(this); err != nil {
		return r, err
	}

	// Re-arm the eval-timeout deadline, as the Eval* functions do (see VM.Call).
	v.vm.enter()
	defer v.vm.leave()
	return v.vm.callValue(v.v, this.v, args...)
}

// checkCall validates the receiver and 'this' of the Value call methods. It
// reports the problem as an error rather than panicking in v.vm because both
// callers already return one.
func (v Value) checkCall(this Value) error {
	if v.vm == nil {
		return fmt.Errorf("cannot call a Value not associated with a VM")
	}

	// Same test as in convertArgs: values with a reference count, ie. those
	// belonging to a particular runtime heap, must not cross VMs.
	if this.vm != v.vm && tag(this.v) < 0 {
		return fmt.Errorf("cannot use a Value from a different VM")
	}

	return nil
}

// Version returns the underlying QuickJS version.
func Version() string {
	return lib.MCONFIG_VERSION
}

// ModuleLoaderFunc defines the signature for a custom module loader.
// It must return the module's source code (Javascript).
type ModuleLoaderFunc func(vm *VM, moduleName string) (string, error)

// ModuleNormalizeFunc defines the signature for a custom module normalizer.
// It receives the base name (the module making the import) and the requested name,
// and should return the normalized, absolute module name.
type ModuleNormalizeFunc func(vm *VM, moduleBaseName, moduleName string) (string, error)

type moduleLoaderState struct {
	vm        *VM
	loader    ModuleLoaderFunc
	normalize ModuleNormalizeFunc
}

// jsMallocString allocates a C string using the QuickJS allocator so that QuickJS
// can safely call js_free() on it. We use lib.Xjs_strdup to let the C side handle
// the 32-bit/64-bit memory tracking alignment natively.
func jsMallocString(tls *libc.TLS, ctx uintptr, s string) uintptr {
	temp, err := libc.CString(validUTF8(s))
	if err != nil {
		return 0
	}
	defer libc.Xfree(tls, temp)

	// Duplicate the string natively into the QuickJS memory pool
	return lib.Xjs_strdup(tls, ctx, temp)
}

// SetModuleLoader configures custom Go functions to load and normalize Javascript modules.
// Passing nil for 'loader' removes the custom loader, and so does Close; until then the
// engine keeps 'loader' and 'normalize', and whatever they close over, reachable. If
// 'normalize' is nil, the engine falls back to the default QuickJS module normalizer.
func (m *VM) SetModuleLoader(loader ModuleLoaderFunc, normalize ModuleNormalizeFunc) {
	customLoadersMu.Lock()
	defer customLoadersMu.Unlock()

	if loader == nil {
		delete(customLoaders, m.runtime.cRuntime)
		lib.XJS_SetModuleLoaderFunc2(m.runtime.tls, m.runtime.cRuntime, 0, 0, 0, 0)
		return
	}

	customLoaders[m.runtime.cRuntime] = moduleLoaderState{
		vm:        m,
		loader:    loader,
		normalize: normalize,
	}

	var normFp uintptr
	if normalize != nil {
		normFp = fp(customModuleNormalize)
	}

	lib.XJS_SetModuleLoaderFunc2(
		m.runtime.tls,
		m.runtime.cRuntime,
		normFp,
		fp(customModuleLoader),
		0,
		m.runtime.cRuntime, // opaque
	)
}

func customModuleNormalize(tls *libc.TLS, ctx uintptr, module_base_name, module_name uintptr, opaque uintptr) uintptr {
	customLoadersMu.Lock()
	state, ok := customLoaders[opaque]
	customLoadersMu.Unlock()

	if !ok || state.normalize == nil {
		return 0
	}

	baseName := ""
	if module_base_name != 0 {
		baseName = cGoString(tls, module_base_name)
	}
	name := ""
	if module_name != 0 {
		name = cGoString(tls, module_name)
	}

	var normName string
	var err error
	state.vm.inHost("module normalizer", func() { normName, err = state.normalize(state.vm, baseName, name) })
	if state.vm.uncatchablePending() {
		// See callGo.
		return 0
	}
	if err != nil {
		throwTypeError(tls, ctx, fmt.Sprintf("module normalization failed: %v", err))
		return 0
	}

	return jsMallocString(tls, ctx, normName)
}

func customModuleLoader(tls *libc.TLS, ctx uintptr, module_name uintptr, opaque uintptr, attrs lib.TJSValue) uintptr {
	customLoadersMu.Lock()
	state, ok := customLoaders[opaque]
	customLoadersMu.Unlock()

	if !ok || state.loader == nil {
		return 0
	}

	name := ""
	if module_name != 0 {
		name = cGoString(tls, module_name)
	}

	var source string
	var err error
	state.vm.inHost("module loader", func() { source, err = state.loader(state.vm, name) })
	if state.vm.uncatchablePending() {
		// See callGo.
		return 0
	}
	if err != nil {
		throwTypeError(tls, ctx, fmt.Sprintf("module load failed: %v", err))
		return 0
	}

	ps, err := libc.CString(source)
	if err != nil {
		throwInternalError(tls, ctx, "OOM")
		return 0
	}
	defer libc.Xfree(tls, ps)

	flags := int32(lib.MJS_EVAL_TYPE_MODULE | lib.MJS_EVAL_FLAG_COMPILE_ONLY)
	v := lib.XJS_Eval(tls, ctx, ps, libc.Tsize_t(len(source)), module_name, flags)

	if isException(v) {
		lib.XFreeValue(tls, ctx, v)
		return 0
	}

	ptr := jsvToPtr(v)
	lib.XFreeValue(tls, ctx, v)

	return ptr
}
