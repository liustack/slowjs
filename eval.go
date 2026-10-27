// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"fmt"
	"unsafe"

	lib "github.com/liustack/slowjs/lib"
	"modernc.org/libc"
)

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
		err = fmt.Errorf("slowjs: aborted draining after %d pending jobs", maxJobs)
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
	ps, err := libc.CString(string(bytecode))
	if err != nil {
		return r, fmt.Errorf("OOM")
	}

	defer libc.Xfree(m.runtime.tls, ps)

	return m.evalBytecode(ps, len(bytecode), lib.MJS_READ_OBJ_BYTECODE)
}

// evalBytecode reads the n bytes of bytecode at p with the flags of
// JS_ReadObject and runs what it read.
func (m *VM) evalBytecode(p uintptr, n int, flags int32) (r Value, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	// https://github.com/bellard/quickjs/blob/eb2c89087def1829ed99630cb14b549d7a98408c/quickjs-libc.c#L4292
	obj := lib.XJS_ReadObject(tls, ctx, p, lib.Tsize_t(n), flags)
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
		// JS_EvalFunction takes obj. The hold kept here lets freeTopLevel
		// reach the module after it ran.
		lib.XDupValue(tls, ctx, obj)
		defer lib.XFreeValue(tls, ctx, obj)
		// 'this' is set to ctx->global_obj
		// see https://github.com/bellard/quickjs/blob/eb2c89087def1829ed99630cb14b549d7a98408c/quickjs.c#L36045
		v = m.settled(lib.XJS_EvalFunction(tls, ctx, obj))
		if !isException(v) {
			if v, err = m.await(v); err != nil {
				return r, err
			}
			freeTopLevel(tls, ctx, obj)
		}
	} else {
		v = m.settled(lib.XJS_EvalFunction(tls, ctx, obj))
	}

	if isException(v) {
		lib.XFreeValue(tls, ctx, v)
		return r, m.errFromException()
	}

	return Value{vm: m, v: v}, nil
}

// errAwaitPending is the error of EvalBytecode and EvalBytecodeValue for a
// module whose top-level await is still pending when no job is left to run.
// What it waits for may still settle from outside the job queue, such as a
// promise that a Go function returned and Go resolves later. Then
// ExecutePendingJobs runs the rest of the module.
var errAwaitPending = errors.New("slowjs: the module's top-level await is still pending and no job is left to run")

// await runs pending jobs until the promise v, which it frees, settles, and
// returns its result, or the exception of a rejected promise. It follows the
// rule of ExecutePendingJobs: a job that throws does not stop it, and the
// first such error is what it returns when v is fulfilled, joined with
// errAwaitPending when no job is left and v is still pending. A rejected v
// is the error, as it is what the module itself ended with. An uncatchable
// exception (an interrupt, the eval timeout, out of memory, a panic in a Go
// function) ends the wait at once, and the jobs left stay queued.
//
// It replaces js_std_await of quickjs-libc.c, which the qjs shell calls. That
// one prints a failed job and goes on, and when the queue is empty it reads
// the state js_std_init_handlers sets up, which a VM has not got.
func (m *VM) await(v lib.TJSValue) (lib.TJSValue, error) {
	tls := m.runtime.tls
	ctx := m.cContext
	rt := m.runtime.cRuntime
	defer lib.XFreeValue(tls, ctx, v)

	var jobErr error
	for {
		switch lib.XJS_PromiseState(tls, ctx, v) {
		case lib.EJS_PROMISE_FULFILLED:
			if jobErr != nil {
				return exception, jobErr
			}
			return lib.XJS_PromiseResult(tls, ctx, v), nil
		case lib.EJS_PROMISE_REJECTED:
			lib.XJS_Throw(tls, ctx, lib.XJS_PromiseResult(tls, ctx, v))
			return exception, m.errFromException()
		case lib.EJS_PROMISE_PENDING:
			r := lib.XJS_ExecutePendingJob(tls, rt, 0)
			if r == 0 {
				if jobErr != nil {
					return exception, errors.Join(errAwaitPending, jobErr)
				}
				return exception, errAwaitPending
			}
			if r > 0 && m.uncatchablePending() {
				r = -1 // the job stopped at an uncatchable exception without failing
			}
			if r < 0 {
				e, uncatchable := m.takeException()
				if uncatchable {
					return exception, e
				}
				if jobErr == nil {
					jobErr = e
				}
			}
		default: // not a promise
			return lib.XDupValue(tls, ctx, v), nil
		}
	}
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
