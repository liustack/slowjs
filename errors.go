// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"fmt"
)

// Error holds structured information from a JavaScript exception. The error
// returned by [VM.Eval], [VM.Call] and the other eval-family methods is an
// *Error; recover the fields with [errors.As]:
//
//	var e *slowjs.Error
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
// The C engine (-tags slowjs_cengine) kills a script that runs on 50 ms past
// its eval timeout or an [VM.Interrupt], as can happen in a Go function it
// called or in a long collection: the call returns an *Error with
// Uncatchable and Interrupted set, and the VM is good only for Close. Its
// methods that return an error return one that matches ErrKilled, the others
// panic, and [Value.Free] does nothing. The exceptions are [VM.MemorySize] and
// [VM.GCThreshold], which only read a counter and answer as they do on any VM.
// The engine of lib never kills.
var ErrKilled = errors.New("slowjs: the script did not stop in time and was killed")

// errKilledVM is the error of a method called after a kill.
var errKilledVM = fmt.Errorf("slowjs: the VM is good only for Close: %w", ErrKilled)

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

// errStrProp reads the string-valued property 'name' from error value 'v',
// returning "" when the property is absent or not a string, when reading it
// throws, or when there is no memory to convert it. Like errString it drops
// the exception, see there. 'v' is borrowed.
func (m *VM) errStrProp(v jsValue, name uint32) (r string) {
	vr := m.jsGetProperty(v, name)
	if isException(vr) {
		m.dropException()
		return ""
	}

	defer m.jsFreeValue(vr)

	if tag(vr) == tagString || tag(vr) == tagStringRope {
		return m.errString(vr)
	}

	return ""
}

// errIntProp reads the integer-valued property 'name' from error value 'v',
// returning 0 when the property is absent or not numeric, or when reading it
// throws. Like errString it drops the exception, see there. 'v' is borrowed.
func (m *VM) errIntProp(v jsValue, name uint32) (r int) {
	vr := m.jsGetProperty(v, name)
	if isException(vr) {
		m.dropException()
		return 0
	}

	defer m.jsFreeValue(vr)

	switch tag(vr) {
	case tagInt:
		return int(jsvToInt32(vr))
	case tagFloat64:
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
	m := v.vm

	return &Error{
		engine:       true,
		raw:          m.errString(v.v),
		Name:         m.errStrProp(v.v, atomName),
		Message:      m.errStrProp(v.v, atomMessage),
		Stack:        m.errStrProp(v.v, atomStack),
		LineNumber:   m.errIntProp(v.v, atomLineNumber),
		ColumnNumber: m.errIntProp(v.v, atomColumnNumber),
		FileName:     m.errStrProp(v.v, atomFileName),
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
	uncatchable = m.uncatchablePending()
	e := m.jsGetException()
	// The flag outlives the exception. Clear it, so that it does not stay
	// with the next exception the engine fails to throw.
	m.jsSetUncatchableException(false)

	defer m.jsFreeValue(e)

	switch {
	case uncatchable && tag(e) == tagNull:
		// The engine throws null when it runs out of memory building an
		// error.
		err = outOfMemoryError()
	case uncatchable:
		err = m.uncatchableError(e)
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
		m.jsThrow(dupValue(e))
		m.jsSetUncatchableException(true)
	}
	return err, uncatchable
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
