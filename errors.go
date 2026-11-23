// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs // import "github.com/liustack/tifajs"

// The errors of calls into javascript: reading an exception into an *Error,
// the exceptions no script can catch, and the panic of a Go function that
// javascript called.

import (
	"errors"
	"fmt"
)

// Error is a javascript exception that ended a call, read into Go. Get it from
// the error of a call with errors.As:
//
//	if _, err := m.Eval(js, tifajs.EvalGlobal); err != nil {
//		var e *tifajs.Error
//		if errors.As(err, &e) && e.Interrupted {
//			// the eval timeout or Interrupt stopped the script
//		}
//	}
//
// The text of Error() is what the engine has always given for the exception,
// such as "TypeError: x is not a function", or the string of a value that is
// not an error, such as "42" for throw 42.
type Error struct {
	// raw is the text of the exception as the engine gave it, which Error
	// returns when engine is set: the error was read from javascript. killed
	// is set on the error of a call whose script was killed.
	raw    string
	engine bool
	killed bool

	// Name and Message are the name and message properties of the exception,
	// empty when it has none, as for throw 42. Stack is its stack property.
	Name    string
	Message string
	Stack   string
	// LineNumber, ColumnNumber and FileName are set for a syntax error only.
	// The error of a script that ran has its place in Stack alone.
	LineNumber   int
	ColumnNumber int
	FileName     string

	// Uncatchable reports that the call ended with an exception that no
	// catch or finally of the script got to see: the VM ran out of memory
	// (OutOfMemory is set too), Interrupt or the eval timeout stopped the
	// script (Interrupted is set too), or a Go function that javascript
	// called panicked (neither is set). Nothing a script throws is
	// uncatchable, whatever its name and message.
	Uncatchable bool
	OutOfMemory bool
	Interrupted bool
}

// Error returns the text of the exception. For an Error made in Go, which
// was not read from javascript, it is what Error.prototype.toString makes of
// Name and Message: Name, "Error" when Name is empty, followed by ": " and
// Message unless Message is empty.
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

// Is reports whether target is [ErrKilled] and e is the error of a call whose
// script was killed.
func (e *Error) Is(target error) bool {
	return e.killed && target == ErrKilled
}

// ErrorFromValue reads the error value v, such as an exception a script
// caught, as the error of a call is read. It borrows v. Reading it runs
// javascript, a getter of its message or a toString of the page, inside the
// eval timeout (see [VM.SetEvalTimeout]). A field it cannot read stays
// empty. It panics when v belongs to no VM, and on a VM a kill left good only
// for Close.
func ErrorFromValue(v Value) *Error {
	return v.vm.errorFrom(v.v, nil)
}

var (
	// errForeignValue is the error of a method given a Value of another VM,
	// which it neither uses nor frees.
	errForeignValue = errors.New("tifajs: a Value of another VM")
	// errKilledVM is the error of a method on a VM a kill left good only for
	// Close.
	errKilledVM = fmt.Errorf("tifajs: the VM is good only for Close: %w", ErrKilled)
)

// foreign reports whether v belongs to a VM other than m. A Value of no VM,
// UndefinedValue or the zero Value, belongs to every VM.
func (m *VM) foreign(v Value) bool {
	return v.vm != m && v.vm != nil
}

// uncatchablePending reports whether the exception pending in m is one no
// script can catch.
func (m *VM) uncatchablePending() bool {
	return m.jsIsUncatchableException()
}

// settled is v, the result of javascript that ran, unless the run left an
// uncatchable exception pending: then v is freed and the exception marker
// takes its place. A built-in that runs out of memory in a step it does not
// check returns as if it had not failed, and the code after it goes on until
// its next call fails, but the call that ran it has failed all the same.
func (m *VM) settled(v jsValue) jsValue {
	if isException(v) || !m.uncatchablePending() {
		return v
	}
	m.jsFreeValue(v)
	return exception
}

// errFromException takes the pending exception out of m and returns it as an
// error.
func (m *VM) errFromException() error {
	err, _ := m.takeException()
	return err
}

