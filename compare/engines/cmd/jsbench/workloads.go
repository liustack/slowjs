// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/liustack/slowjs/compare/engines/engine"
)

func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		fatal(err)
	}
	return string(b)
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

// load runs src on vm the way the engine recommends: compiled once, then run.
func load(vm engine.VM, name, src string) error {
	p, err := vm.Compile(name, src)
	if err != nil {
		return err
	}
	_, err = vm.Run(p)
	return err
}

// ---- React SSR ----

type reactResult struct {
	OK        bool   `json:"ok"`
	Err       string `json:"err,omitempty"`
	Match     bool   `json:"match"`
	HTMLBytes int    `json:"html_bytes"`
	HTMLSum   string `json:"html_sum"`
	Diff      string `json:"diff,omitempty"`
	Render    stats  `json:"render"`
	// CPUMS is the process CPU time (user and system, all threads) of the
	// timed renders divided by their number: it counts work on other
	// threads, such as the Go garbage collector, that wall time can hide.
	CPUMS float64 `json:"cpu_ms_per_op"`
}

func runReact(e *engine.Engine, bundle, expected string, warm, n int) (r reactResult) {
	src, want := readFile(bundle), readFile(expected)
	vm, err := e.New(engine.Options{})
	if err != nil {
		r.Err = errString(err)
		return
	}
	defer vm.Close()
	if err := load(vm, "react-ssr.js", src); err != nil {
		r.Err = "load: " + errString(err)
		return
	}
	html, err := vm.Call("render")
	if err != nil {
		r.Err = "render: " + errString(err)
		return
	}
	r.HTMLBytes, r.HTMLSum, r.Match = len(html), sum(html), html == want
	if !r.Match {
		r.Diff = firstDiff(html, want)
	}
	for i := 0; i < warm; i++ {
		if _, err := vm.Call("render"); err != nil {
			r.Err = errString(err)
			return
		}
	}
	d := make([]time.Duration, n)
	c0 := cpuTime()
	for i := range d {
		t := time.Now()
		h, err := vm.Call("render")
		d[i] = time.Since(t)
		if err != nil {
			r.Err = errString(err)
			return
		}
		if len(h) != len(html) {
			r.Err = "output changed between renders"
			return
		}
	}
	r.CPUMS = ms(cpuTime()-c0) / float64(n)
	r.OK, r.Render = true, summarize(d)
	return
}

func firstDiff(got, want string) string {
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	lo := max(0, i-40)
	return fmt.Sprintf("at byte %d: got %q, want %q", i, got[lo:min(len(got), i+40)], want[lo:min(len(want), i+40)])
}

type concurrentResult struct {
	OK      bool    `json:"ok"`
	Err     string  `json:"err,omitempty"`
	Renders int     `json:"renders"`
	WallMS  float64 `json:"wall_ms"`
}

