// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build cengine && testhooks

package cengine

import (
	"errors"
	"runtime"
	"testing"

	"github.com/liustack/slowjs/internal/gohost"
	"github.com/liustack/slowjs/internal/hostmem"
)

// These tests need the hooks that only a build with the tag testhooks has:
// they fail the OS's calls (hostmem.SetFault) and stop the engine's callbacks
// (callbackHook).

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
		if teardowns(v) != 1 {
			t.Errorf("%s: the engine was freed %d times", how, teardowns(v))
		}
		if _, err := v.Close(); err != nil {
			t.Errorf("%s: Close again: %v", how, err)
		}
		if teardowns(v) != 1 {
			t.Errorf("%s: Close again entered the engine", how)
		}
		if _, err := v.Eval(`1`, "after.js"); err == nil {
			t.Errorf("%s: Eval after Close did not fail", how)
		}
	}
}

// TestCloseRetry: when the OS refuses to unmap a chunk, Close returns the
// error and keeps the chunk, and Close again unmaps it without freeing the
// engine a second time.
func TestCloseRetry(t *testing.T) {
	inChild(t, func(t *testing.T) {
		v := newVM(t, Options{})
		if got, err := v.Eval(`[1, 2, 3].join()`, "x.js"); err != nil || got != "1,2,3" {
			t.Fatalf("%q, %v", got, err)
		}
		h := v.Host()
		refused := 0
		hostmem.SetFault(func(call string, addr, size uintptr) error {
			if call == "unmap" && refused == 0 {
				refused++
				return errStop
			}
			return nil
		})
		leaked, err := v.Close()
		hostmem.SetFault(nil)
		if !errors.Is(err, errStop) || leaked != 0 || refused != 1 {
			t.Fatalf("Close with an unmap refused: %d blocks, %v", leaked, err)
		}
		if u := h.Memory().Usage(); u.Chunks != 1 {
			t.Fatalf("after the refused unmap the host holds %+v, want the one chunk", u)
		}
		leaked, err = v.Close()
		if err != nil || leaked != 0 {
			t.Fatalf("Close again: %d blocks, %v", leaked, err)
		}
		if teardowns(v) != 1 {
			t.Errorf("the engine was freed %d times", teardowns(v))
		}
		gone(t, h, "after Close again")
	})
}

// TestCloseTwice: Close after a Close that went well does nothing, and the
// other entries refuse the closed VM.
func TestCloseTwice(t *testing.T) {
	inChild(t, func(t *testing.T) {
		v := newVM(t, Options{})
		h := v.Host()
		closeVM(t, v)
		if leaked, err := v.Close(); err != nil || leaked != 0 {
			t.Fatalf("Close again: %d blocks, %v", leaked, err)
		}
		if teardowns(v) != 1 {
			t.Errorf("the engine was freed %d times", teardowns(v))
		}
		gone(t, h, "after two Closes")
		if _, err := v.Eval(`1`, "after.js"); !errors.Is(err, ErrClosed) {
			t.Errorf("Eval after Close: %v", err)
		}
		if _, err := v.ReleaseMemory(0); !errors.Is(err, ErrClosed) {
			t.Errorf("ReleaseMemory after Close: %v", err)
		}
		for name, f := range map[string]func(){"RunGC": v.RunGC, "Usage": func() { v.Usage() }} {
			if r := onOwnGoroutine(f); r != ErrClosed {
				t.Errorf("%s after Close: %v", name, r)
			}
		}
	})
}

// refuseUnmaps makes the OS refuse every unmap until the test ends or
// hostmem.SetFault(nil).
func refuseUnmaps(t *testing.T) {
	t.Cleanup(func() { hostmem.SetFault(nil) })
	hostmem.SetFault(func(call string, addr, size uintptr) error {
		if call == "unmap" {
			return errStop
		}
		return nil
	})
}

// leftover checks that the host lists the VM with id among its leftovers,
// with the refusal, and nothing else.
func leftover(t *testing.T, id uint32, when string) {
	t.Helper()
	l := gohost.Leftovers()
	if len(l) != 1 || l[0].ID != id || !errors.Is(l[0].Err, errStop) || l[0].Usage.Stack == 0 {
		t.Errorf("%s: leftovers %+v, want VM %d with its stack and the refusal", when, l, id)
	}
}

// TestNewCleanupRefused: when the OS refuses to unmap the memory of a VM
// that New gives up on after a panic or a Goexit, the panic goes on and the
// memory stays among the host's leftovers, which give it back once the OS
// takes it.
func TestNewCleanupRefused(t *testing.T) {
	for _, how := range []string{"panic", "goexit"} {
		var h *gohost.VM
		callbackHook = func(name string, vm uintptr) {
			if name == "host_memory_grow" && h == nil {
				h = gohost.Lookup(uint32(vm))
				refuseUnmaps(t)
				leave(how)
			}
		}
		var v *VM
		r := onOwnGoroutine(func() { v, _ = New(Options{}) })
		callbackHook = nil
		if h == nil {
			t.Fatal("the callback did not run")
		}
		if v != nil || (how == "panic" && r != errStop) || (how == "goexit" && r != nil) {
			t.Errorf("%s: New gave %v and %v", how, v, r)
		}
		leftover(t, h.ID(), "after a "+how+" in New")
		hostmem.SetFault(nil)
		if err := gohost.CloseLeftovers(); err != nil {
			t.Fatalf("%s: CloseLeftovers: %v", how, err)
		}
		if l := gohost.Leftovers(); len(l) != 0 {
			t.Errorf("%s: leftovers %+v after CloseLeftovers", how, l)
		}
		gone(t, h, "after CloseLeftovers")
	}
}

// TestCloseCleanupRefused: when the OS refuses to unmap the memory of a VM
// whose teardown a panic or a Goexit left, the panic goes on, the host lists
// what is left with the refusal, and Close again gives it back.
func TestCloseCleanupRefused(t *testing.T) {
	for _, how := range []string{"panic", "goexit"} {
		v := newVM(t, Options{})
		if _, err := v.Eval(`globalThis.big = "x".repeat(1 << 24); 0`, "big.js"); err != nil {
			t.Fatal(err)
		}
		h := v.Host()
		fired := false
		callbackHook = func(name string, vm uintptr) {
			if name == "host_call" && !fired {
				fired = true
				refuseUnmaps(t)
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
		leftover(t, h.ID(), "after a "+how+" in Close")
		hostmem.SetFault(nil)
		if _, err := v.Close(); err != nil {
			t.Fatalf("%s: Close again: %v", how, err)
		}
		if l := gohost.Leftovers(); len(l) != 0 {
			t.Errorf("%s: leftovers %+v after Close again", how, l)
		}
		gone(t, h, "after Close again")
		if teardowns(v) != 1 {
			t.Errorf("%s: the engine was freed %d times", how, teardowns(v))
		}
	}
}
