// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package cengine

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/liustack/slowjs/internal/gohost"
)

// The memory size, the limit and the threshold of a VM, for the tests of the
// wait of the collections before large blocks.
func memorySize(v *VM) (n uintptr) {
	v.enter(func() { n = JS_GetMemorySize(v.sp, v.rt) })
	return n
}

func setMemoryLimit(v *VM, n uintptr) { v.enter(func() { JS_SetMemoryLimit(v.sp, v.rt, n) }) }

func gcThreshold(v *VM) (n uintptr) {
	v.enter(func() { n = JS_GetGCThreshold(v.sp, v.rt) })
	return n
}

// TestEvalEndsBlockWait pins that Eval starts with no wait of the
// collections before large blocks (JS_StartCall), as each outermost call of
// the Go API does (TestBlockWaitEndsWithTheCall in the root package): an
// Eval that ends with out of memory in a wait leaves the next one to collect
// at its first block near the limit, which fits once the cycles it made
// before are freed.
func TestEvalEndsBlockWait(t *testing.T) {
	v := newVM(t, Options{})
	defer closeVM(t, v)
	eval := func(src string) (string, error) { return v.Eval(src, "<blockwait>") }

	if _, err := eval(`
function cyc(n) { for (var i = 0; i < n; i++) { var a = {i: i}; a.self = a } }
function near(k) { for (var i = 0; i < k; i++) 'x'.repeat(6000) }
cyc(1000)`); err != nil {
		t.Fatal(err)
	}
	// the bytes of one cycle, with the collections of the threshold off
	v.RunGC()
	v.SetGCThreshold(1 << 40)
	before := memorySize(v)
	if _, err := eval("cyc(1000)"); err != nil {
		t.Fatal(err)
	}
	per := (memorySize(v) - before) / 1000
	v.RunGC()
	if per == 0 {
		t.Fatal("1000 cycles took no memory")
	}

	// 100 blocks of 6000 bytes, each within the slack of the limit: each
	// collection leaves no room and starts a longer wait, and the Eval ends
	// in one, with out of memory at a block that does not fit.
	setMemoryLimit(v, memorySize(v)+12000)
	v.SetGCThreshold(1 << 40)
	_, err := eval("near(100); 'x'.repeat(1 << 20).length")
	var e *JSError
	if !errors.As(err, &e) || !e.Uncatchable || !strings.Contains(e.Message, "out of memory") {
		t.Fatalf("the first Eval returned %v, want out of memory", err)
	}

	// 250 KiB of cycles and a block of 150 KiB, 300 KiB under the limit:
	// the block fits once the cycles are collected, and only then
	v.SetGCThreshold(1 << 40)
	setMemoryLimit(v, memorySize(v)+300<<10)
	out, err := eval(fmt.Sprintf("cyc(%d); globalThis.b = 'x'.repeat(150 << 10); b.length", (250<<10)/per))
	if err != nil || out != "153600" {
		t.Fatalf("the Eval after the one that ran out of memory: %q, %v", out, err)
	}
	if gcThreshold(v) == 1<<40 {
		t.Fatal("the Eval after the one that ran out of memory made its block with no collection")
	}
	setMemoryLimit(v, ^uintptr(0))
	v.RunGC()
}

// TestNestedEvalKeepsBlockWait pins that only the outermost entry ends the
// wait: an eval that a function of the host runs inside a script is part of
// the call of that script, and its block near the limit waits as the next
// block of the script would. Were the wait to end there, a script that calls
// back into the engine at each block would collect at each block again.
func TestNestedEvalKeepsBlockWait(t *testing.T) {
	v := newVM(t, Options{})
	defer closeVM(t, v)
	testVMs[v.Host().ID()] = v
	defer delete(testVMs, v.Host().ID())

	// a block of 60000 bytes under a limit 64 KiB away is within the slack
	// (8192 bytes) of the limit, and fits once the one before is freed
	var collectedBefore, collectedNested bool
	testHostFuncs[9002] = func(v *VM, sp uintptr, _ []gohost.Value) (uintptr, uintptr) {
		old := v.sp
		v.sp = AlignSP(sp)
		defer func() { v.sp = old }()
		collectedBefore = gcThreshold(v) != 1<<40
		v.SetGCThreshold(1 << 40)
		var out string
		var err error
		v.enterScript(func() { out, err = v.eval(`'x'.repeat(60000).length`, "nested.js") })
		if err != nil || out != "60000" {
			panic(fmt.Sprintf("the nested eval gave %q, %v", out, err))
		}
		collectedNested = gcThreshold(v) != 1<<40
		return 0, tagInt
	}
	defer delete(testHostFuncs, 9002)
	if thrown := defineHostFunc(v, "nested", 0, 9002); thrown != "" {
		t.Fatal(thrown)
	}

	v.RunGC()
	setMemoryLimit(v, memorySize(v)+64<<10)
	v.SetGCThreshold(1 << 40)
	out, err := v.Eval(`'x'.repeat(60000).length + nested()`, "<blockwait>")
	setMemoryLimit(v, ^uintptr(0))
	if err != nil || out != "60000" {
		t.Fatalf("the Eval gave %q, %v", out, err)
	}
	// the block of the script collects and leaves it near the limit, which
	// starts a wait of one block
	if !collectedBefore {
		t.Fatal("the block of the script made no collection")
	}
	if collectedNested {
		t.Fatal("the nested eval ended the wait of the script and collected at its block")
	}
	v.RunGC()
}
