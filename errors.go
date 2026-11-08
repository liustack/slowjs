// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"fmt"
)

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
	raw    string
	engine bool // made of an exception, raw is the engine's text
	killed bool // the script was killed (ErrKilled)

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

// ErrKilled is what [errors.Is] finds in the error of a call whose script
// was killed, and in the error of each method called on its VM after that.
// A script that runs on 50 ms past its eval timeout or an [VM.Interrupt], and
// has kept the engine at work for 1000 of its polls since (6 to 30 ms of a
// loop on a fast core), is killed, as happens in a loop of the engine that
// does not look for the interrupt or in a long collection. Time in which the
// engine does no work, in a Go function the script called or while the OS or
// the Go scheduler runs something else, counts toward the 50 ms but not the
// polls. The call returns an *Error with Uncatchable and Interrupted set, and
// the VM is good only for Close. Its methods that return an error return one
// that matches ErrKilled, the others panic, and [Value.Free] does nothing.
// The exceptions are [VM.MemorySize] and [VM.GCThreshold], which only read a
// counter and answer as they do on any VM.
var ErrKilled = errors.New("quickjs: the script did not stop in time and was killed")

// errKilledVM is the error of a method called after a kill.
var errKilledVM = fmt.Errorf("quickjs: the VM is good only for Close: %w", ErrKilled)

// Is reports whether e is the error of a killed call, for target ErrKilled.
func (e *Error) Is(target error) bool {
	return e.killed && target == ErrKilled
}

// Error implements the error interface. It returns the same string the engine
// produces for the exception, unchanged from earlier releases. For an Error
// made in Go, say to throw from a [Builtin], it returns what javascript's
// Error.prototype.toString does: Name, or "Error" when Name is empty, then
// ": " and Message when Message is not empty.
func (e *Error) Error() string {
	if e.engine {
		return e.raw
	}

	name := e.Name
	if name == "" {
		name = "Error"
	}
	if e.Message == "" {
		return name
	}

	return name + ": " + e.Message
}

// errStrProp reads the string-valued property 'name' from error value 'v'
// for readError, returning "" when the property is absent or not a string,
// when reading it throws, or when converting it does. ok is false when that
// was an uncatchable exception, which stays pending (see readError). 'v' is
// borrowed.
func (m *VM) errStrProp(v jsValue, name uint32) (r string, ok bool) {
	vr := m.jsGetProperty(v, name)
	if isException(vr) {
		return "", m.dropCatchable()
	}

	defer m.jsFreeValue(vr)

	if tag(vr) == tagString || tag(vr) == tagStringRope {
		return m.errString(vr)
	}

	return "", true
}

// errIntProp reads the integer-valued property 'name' from error value 'v'
// for readError, returning 0 when the property is absent or not numeric, or
// when reading it throws. ok is as for errStrProp. 'v' is borrowed.
func (m *VM) errIntProp(v jsValue, name uint32) (r int, ok bool) {
	vr := m.jsGetProperty(v, name)
	if isException(vr) {
		return 0, m.dropCatchable()
	}

	defer m.jsFreeValue(vr)

	switch tag(vr) {
	case tagInt:
		return int(jsvToInt32(vr)), true
	case tagFloat64:
		return int(jsvToFloat64(vr)), true
	}

	return 0, true
}

// ErrorFromValue extracts structured error information from JavaScript error
// value 'v', typically an Error instance caught from a JS exception.
//
// 'v' is borrowed: ErrorFromValue does not free it and the caller remains
// responsible for its lifetime. Like all VM operations it must be called from
// the VM's own goroutine. See [Error] for which fields are populated.
//
// A field that cannot be read because of what the script throws is left
// empty: the string form of 'v' (the result of Error) when its toString
// throws, and a property whose getter throws. That exception is dropped, so
// ErrorFromValue leaves no exception pending.
//
// Reading runs javascript (a getter, or a toString that a script replaced), so
// it runs inside the eval timeout like [VM.Eval]. When the read is stopped
// by what ends a call, the eval timeout, [VM.Interrupt] or running out of
// memory, ErrorFromValue returns that error in place of the fields of 'v':
// Uncatchable is set, and Interrupted or OutOfMemory says which, as in the
// error of a call. The read stops there. Inside a Go function that
// javascript called, that error also stays pending and ends the javascript
// that called the function, as every call into javascript does then.
//
// A panic of a Go function that the read calls is not returned: it goes on
// to the caller of ErrorFromValue, as from any call into javascript. On a VM
// a kill left good only for Close, ErrorFromValue panics.
func ErrorFromValue(v Value) *Error {
	return v.vm.errorFrom(v.v)
}

// errorFrom is ErrorFromValue of v, borrowed, which can also be the
// exception takeException took. A script cannot make an *Error with
// Uncatchable set: only an uncatchable exception and a kill give one.
func (m *VM) errorFrom(v jsValue) (e *Error) {
	var err error
	if err = m.enter(); err != nil {
		panic(brokenPanic)
	}
	defer func() {
		if err != nil {
			// A poll killed the script that read the error.
			e = killedError()
		}
	}()
	defer m.leave(&err)

	e, ok := m.readError(v)
	if !ok {
		e, _ = m.takeException()
	}
	return e
}

