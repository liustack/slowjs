// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/liustack/slowjs/compare/engines/engine"
)

type feature struct {
	ID      string `json:"id"`
	Edition string `json:"edition"`
	Kind    string `json:"kind"`
	Src     string `json:"src"`
	Expect  string `json:"expect"`
}

type featureResult struct {
	ID     string  `json:"id"`
	Status string  `json:"status"` // pass, fail, n/a
	Got    string  `json:"got,omitempty"`
	Err    string  `json:"err,omitempty"`
	MS     float64 `json:"ms"`
}

// Probes run from Go rather than from features.json.
var goProbes = map[string]func(*engine.Engine) featureResult{
	"interrupt":             probeInterrupt(`for (;;) {}`),
	"interrupt-uncatchable": probeInterrupt(`for (;;) { try { for (;;) {} } catch (e) {} finally { } }`),
	"memory-limit":          probeMemoryLimit,
	"recursion-depth":       probeDepth,
	"recursion-overflow":    probeScript(`function g(n) { return g(n + 1) + 1 } try { g(0); "no error" } catch (e) { e instanceof RangeError ? "RangeError" : String(e && e.name) }`, "RangeError"),
	"tail-call":             probeScript(`"use strict"; function t(n) { if (n === 0) return "ok"; return t(n - 1) } t(1000000)`, "ok"),
}

func runFeature(e *engine.Engine, file, id string) featureResult {
	if p, ok := goProbes[id]; ok {
		return p(e)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		fatal(err)
	}
	var table struct{ Features []feature }
	if err := json.Unmarshal(b, &table); err != nil {
		fatal(err)
	}
	for _, f := range table.Features {
		if f.ID == id {
			return runProbe(e, f)
		}
	}
	fatal(fmt.Errorf("no feature %q", id))
	return featureResult{}
}

func newFeatureVM(e *engine.Engine, dir string) (engine.VM, error) {
	return e.New(engine.Options{Intl: true, ModuleDir: dir, Interruptible: true})
}