// takeException takes the pending exception out of m, reads it into an
// *Error and reports whether it was uncatchable.
func (m *VM) takeException() (err *Error, uncatchable bool) {
	return m.takeExceptionCut(nil)
}

// takeExceptionCut is takeException with the texts cut as t says, nil for
// whole.
//
// The uncatchable flag is cleared as the exception is taken: it outlives the
// exception in the engine, and left set it would make the next exception that
// the engine fails to throw look uncatchable. An uncatchable exception is
// read without running javascript (uncatchableError). Inside a Go function
// that javascript called it is thrown again, flag and all, so that it also
// ends the javascript that called the function when the function returns.
func (m *VM) takeExceptionCut(t *textCut) (err *Error, uncatchable bool) {
	uncatchable = m.uncatchablePending()
	e := m.jsGetException()
	m.jsSetUncatchableException(false)
	// Deferred: reading e runs Go code that may panic, and the panic goes on
	// through here.
	defer m.jsFreeValue(e)
	if !uncatchable {
		err = m.errorFrom(e, t)
		// An uncatchable exception met while e was read is the error.
		return err, err.Uncatchable
	}
	if tag(e) == tagNull {
		// What the engine throws when it has no memory to make the error.
		err = outOfMemoryError()
	} else {
		err = m.uncatchableError(e, t)
	}
	err.Uncatchable = true
	switch err.Message {
	case "out of memory":
		err.OutOfMemory = true
	case "interrupted":
		err.Interrupted = true
	}
	// A Go function's panic goes on here, before e is freed.
	defer m.repanic()
	if m.host > 0 {
		m.jsThrow(dupValue(e))
		m.jsSetUncatchableException(true)
	}
	return err, true
}

// outOfMemoryError is the error of running out of memory, for an exception
// the engine could not make. The caller sets Uncatchable.
func outOfMemoryError() *Error {
	return &Error{engine: true, raw: "InternalError: out of memory", Name: "InternalError", Message: "out of memory"}
}

// errorFrom reads v into an *Error, inside a call into javascript of its own:
// the reads run javascript, which the eval timeout bounds. It borrows v. When
// the read meets an uncatchable exception, that is the error.
func (m *VM) errorFrom(v jsValue, t *textCut) (e *Error) {
	var err error
	if m.enter() != nil {
		panic(brokenPanic)
	}
	// Registered first, so that it runs after leave: leave sets err when the
	// script of the read was killed.
	defer func() {
		if err != nil {
			e = killedError()
		}
	}()
	defer m.leave(&err)
	e, ok := m.readError(v, t)
	if !ok {
		e, _ = m.takeExceptionCut(t)
	}
	return e
}

// readError reads the text of v and its properties name, message, stack,
// lineNumber, columnNumber and fileName, in that order, each once. ok is
// false, with the uncatchable exception pending, when the read met one: what
// it read so far is dropped. An exception the javascript of a read throws is
// dropped, and the field stays empty: v is the error to report.
func (m *VM) readError(v jsValue, t *textCut) (e *Error, ok bool) {
	e = &Error{engine: true}
	if e.raw, ok = m.errString(v, t); !ok {
		return nil, false
	}
	if e.Name, ok = m.stringProp(v, atomName, t); !ok {
		return nil, false
	}
	if e.Message, ok = m.stringProp(v, atomMessage, t); !ok {
		return nil, false
	}
	if e.Stack, ok = m.stringProp(v, atomStack, t); !ok {
		return nil, false
	}
	if e.LineNumber, ok = m.intProp(v, atomLineNumber); !ok {
		return nil, false
	}
	if e.ColumnNumber, ok = m.intProp(v, atomColumnNumber); !ok {
		return nil, false
	}
	if e.FileName, ok = m.stringProp(v, atomFileName, t); !ok {
		return nil, false
	}
	// The javascript of a read can leave an uncatchable exception pending and
	// return as if it had not failed.
	if m.uncatchablePending() {
		return nil, false
	}
	return e, true
}

// errString is the string of v, as String(v) makes it, cut as t says.
func (m *VM) errString(v jsValue, t *textCut) (string, bool) {
	var s string
	var ok bool
	if t != nil {
		s, ok = m.cutString(v, t)
	} else {
		s, ok = m.argString(v, false)
	}
	if !ok {
		return "", m.dropCatchable()
	}
	return s, true
}

