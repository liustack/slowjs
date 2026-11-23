// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build testhooks

package tifajs // import "github.com/liustack/tifajs"

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestCompileMarkCleared: the word after the interrupt flag, which the
// engine sets from the stop of a compile to its return so that the freeing
// of its code is left out of the kill (JS_GetInterruptFlag in
// engine/quickjs.h), is 0 again after each way a compile of 50 KB ends: a
// stop at every poll of the compile, at the top level and in a direct eval
// that the code of another eval runs, and a stop in the middle with 0 to
// 4 MiB of memory left, so that some compiles end out of memory and some at
// the stop. No callback of the compile runs javascript, so no compile starts
// inside another. The VM then still kills a script that frees for longer
// than KillPolls polls after its stop.
func TestCompileMarkCleared(t *testing.T) {
	m := newTestVM(t)
	if _, err := m.Eval(`var a = 0, s = '(function(){' + 'a=a+1;'.repeat(1 << 13) + '})'; 0`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	h := m.h
	wantClear := func(what string, err error) {
		t.Helper()
		var e *Error
		if errors.Is(err, ErrKilled) || err != nil && !errors.As(err, &e) {
			t.Fatalf("%s: got %v", what, err)
		}
		if n := h.FreeingCode(); n != 0 {
			t.Fatalf("%s: the compile ended with %v and left the word after the flag at %d", what, err, n)
		}
	}
	for _, c := range []struct {
		name, call string
		flags      int
	}{
		{"an eval", `(0, eval)(s); 0`, EvalGlobal},
		{"a direct eval in an eval", `(0, eval)('eval(s)'); 0`, EvalGlobal},
		{"a module of 800 imports", importsModule(800), EvalModule},
	} {
		before := h.Polls()
		if _, err := m.Eval(c.call, c.flags); err != nil {
			t.Fatal(err)
		}
		span := h.Polls() - before
		stopped := 0
		for at := int64(1); at <= span; at++ {
			h.InterruptAtPoll(h.Polls() + at)
			_, err := m.Eval(c.call, c.flags)
			h.InterruptAtPoll(0)
			if err != nil {
				stopped++
			}
			wantClear(fmt.Sprintf("%s stopped at poll %d of %d", c.name, at, span), err)
		}
		t.Logf("%s: %d of %d stops ended the call", c.name, stopped, span)
	}
	before := h.Polls()
	if _, err := m.Eval(`(0, eval)(s); 0`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	mid := (h.Polls() - before) / 2
	ends := map[string]int{}
	for room := uintptr(0); room <= 4<<20; room += 16 << 10 {
		// the collection of vmMemory frees what the calls before left, with
		// polls, so the stop is armed after it
		m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + room)
		h.InterruptAtPoll(h.Polls() + mid)
		_, err := m.Eval(`(0, eval)(s); 0`, EvalGlobal)
		m.SetMemoryLimit(^uintptr(0))
		h.InterruptAtPoll(0)
		var e *Error
		switch {
		case err == nil:
			ends["ran"]++
		case errors.As(err, &e) && e.Interrupted:
			ends["interrupted"]++
		default:
			ends[err.Error()]++
		}
		wantClear(fmt.Sprintf("a stop at poll %d with %d bytes left", mid, room), err)
	}
	t.Logf("a stop at poll %d with 0 to 4 MiB left: %v", mid, ends)
	if ends["interrupted"] == 0 || len(ends) < 2 {
		t.Fatalf("the sweep of memory left gave %v: want both stops and other ends", ends)
	}
	wantKillWhileFreeing(t, m, "after the compiles")
}

// compileStopPolls is the most polls wantStopsNotKilled lets count after a
// stop in a compile: two steps of its readings, as compileStopPolls of
// internal/cengine.
const compileStopPolls = 16

// TestStoppedCompileFreeNotKilled stops the compile of a source of 8.4 MB, a
// function of 1.4 million statements, at the polls two, five and nine tenths
// into it, with no grace: the stop comes at the same place of the compile on
// any machine, in the parse and in the passes. The compile then frees the
// code it made, in the pass that stopped and in the function defs and the
// functions made of them, about one poll for each 10,000 instructions: more
// than KillPolls polls for that much code, which killed the VM. Those polls
// do not count toward the kill (JS_GetInterruptFlag in engine/quickjs.h), so
// the call ends interrupted after at most compileStopPolls counted, and the
// VM goes on. The function the compile made whole is kept in f while the
// polls of a compile are counted, and nothing the stopped calls make is kept
// after them, so no other freeing follows a stop. The VM then still kills a
// script that frees what it made for longer than KillPolls polls
// (wantKillWhileFreeing), so the end of the compile cleared the mark.
func TestStoppedCompileFreeNotKilled(t *testing.T) {
	m := newTestVM(t)
	if _, err := m.Eval(`var a = 0, f, g, s = '(function(){' + 'a=a+1;'.repeat(1400000) + '})'; 0`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	wantStopsNotKilled(t, m, `f = (0, eval)(s); 0`, `g = (0, eval)(s); 0`, EvalGlobal)
	if _, err := m.Eval(`f = null; 0`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	wantKillWhileFreeing(t, m, "after the stopped compiles")
}

// TestStoppedModuleFreeNotKilled is TestStoppedCompileFreeNotKilled for a
// module of 8,000 imports of 64 attributes each, 5.7 MB: a stop of its
// compile frees the module with its table of imports and the object of the
// attributes of each after the code, and those polls do not count toward the
// kill either. The module the first call made whole stays in the VM.
func TestStoppedModuleFreeNotKilled(t *testing.T) {
	m := newTestVM(t)
	src := importsModule(8000)
	wantStopsNotKilled(t, m, src, src, EvalModule)
	wantKillWhileFreeing(t, m, "after the stopped compiles of the module")
}

// TestStoppedResolveFreeNotKilled stops a module of 8,000 imports of 64
// attributes each, 5.7 MB, made but not run, at the polls five, seven and
// nine tenths into the call, with no grace. 2,000 modules evaluated before it
// come first among the loaded modules, so the resolution of each import
// looks through them, and the resolution, which runs once the compile has
// returned, takes most of the call. A stop there frees the module, its
// table of imports, the object of the attributes of each and its code: those
// polls counted toward the kill, as the compile was over. They do not now
// (js_freeing_start in engine/quickjs.c): the call ends interrupted after
// at most compileStopPolls counted, and the VM goes on.
func TestStoppedResolveFreeNotKilled(t *testing.T) {
	m := newTestVM(t)
	h := m.h
	for i := 0; i < 2000; i++ {
		if _, err := m.EvalFile(fmt.Sprintf("m%d", i), "export var x;", EvalModule); err != nil {
			t.Fatal(err)
		}
	}
	m.SetGCThreshold(^uintptr(0))
	src := importsModule(8000)
	// counted is StopPolls at the end of the call: taking its exception is a
	// call of its own, which starts the count again
	var counted int64
	compile := func() error {
		v, err := m.eval(src, EvalModule|evalFlagCompileOnly)
		counted = h.StopPolls()
		if err != nil {
			return err
		}
		if isException(v) {
			return m.errFromException()
		}
		m.jsFreeValue(v)
		return nil
	}
	if err := compile(); err != nil {
		t.Fatal(err)
	}
	m.RunGC()
	before := h.Polls()
	if err := compile(); err != nil {
		t.Fatal(err)
	}
	span := h.Polls() - before
	h.SetKillGrace(0)
	for _, part := range []int64{5, 7, 9} {
		at := span * part / 10
		m.RunGC()
		before := h.Polls()
		h.InterruptAtPoll(before + at)
		err := compile()
		h.InterruptAtPoll(0)
		what := fmt.Sprintf("stopped at poll %d of the %d of the call", at, span)
		t.Logf("%s: %d polls counted toward the kill, %d polls in the call", what, counted, h.Polls()-before)
		var e *Error
		if errors.Is(err, ErrKilled) || !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
			t.Fatalf("%s: got %v (killed %v) after %d polls counted toward the kill, want interrupted", what, err, errors.Is(err, ErrKilled), counted)
		}
		if counted > compileStopPolls {
			t.Errorf("%s: %d polls counted toward the kill, want at most %d", what, counted, compileStopPolls)
		}
		if v, err := m.Eval(`1+1`, EvalGlobal); err != nil || v != 2 {
			t.Fatalf("the VM after the stop: 1+1 got %v, %v", v, err)
		}
	}
	wantKillWhileFreeing(t, m, "after the stopped resolutions of the module")
}

// importsModule is a module that imports itself, by the name <eval> that
// Eval gives it, n times, each with an object of 64 attributes.
func importsModule(n int) string {
	var b strings.Builder
	b.WriteString(`import "<eval>" with {k0: "v"`)
	for i := 1; i < 64; i++ {
		fmt.Fprintf(&b, `, k%d: "v"`, i)
	}
	b.WriteString("};")
	return strings.Repeat(b.String(), n)
}

// wantStopsNotKilled evaluates whole twice, the second time to count the
// polls of the call, as the first of a module takes longer, then stops
// stopped, a call that compiles the same source, at the polls two, five and
// nine tenths into it, with no grace: each stop ends the call interrupted
// after at most compileStopPolls polls counted toward the kill, and the VM
// goes on.
func wantStopsNotKilled(t *testing.T, m *VM, whole, stopped string, flags int) {
	t.Helper()
	h := m.h
	if _, err := m.Eval(whole, flags); err != nil {
		t.Fatal(err)
	}
	before := h.Polls()
	if _, err := m.Eval(whole, flags); err != nil {
		t.Fatal(err)
	}
	span := h.Polls() - before
	h.SetKillGrace(0)
	for _, part := range []int64{2, 5, 9} {
		at := span * part / 10
		before := h.Polls()
		h.InterruptAtPoll(before + at)
		_, err := m.Eval(stopped, flags)
		h.InterruptAtPoll(0)
		polls, counted := h.Polls()-before, h.StopPolls()
		what := fmt.Sprintf("stopped at poll %d of the %d of the compile", at, span)
		t.Logf("%s: %d polls counted toward the kill, %d polls in the call", what, counted, polls)
		var e *Error
		if errors.Is(err, ErrKilled) || !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
			t.Fatalf("%s: got %v (killed %v) after %d polls counted toward the kill, want interrupted", what, err, errors.Is(err, ErrKilled), counted)
		}
		if counted > compileStopPolls {
			t.Errorf("%s: %d polls counted toward the kill, want at most %d", what, counted, compileStopPolls)
		}
		if v, err := m.Eval(`1+1`, EvalGlobal); err != nil || v != 2 {
			t.Fatalf("the VM after the stop: 1+1 got %v, %v", v, err)
		}
	}
}

// TestModuleCompileStops stops the compile of a module of 1,024 vars, each
// exported under another name, and 1,024 imports of the module itself, then
// a loop that runs for ever, at every poll that a call of it without the
// loop takes (the second call, as the first takes longer): each stop must
// come within compileStopPolls, with the error interrupted. The parse looks
// for each export among those before it and for each import among the
// bindings before it, and the compile looks for each export among the imports
// and the variables, loops whose work grows as a square: they read no flag,
// and a stop waited for them.
func TestModuleCompileStops(t *testing.T) {
	m := newTestVM(t)
	h := m.h
	const n = 1024
	var b strings.Builder
	b.WriteString("var a0")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b, ",a%d", i)
	}
	b.WriteString(";export {a0 as e0")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b, ",a%d as e%d", i, i)
	}
	b.WriteString("};")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `import * as b%d from "<eval>";`, i)
	}
	b.WriteString("if (globalThis.spin) for (;;) {}")
	src := b.String()
	m.SetGCThreshold(^uintptr(0))
	if _, err := m.Eval(src, EvalModule); err != nil {
		t.Fatal(err)
	}
	m.RunGC()
	before := h.Polls()
	if _, err := m.Eval(src, EvalModule); err != nil {
		t.Fatal(err)
	}
	span := h.Polls() - before
	if _, err := m.Eval(`globalThis.spin = true`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	var worst int64
	var worstAt string
	for at := int64(1); at <= span; at++ {
		m.RunGC()
		h.InterruptAtPoll(h.Polls() + at)
		_, err := m.Eval(src, EvalModule)
		h.InterruptAtPoll(0)
		what := fmt.Sprintf("the stop at poll %d of %d", at, span)
		var e *Error
		if errors.Is(err, ErrKilled) || !errors.As(err, &e) || !e.Interrupted {
			t.Fatalf("%s: got %v, want the error interrupted", what, err)
		}
		if n := h.StopPolls(); n > worst {
			worst, worstAt = n, what
		}
	}
	t.Logf("one call took %d polls, at most %d saw the request (%s)", span, worst, worstAt)
	if worst > compileStopPolls {
		t.Errorf("%s: %d polls saw the request, want at most %d", worstAt, worst, compileStopPolls)
	}
}

