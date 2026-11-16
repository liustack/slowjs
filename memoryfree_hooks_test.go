// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/liustack/slowjs/internal/gohost"
)

// inEngineFunc reports whether the engine polled under the function name of
// the generated engine. It reads the frames of the goroutine that runs the
// engine, from the hook of a poll.
func inEngineFunc(name string) bool {
	pcs := make([]uintptr, 128)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		f, more := frames.Next()
		if strings.HasSuffix(f.Function, "/cengine."+name) {
			return true
		}
		if !more {
			return false
		}
	}
}

// drainVM is a VM whose front cache and cache of large blocks hold blocks:
// the objects, strings and arrays of junk are freed as each turn ends. Its
// function drain runs a full collection, which gives those blocks back to
// their arenas at its end (js_front_drain), with a poll at every tick: at
// runs before it and after after it.
func drainVM(t *testing.T, at, after func(m *VM)) *VM {
	t.Helper()
	m := newTestVM(t)
	if _, err := m.Eval(`globalThis.junk = function () {
	for (let i = 0; i < 3000; i++) { const o = { i, s: 'v' + i, a: new Array(100 + i % 300).fill(i) } }
}`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	defineBuiltin(t, m, "drain", NewBuiltin("drain", 0, func(a Args) (Value, error) {
		m.h.SetPollInterval(1)
		at(m)
		m.RunGC()
		after(m)
		return Value{}, nil
	}))
	return m
}

// TestMemoryFreeKilledInDrain: a kill at any poll of the drain of the
// caches leaves MemorySize less MemoryFree what it is after the whole
// collection: a block moves from a cache to its arena without changing the
// bytes in use, and an arena that empties leaves both figures at once. A
// first run finds the polls under js_front_drain, then each of them kills a
// run of its own.
func TestMemoryFreeKilledInDrain(t *testing.T) {
	var polls []int64
	var used uintptr
	m := drainVM(t, func(m *VM) {
		m.h.SetKillGrace(-1)
		m.h.InterruptAtPoll(m.h.Polls() + 1)
		m.h.SetStopTrace(func() {
			if inEngineFunc("js_front_drain") {
				polls = append(polls, m.h.Polls())
			}
		})
	}, func(m *VM) {
		m.h.SetStopTrace(nil)
		used = m.MemorySize() - m.MemoryFree()
	})
	if _, err := m.Eval(`junk(); drain(); for (;;) {}`, EvalGlobal); err == nil {
		t.Fatal("the run that finds the polls was not interrupted")
	}
	if len(polls) < 10 {
		t.Fatalf("%d polls under js_front_drain: the caches held too few blocks", len(polls))
	}
	t.Logf("%d polls under js_front_drain, %d bytes in use after the collection", len(polls), used)
	for _, p := range polls {
		var inDrain bool
		m := drainVM(t, func(m *VM) {
			m.h.SetKillGrace(0)
			if p-gohost.KillPolls <= m.h.Polls() {
				t.Fatalf("poll %d comes too soon after the start of drain at %d", p, m.h.Polls())
			}
			m.h.InterruptAtPoll(p - gohost.KillPolls)
			m.h.SetStopTrace(func() {
				if m.h.StopPolls() == gohost.KillPolls+1 {
					inDrain = inEngineFunc("js_front_drain")
				}
			})
		}, func(m *VM) {})
		if _, err := m.Eval(`junk(); drain(); for (;;) {}`, EvalGlobal); !errors.Is(err, ErrKilled) {
			t.Fatalf("kill at poll %d: got %v", p, err)
		}
		if !inDrain {
			t.Fatalf("kill at poll %d: the kill was not under js_front_drain", p)
		}
		var size, free uintptr
		if r := panicked(func() { size, free = m.MemorySize(), m.MemoryFree() }); r != nil {
			t.Fatalf("kill at poll %d: reading the memory panicked with %v", p, r)
		}
		if free > size || size-free != used {
			t.Fatalf("kill at poll %d: MemoryFree %d of MemorySize %d, %d in use, want %d", p, free, size, int64(size)-int64(free), used)
		}
		m.Close()
	}
}
