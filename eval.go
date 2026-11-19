// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// Evaluating source and bytecode, compiling, and the queue of jobs.

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// Eval evaluates javascript, a script or, with EvalModule in flags, a module,
// and returns its value as a Go value:
//
//	javascript              Go
//	----------              --
//	null                    nil
//	undefined               Undefined{}
//	a boolean               bool
//	a number that is int32  int
//	any other number        float64
//	a string                string, a lone surrogate as U+FFFD
//	a BigInt                *big.Int
//	an object, an array     *Object, its JSON
//	a symbol and the rest   Unsupported{}
//
// A number the engine holds as a float, such as the result of some
// arithmetic, is a float64 even when its value is whole. A module's value is
// the promise of its evaluation, which Eval does not wait for. An exception
// is returned as an [*Error].
//
// flags are given to the engine as they are, so other flag bits of QuickJS
// work too.
func (m *VM) Eval(javascript string, flags int) (r any, err error) {
	if err = m.enter(); err != nil {
		return nil, err
	}
	defer m.leave(&err)
	v, err := m.jsEval(evalName, javascript, int32(flags))
	if err != nil {
		return nil, err
	}
	return m.value(m.settled(v), true)
}

// EvalValue is [VM.Eval] that returns the value as a [Value], which the caller
// frees.
func (m *VM) EvalValue(javascript string, flags int) (r Value, err error) {
	if err = m.enter(); err != nil {
		return r, err
	}
	defer m.leave(&err)
	return m.evalValue(evalName, javascript, flags)
}

// EvalFile is [VM.EvalValue] with name as the file of the script: its stack
// frames and its syntax errors show name where those of the other Eval
// methods show <eval>, and a module has name as its module name. No file is
// read. name is cleaned first: to valid UTF-8 of one line, with no control
// character, U+2028 or U+2029 (each becomes U+FFFD), of at most 1,024 bytes
// (cut where a character starts). An empty name is <eval>.
//
// The name of a module is what tells it apart, so a module name the cleaning
// would change is an error, and nothing runs. Two modules of the same name
// both run, and import.meta and an import of that name find the first.
func (m *VM) EvalFile(name, javascript string, flags int) (r Value, err error) {
	clean := cleanEvalName(name)
	if flags&evalTypeMask == EvalModule && name != "" && clean != name {
		return r, fmt.Errorf("slowjs: EvalFile: the module name %.64q is not usable: %s", name, evalNameFault(name))
	}
	if err = m.enter(); err != nil {
		return r, err
	}
	defer m.leave(&err)
	return m.evalValue(clean, javascript, flags)
}

// evalValue evaluates javascript as the file name, inside a call (enter).
func (m *VM) evalValue(name, javascript string, flags int) (Value, error) {
	v, err := m.jsEval(name, javascript, int32(flags))
	if err != nil {
		return Value{}, err
	}
	if v = m.settled(v); isException(v) {
		return Value{}, m.errFromException()
	}
	return Value{vm: m, v: v}, nil
}

// EvalThis is [VM.Eval] with obj as this. It takes obj and frees it when it
// returns, whatever happens. A Value of another VM is an error, and is
// neither used nor freed.
func (m *VM) EvalThis(obj Value, javascript string, flags int) (r any, err error) {
	if m.foreign(obj) {
		return nil, errForeignValue
	}
	defer obj.Free()
	if err = m.enter(); err != nil {
		return nil, err
	}
	defer m.leave(&err)
	v, err := m.jsEvalThis(obj.jsValue(), javascript, int32(flags))
	if err != nil {
		return nil, err
	}
	return m.value(m.settled(v), true)
}

// EvalThisValue is [VM.EvalThis] that returns the value as a [Value], which
// the caller frees. It takes obj as EvalThis does.
func (m *VM) EvalThisValue(obj Value, javascript string, flags int) (r Value, err error) {
	if m.foreign(obj) {
		return r, errForeignValue
	}
	defer obj.Free()
	if err = m.enter(); err != nil {
		return r, err
	}
	defer m.leave(&err)
	v, err := m.jsEvalThis(obj.jsValue(), javascript, int32(flags))
	if err != nil {
		return r, err
	}
	if v = m.settled(v); isException(v) {
		return r, m.errFromException()
	}
	return Value{vm: m, v: v}, nil
}