// runConcurrent renders on g VMs at once, one goroutine each, and checks
// every page.
func runConcurrent(e *engine.Engine, bundle, expected string, g, n int) (r concurrentResult) {
	src, want := readFile(bundle), readFile(expected)
	var mu sync.Mutex
	var errs []string
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < g; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fail := func(s string) { mu.Lock(); errs = append(errs, s); mu.Unlock() }
			defer func() {
				if p := recover(); p != nil {
					fail(fmt.Sprint("panic: ", p))
				}
			}()
			vm, err := e.New(engine.Options{})
			if err != nil {
				fail(errString(err))
				return
			}
			defer vm.Close()
			if err := load(vm, "react-ssr.js", src); err != nil {
				fail(errString(err))
				return
			}
			for j := 0; j < n; j++ {
				h, err := vm.Call("render")
				if err != nil {
					fail(errString(err))
					return
				}
				if h != want {
					fail("wrong output")
					return
				}
				mu.Lock()
				r.Renders++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	r.WallMS = float64(time.Since(start).Microseconds()) / 1000
	if len(errs) > 0 {
		r.Err = strings.Join(errs, "; ")
		return
	}
	r.OK = true
	return
}

// ---- V8 benchmark suite, version 7 ----

var v8Files = []string{"base.js", "richards.js", "deltablue.js", "crypto.js", "raytrace.js", "earley-boyer.js", "regexp.js", "splay.js", "navier-stokes.js"}

type v8Result struct {
	Scores map[string]string `json:"scores"` // benchmark -> score, or "error: ..."
	Load   map[string]string `json:"load_errors,omitempty"`
	WallS  float64           `json:"wall_s"`
	CPUS   float64           `json:"cpu_s"` // process CPU time of the whole suite
}

func runV8(e *engine.Engine, dir, only string) (r v8Result) {
	files := v8Files
	if only != "" {
		if !slices.Contains(v8Files, only+".js") {
			fatal(fmt.Errorf("no V8 benchmark %q", only))
		}
		files = []string{"base.js", only + ".js"}
	}
	r.Scores, r.Load = map[string]string{}, map[string]string{}
	start, c0 := time.Now(), cpuTime()
	defer func() { r.WallS, r.CPUS = time.Since(start).Seconds(), (cpuTime() - c0).Seconds() }()
	vm, err := e.New(engine.Options{})
	if err != nil {
		r.Load["vm"] = errString(err)
		return
	}
	defer vm.Close()
	for _, f := range files {
		if _, err := vm.Eval(f, readFile(filepath.Join(dir, f))); err != nil {
			r.Load[f] = errString(err)
		}
	}
	out, err := vm.Eval("run.js", `var __res = {};
BenchmarkSuite.RunSuites({
  NotifyResult: function (name, result) { __res[name] = String(result); },
  NotifyError: function (name, error) { __res[name] = "error: " + error; },
  NotifyScore: function (score) { __res.Score = String(score); }
});
JSON.stringify(__res)`)
	if err != nil {
		r.Load["run.js"] = errString(err)
		return
	}
	if err := json.Unmarshal([]byte(out), &r.Scores); err != nil {
		r.Load["result"] = errString(err) + ": " + out
	}
	return
}

// ---- micro workloads ----

// Each workload sets up its input once and defines bench(), which does a
// fixed amount of work and returns a check value every engine must agree
// on. They are written in plain ES5 and use common regular expressions.
var micro = map[string]string{
	"json-parse": jsonSetup + `
function bench() { var o = JSON.parse(DOC); return String(o.items.length + o.items[999].tags.length); }`,
	"json-stringify": jsonSetup + `
var OBJ = JSON.parse(DOC);
function bench() { return String(JSON.stringify(OBJ).length); }`,
	"string-concat": `
function bench() {
  var s = "";
  for (var i = 0; i < 20000; i++) { s += "item-" + i + ";"; }
  var parts = [];
  for (var j = 0; j < 5000; j++) { parts.push("<li class=\"row\">" + j + "</li>"); }
  var t = parts.join("");
  return String(s.length + t.length + s.charCodeAt(1000) + t.indexOf("4999"));
}`,
	"regexp": `
var LOG = (function () {
  var lines = [];
  for (var i = 0; i < 2000; i++) {
    lines.push("2026-10-" + (10 + i % 20) + " 12:" + (10 + i % 50) + ":0" + (i % 10) + " GET /api/v1/items/" + i + "?q=abc HTTP/1.1 " + (i % 7 === 0 ? 500 : 200) + " 10.0." + (i % 256) + "." + (i * 7 % 256) + " user" + i + "@example.com");
  }
  return lines.join("\n");
})();
function bench() {
  var ip = LOG.match(/\b(\d{1,3}\.){3}\d{1,3}\b/g).length;
  var mail = LOG.match(/[a-z0-9]+@[a-z]+\.[a-z]{2,}/g).length;
  var errs = 0, re = /GET (\S+) HTTP\/1\.1 (\d{3})/g, m;
  while ((m = re.exec(LOG)) !== null) { if (m[2] === "500") errs++; }
  var squeezed = LOG.replace(/\s+/g, " ").length;
  var fields = LOG.split(/[ \n]/).length;
  return ip + "," + mail + "," + errs + "," + squeezed + "," + fields;
}`,
}

const jsonSetup = `
var DOC = (function () {
  var items = [];
  for (var i = 0; i < 1000; i++) {
    items.push({ id: i, name: "Product " + i + " éè", price: i * 1.25, stock: i % 17, active: i % 3 === 0,
      tags: ["a" + i % 5, "b" + i % 7, "c"], dims: { w: i % 100, h: 20.5, d: null }, note: "line\n\"quoted\"" });
  }
  return JSON.stringify({ version: 3, items: items });
})();`

type microResult struct {
	OK    bool    `json:"ok"`
	Err   string  `json:"err,omitempty"`
	Check string  `json:"check"`
	Call  stats   `json:"call"`
	CPUMS float64 `json:"cpu_ms_per_op"` // as in reactResult
}

func runMicro(e *engine.Engine, name string, warm, n int) (r microResult) {
	src, ok := micro[name]
	if !ok {
		fatal(fmt.Errorf("no micro workload %q", name))
	}
	vm, err := e.New(engine.Options{})
	if err != nil {
		r.Err = errString(err)
		return
	}
	defer vm.Close()
	if err := load(vm, name+".js", src); err != nil {
		r.Err = "load: " + errString(err)
		return
	}
	r.Check, err = vm.Call("bench")
	if err != nil {
		r.Err = errString(err)
		return
	}
	for i := 0; i < warm; i++ {
		vm.Call("bench")
	}
	d := make([]time.Duration, n)
	c0 := cpuTime()
	for i := range d {
		t := time.Now()
		c, err := vm.Call("bench")
		d[i] = time.Since(t)
		if err != nil || c != r.Check {
			r.Err = "result changed: " + errString(err)
			return
		}
	}
	r.CPUMS = ms(cpuTime()-c0) / float64(n)
	r.OK, r.Call = true, summarize(d)
	return
}

// ---- QuickJS micro benchmarks (compare/testdata) ----

type upstreamResult struct {
	Tests map[string]upstreamTest `json:"tests"`
}

type upstreamTest struct {
	NsPerN float64 `json:"ns_per_n,omitempty"`
	N      int     `json:"n,omitempty"`
	Err    string  `json:"err,omitempty"`
}

var callLine = regexp.MustCompile(`(?m)^(\w+)\(\$N\);\s*$`)

func runUpstream(e *engine.Engine, dir string, budget time.Duration, only string) (r upstreamResult) {
	r.Tests = map[string]upstreamTest{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.js"))
	sort.Strings(files)
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".js")
		// filepath.Glob matches hidden files too, such as the ._ files
		// macOS tar adds; they are not tests.
		if strings.HasPrefix(name, ".") {
			continue
		}
		if ok, _ := filepath.Match(only, name); only != "" && !ok {
			continue
		}
		r.Tests[name] = upstreamOne(e, readFile(f), budget)
	}
	return
}

