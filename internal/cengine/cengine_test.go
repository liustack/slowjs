// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package cengine

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/liustack/slowjs/internal/gohost"
	"github.com/liustack/slowjs/internal/hostmem"
)

func newVM(t *testing.T, o Options) *VM {
	t.Helper()
	v, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// closeVM closes v and checks that the engine had no block left and the host
// unmapped everything.
func closeVM(t *testing.T, v *VM) {
	t.Helper()
	leaked, err := v.Close()
	if err != nil {
		t.Fatal(err)
	}
	if leaked != 0 {
		t.Errorf("JS_FreeRuntime: %d blocks still in use", leaked)
	}
	if u := v.Host().Memory().Usage(); u.Chunks != 0 || u.Footprint != 0 || u.Stack != 0 {
		t.Errorf("after Close the host holds %+v", u)
	}
}

// sameFootprint checks that the engine and the host count the same chunks.
func sameFootprint(t *testing.T, v *VM, when string) Usage {
	t.Helper()
	u := v.Usage()
	if u.Footprint != u.Host.Footprint || int(u.Chunks) != u.Host.Chunks {
		t.Errorf("%s: the engine counts %d bytes in %d chunks, the host %d in %d",
			when, u.Footprint, u.Chunks, u.Host.Footprint, u.Host.Chunks)
	}
	return u
}

// TestExpressions runs the expressions of the first runs of the generated
// engine and more, each in a new VM. The expected results are what package
// slowjs gives.
func TestExpressions(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`1+1`, `2`},
		{`6*7`, `42`},
		{`"a"+1`, `a1`},
		{`[1,2,3].map(x=>x*2).join()`, `2,4,6`},
		{`JSON.stringify({a:"x\"y\n ",b:[1,2,{c:null}]})`, "{\"a\":\"x\\\"y\\n \",\"b\":[1,2,{\"c\":null}]}"},
		{`(1.5).toFixed(3)`, `1.500`},
		{`Math.PI`, `3.141592653589793`},
		{`function fib(n){return n<2?n:fib(n-1)+fib(n-2)} fib(22)`, `17711`},
		{`new Date(0).toISOString()`, `1970-01-01T00:00:00.000Z`},
		{`/(\d+)-(\d+)/.exec("x10-20y").slice(1).join("|")`, `10|20`},
		{`String(123.456e30)`, `1.23456e+32`},
		{`try{null.x}catch(e){e.name+": "+e.message}`, `TypeError: cannot read property 'x' of null`},
		{`try{nope}catch(e){e.name+": "+e.message}`, `ReferenceError: 'nope' is not defined`},
		{`var s=0; for(var i=0;i<1e6;i++) s+=i; s`, `499999500000`},
		{`class A{#x=5; get x(){return this.#x}} new A().x`, `5`},
		{`String(10n**30n)`, `1000000000000000000000000000000`},
		{`[3,1,2,10].sort().join()`, `1,10,2,3`},
		{`[3,1,2,10].sort((a,b)=>a-b).join()`, `1,2,3,10`},
		{`typeof (async()=>1)()`, `object`},
		{`eval("1+2")`, `3`},
		{`"a".localeCompare("b")`, `-1`},
		{`Array.from({length:10},(_, i)=>i).reduce((a,b)=>a+b)`, `45`},
		{`"abc".replace(/b/g, "X") + "a-b-c".split("-").length`, `aXc3`},
		{`Object.keys({b:1,a:2,1:3}).join()`, `1,b,a`},
		{`[..."héllo"].reverse().join("")`, `olléh`},
		{`new Map([[1,2]]).get(1) + new Set([1,1,2]).size`, `4`},
		{`parseFloat("3.25e2") + parseInt("ff", 16)`, `580`},
		{`(0.1+0.2).toString()`, `0.30000000000000004`},
		{`Math.max(...[1,5,3])`, `5`},
		{`JSON.parse('{"x":[1,{"y":"z"}]}').x[1].y`, `z`},
	} {
		v := newVM(t, Options{Seed: 1})
		got, err := v.Eval(c.src, "expr.js")
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
		} else if got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
		closeVM(t, v)
	}
}

