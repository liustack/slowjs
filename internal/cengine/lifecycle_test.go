// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine

package cengine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime/debug"
	"testing"
	"time"
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

// brokenAfter checks what a VM must be after the engine failed in it: broken,
// refusing every entry with ErrBroken, and closing without a leak of host
// memory. The goroutine must have its setting of SetPanicOnFault back.
func brokenAfter(t *testing.T, v *VM, what string) {
	t.Helper()
	if !v.Host().Broken() {
		t.Errorf("%s: the VM is not broken", what)
	}
	if _, err := v.Eval("1", "after.js"); !errors.Is(err, ErrBroken) {
		t.Errorf("%s: Eval after the failure gave %v, want ErrBroken", what, err)
	}
	if _, err := v.ReleaseMemory(0); !errors.Is(err, ErrBroken) {
		t.Errorf("%s: ReleaseMemory after the failure gave %v, want ErrBroken", what, err)
	}
	if got := catch(v.RunGC); !errors.Is(asError(got), ErrBroken) {
		t.Errorf("%s: RunGC after the failure panicked with %v, want ErrBroken", what, got)
	}
	if _, err := v.Close(); err != nil {
		t.Errorf("%s: Close: %v", what, err)
	}
	if u := v.Host().Memory().Usage(); u.Chunks != 0 || u.Footprint != 0 || u.Stack != 0 {
		t.Errorf("%s: after Close the host holds %+v", what, u)
	}
	if debug.SetPanicOnFault(false) {
		t.Errorf("%s: the goroutine still has SetPanicOnFault on", what)
	}
	// A process that survived must also run a new VM.
	w := newVM(t, Options{Seed: 1})
	if got, err := w.Eval(`[1,2,3].map(x => x * 2).join()`, "next.js"); err != nil || got != "2,4,6" {
		t.Errorf("%s: a new VM gave %q, %v", what, got, err)
	}
	closeVM(t, w)
}

func catch(f func()) (got any) {
	defer func() { got = recover() }()
	f()
	return nil
}

func asError(x any) error {
	err, _ := x.(error)
	return err
}