// EvalBytecode loads bytecode that [VM.Compile] made, runs it and returns its
// value as [VM.Eval] does. this is the global object. A module's top-level
// await is waited for. Loading, running and converting the value are one
// call, inside the eval timeout.
//
// The format of bytecode belongs to the release of QuickJS the engine is, and
// nothing in it is checked: never load bytecode from a source you do not
// trust.
func (m *VM) EvalBytecode(bytecode []byte) (r any, err error) {
	if err = m.enter(); err != nil {
		return nil, err
	}
	defer m.leave(&err)
	v, err := m.runBytecode(bytecode)
	if err != nil {
		return nil, err
	}
	return m.value(v, true)
}

// EvalBytecodeValue is [VM.EvalBytecode] that returns the value as a
// [Value], which the caller frees.
func (m *VM) EvalBytecodeValue(bytecode []byte) (r Value, err error) {
	if err = m.enter(); err != nil {
		return r, err
	}
	defer m.leave(&err)
	v, err := m.runBytecode(bytecode)
	if err != nil {
		return r, err
	}
	return Value{vm: m, v: v}, nil
}

// runBytecode reads bytecode and runs it, inside a call (enter).
func (m *VM) runBytecode(bytecode []byte) (jsValue, error) {
	obj, err := m.jsReadObject(bytecode, readObjBytecode)
	if err != nil {
		return exception, err
	}
	return m.evalBytecode(obj)
}

// evalBytecode runs obj, a script or a module that was read from bytecode,
// and returns its value. It takes obj. It runs inside a call (enter).
func (m *VM) evalBytecode(obj jsValue) (jsValue, error) {
	if isException(obj) {
		return exception, m.errFromException()
	}
	var v jsValue
	if tag(obj) == tagModule {
		if m.jsResolveModule(obj) < 0 {
			m.jsFreeValue(obj)
			return exception, m.errFromException()
		}
		// What fails here stays pending, as qjs leaves it.
		m.setImportMeta(obj)
		// JS_EvalFunction takes obj: the Dup keeps the module for
		// freeTopLevel once it has run.
		dupValue(obj)
		defer m.jsFreeValue(obj)
		v = m.settled(m.jsEvalFunction(obj))
		if !isException(v) {
			var err error
			if v, err = m.await(v); err != nil {
				return exception, err
			}
			// A module that failed or still waits keeps its top level.
			m.freeTopLevel(obj)
		}
	} else {
		v = m.settled(m.jsEvalFunction(obj))
	}
	if isException(v) {
		return exception, m.errFromException()
	}
	return v, nil
}

// errAwaitPending is the error of a module whose top-level await is still
// pending once no job is left to run.
var errAwaitPending = errors.New("slowjs: the module's top-level await is still pending and no job is left to run")

// await runs the jobs of m until the promise v settles, and returns its value
// or its error. It takes v. The first job that fails is the error, once v is
// fulfilled. An uncatchable error ends it at once, and the jobs after it stay
// queued. It does not use js_std_await of qjs, which read state a VM never
// made when the queue ran dry, and printed the jobs that failed to stderr.
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
			m.endJob()
			r := m.jsExecutePendingJob()
			if r == 0 {
				if jobErr == nil {
					return exception, errAwaitPending
				}
				return exception, errors.Join(errAwaitPending, jobErr)
			}
			if r > 0 && !m.uncatchablePending() {
				continue
			}
			e, uncatchable := m.takeException()
			if uncatchable {
				return exception, e
			}
			if jobErr == nil {
				jobErr = e
			}
		default:
			// Not a promise.
			return dupValue(v), nil
		}
	}
}

// Compile compiles javascript, a script or with EvalModule a module, to
// bytecode, which [VM.EvalBytecode] or [VM.EvalBytecodeValue] runs. The
// format of the bytecode belongs to the release of QuickJS the engine is.
// Compiling is inside the eval timeout.
func (m *VM) Compile(javascript string, flags int) ([]byte, error) {
	v, err := m.eval(javascript, flags|evalFlagCompileOnly)
	if err != nil {
		return nil, err
	}
	defer m.jsFreeValue(v)
	if isException(v) {
		return nil, m.errFromException()
	}
	return m.writeBytecode(v)
}