func TestJobs(t *testing.T) {
	v := newVM(t, Options{})
	defer closeVM(t, v)
	if _, err := v.Eval(`var r = []; (async () => { r.push(await 5); r.push(await Promise.resolve(6)) })(); r.push(1); 0`, "jobs.js"); err != nil {
		t.Fatal(err)
	}
	if got, err := v.Eval(`r.join()`, "jobs.js"); err != nil || got != "1,5,6" {
		t.Fatalf("the jobs ran to %q, %v, want 1,5,6", got, err)
	}
	// A reaction that throws rejects its promise: the job itself does not
	// fail, and the next script sees the rejection.
	if _, err := v.Eval(`var why; Promise.resolve().then(() => { throw new RangeError("in a job") }).catch(e => why = e.message); 0`, "jobs.js"); err != nil {
		t.Fatal(err)
	}
	if got, err := v.Eval(`why`, "jobs.js"); err != nil || got != "in a job" {
		t.Fatalf("the rejection gave %q, %v", got, err)
	}
}

func TestException(t *testing.T) {
	v := newVM(t, Options{})
	defer closeVM(t, v)
	_, err := v.Eval("function f() {\n  throw new TypeError('boom')\n}\nf()", "throw.js")
	var e *JSError
	if !errors.As(err, &e) {
		t.Fatalf("got %v, want a JSError", err)
	}
	if e.Message != "TypeError: boom" || !strings.Contains(e.Stack, "throw.js:2") {
		t.Fatalf("message %q, stack %q", e.Message, e.Stack)
	}
	if _, err := v.Eval(`throw 42`, "throw.js"); err == nil || err.Error() != "42" {
		t.Fatalf("throw 42 gave %v", err)
	}
	if got, err := v.Eval(`"still " + "fine"`, "after.js"); err != nil || got != "still fine" {
		t.Fatalf("after the exceptions: %q, %v", got, err)
	}
}

// TestFileName: a file name that fits the scratch shows whole in the stack of
// an exception, and one that does not is refused with an error, not cut to
// the same name as another that differs only at its end.
func TestFileName(t *testing.T) {
	v := newVM(t, Options{})
	defer closeVM(t, v)
	fits := strings.Repeat("a", maxFileName-len("/end.js")) + "/end.js"
	_, err := v.Eval("throw new Error('x')", fits)
	var e *JSError
	if !errors.As(err, &e) || !strings.Contains(e.Stack, "("+fits+":1") {
		t.Fatalf("a name of %d bytes: got %v, want its stack to hold the whole name", len(fits), err)
	}
	for _, name := range []string{fits + "1", fits + "2", strings.Repeat("b", 1<<20)} {
		got, err := v.Eval("1", name)
		if err == nil || !strings.Contains(err.Error(), "file name") {
			t.Errorf("a name of %d bytes gave %q, %v, want an error about the file name", len(name), got, err)
		}
	}
	if got, err := v.Eval("1+1", "after.js"); err != nil || got != "2" {
		t.Fatalf("after the refused names: %q, %v", got, err)
	}
}