func runProbe(e *engine.Engine, f feature) (r featureResult) {
	r.ID = f.ID
	start := time.Now()
	defer func() { r.MS = float64(time.Since(start).Microseconds()) / 1000 }()
	defer func() {
		if p := recover(); p != nil {
			r.Status, r.Err = "fail", fmt.Sprintf("panic: %v", p)
		}
	}()
	dir := ""
	if f.Kind == "module" {
		if !e.Modules {
			r.Status, r.Err = "n/a", "no module API"
			return
		}
		var err error
		dir, err = os.MkdirTemp("", "jsbench-mod")
		if err != nil {
			fatal(err)
		}
		defer os.RemoveAll(dir)
		os.WriteFile(filepath.Join(dir, "dep.js"), []byte("export const x = 1; export default function () { return 2 }\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"a": 1}`+"\n"), 0o644)
	}
	vm, err := newFeatureVM(e, dir)
	if err != nil {
		r.Status, r.Err = "fail", errString(err)
		return
	}
	defer vm.Close()
	stop := engine.Watchdog(vm, 10*time.Second)
	defer stop()
	switch f.Kind {
	case "script", "host":
		r.Got, err = vm.Eval("probe.js", f.Src)
	case "async":
		if _, err = vm.Eval("probe.js", f.Src); err == nil {
			if err = vm.RunJobs(); err == nil {
				r.Got, err = vm.Eval("result.js", "String(globalThis.__result)")
			}
		}
	case "module":
		mvm, ok := vm.(engine.ModuleVM)
		if !ok {
			r.Status, r.Err = "n/a", "no module API"
			return
		}
		if err = mvm.EvalModule(filepath.Join(dir, "main.js"), f.Src); err == nil {
			if err = vm.RunJobs(); err == nil {
				r.Got, err = vm.Eval("result.js", "String(globalThis.__result)")
			}
		}
	}
	if err != nil {
		r.Status, r.Err = "fail", errString(err)
		return
	}
	switch {
	case f.Kind == "host":
		r.Status = "pass"
		if r.Got == "undefined" {
			r.Status = "fail"
		}
	case r.Got == f.Expect:
		r.Status = "pass"
	default:
		r.Status = "fail"
		r.Err = "expected " + f.Expect
	}
	return
}

func probeScript(src, want string) func(*engine.Engine) featureResult {
	return func(e *engine.Engine) featureResult {
		return runProbe(e, feature{ID: "script", Kind: "script", Src: src, Expect: want})
	}
}

// probeInterrupt runs src, which never ends, and interrupts it after 100 ms.
// It passes when the call returns within 5 s.
func probeInterrupt(src string) func(*engine.Engine) featureResult {
	return func(e *engine.Engine) (r featureResult) {
		if !e.Interrupt {
			return featureResult{Status: "n/a", Err: "no interrupt API"}
		}
		vm, err := e.New(engine.Options{Interruptible: true})
		if err != nil {
			return featureResult{Status: "fail", Err: errString(err)}
		}
		defer vm.Close()
		done := make(chan error, 1)
		start := time.Now()
		time.AfterFunc(100*time.Millisecond, vm.Interrupt)
		go func() {
			_, err := vm.Eval("loop.js", src)
			done <- err
		}()
		select {
		case err := <-done:
			r.MS = float64(time.Since(start).Microseconds()) / 1000
			if err == nil {
				return featureResult{Status: "fail", Err: "script ended without an error", MS: r.MS}
			}
			return featureResult{Status: "pass", Got: errString(err), MS: r.MS}
		case <-time.After(5 * time.Second):
			return featureResult{Status: "fail", Err: "still running 5 s after the interrupt"}
		}
	}
}

// probeMemoryLimit sets a 32 MiB limit and tries three ways of going past
// it: arrays of numbers, many small objects, and a string doubled to
// 256 MiB. It passes when each one either fails with an error or, for the
// string, gives the right length (QuickJS keeps such a string as a rope and
// never allocates it).
func probeMemoryLimit(e *engine.Engine) (r featureResult) {
	if !e.MemoryLimit {
		return featureResult{Status: "n/a", Err: "no memory limit API"}
	}
	start := time.Now()
	cases := []struct{ name, src, ok string }{
		{"arrays", `var a = []; for (var i = 0; i < 200; i++) a.push(new Array(100000).fill(i)); String(a.length)`, ""},
		{"objects", `var o = []; for (var i = 0; i < 3000000; i++) o.push({x: i, s: "k" + i}); String(o.length)`, ""},
		{"string", `var s = "x"; for (var i = 0; i < 28; i++) s = s + s; String(s.length)`, "268435456"},
	}
	var got []string
	pass := true
	for _, c := range cases {
		vm, err := e.New(engine.Options{MemoryLimit: 32 << 20, Interruptible: true})
		if err != nil {
			return featureResult{Status: "fail", Err: errString(err)}
		}
		stop := engine.Watchdog(vm, 20*time.Second)
		out, err := vm.Eval(c.name+".js", c.src)
		stop()
		vm.Close()
		switch {
		case err != nil && strings.Contains(strings.ToLower(errString(err)), "interrupt"):
			got, pass = append(got, c.name+": still running after 20 s"), false
		case err != nil:
			got = append(got, c.name+": stopped ("+errString(err)+")")
		case c.ok != "" && out == c.ok:
			got = append(got, c.name+": right result without allocating it")
		case c.ok != "":
			got, pass = append(got, c.name+": wrong result "+out+" without an error"), false
		default:
			got, pass = append(got, c.name+": went past the limit"), false
		}
	}
	r = featureResult{Status: "pass", Got: strings.Join(got, "; "), MS: float64(time.Since(start).Microseconds()) / 1000}
	if !pass {
		r.Status, r.Err = "fail", r.Got
	}
	return r
}

// probeDepth finds how deep a plain recursive function can go before the
// engine throws RangeError, up to 100000. A crash of the process shows as a
// crash in the results.
func probeDepth(e *engine.Engine) featureResult {
	r := runProbe(e, feature{ID: "recursion-depth", Kind: "script", Expect: "", Src: `
function f(n) { return n === 0 ? 0 : 1 + f(n - 1) }
function ok(n) { try { f(n); return true } catch (e) { if (e instanceof RangeError) return false; throw e } }
var lo = 0, hi = 1000, out;
while (hi <= 100000 && ok(hi)) { lo = hi; hi *= 2 }
if (hi > 100000 && ok(100000)) out = ">=100000";
else { if (hi > 100000) hi = 100000; while (hi - lo > 1) { var m = (lo + hi) >> 1; if (ok(m)) lo = m; else hi = m } out = String(lo) }
out`})
	if r.Err == "expected " {
		r.Status, r.Err = "pass", ""
	}
	return r
}