// readError reads the fields of 'v', borrowed, as ErrorFromValue says. ok is
// false when a read met an uncatchable exception: it stays pending, for the
// caller to take as the error in place of 'v', and the fields are not read
// further. So is one that javascript the read ran left pending without
// failing (see settled).
func (m *VM) readError(v jsValue) (e *Error, ok bool) {
	e = &Error{engine: true}
	if e.raw, ok = m.errString(v); !ok {
		return nil, false
	}
	if e.Name, ok = m.errStrProp(v, atomName); !ok {
		return nil, false
	}
	if e.Message, ok = m.errStrProp(v, atomMessage); !ok {
		return nil, false
	}
	if e.Stack, ok = m.errStrProp(v, atomStack); !ok {
		return nil, false
	}
	if e.LineNumber, ok = m.errIntProp(v, atomLineNumber); !ok {
		return nil, false
	}
	if e.ColumnNumber, ok = m.errIntProp(v, atomColumnNumber); !ok {
		return nil, false
	}
	if e.FileName, ok = m.errStrProp(v, atomFileName); !ok {
		return nil, false
	}
	if m.uncatchablePending() {
		return nil, false
	}
	return e, true
}

func (m *VM) errFromException() error {
	err, _ := m.takeException()
	return err
}

// takeException takes the pending exception out of the context and returns
// it as an *Error, and whether it was uncatchable (an interrupt, running out
// of memory, or a panic in a Go function). Reading an exception the script
// threw runs javascript: when that meets an uncatchable exception, the
// uncatchable one is what takeException takes and returns, as the call
// ends with it.
func (m *VM) takeException() (err *Error, uncatchable bool) {
	uncatchable = m.uncatchablePending()
	e := m.jsGetException()
	// The flag outlives the exception. Clear it, so that it does not stay
	// with the next exception the engine fails to throw.
	m.jsSetUncatchableException(false)
	// Deferred: a Go function that panics while e is read goes on panicking
	// through here (repanic).
	defer m.jsFreeValue(e)

	if !uncatchable {
		// Reading e runs javascript, inside the eval timeout. When that
		// met an uncatchable exception, it is the error (errorFrom).
		err = m.errorFrom(e)
		return err, err.Uncatchable
	}

	if tag(e) == tagNull {
		// The engine throws null when it runs out of memory building an
		// error.
		err = outOfMemoryError()
	} else {
		err = m.uncatchableError(e)
	}
	// No script can throw an uncatchable exception, and none runs between
	// the engine making it and here (uncatchableError runs none either), so
	// this is one of the errors the engine or the VM ends a call with, with
	// the message it was made with: an interrupt, running out of memory, or
	// a panic in a Go function (hostPanicked).
	err.Uncatchable = true
	switch err.Message {
	case "out of memory":
		err.OutOfMemory = true
	case "interrupted":
		err.Interrupted = true
	}
	defer m.repanic()
	if m.host > 0 {
		// Inside a Go function called from javascript, the exception ends
		// that javascript too: it stays pending, so that every call into
		// javascript fails until callGo returns it to the engine.
		m.jsThrow(dupValue(e))
		m.jsSetUncatchableException(true)
	}
	return err, true
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
	m.jsThrowError(errorKindInternal, fmt.Sprintf("panic in Go function %s: %v", name, p))
	m.jsSetUncatchableException(true)
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
func (m *VM) uncatchableError(e jsValue) *Error {
	message := m.ownString(e, atomMessage)
	raw := "InternalError"
	if message != "" {
		raw += ": " + message
	}
	return &Error{engine: true, raw: raw, Name: "InternalError", Message: message, Stack: m.ownString(e, atomStack)}
}

// ownString returns the own property atom of v, an error the engine or the
// VM made, when it is a string, and "" otherwise. It runs no javascript: the
// engine defined message and stack on v as data properties, and nothing ran
// since, so the read finds them on v and calls nothing, and a property v
// lacks is not looked up on the prototype, where page code could have put a
// getter. Reading a stack the engine has not formatted yet formats it, and
// making the Go string of a string built by concatenation needs memory:
// running out of memory there is dropped, uncatchable flag and all.
func (m *VM) ownString(v jsValue, atom uint32) string {
	switch m.jsGetOwnProperty(v, atom) {
	case 0:
		return ""
	case 1:
	default:
		m.dropUncatchable()
		return ""
	}

	p := m.jsGetPropertyInternal(v, atom, v, 0)
	if isException(p) {
		m.dropUncatchable()
		return ""
	}

	defer m.jsFreeValue(p)

	if tag(p) != tagString && tag(p) != tagStringRope {
		return ""
	}

	str, ok := m.argString(p, false)
	if !ok {
		m.dropUncatchable()
	}
	return str
}

// dropUncatchable drops the exception pending in ctx and clears the
// uncatchable flag running out of memory sets, so that it does not stay
// with the next exception thrown (see takeException).
func (m *VM) dropUncatchable() {
	m.dropException()
	m.jsSetUncatchableException(false)
}

// outOfMemoryError is the error the engine throws when it runs out of
// memory.
func outOfMemoryError() *Error {
	return &Error{engine: true, raw: "InternalError: out of memory", Name: "InternalError", Message: "out of memory"}
}

// uncatchablePending reports whether an uncatchable exception (an
// interrupt) is pending in m's context. Every call into javascript fails
// while one is.
func (m *VM) uncatchablePending() bool {
	return m.jsIsUncatchableException()
}

// settled returns v, the result of a call into javascript, or the exception
// value when an uncatchable exception is still pending after the call. A
// built-in that only records an error, such as a promise resolve function
// that rejects its promise when reading 'then' throws, returns as if it had
// not failed, and the code that called it can go on until its next call,
// which fails. The call that ran it fails all the same.
func (m *VM) settled(v jsValue) jsValue {
	if isException(v) || !m.uncatchablePending() {
		return v
	}

	m.jsFreeValue(v)
	return exception
}
