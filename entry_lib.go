// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

import (
	"sync"
	"sync/atomic"
	"unsafe"

	"modernc.org/libc"
)

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

type interruptData struct {
	interrupt int32 // Interrupt() pending
	timeLimit int64 // stop when monoNow() >= timeLimit, 0 for no limit
}

// interrupter is how Interrupt reaches the interrupt flag, which lives in C
// memory that Close frees. It lives in Go memory, so it stays valid after
// Close, and its mutex orders Interrupt and Close: the race detector does not
// see the atomic operations on the flag itself, which is outside Go's heap.
type interrupter struct {
	mu   sync.Mutex
	flag uintptr // the interruptData, 0 once Close has started
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
//
// A Go panic that went up through the engine left its TLS frames on the
// stack: leave puts the stack back where enter found it, and enter panics
// from then on (see hostEnded). When the outermost call returned and left
// frames, an engine function did not free its frame: leave panics with
// framesLeftPanic.
func (m *VM) enter() {
	if m.broken {
		panic(brokenPanic)
	}
	if m.depth == 0 {
		m.entry = markTLS(m.runtime.tls)
		m.configureInterrupt()
	}
	m.depth++
}

func (m *VM) leave() {
	m.depth--
	if m.depth == 0 {
		// leave recovers only to tell a return from a panic: a panic goes
		// on with the same value, and the stack trace still holds where it
		// started, as leave panics again before it returns.
		p := recover()
		if m.entry.restore(m.runtime.tls) {
			// A Goexit that went up through the engine set broken in
			// the Go function that called it.
			if p == nil && !m.broken {
				p = framesLeftPanic
			}
			m.broken = true
		}
		if p != nil {
			m.panicked = nil
			panic(p)
		}
	}
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

// inHost runs f, Go code that javascript called, counted in m.host. A
// panic in f does not go through the engine (see hostPanicked).
func (m *VM) inHost(name string, f func()) {
	m.host++
	mark := markTLS(m.runtime.tls)
	returned := false
	defer func() {
		m.host--
		if m.hostEnded(mark, returned) {
			return
		}
		switch p := recover(); {
		case p != nil:
			m.hostPanicked(name, p)
		case !returned:
			m.broken = true // runtime.Goexit, which goes on up through the engine
		}
	}()
	f()
	returned = true
}

// hostEnded puts the TLS stack back at mark, taken where javascript called
// a Go function that has now ended, and reports whether engine frames were
// left above it. The engine below the Go function goes on, and must not
// free such a frame as its own (see tlsMark).
//
// When the Go function did not return, a Go panic or runtime.Goexit went
// up through engine functions, javascript that the Go function evaluated,
// and left the engine in a state that is not known. The engine's stack, for
// one, still points at the frames that were dropped. So the panic is not
// recovered there: it goes on up through the engine, which frees nothing on
// the way, to the Go code that called javascript. When the Go function
// returned, either it recovered such a panic, or an engine function did
// not free its frame, and hostEnded panics with framesLeftPanic, which goes
// up the same way. Either way the VM is then good only for Close: every
// later call into javascript panics with brokenPanic.
func (m *VM) hostEnded(mark tlsMark, returned bool) bool {
	if !mark.restore(m.runtime.tls) {
		return false
	}
	m.broken = true
	if returned {
		panic(framesLeftPanic)
	}
	return true
}

const (
	// framesLeftPanic is what the VM panics with when engine frames are
	// left on the TLS stack after javascript returned to Go.
	framesLeftPanic = "slowjs: the engine returned with frames left on its TLS stack: a frame was not freed, or a Go function recovered a panic that went up through the engine"
	// brokenPanic is what a call into javascript panics with after a Go
	// panic went up through the engine.
	brokenPanic = "slowjs: a Go panic went up through the engine of this VM, which is good only for Close"
)