func upstreamOne(e *engine.Engine, src string, budget time.Duration) (t upstreamTest) {
	defer func() {
		if p := recover(); p != nil {
			t = upstreamTest{Err: fmt.Sprint("panic: ", p)}
		}
	}()
	m := callLine.FindStringSubmatch(src)
	if m == nil {
		return upstreamTest{Err: "no call line"}
	}
	src = callLine.ReplaceAllString(src, "globalThis.__u = function (n) { return "+m[1]+"(n) };")
	vm, err := e.New(engine.Options{Interruptible: true})
	if err != nil {
		return upstreamTest{Err: errString(err)}
	}
	defer vm.Close()
	if err := load(vm, m[1]+".js", src); err != nil {
		return upstreamTest{Err: errString(err)}
	}
	stop := engine.Watchdog(vm, 60*time.Second)
	defer stop()
	run := func(n int) (time.Duration, error) {
		if _, err := vm.Eval("n.js", "globalThis.__n = "+strconv.Itoa(n)); err != nil {
			return 0, err
		}
		t := time.Now()
		_, err := vm.Eval("call.js", "__u(__n)")
		return time.Since(t), err
	}
	n := 1
	for {
		d, err := run(n)
		if err != nil {
			return upstreamTest{Err: errString(err)}
		}
		if d >= budget/4 || n >= 1<<26 {
			break
		}
		n *= 2
	}
	ds := make([]time.Duration, 3)
	for i := range ds {
		d, err := run(n)
		if err != nil {
			return upstreamTest{Err: errString(err)}
		}
		ds[i] = d
	}
	s := summarize(ds)
	return upstreamTest{NsPerN: s.Median / float64(n), N: n}
}

// ---- cold start ----

type coldResult struct {
	OK          bool   `json:"ok"`
	Err         string `json:"err,omitempty"`
	NewVM       stats  `json:"new_vm"`
	FromSource  stats  `json:"new_vm_and_load_source"`
	FromProgram *stats `json:"new_vm_and_load_program,omitempty"`
	FirstRender stats  `json:"first_render"`
}

