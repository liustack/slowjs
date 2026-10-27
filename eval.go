// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"fmt"
)

// Eval flags.
const (
	EvalGlobal = 0 // global code
	EvalModule = 1 // module code
)

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
	m.enter()
	defer m.leave(&err)
	v, err := m.jsEval(javascript, int32(flags))
	if err != nil {
		return nil, err
	}

	return m.value(m.settled(v), true)
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
	// Re-arm the eval-timeout deadline, as the Eval* functions do. Draining runs JS
	// (Promise reactions and async continuations), so without this the whole drain
	// inherits a stale deadline from the last Eval — one that has kept
	// ticking through idle time — and is spuriously interrupted.
	m.enter()
	defer m.leave(&err)

	const maxJobs = 1 << 20 // bound a self-feeding microtask loop so it cannot hang the host
	for n < maxJobs {
		r := m.jsExecutePendingJob()
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

	m.enter()
	defer m.leave(&err)
	v, err := m.jsEvalThis(obj.v, javascript, int32(flags))
	if err != nil {
		return nil, err
	}

	return m.value(m.settled(v), true)
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
	m.enter()
	defer m.leave(&err)
	v, err := m.jsEval(javascript, int32(flags))
	if err != nil {
		return r, err
	}

	v = m.settled(v)
	if isException(v) {
		m.jsFreeValue(v)
		return r, m.errFromException()
	}

	return Value{vm: m, v: v}, nil
}

// EvalThisValue is like EvalValue but sets javascript 'this' to obj.
func (m *VM) EvalThisValue(obj Value, javascript string, flags int) (r Value, err error) {
	defer obj.Free()

	m.enter()
	defer m.leave(&err)
	v, err := m.jsEvalThis(obj.v, javascript, int32(flags))
	if err != nil {
		return r, err
	}

	v = m.settled(v)
	if isException(v) {
		m.jsFreeValue(v)
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
	// https://github.com/bellard/quickjs/blob/eb2c89087def1829ed99630cb14b549d7a98408c/quickjs-libc.c#L4292
	obj, err := m.jsReadObject(bytecode, readObjBytecode)
	if err != nil {
		return r, err
	}

	return m.evalBytecode(obj)
}

// evalBytecode runs obj, what JS_ReadObject read, which it takes.
func (m *VM) evalBytecode(obj jsValue) (r Value, err error) {
	if isException(obj) {
		return r, m.errFromException()
	}

	m.enter()
	defer m.leave(&err)

	var v jsValue
	if tag(obj) == tagModule {
		if m.jsResolveModule(obj) < 0 {
			m.jsFreeValue(obj)
			return r, m.errFromException()
		}

		m.setImportMeta(obj)
		// JS_EvalFunction takes obj. The hold kept here lets freeTopLevel
		// reach the module after it ran.
		dupValue(obj)
		defer m.jsFreeValue(obj)
		// 'this' is set to ctx->global_obj
		// see https://github.com/bellard/quickjs/blob/eb2c89087def1829ed99630cb14b549d7a98408c/quickjs.c#L36045
		v = m.settled(m.jsEvalFunction(obj))
		if !isException(v) {
			if v, err = m.await(v); err != nil {
				return r, err
			}
			m.freeTopLevel(obj)
		}
	} else {
		v = m.settled(m.jsEvalFunction(obj))
	}

	if isException(v) {
		m.jsFreeValue(v)
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
func (m *VM) await(v jsValue) (jsValue, error) {
	defer m.jsFreeValue(v)

	var jobErr error
	for {
		switch m.jsPromiseState(v) {
		case promiseFulfilled:
			if jobErr != nil {
				return exception, jobErr
			}
			return m.jsPromiseResult(v), nil
		case promiseRejected:
			m.jsThrow(m.jsPromiseResult(v))
			return exception, m.errFromException()
		case promisePending:
			r := m.jsExecutePendingJob()
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
			return dupValue(v), nil
		}
	}
}

// Compile compiles the bytecode representation of the passed script.
//
// The returned bytecode can be later executed using EvalBytecode or EvalBytecodeValue.
//
// Note: The bytecode format is linked to a given QuickJS version.
func (m *VM) Compile(javascript string, flags int) ([]byte, error) {
	// https://github.com/bellard/quickjs/blob/eb2c89087def1829ed99630cb14b549d7a98408c/qjsc.c#L346-L360

	flags |= evalFlagCompileOnly

	v, err := m.eval(javascript, flags)
	if err != nil {
		return nil, err
	}

	defer m.jsFreeValue(v)

	if isException(v) {
		return nil, m.errFromException()
	}

	return m.writeBytecode(v)
}

// writeBytecode returns the bytecode of the compiled script v.
func (m *VM) writeBytecode(v jsValue) ([]byte, error) {
	b, ok := m.jsWriteObject(v)
	if !ok {
		return nil, m.errFromException()
	}

	return b, nil
}

func (m *VM) eval(js string, flags int) (r jsValue, err error) {
	m.enter()
	defer m.leave(&err)
	v, err := m.jsEval(js, int32(flags))
	if err != nil {
		return r, err
	}

	return m.settled(v), nil
}

// GlobalObject returns m's global object.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) GlobalObject() (r Value) {
	return Value{vm: m, v: m.globalObject()}
}

func (m *VM) globalObject() jsValue {
	return m.jsGetGlobalObject()
}
