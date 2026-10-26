// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build cengine

package cengine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/liustack/slowjs/internal/gohost"
)

// inChild runs the test again in a child process and fails when the child
// fails or does not end within a minute. A bug that never returns, blows the
// Go stack or touches unmapped memory then fails the test instead of hanging
// or ending this process.
func inChild(t *testing.T, run func(t *testing.T)) {
	t.Helper()
	if os.Getenv("CENGINE_CHILD") == t.Name() {
		run(t)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
	cmd.Env = append(os.Environ(), "CENGINE_CHILD="+t.Name())
	out, err := cmd.CombinedOutput()
	if len(out) > 4000 {
		out = append([]byte("...\n"), out[len(out)-4000:]...)
	}
	if ctx.Err() != nil {
		t.Fatalf("the child did not end within a minute:\n%s", out)
	}
	if err != nil {
		t.Fatalf("the child failed: %v\n%s", err, out)
	}
}

var errStop = errors.New("stopped by the test")

// leave panics with errStop or calls Goexit.
func leave(how string) {
	if how == "panic" {
		panic(errStop)
	}
	runtime.Goexit()
}

// onOwnGoroutine runs f on a goroutine of its own, so that a Goexit ends only
// that one, and returns what it panicked with.
func onOwnGoroutine(f func()) (recovered any) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { recovered = recover() }()
		f()
	}()
	<-done
	return recovered
}

// gone checks that the host forgot the VM and holds none of its memory.
func gone(t *testing.T, h *gohost.VM, when string) {
	t.Helper()
	if gohost.Lookup(h.ID()) != nil {
		t.Errorf("%s: VM %d is still in the host's registry", when, h.ID())
	}
	if u := h.Memory().Usage(); u.Chunks != 0 || u.Footprint != 0 || u.Stack != 0 {
		t.Errorf("%s: the host still holds %+v", when, u)
	}
}

// TestExceptionConversionThrows: when String() of what was thrown throws
// again, or reading or converting its stack does, Eval still returns an
// error, at once, and the VM goes on.
func TestExceptionConversionThrows(t *testing.T) {
	inChild(t, func(t *testing.T) {
		const unprintable = "an exception whose String() threw"
		for _, c := range []struct{ src, msg string }{
			{`const e = { toString() { throw this } }; throw e`, unprintable},
			{`const e = new Error("x"); Object.defineProperty(e, "stack", { value: { toString() { throw this } } }); throw e`, "Error: x"},
			{`const e = new Error("y"); Object.defineProperty(e, "stack", { get() { throw e } }); throw e`, "Error: y"},
			{`({ toString() { throw this } })`, unprintable},
		} {
			v := newVM(t, Options{})
			_, err := v.Eval(c.src, "throws.js")
			var e *JSError
			if !errors.As(err, &e) || e.Message != c.msg || e.Stack != "" {
				t.Errorf("%s: got %#v, want the message %q and no stack", c.src, err, c.msg)
			}
			if got, err := v.Eval(`1+1`, "after.js"); err != nil || got != "2" {
				t.Errorf("%s: then 1+1 gave %q, %v", c.src, got, err)
			}
			closeVM(t, v)
		}
	})
}

// TestNewLeavesNothing: a panic or a Goexit from a callback at each chunk New
// asks for, in JS_NewRuntime or JS_NewContext, goes on out of New, and the
// host forgets the VM and unmaps its memory.
func TestNewLeavesNothing(t *testing.T) {
	grows := 0
	callbackHook = func(name string, vm uintptr) {
		if name == "host_memory_grow" {
			grows++
		}
	}
	closeVM(t, newVM(t, Options{}))
	callbackHook = nil
	if grows < 2 {
		t.Fatalf("New asked for %d chunks, want 2 or more to stop at", grows)
	}
	for n := 1; n <= grows; n++ {
		for _, how := range []string{"panic", "goexit"} {
			var h *gohost.VM
			k := 0
			callbackHook = func(name string, vm uintptr) {
				if name != "host_memory_grow" {
					return
				}
				if k++; k == n {
					h = gohost.Lookup(uint32(vm))
					leave(how)
				}
			}
			var v *VM
			r := onOwnGoroutine(func() { v, _ = New(Options{}) })
			callbackHook = nil
			if h == nil {
				t.Fatalf("chunk %d: the callback did not run", n)
			}
			if v != nil || (how == "panic" && r != errStop) || (how == "goexit" && r != nil) {
				t.Errorf("chunk %d, %s: New gave %v and %v", n, how, v, r)
			}
			gone(t, h, "after a "+how+" in New")
		}
	}
}

// TestCloseLeavesNothing: a panic or a Goexit from a callback while Close
// frees the engine goes on out of Close, the host still forgets the VM and
// unmaps its memory, and nothing enters the engine again.
func TestCloseLeavesNothing(t *testing.T) {
	for _, how := range []string{"panic", "goexit"} {
		v := newVM(t, Options{})
		// A block of its own: freeing it gives its chunk back through
		// host_call while the engine is freed.
		if _, err := v.Eval(`globalThis.big = "x".repeat(1 << 24); 0`, "big.js"); err != nil {
			t.Fatal(err)
		}
		h := v.Host()
		fired := false
		callbackHook = func(name string, vm uintptr) {
			if name == "host_call" && !fired {
				fired = true
				leave(how)
			}
		}
		r := onOwnGoroutine(func() { v.Close() })
		callbackHook = nil
		if !fired {
			t.Fatal("no callback while Close freed the engine")
		}
		if (how == "panic" && r != errStop) || (how == "goexit" && r != nil) {
			t.Errorf("%s: Close ended with %v", how, r)
		}
		gone(t, h, "after a "+how+" in Close")
		if v.teardowns != 1 {
			t.Errorf("%s: the engine was freed %d times", how, v.teardowns)
		}
		if _, err := v.Close(); err != nil {
			t.Errorf("%s: Close again: %v", how, err)
		}
		if v.teardowns != 1 {
			t.Errorf("%s: Close again entered the engine", how)
		}
		if _, err := v.Eval(`1`, "after.js"); err == nil {
			t.Errorf("%s: Eval after Close did not fail", how)
		}
	}
}
