// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// pollDue counts the counter of a context down as js_poll_interrupts does.
func TestPollDue(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	// The engine's memory, as the context the interpreter polls is.
	ctx := libc.Xcalloc(tls, 1, Tsize_t(unsafe.Sizeof(TJSContext{})))
	if ctx == 0 {
		t.Fatal("out of memory")
	}
	defer libc.Xfree(tls, ctx)
	c := (*TJSContext)(unsafe.Pointer(ctx))
	p := ctx + unsafe.Offsetof(c.Finterrupt_counter)
	for _, n := range []int32{3, 2, 1, 0, -5} {
		c.Finterrupt_counter = n
		got := pollDue(p)
		if want := n-1 <= 0; got != want || c.Finterrupt_counter != n-1 {
			t.Errorf("counter %d: pollDue = %v, counter %d, want %v, %d", n, got, c.Finterrupt_counter, want, n-1)
		}
	}
}

// The interpreter calls the interrupt handler once in
// JS_INTERRUPT_COUNTER_INIT polls, and a handler that answers 1 stops the
// script at the next call.
func TestPollHandler(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)
	XJS_SetInterruptHandler(tls, rt, funcPointer(pollInterrupt), 0)

	// Each pass of the loop polls twice: at the jump of its test and at
	// the jump back. Every jump of the interpreter polls.
	const loops = 20 * MJS_INTERRUPT_COUNTER_INIT
	const want = 2 * loops / MJS_INTERRUPT_COUNTER_INIT
	pollCalls, pollStop = 0, 0
	v := eval0(t, tls, ctx, "for (let i = 0; i < 200000; i++) {}")
	if tag(v) == EJS_TAG_EXCEPTION {
		t.Fatal("the loop threw")
	}
	XFreeValue(tls, ctx, v)
	if pollCalls < want || pollCalls > want+1 {
		t.Errorf("%d passes: %d calls of the handler, want %d or %d", loops, pollCalls, want, want+1)
	}

	pollCalls, pollStop = 0, 1
	v = eval0(t, tls, ctx, "for (;;) {}")
	if tag(v) != EJS_TAG_EXCEPTION || pollCalls != 1 {
		t.Errorf("an endless loop: tag %d, %d calls of the handler, want an exception after 1", tag(v), pollCalls)
	}
	XFreeValue(tls, ctx, v)
}

var (
	pollCalls int   // the calls of pollInterrupt
	pollStop  int32 // what pollInterrupt returns
)

func pollInterrupt(tls *libc.TLS, rt, opaque uintptr) int32 {
	pollCalls++
	return pollStop
}
