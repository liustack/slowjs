// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liustack/slowjs"
)

var (
	stressDuration = flag.Duration("stress-duration", 0, "how long TestStress renders (default: 4s, 2s with -short)")
	stressWorkers  = flag.Int("stress-workers", 2, "the workers of TestStress")
)

// TestStress is a short run of the program: every check of a long run, on
// fewer renders. -stress-duration runs it longer, as under the race detector:
//
//	CGO_ENABLED=1 go test -race -run TestStress ./internal/stress -args -stress-duration 10m
func TestStress(t *testing.T) {
	d := *stressDuration
	if d == 0 {
		d = 4 * time.Second
		if testing.Short() {
			d = 2 * time.Second
		}
	}
	c := config{
		duration: d, workers: *stressWorkers, churn: 1, kinds: []string{"store", "wb", "mui"},
		renderTimeout: time.Minute, measureEvery: 16,
		oom: 12, timeout: 12, interrupt: 12, kill: 5,
		tolerance: 0.05, slack: 1 << 20, maxLog: 50, log: t.Output(),
		cpuprofile: filepath.Join(t.TempDir(), "cpu.prof"),
	}
	if err := run(c); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(c.cpuprofile); err != nil || fi.Size() == 0 {
		t.Fatalf("the CPU profile: %v", err)
	}
}

// What a render gave is classified by the error and the fields the
// documented errors have, and the HTML by its hash.
func TestClassify(t *testing.T) {
	p := &page{name: "p", hash: sha256.Sum256([]byte("<p>ok</p>"))}
	killed := makeKilled(t)
	for _, c := range []struct {
		name string
		html string
		err  error
		want outcome
	}{
		{"html", "<p>ok</p>", nil, outHTML},
		{"other html", "<p>no</p>", nil, outWrongHTML},
		{"interrupted", "", &slowjs.Error{Name: "InternalError", Message: "interrupted", Uncatchable: true, Interrupted: true}, outInterrupted},
		{"out of memory", "", &slowjs.Error{Name: "InternalError", Message: "out of memory", Uncatchable: true, OutOfMemory: true}, outOOM},
		{"killed", "", killed, outKilled},
		{"thrown", "", &slowjs.Error{Name: "TypeError", Message: "x is undefined"}, outThrown},
		{"uncatchable panic", "", &slowjs.Error{Name: "Error", Uncatchable: true}, outOther},
		{"interrupted and out of memory", "", &slowjs.Error{Uncatchable: true, Interrupted: true, OutOfMemory: true}, outOther},
		{"budget", "", errBudget, outBudget},
		{"unresolved", "", errUnresolved, outUnresolved},
		{"result", "", &resultError{`{"error":"x"}`}, outResult},
		{"go error", "", errors.New("something else"), outOther},
		{"wrapped", "", fmt.Errorf("render: %w", &slowjs.Error{Uncatchable: true, Interrupted: true}), outInterrupted},
	} {
		if got, _ := classify(p, c.html, c.err); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, outcomeNames[got], outcomeNames[c.want])
		}
	}
}

// makeKilled kills a script, which only the engine can do, and returns the
// error of the call.
func makeKilled(t *testing.T) error {
	vm, err := slowjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer vm.Close()
	if err := vm.SetEvalTimeout(time.Millisecond); err != nil {
		t.Fatal(err)
	}
	_, err = vm.Eval(`Array.prototype.lastIndexOf.call({ length: 2 ** 31 - 1 }, -1)`, slowjs.EvalGlobal)
	if !errors.Is(err, slowjs.ErrKilled) {
		t.Fatalf("the loop gave %v, want a kill", err)
	}
	return err
}

// A VM that never stopped gives its HTML or the error of the stop it was
// given, and only one that went on after a stop may fail otherwise.
func TestAllowed(t *testing.T) {
	for o := range nOps {
		for x := range nOutcomes {
			want := x == outHTML
			switch {
			case x == outInterrupted:
				want = o == opTimeout || o == opInterrupt || o == opKill
			case x == outBudget:
				want = o == opTimeout || o == opKill
			case x == outOOM:
				want = o == opOOM
			case x == outKilled:
				want = o == opKill
			case x == outAborted:
				want = o == opAbort
			}
			if got := allowed(o, false, x); got != want {
				t.Errorf("%s on a VM that never stopped, %s: %v, want %v", opNames[o], outcomeNames[x], got, want)
			}
			if x == outThrown || x == outUnresolved || x == outResult {
				want = true
			}
			if got := allowed(o, true, x); got != want {
				t.Errorf("%s on a VM that went on after a stop, %s: %v, want %v", opNames[o], outcomeNames[x], got, want)
			}
		}
	}
}

// The check of drift passes a flat series and one that moves within the
// tolerance, and fails one that grows past it.
func TestDrift(t *testing.T) {
	rows := func(f func(i int) int64) []sample {
		var rs []sample
		for i := range 30 {
			r := sample{t: time.Duration(i) * time.Second, steadyN: 1}
			v := f(i)
			r.steadyKept, r.steadySize, r.engine, r.totals.Heap, r.goLive = v, v, v, v, v
			rs = append(rs, r)
		}
		return rs
	}
	c := config{warmup: 5 * time.Second, tolerance: 0.05, slack: 0, log: io.Discard}
	for _, tc := range []struct {
		name string
		f    func(i int) int64
		ok   bool
	}{
		{"flat", func(int) int64 { return 40 << 20 }, true},
		{"noise", func(i int) int64 { return 40<<20 + int64(i%3)<<18 }, true},
		{"a leak", func(i int) int64 { return 40<<20 + int64(i)<<20 }, false},
	} {
		err := (&series{rows: rows(tc.f)}).drift(c)
		if (err == nil) != tc.ok {
			t.Errorf("%s: %v", tc.name, err)
		}
		if err != nil && !strings.Contains(err.Error(), "steady_kept") {
			t.Errorf("%s: the error names no measure: %v", tc.name, err)
		}
	}
}