// eval evaluates javascript as the eval methods do, in a call of its own, and
// returns its value, the exception marker when it threw.
func (m *VM) eval(javascript string, flags int) (r jsValue, err error) {
	if err = m.enter(); err != nil {
		return exception, err
	}
	defer m.leave(&err)
	v, err := m.jsEval(evalName, javascript, int32(flags))
	if err != nil {
		return exception, err
	}
	return m.settled(v), nil
}

// writeBytecode returns the bytecode of v, a compiled script or module.
func (m *VM) writeBytecode(v jsValue) ([]byte, error) {
	b, ok := m.jsWriteObject(v)
	if !ok {
		return nil, m.errFromException()
	}
	return b, nil
}

// maxPendingJobs is how many jobs ExecutePendingJobs runs at most in one call.
const maxPendingJobs = 1 << 20

// ExecutePendingJobs runs the jobs queued in m, the reactions of promises and
// what follows an await, until the queue is empty, and returns how many it
// ran. Nothing runs them on its own: a program that uses promises calls it,
// such as at the end of each turn of its event loop.
//
// A job that throws does not stop the others, and the first such exception
// is the error. A throw in a then callback is no such job: it rejects the
// promise then returned. An uncatchable error, such as the eval timeout,
// ends the call at once, and the jobs after it stay queued. A queue that
// keeps feeding itself is cut after 1,048,576 jobs, with an error.
func (m *VM) ExecutePendingJobs() (n int, err error) {
	if err = m.enter(); err != nil {
		return 0, err
	}
	defer m.leave(&err)
	for n < maxPendingJobs {
		m.endJob()
		r := m.jsExecutePendingJob()
		if r == 0 {
			return n, err
		}
		n++
		if r > 0 && !m.uncatchablePending() {
			continue
		}
		e, uncatchable := m.takeException()
		if uncatchable {
			return n, e
		}
		if err == nil {
			err = e
		}
	}
	if err == nil {
		err = fmt.Errorf("slowjs: aborted draining after %d pending jobs", maxPendingJobs)
	}
	return n, err
}

// GlobalObject returns the global object of m, which the caller frees.
func (m *VM) GlobalObject() (r Value) {
	return Value{vm: m, v: m.jsGetGlobalObject()}
}

// maxEvalName is the most bytes of the name of an eval (cleanEvalName).
const maxEvalName = 1024

// cleanEvalName returns the file name the engine is given for name: <eval>
// for none, and else name as valid UTF-8 of one line, each character that is
// a control character (U+0000 to U+001F, U+007F, U+0080 to U+009F), U+2028
// or U+2029, and each byte that starts no valid character, made U+FFFD, cut
// before the first character that would take it past maxEvalName bytes. A
// stack takes a line for each frame, and the engine reads the name as a C
// string. A name that needs none of it is returned as it is.
func cleanEvalName(name string) string {
	if name == "" {
		return evalName
	}
	if len(name) <= maxEvalName && printableASCII(name) {
		return name
	}
	b := make([]byte, 0, min(len(name)+8, maxEvalName))
	for _, c := range name {
		if badNameChar(c) {
			c = utf8.RuneError
		}
		if len(b)+utf8.RuneLen(c) > maxEvalName {
			break
		}
		b = utf8.AppendRune(b, c)
	}
	return string(b)
}

// printableASCII reports whether every byte of s is from 0x20 to 0x7e.
func printableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// badNameChar reports whether c has no place in a file name of a stack.
func badNameChar(c rune) bool {
	return c < 0x20 || c == 0x7f || (c >= 0x80 && c < 0xa0) || c == 0x2028 || c == 0x2029
}

// evalNameFault says why cleanEvalName changes name.
func evalNameFault(name string) string {
	if !utf8.ValidString(name) {
		return "it is not valid UTF-8"
	}
	for _, c := range name {
		if badNameChar(c) {
			return "it has a control character, U+2028 or U+2029"
		}
	}
	return fmt.Sprintf("it is longer than %d bytes", maxEvalName)
}