func TestSeed(t *testing.T) {
	draw := func(seed uint64) string {
		v := newVM(t, Options{Seed: seed})
		defer closeVM(t, v)
		r, err := v.Eval(`Array.from({length: 4}, Math.random).join()`, "random.js")
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	a, b, c := draw(7), draw(7), draw(8)
	if a != b || a == c {
		t.Fatalf("seed 7 gave %s and %s, seed 8 %s", a, b, c)
	}
}

func TestClock(t *testing.T) {
	v := newVM(t, Options{})
	defer closeVM(t, v)
	now, err := v.Eval(`Date.now()`, "clock.js")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(time.UnixMilli(atoi(t, now))); d < -time.Second || d > time.Second {
		t.Fatalf("Date.now() on the default clock is %s", now)
	}
	fixed := time.Date(2026, 3, 4, 5, 6, 7, 8e6, time.UTC)
	v.SetWallClock(fixed, false)
	got, err := v.Eval(`var a = Date.now(); for (var i = 0; i < 1e5; i++); [a, Date.now(), new Date().toISOString()].join()`, "clock.js")
	if want := "1772600767008,1772600767008,2026-03-04T05:06:07.008Z"; err != nil || got != want {
		t.Fatalf("on a fixed clock: %q, %v, want %q", got, err, want)
	}
	v.SetWallClock(fixed, true)
	got, err = v.Eval(`var a = Date.now(), b; do { b = Date.now() } while (b === a); (b > a) + ":" + (b - a < 1000)`, "clock.js")
	if err != nil || got != "true:true" {
		t.Fatalf("on a moving clock: %q, %v", got, err)
	}
}

func atoi(t *testing.T, s string) int64 {
	t.Helper()
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("%q is not a whole number", s)
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

// The allocation scripts: each frees what it made when its function returns.
var heavy = []struct{ name, src, want string }{
	{"array", `(function () { var a = []; for (var i = 0; i < 1e6; i++) a.push(i); return a.length + ":" + a[999999] })()`, "1000000:999999"},
	{"string", `(function () { var s = "x".repeat(1 << 24); var t = ""; for (var i = 0; i < 20000; i++) t += "ab" + i; return s.length + ":" + t.length })()`, "16777216:128890"},
	{"json", `(function () { var o = []; for (var i = 0; i < 1e5; i++) o.push({i: i, s: "v" + i, a: [i, i + 1]}); var j = JSON.stringify(o); var p = JSON.parse(j); return p.length + ":" + j.length + ":" + p[99999].s })()`, "100000:4255566:v99999"},
	{"objects", `(function () { var m = new Map(); for (var i = 0; i < 2e5; i++) m.set("k" + i, {i: i}); var n = 0; for (var [k, v] of m) n += v.i; return m.size + ":" + n })()`, "200000:19999900000"},
	{"cycles", `(function () { for (var i = 0; i < 1e5; i++) { var a = {}, b = {a: a}; a.b = b } return "ok" })()`, "ok"},
	{"regexp", `(function () { var s = "ab12cd34".repeat(1e5); return s.replace(/\d+/g, function (m) { return "<" + m + ">" }).length })()`, "1200000"},
}

// TestAllocation runs scripts that allocate much in one VM, and checks the
// counts of the engine and the host at each step: big blocks get dedicated
// chunks that go back at once (and grow by remap on linux), a GC and a
// release give the free memory back, and Close leaves nothing.
func TestAllocation(t *testing.T) {
	v := newVM(t, Options{Seed: 1})
	base := sameFootprint(t, v, "new")
	t.Logf("new: footprint %d in %d chunks, MemorySize %d", base.Footprint, base.Chunks, base.MemorySize)
	for _, c := range heavy {
		got, err := v.Eval(c.src, c.name+".js")
		if err != nil || got != c.want {
			t.Fatalf("%s: %q, %v, want %q", c.name, got, err, c.want)
		}
		u := sameFootprint(t, v, c.name)
		t.Logf("%s: footprint %d in %d chunks, MemorySize %d, counts %+v", c.name, u.Footprint, u.Chunks, u.MemorySize, u.Counts)
	}
	k := v.Host().Counts()
	if k.Frees == 0 {
		t.Error("no dedicated chunk went back")
	}
	if hostmem.CanRemap && k.Remaps == 0 {
		t.Error("no dedicated chunk grew by remap")
	}
	if !hostmem.CanRemap && k.Remaps != 0 {
		t.Error("remaps where the OS cannot remap")
	}
	if k.GrowFailures+k.FreeFailures+k.RemapFailures != 0 {
		t.Errorf("the OS refused: %+v, last %v", k, v.Host().LastError())
	}
	v.RunGC()
	gc := sameFootprint(t, v, "gc")
	r, err := v.ReleaseMemory(0)
	if err != nil {
		t.Fatal(err)
	}
	rel := sameFootprint(t, v, "release")
	t.Logf("gc: footprint %d in %d chunks, MemorySize %d", gc.Footprint, gc.Chunks, gc.MemorySize)
	t.Logf("release: %+v, then footprint %d in %d chunks, MemorySize %d", r, rel.Footprint, rel.Chunks, rel.MemorySize)
	if rel.Footprint > gc.Footprint || rel.Footprint != gc.Footprint-r.Chunks {
		t.Errorf("release of %d bytes of chunks took the footprint from %d to %d", r.Chunks, gc.Footprint, rel.Footprint)
	}
	// The chunks left keep a few blocks each: their free pages went back, and
	// what stays in memory is about what a new VM had.
	if kept := rel.Footprint - r.Pages; kept > base.Footprint+1<<20 {
		t.Errorf("after the release %d bytes of chunks stay in memory, %d when new", kept, base.Footprint)
	}
	// What is left is about what a new VM had: the scripts kept nothing.
	if rel.MemorySize > base.MemorySize+256<<10 {
		t.Errorf("MemorySize %d after the scripts, %d when new", rel.MemorySize, base.MemorySize)
	}
	if got, err := v.Eval(heavy[2].src, "again.js"); err != nil || got != heavy[2].want {
		t.Fatalf("after the release: %q, %v", got, err)
	}
	closeVM(t, v)
}

// TestGoHeap checks that a VM leaves nothing on the Go heap: the live heap
// after many runs is what it was after a few.
func TestGoHeap(t *testing.T) {
	run := func() {
		v := newVM(t, Options{Seed: 1})
		if got, err := v.Eval(heavy[3].src, "objects.js"); err != nil || got != heavy[3].want {
			t.Fatalf("%q, %v", got, err)
		}
		v.RunGC()
		if _, err := v.ReleaseMemory(0); err != nil {
			t.Fatal(err)
		}
		closeVM(t, v)
	}
	live := func() (uint64, uint64) {
		runtime.GC()
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		return ms.HeapAlloc, ms.HeapObjects
	}
	for range 5 {
		run()
	}
	b0, o0 := live()
	const n = 50
	for range n {
		run()
	}
	b1, o1 := live()
	t.Logf("live Go heap: %d bytes in %d objects after 5 runs, %d in %d after %d more", b0, o0, b1, o1, n)
	if b1 > b0+64<<10 {
		t.Errorf("the live Go heap grew by %d bytes over %d runs", b1-b0, n)
	}
}

// TestDeepRecursion: a recursion within the call depth limit of the engine
// (5000 levels, of which the entry of Eval and the global code take two, so
// f(4997) is the deepest) runs to its end, a recursion past the limit ends in
// a RangeError that the script can catch and Eval returns, not in a fault of
// the process, and the VM recurses as deep as before. The levels need room on
// the shadow stack, so it runs only for an engine generated for the whole
// 32 MiB stack of the host (-shadow-size-log2=25 of the generator, which
// engine_decl_<arch>.go records).
func TestDeepRecursion(t *testing.T) {
	if shadowSizeLog2 < 25 {
		t.Skipf("the engine was generated for a shadow stack of 2^%d bytes, and a deep recursion needs 2^25", shadowSizeLog2)
	}
	v := newVM(t, Options{})
	defer closeVM(t, v)
	within := func(n, label string) {
		t.Helper()
		got, err := v.Eval("f("+n+")", "deep.js")
		if err != nil || got != n {
			t.Fatalf("%s: f(%s) gave %q, %v", label, n, got, err)
		}
	}
	if _, err := v.Eval(`function f(n) { return n ? 1 + f(n - 1) : 0 }`, "deep.js"); err != nil {
		t.Fatal(err)
	}
	within("4000", "before")
	within("4997", "the deepest")

	for _, n := range []string{"4998", "5000", "1e6"} {
		_, err := v.Eval("f("+n+")", "deeper.js")
		var e *JSError
		if !errors.As(err, &e) || e.Message != "RangeError: Maximum call stack size exceeded" {
			t.Fatalf("f(%s) gave %v, want the RangeError of the call depth", n, err)
		}
	}
	got, err := v.Eval(`try { f(1e6) } catch (e) { e instanceof RangeError }`, "caught.js")
	if err != nil || got != "true" {
		t.Fatalf("a script catching the overflow: %q, %v", got, err)
	}
	within("4000", "after")
	within("4997", "the deepest again")
}

// TestStackSmallerThanGenerated: New refuses a shadow stack smaller than the
// region the engine was generated for.
func TestStackSmallerThanGenerated(t *testing.T) {
	if shadowSizeLog2 <= 16 {
		t.Skip("the engine was generated for the smallest stack the host makes")
	}
	v, err := New(Options{StackSize: 1 << (shadowSizeLog2 - 1)})
	if err == nil {
		closeVM(t, v)
		t.Fatal("New took a stack half the size the engine was generated for")
	}
	if !strings.Contains(err.Error(), "generated for") {
		t.Fatalf("New fails for another reason: %v", err)
	}
}

// TestStackLargerThanGenerated: New refuses a shadow stack larger than the
// region the engine was generated for. The engine finds the base of its stack
// by rounding its stack pointer down to that size, so in a larger stack the
// base, the poll counter and the VM id it reads there would fall into live
// frames.
func TestStackLargerThanGenerated(t *testing.T) {
	v, err := New(Options{StackSize: 2 << shadowSizeLog2})
	if err == nil {
		closeVM(t, v)
		t.Fatal("New took a stack twice the size the engine was generated for")
	}
	if !strings.Contains(err.Error(), "generated for") {
		t.Fatalf("New fails for another reason: %v", err)
	}
}

// TestStackDefault: with no size given, the shadow stack is the region the
// engine was generated for.
func TestStackDefault(t *testing.T) {
	v := newVM(t, Options{})
	defer closeVM(t, v)
	if _, size := v.Host().Stack(); size != 1<<shadowSizeLog2 {
		t.Fatalf("a shadow stack of %d bytes, want %d", size, uintptr(1)<<shadowSizeLog2)
	}
}

// TestGCWhileRunning runs scripts while another goroutine keeps starting
// collections. Each stops the engine's goroutine at a function prologue, and
// may shrink its stack with engine frames on it: the results must not
// change.
func TestGCWhileRunning(t *testing.T) {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				runtime.GC()
			}
		}
	}()
	defer func() { close(stop); <-done }()
	for i := range 300 {
		v := newVM(t, Options{Seed: 1})
		got, err := v.Eval(`function fib(n){return n<2?n:fib(n-1)+fib(n-2)} fib(15)`, "fib.js")
		if err != nil || got != "610" {
			v.Host().MarkBroken()
			v.Close()
			t.Fatalf("run %d with collections going on: %q, %v", i, got, err)
		}
		closeVM(t, v)
	}
}