// exportsModule is a module of n variables, each exported under its name
// e0, e1 and on. The first module a VM evaluates is the one other modules
// get when they import "<eval>".
func exportsModule(n int) string {
	var b strings.Builder
	b.WriteString("export var e0")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b, ",e%d", i)
	}
	b.WriteString(";")
	return b.String()
}

// importerModule imports each export of exportsModule(n) by name, then all
// of them as a namespace, then the module n more times for nothing, and
// loops for ever when globalThis.spin is set.
func importerModule(n int) string {
	var b strings.Builder
	b.WriteString("import {e0")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b, ",e%d", i)
	}
	b.WriteString(`} from "<eval>"; import * as ns from "<eval>";`)
	b.WriteString(strings.Repeat(`import "<eval>";`, n))
	b.WriteString("if (globalThis.spin) for (;;) {}")
	return b.String()
}

// TestModuleLinkStops stops a module of 2,000 imports by name of a module
// of 2,000 exports at every poll of a call of it (the second, as the first
// also makes the namespace), then a loop that runs for ever: each stop must
// come within compileStopPolls, with the error interrupted and no kill. The
// link looks for each import among the exports, work that grows as a
// square: it read no flag, and a stop waited for it.
func TestModuleLinkStops(t *testing.T) {
	m := newTestVM(t)
	h := m.h
	const n = 2000
	if _, err := m.Eval(exportsModule(n), EvalModule); err != nil {
		t.Fatal(err)
	}
	src := importerModule(n)
	m.SetGCThreshold(^uintptr(0))
	if _, err := m.Eval(src, EvalModule); err != nil {
		t.Fatal(err)
	}
	m.RunGC()
	before := h.Polls()
	if _, err := m.Eval(src, EvalModule); err != nil {
		t.Fatal(err)
	}
	span := h.Polls() - before
	if _, err := m.Eval(`globalThis.spin = true`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	h.SetKillGrace(0)
	var worst int64
	var worstAt string
	for at := int64(1); at <= span; at++ {
		m.RunGC()
		h.InterruptAtPoll(h.Polls() + at)
		_, err := m.Eval(src, EvalModule)
		h.InterruptAtPoll(0)
		what := fmt.Sprintf("the stop at poll %d of %d", at, span)
		var e *Error
		if errors.Is(err, ErrKilled) || !errors.As(err, &e) || !e.Interrupted {
			t.Fatalf("%s: got %v, want the error interrupted", what, err)
		}
		if n := h.StopPolls(); n > worst {
			worst, worstAt = n, what
		}
	}
	t.Logf("one call took %d polls, at most %d saw the request (%s)", span, worst, worstAt)
	if worst > compileStopPolls {
		t.Errorf("%s: %d polls saw the request, want at most %d", worstAt, worst, compileStopPolls)
	}
}

// TestStoppedLinkNotKilled stops the first call of a module of 8,000
// imports by name of a module of 8,000 exports, and of all of them as a
// namespace, at the polls two, five and nine tenths into it, each in a VM of
// its own, with no grace: the link and the making of the namespace look for
// each name among the others, and read the flag as they go, so the call
// ends interrupted after at most compileStopPolls polls counted toward the
// kill, and the VM goes on.
func TestStoppedLinkNotKilled(t *testing.T) {
	const n = 8000
	src := importerModule(n)
	setup := func() *VM {
		m := newTestVM(t)
		if _, err := m.Eval(exportsModule(n), EvalModule); err != nil {
			t.Fatal(err)
		}
		m.SetGCThreshold(^uintptr(0))
		m.RunGC()
		return m
	}
	m := setup()
	before := m.h.Polls()
	if _, err := m.Eval(src, EvalModule); err != nil {
		t.Fatal(err)
	}
	span := m.h.Polls() - before
	for _, part := range []int64{2, 5, 9} {
		m := setup()
		h := m.h
		h.SetKillGrace(0)
		at := span * part / 10
		before := h.Polls()
		h.InterruptAtPoll(before + at)
		_, err := m.Eval(src, EvalModule)
		h.InterruptAtPoll(0)
		polls, counted := h.Polls()-before, h.StopPolls()
		what := fmt.Sprintf("stopped at poll %d of the %d of the call", at, span)
		t.Logf("%s: %d polls counted toward the kill, %d polls in the call", what, counted, polls)
		var e *Error
		if errors.Is(err, ErrKilled) || !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
			t.Fatalf("%s: got %v (killed %v) after %d polls counted toward the kill, want interrupted", what, err, errors.Is(err, ErrKilled), counted)
		}
		if counted > compileStopPolls {
			t.Errorf("%s: %d polls counted toward the kill, want at most %d", what, counted, compileStopPolls)
		}
		if v, err := m.Eval(`1+1`, EvalGlobal); err != nil || v != 2 {
			t.Fatalf("the VM after the stop: 1+1 got %v, %v", v, err)
		}
	}
}
