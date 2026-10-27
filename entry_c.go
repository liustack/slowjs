// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && linux && amd64

package slowjs // import "github.com/liustack/slowjs"

// Entering and leaving the C engine: the timeout and Interrupt, which the
// host's polls carry out (gohost), and the VM a panic through the engine's
// frames leaves broken.

import (
	"github.com/liustack/slowjs/internal/gohost"
)

// interrupter is how Interrupt reaches the host of the VM, which stays in Go
// memory after Close.
type interrupter struct {
	h *gohost.VM
}

// Interrupt requests 'm' to interrupt Javascript evaluation. Interrupt
// can be called asynchronously, from any goroutine, at any time m is
// evaluating a script. The request stays pending until the outermost call
// into javascript returns, so javascript a host function evaluates on m
// meanwhile does not clear it. Interrupt is also safe while m closes and after
// Close, where it does nothing, so a watchdog need not know whether the
// script it guards has finished. A script that runs on past the grace of the
// host after it is killed: the call returns an interrupted error, and the VM
// is good only for Close.
func (m *VM) Interrupt() {
	m.intr.h.Interrupt()
}

// enter marks the start of a call into javascript, leave its end. Only the
// outermost call arms the eval timeout and clears a pending Interrupt (the
// host does both). A host function evaluating more javascript runs inside
// the budget of the call that got it there, and an Interrupt requested
// meanwhile stays pending.
//
// leave takes the error the call returns, deferred as m.leave(&err): when a
// poll killed the script, the outermost leave sets it to the interrupted
// error, and the VM is broken.
//
// A memory fault that leaves the call went up through the engine's frames:
// each level marks the VM broken, so that a Go function that called this one
// from javascript and recovers the panic finds it so (called). A panic or a
// Goexit that started in Go code the engine called is marked where the engine
// called it (called, gohost). A panic of a Go function called from
// javascript (repanic) went up through no frame of the engine and leaves the
// VM as it is.
func (m *VM) enter() {
	if m.h.Broken() {
		panic(brokenPanic)
	}
	if m.entered == 0 && m.sp != m.start {
		panic("slowjs: the shadow stack of a VM that runs nothing is not empty")
	}
	m.h.EnterScript(m.timeout)
	m.entered++
	m.depth++
}

func (m *VM) leave(err *error) {
	m.depth--
	m.entered--
	m.h.Leave()
	// leave recovers only to tell a return from a panic: a panic goes on with
	// the same value.
	if p := recover(); p != nil {
		if isFault(p) {
			m.h.MarkBroken()
		}
		if m.entered == 0 {
			m.panicked = nil
			if p == gohost.ErrKilled {
				*err = killedError()
				return
			}
		}
		panic(p)
	}
	m.repanic()
}

// killedError is the error of a call whose script a poll killed.
func killedError() *Error {
	return &Error{engine: true, raw: "InternalError: interrupted", Name: "InternalError", Message: "interrupted",
		Uncatchable: true, Interrupted: true}
}

// outer runs f, which calls into the engine and runs no script, inside an
// entry of the host: a poll in it never kills, and a memory fault in it is a
// panic. A panic or a Goexit that leaves f went through the engine's frames,
// and the VM is broken.
func (m *VM) outer(f func()) {
	if m.h.Broken() {
		panic(brokenPanic)
	}
	if m.sp != m.start {
		panic("slowjs: the shadow stack of a VM that runs nothing is not empty")
	}
	m.h.Enter()
	m.entered++
	ok := false
	defer func() {
		m.entered--
		m.h.Leave()
		if !ok {
			m.h.MarkBroken()
		}
	}()
	f()
	ok = true
}

// isFault reports whether p is a memory fault, which SetPanicOnFault makes a
// panic of while the engine runs.
func isFault(p any) bool {
	_, ok := p.(interface{ Addr() uintptr })
	return ok
}

const (
	// framesLeftPanic is what a call from javascript panics with when the
	// Go function it called returned after a panic went up through the
	// engine below it: the frames of the engine it left are gone.
	framesLeftPanic = "slowjs: a Go function recovered a panic that went up through the engine"
	// brokenPanic is what a call into javascript panics with after a Go
	// panic went up through the engine.
	brokenPanic = "slowjs: a Go panic went up through the engine of this VM, which is good only for Close"
)