func runCold(e *engine.Engine, bundle string, n int) (r coldResult) {
	src := readFile(bundle)
	var dNew, dSrc, dProg, dFirst []time.Duration
	var prog engine.Program
	if e.SharedPrograms {
		var err error
		if prog, err = e.CompileShared("react-ssr.js", src); err != nil {
			r.Err = "compile: " + errString(err)
			return
		}
	}
	for i := 0; i < n; i++ {
		runtime.GC()
		t := time.Now()
		vm, err := e.New(engine.Options{})
		dNew = append(dNew, time.Since(t))
		if err != nil {
			r.Err = errString(err)
			return
		}
		vm.Close()

		runtime.GC()
		t = time.Now()
		vm, err = e.New(engine.Options{})
		if err == nil {
			_, err = vm.Eval("react-ssr.js", src)
		}
		dSrc = append(dSrc, time.Since(t))
		if err != nil {
			r.Err = "load: " + errString(err)
			return
		}
		t = time.Now()
		html, err := vm.Call("render")
		dFirst = append(dFirst, time.Since(t))
		vm.Close()
		if err != nil {
			r.Err = "render: " + errString(err)
			return
		}

		if prog != nil {
			runtime.GC()
			t = time.Now()
			vm, err = e.New(engine.Options{})
			if err == nil {
				_, err = vm.Run(prog)
			}
			dProg = append(dProg, time.Since(t))
			if err != nil {
				r.Err = "run program: " + errString(err)
				return
			}
			// Not timed: a VM loaded from the shared program renders the
			// same page as one loaded from source.
			h, err := vm.Call("render")
			vm.Close()
			if err != nil || h != html {
				r.Err = fmt.Sprintf("render after the shared program: different page (%d bytes, error %v)", len(h), err)
				return
			}
		}
	}
	r.OK, r.NewVM, r.FromSource, r.FirstRender = true, summarize(dNew), summarize(dSrc), summarize(dFirst)
	if prog != nil {
		s := summarize(dProg)
		r.FromProgram = &s
	}
	return
}

// ---- memory per VM ----

type memResult struct {
	OK          bool    `json:"ok"`
	Err         string  `json:"err,omitempty"`
	K           int     `json:"k"`
	Loaded      string  `json:"loaded"` // "program" or "source"
	RSSPerVM    float64 `json:"rss_per_vm_bytes"`
	GoHeapPerVM float64 `json:"go_heap_per_vm_bytes"`
	RSSBase     float64 `json:"rss_base_bytes"`
}

func rss() float64 {
	if runtime.GOOS == "linux" {
		b, err := os.ReadFile("/proc/self/statm")
		if err == nil {
			f := strings.Fields(string(b))
			if len(f) > 1 {
				p, _ := strconv.ParseFloat(f[1], 64)
				return p * float64(os.Getpagesize())
			}
		}
	}
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		fatal(err)
	}
	kb, _ := strconv.ParseFloat(string(bytes.TrimSpace(out)), 64)
	return kb * 1024
}

func heap() float64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapInuse)
}

// runMem keeps k VMs alive, each with the bundle loaded and one page
// rendered, and reports the growth of the process per VM.
func runMem(e *engine.Engine, bundle string, k int) (r memResult) {
	src := readFile(bundle)
	r.K, r.Loaded = k, "source"
	var prog engine.Program
	if e.SharedPrograms {
		var err error
		if prog, err = e.CompileShared("react-ssr.js", src); err != nil {
			r.Err = errString(err)
			return
		}
		r.Loaded = "program"
	}
	// One VM first, so that what every process loads once (wasm compilation,
	// Intl tables, code pages) is in the baseline.
	warm, err := e.New(engine.Options{})
	if err != nil {
		r.Err = errString(err)
		return
	}
	if prog != nil {
		_, err = warm.Run(prog)
	} else {
		_, err = warm.Eval("react-ssr.js", src)
	}
	if err == nil {
		_, err = warm.Call("render")
	}
	if err != nil {
		r.Err = errString(err)
		return
	}
	h0 := heap()
	r.RSSBase = rss()
	vms := make([]engine.VM, 0, k)
	for i := 0; i < k; i++ {
		vm, err := e.New(engine.Options{})
		if err != nil {
			r.Err = errString(err)
			return
		}
		if prog != nil {
			_, err = vm.Run(prog)
		} else {
			_, err = vm.Eval("react-ssr.js", src)
		}
		if err == nil {
			_, err = vm.Call("render")
		}
		if err != nil {
			r.Err = errString(err)
			return
		}
		vms = append(vms, vm)
	}
	h1 := heap()
	r1 := rss()
	r.RSSPerVM = (r1 - r.RSSBase) / float64(k)
	r.GoHeapPerVM = (h1 - h0) / float64(k)
	for _, vm := range vms {
		vm.Close()
	}
	warm.Close()
	r.OK = true
	return
}

func sprint(v any) string { return fmt.Sprint(v) }