// stringProp reads the property atom of v when it is a string, and "" for
// any other value.
func (m *VM) stringProp(v jsValue, atom uint32, t *textCut) (string, bool) {
	x := m.jsGetProperty(v, atom)
	if isException(x) {
		return "", m.dropCatchable()
	}
	defer m.jsFreeValue(x)
	switch tag(x) {
	case tagString, tagStringRope:
		return m.errString(x, t)
	}
	return "", true
}

// intProp reads the property atom of v when it is a number, cut to an int,
// and 0 for any other value.
func (m *VM) intProp(v jsValue, atom uint32) (int, bool) {
	x := m.jsGetProperty(v, atom)
	switch tag(x) {
	case tagException:
		return 0, m.dropCatchable()
	case tagInt:
		return int(jsvToInt32(x)), true
	case tagFloat64:
		return int(jsvToFloat64(x)), true
	}
	m.jsFreeValue(x)
	return 0, true
}

// dropCatchable drops the exception pending in m, one the javascript of a
// read threw, and returns true. An uncatchable one stays, as it is to end the
// call, and dropCatchable returns false. Left pending, an exception the read
// threw would be taken by the next call into m, which has nothing to do with
// it.
func (m *VM) dropCatchable() bool {
	if m.uncatchablePending() {
		return false
	}
	m.jsFreeValue(m.jsGetException())
	return true
}

// uncatchableError reads the uncatchable exception e, an InternalError that
// the engine or hostPanicked made, without running javascript: the moment e
// leaves the engine javascript could run, and a toString or a name getter of
// the page could rewrite the message, which tells what ended the call. It
// reads only the own data properties message and stack.
func (m *VM) uncatchableError(e jsValue, t *textCut) *Error {
	msg := m.ownString(e, atomMessage, t)
	stack := m.ownString(e, atomStack, t)
	raw := "InternalError"
	if msg != "" {
		raw += ": " + msg
	}
	return &Error{engine: true, raw: raw, Name: "InternalError", Message: msg, Stack: stack}
}

// ownString reads the own property atom of v when it is a string, and "" for
// anything else. Whatever fails on the way, such as running out of memory as
// the engine makes the text of a stack for its first read, is dropped with
// the uncatchable flag: the error being read is the one to report.
func (m *VM) ownString(v jsValue, atom uint32, t *textCut) string {
	switch m.jsGetOwnProperty(v, atom) {
	case 0:
		return ""
	case 1:
	default:
		m.dropUncatchable()
		return ""
	}
	x := m.jsGetPropertyInternal(v, atom, v, 0)
	if isException(x) {
		m.dropUncatchable()
		return ""
	}
	defer m.jsFreeValue(x)
	switch tag(x) {
	case tagString, tagStringRope:
	default:
		return ""
	}
	var s string
	var ok bool
	if t != nil {
		s, ok = m.cutString(x, t)
	} else {
		s, ok = m.argString(x, false)
	}
	if !ok {
		m.dropUncatchable()
	}
	return s
}

// dropUncatchable drops the pending exception and the uncatchable flag.
func (m *VM) dropUncatchable() {
	m.jsFreeValue(m.jsGetException())
	m.jsSetUncatchableException(false)
}

// hostPanicked turns the panic p of the Go function name, which javascript
// called, into an uncatchable exception that ends the javascript as an
// interrupt does. p is kept for repanic, which goes on with it where the Go
// code that called javascript gets back. The panic must not go up through the
// engine: the engine would keep pointing at its frames that are gone, and
// walk them the next time it makes an error.
func (m *VM) hostPanicked(name string, p any) {
	m.panicked = p
	m.jsThrowError(errorKindInternal, fmt.Sprintf("panic in Go function %s: %v", name, p))
	m.jsSetUncatchableException(true)
}

// repanic goes on with the panic of a Go function that hostPanicked kept,
// if any.
func (m *VM) repanic() {
	if p := m.panicked; p != nil {
		m.panicked = nil
		panic(p)
	}
}