// TestShapeCache: a new VM keeps the shapes of plain objects after their last
// object is gone, at most DefaultShapeCache of them, as a VM of package
// slowjs does. Shapes no later object reuses do not build up, and turning the
// cache off lets the kept shapes go.
func TestShapeCache(t *testing.T) {
	v := newVM(t, Options{Seed: 1})
	defer closeVM(t, v)
	// Each round makes objects that die with a key no other object has.
	unique := `(function (round) {
		for (let i = 0; i < 3000; i++) {
			const o = {id: i}
			o["u" + round + "_" + i] = i
		}
		return round
	})`
	var settled uintptr
	for n := range 8 {
		src := unique + "(" + string(rune('0'+n)) + ")"
		if got, err := v.Eval(src, "unique.js"); err != nil || got != string(rune('0'+n)) {
			t.Fatalf("round %d: %q, %v", n, got, err)
		}
		v.RunGC()
		used := v.Usage().MemorySize
		switch {
		case n == 3:
			settled = used
		case n > 3 && used > settled:
			t.Fatalf("round %d: MemorySize %d after GC, %d after round 3", n, used, settled)
		}
	}
	shapes := func() int64 {
		v.Usage()
		return (*memoryUsage)(addr(v.base + scratchUsage)).ShapeCount
	}
	kept := shapes()
	var r uintptr
	v.enter(func() { r = JS_SetShapeCache(v.sp, v.ctx, 0) })
	if int32(r) != 0 {
		t.Fatalf("JS_SetShapeCache(0) returned %d", int32(r))
	}
	if got := shapes(); kept-got < DefaultShapeCache {
		t.Errorf("%d shapes with the cache, %d without, want at least %d fewer", kept, got, DefaultShapeCache)
	}
}

// TestStackHeaderKept: what the host passes the engine by address in the
// scratch leaves the header of gohost alone, so a poll after an Eval still
// finds its VM by the shadow pointer.
func TestStackHeaderKept(t *testing.T) {
	v := newVM(t, Options{})
	defer closeVM(t, v)
	id := v.Host().ID()
	for i := range 3 {
		if got, err := v.Eval(`"a".repeat(1000) + new Error("x").stack`, "header.js"); err != nil || len(got) < 1000 {
			t.Fatalf("Eval: %d bytes, %v", len(got), err)
		}
		v.Usage()
		header := (*[2]uint32)(addr(v.base))
		if header[0] != id || header[1] != ^id {
			t.Fatalf("round %d: the header is %#x %#x, want the id %d and its complement", i, header[0], header[1], id)
		}
		if gohost.ByStack(v.sp) != v.Host() {
			t.Fatalf("round %d: ByStack does not find the VM", i)
		}
	}
}
