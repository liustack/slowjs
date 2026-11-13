// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"errors"
	"go/ast"
	"go/token"
	"runtime"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/whatwgurl"
)

// The work package url does for a script stays inside the limits of its VM:
// the eval timeout and Interrupt end it, and so does the memory limit, for
// what a URL takes and for what Go would take to make it. A call of Go is
// short, as its strings are at most whatwgurl.MaxUnits long, and the loops
// over a list are in javascript.

// wantUncatchable checks that err ended the call as running out of memory, or
// as an interrupt when interrupted is set.
func wantUncatchable(t *testing.T, err error, interrupted bool) {
	t.Helper()
	var e *quickjs.Error
	if !errors.As(err, &e) || !e.Uncatchable || e.Interrupted != interrupted || e.OutOfMemory == interrupted {
		t.Fatalf("got %v (%#v), want the uncatchable error of %s", err, e, map[bool]string{false: "running out of memory", true: "an interrupt"}[interrupted])
	}
}

// slowness is how many times slower than usual a VM runs now, 1 for the
// usual: the race detector slows it, and a loaded machine runs it at a share
// of a core. A URL of a path of MaxUnits CJK characters, the slowest a call
// can be but for IDNA, takes about 2 ms usually.
func slowness(t testing.TB) time.Duration {
	vm := newVM(t)
	evalString(t, vm, `var s = "http://h/" + "一".repeat(7000); 'ok'`)
	best := time.Hour
	for i := 0; i < 5; i++ {
		start := time.Now()
		evalString(t, vm, `new URL(s).href.length + ""`)
		best = min(best, time.Since(start))
	}
	return max(1, best/(2*time.Millisecond))
}

// grace is how long after its deadline a call may end: the engine looks at
// the stop flag at its polls, every 12.5 ms of work at most, and a call of Go
// is between two of them.
const grace = 50 * time.Millisecond

// stuck are scripts that do not end, each in the work of a URL or a list: in
// Go, at the limit (a parse, a setter, IDNA), and in javascript (splitting a
// query string, sorting and walking a list, changing a list a URL holds).
// hungry is the same work, with what it makes kept, so that it needs more and
// more memory.
var stuck = []struct{ name, js, hungry string }{
	{"constructor", `for (;;) new URL(path)`, `for (;;) keep.push(new URL(path))`},
	{"constructor idna", `for (;;) new URL(idna)`, `for (;;) keep.push(new URL(idna))`},
	{"base", `for (;;) new URL("x", path)`, `for (;;) keep.push(new URL("x", path).href + path.slice(1))`},
	{"canParse", `for (;;) URL.canParse(path)`, `for (;;) keep.push(URL.canParse(path) + path.slice(1))`},
	{"pathname setter", `var u = new URL("http://h/"); for (;;) u.pathname = wide`, `for (;;) { var u = new URL("http://h/"); u.pathname = wide; keep.push(u) }`},
	{"host setter", `var u = new URL("http://h/"); for (;;) u.host = idna.slice(7, -1)`, `for (;;) { var u = new URL("http://h/"); u.host = idna.slice(7, -1); keep.push(u) }`},
	{"split 16 MiB", `for (;;) new URLSearchParams(pairs)`, `for (;;) keep.push(new URLSearchParams(pairs))`},
	{"split 16 MiB of percent", `for (;;) new URLSearchParams(pct)`, `for (;;) keep.push(new URLSearchParams(pct))`},
	{"sort", `for (;;) list.sort()`, `for (;;) { var l = new URLSearchParams(list); l.sort(); keep.push(l) }`},
	{"iterate", `for (;;) for (const e of list) {}`, `for (;;) for (const e of list) keep.push(e)`},
	{"forEach", `for (;;) list.forEach(function () {})`, `for (;;) list.forEach(function (v, k) { keep.push(k + v) })`},
	{"toString", `for (;;) list.toString()`, `for (;;) keep.push(list.toString())`},
	{"tied set", `var u = new URL("http://h/?a=b"); for (let i = 0; ; i++) u.searchParams.set("k" + (i % 64), "v")`, `for (let i = 0; ; i++) { var u = new URL("http://h/?a=b"); u.searchParams.set("k" + i, "v"); keep.push(u) }`},
	{"search and params", `var u = new URL("http://h/"); for (;;) { u.search = pct.slice(0, 60000); u.searchParams.size }`, `for (;;) { var u = new URL("http://h/"); u.search = pct.slice(0, 60000); u.searchParams.size; keep.push(u) }`},
}

// prepare makes the strings and the list the scripts use, with no limit set.
const prepare = `
	var path = "http://h/" + "一".repeat(7000);
	var wide = "一".repeat(7000);
	var idna = "http://" + Array.from({length: 1024}, (_, i) => String.fromCharCode(0x4e00 + i)).join("") + "/";
	var pairs = "a=b&".repeat(4 << 20);
	var pct = "%41=%42&".repeat(2 << 20);
	var list = new URLSearchParams(Array.from({length: 1 << 18}, (_, i) => ["k" + ((i * 7919) % 262147), "v" + i]));
	"ok"`

// TestTimeout checks that each stuck script ends with the interrupt soon
// after its timeout: within the grace, as the time between two polls of the
// engine is short and a call of Go is short. A late end the machine made, a
// pause of the process or another one on the core, is not there the next
// time, and one the code made is: the test fails when three tries are late.
// The times mean nothing under the race detector, which slows Go and not the
// engine, so it checks only how the script ends there.
func TestTimeout(t *testing.T) {
	slow := slowness(t)
	vm := newVM(t)
	evalString(t, vm, prepare)
	for _, s := range stuck {
		js := s.js
		t.Run(s.name, func(t *testing.T) {
			const timeout = 20 * time.Millisecond
			late := time.Hour
			for try := 0; try < 3 && late > grace*slow; try++ {
				vm.SetEvalTimeout(timeout)
				start := time.Now()
				_, err := vm.Eval(quiet(js), quickjs.EvalGlobal)
				late = min(late, time.Since(start)-timeout)
				vm.SetEvalTimeout(0)
				wantUncatchable(t, err, true)
				if raceEnabled {
					break
				}
			}
			if late > grace*slow && !raceEnabled {
				t.Errorf("ended %v after its timeout of %v", late, timeout)
			}
			if got := evalString(t, vm, `new URL("/a?b#c", "http://h/x").href`); got != "http://h/a?b#c" {
				t.Errorf("after the timeout: %s", got)
			}
		})
	}
}

// TestInterrupt is TestTimeout with Interrupt.
func TestInterrupt(t *testing.T) {
	slow := slowness(t)
	vm := newVM(t)
	evalString(t, vm, prepare)
	for _, s := range stuck {
		js := s.js
		t.Run(s.name, func(t *testing.T) {
			late := time.Hour
			for try := 0; try < 3 && late > grace*slow; try++ {
				var fired time.Time
				timer := time.AfterFunc(20*time.Millisecond, func() {
					fired = time.Now()
					vm.Interrupt()
				})
				_, err := vm.Eval(quiet(js), quickjs.EvalGlobal)
				end := time.Now()
				timer.Stop()
				wantUncatchable(t, err, true)
				late = min(late, end.Sub(fired))
				if raceEnabled {
					break
				}
			}
			if late > grace*slow && !raceEnabled {
				t.Errorf("ended %v after the interrupt", late)
			}
			if got := evalString(t, vm, `new URL("http://h/a").pathname`); got != "/a" {
				t.Errorf("after the interrupt: %s", got)
			}
		})
	}
}

// TestOutOfMemory checks that each stuck script, given a little room, ends with
// the out of memory error and leaves the VM good: what a call of Go would take
// is checked against the room before it reads its strings, and what the
// loops of javascript make counts as any object of the VM does.
func TestOutOfMemory(t *testing.T) {
	vm := newVM(t)
	evalString(t, vm, prepare)
	for _, s := range stuck {
		t.Run(s.name, func(t *testing.T) {
			keep := "var keep = []; " + s.hungry
			vm.RunGC()
			vm.SetMemoryLimit(vm.MemorySize() + 8<<20)
			_, err := vm.Eval(quiet(keep), quickjs.EvalGlobal)
			vm.SetMemoryLimit(^uintptr(0))
			wantUncatchable(t, err, false)
			vm.RunGC()
			if got := evalString(t, vm, `new URL("/a", "http://h/x").href`); got != "http://h/a" {
				t.Errorf("after running out of memory: %s", got)
			}
		})
	}
}

// TestNoRoomNoRead checks that a call that has not the room it needs ends
// with the out of memory error before Go reads its strings.
func TestNoRoomNoRead(t *testing.T) {
	vm := newVM(t)
	evalString(t, vm, prepare)
	for _, js := range []string{`new URL(path)`, `URL.canParse("x", path)`, `new URL("http://h/").pathname = wide`, `new URLSearchParams("a=%" + wide)`, `new URLSearchParams([["a", wide]]).toString()`} {
		vm.RunGC()
		vm.SetMemoryLimit(vm.MemorySize() + 1<<20)
		grown, room, err := measureCalls(t, vm, js)
		vm.SetMemoryLimit(^uintptr(0))
		if !isOOM(err) {
			t.Errorf("%s with 1 MiB of room: %v, want the out of memory error", js, err)
			continue
		}
		last := len(grown) - 1
		if room[last] < 1<<20 || grown[last] > 4<<10 {
			t.Errorf("%s: the last call needed %d bytes and allocated %d", js, room[last], grown[last])
		}
	}
}

// TestCanParseCountsFixedAllocations checks the fixed part of the room, the
// record of the URL and what is not in the length of the input: a call with
// no room left for it ends with the out of memory error, an empty string
// too.
func TestCanParseCountsFixedAllocations(t *testing.T) {
	vm := newVM(t)
	for _, js := range []string{`URL.canParse("")`, `URL.canParse("a:")`, `URL.canParse("http://h/")`, `new URL("http://h/")`} {
		vm.RunGC()
		vm.SetMemoryLimit(vm.MemorySize() + 1000)
		_, err := vm.Eval(js, quickjs.EvalGlobal)
		vm.SetMemoryLimit(^uintptr(0))
		if !isOOM(err) {
			t.Errorf("%s with 1,000 bytes of room: err = %v, want the out of memory error", js, err)
		}
	}
}

// TestAtTheLimit checks the limits as a script sees them, at the limit and one
// unit past: an input or a base of MaxUnits UTF-16 units, counting the spaces,
// tabs and newlines the parser removes (the seventh finding of the third
// review), a host that is not ASCII of MaxDomainUnits units after
// percent-decoding, whatever IDNA would map it to (the sixth), and a URL
// whose serialization is MaxUnits.
func TestAtTheLimit(t *testing.T) {
	vm := newVM(t)
	evalString(t, vm, "var C = "+strconv.Itoa(whatwgurl.MaxUnits)+", H = "+strconv.Itoa(whatwgurl.MaxDomainUnits)+"; 'ok'")
	for js, want := range map[string]string{
		`new URL(" ".repeat(C - 9) + "http://h/").href`:                               "http://h/",
		`URL.canParse(" ".repeat(C - 8) + "http://h/")`:                               "false",
		`URL.canParse("http://h/\t\n".repeat(C / 11))`:                                "true",
		`URL.canParse("http://h/\t\n".repeat(C / 11) + "\t".repeat(C % 11 + 1))`:      "false",
		`new URL("x", "http://h/" + "a".repeat(C - 9)).href`:                          "http://h/x",
		`URL.canParse("x", "http://h/" + "a".repeat(C - 8))`:                          "false",
		`new URL("http://a" + "­".repeat(H - 5) + ".com/").href`:                      "http://a.com/",
		`URL.canParse("http://a" + "­".repeat(H - 4) + ".com/")`:                      "false",
		`new URL("http://" + "０".repeat(H - 1) + "１/").href`:                          "http://0.0.0.1/",
		`URL.canParse("http://" + "０".repeat(H) + "１/")`:                              "false",
		`new URL("http://" + "a".repeat(C - 8) + "/").host.length`:                    strconv.Itoa(whatwgurl.MaxUnits - 8),
		`new URL("http://h/" + "é".repeat((C - 9) / 6)).href.length`:                  strconv.Itoa(9 + (whatwgurl.MaxUnits-9)/6*6),
		`URL.canParse("http://h/" + "é".repeat((C - 9) / 6 + 1))`:                     "false",
		`var u = new URL("http://h/"); u.pathname = "a".repeat(C - 9); u.href.length`: strconv.Itoa(whatwgurl.MaxUnits),
		`var u = new URL("http://h/"); u.pathname = "a".repeat(C - 8); u.href`:        "http://h/",
	} {
		if got := evalString(t, vm, "String("+jsEval(js)+")"); got != want {
			t.Errorf("%s = %s, want %s", js, got, want)
		}
	}
}

// jsEval is javascript that evaluates js, statements and all.
func jsEval(js string) string { return "eval(" + strconv.Quote(js) + ")" }

// TestOwnOutputGoesBack checks that what the package made can always be given
// back to it, as nothing it makes is longer than the limit: a URL at the limit
// can still be changed, its href set to itself, its search read into a list
// and a list it holds changed (the third and fourth findings of the third
// review). A change that would make it too long leaves the URL and its list
// as they were.
func TestOwnOutputGoesBack(t *testing.T) {
	vm := newVM(t)
	evalString(t, vm, "var C = "+strconv.Itoa(whatwgurl.MaxUnits)+"; 'ok'")
	for js, want := range map[string]string{
		// A URL at the limit, made from 7,280 CJK characters.
		`var u = new URL("http://h/" + "一".repeat(7280)); u.href = u.href; u.pathname = u.pathname; u.href.length`:    strconv.Itoa(9 + 7280*9),
		`var u = new URL("http://h/" + "a".repeat(C - 9)); u.hash = "x"; u.href.length`:                               strconv.Itoa(whatwgurl.MaxUnits),
		`var u = new URL("http://h/" + "a".repeat(C - 9)); u.pathname = "/b"; u.href`:                                 "http://h/b",
		`var u = new URL("http://h/?" + "a=" + "%41".repeat((C - 12) / 3)); u.searchParams.get("a").length`:           strconv.Itoa((whatwgurl.MaxUnits - 12) / 3),
		`var u = new URL("http://h/?" + "a=" + "%41".repeat((C - 12) / 3)); u.search = u.search; u.searchParams.size`: "1",
		`var u = new URL("http://h/?" + "a=b&".repeat((C - 10) / 4)); u.searchParams.sort(); u.searchParams.size`:     strconv.Itoa((whatwgurl.MaxUnits - 10) / 4),
		`new URLSearchParams(new URL("http://h/?" + "a".repeat(C - 10)).search).keys().next().value.length`:           strconv.Itoa(whatwgurl.MaxUnits - 10),
		// Too long: an error, and nothing changes.
		`var u = new URL("http://h/?a=" + "b".repeat(C - 12)), sp = u.searchParams; var e; try { sp.append("c", "d") } catch (x) { e = x.name } e + " " + sp.size + " " + u.href.length`: "RangeError 1 " + strconv.Itoa(whatwgurl.MaxUnits),
		`var u = new URL("http://h/?a=" + "b".repeat(C - 12)), sp = u.searchParams; var e; try { sp.set("a", "一".repeat(8000)) } catch (x) { e = x.name } e + " " + sp.get("a").length`:  "RangeError " + strconv.Itoa(whatwgurl.MaxUnits-12),
		`var u = new URL("http://h/?b=1&a=" + "b".repeat(C - 16)), sp = u.searchParams; sp.sort(); sp.toString().slice(0, 4)`:                                                            "a=bb",
		`var u = new URL("http://h/?a=1&a=2"), sp = u.searchParams; sp.delete("a"); u.href`:                                                                                              "http://h/",
	} {
		if got := evalString(t, vm, "String("+jsEval(js)+")"); got != want {
			t.Errorf("%s = %s, want %s", js, got, want)
		}
	}
}

// URLs are objects of the VM: made without end, they run the VM out of memory,
// and Go's heap does not grow with them.
func TestManyURLsCountedInVM(t *testing.T) {
	vm := newVM(t)
	vm.SetMemoryLimit(64 << 20)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := vm.Eval(`
		var keep = [];
		for (let i = 0; ; i++) {
			const u = new URL("/products/" + i + "?page=" + i + "#h", "http://localhost");
			u.searchParams.get("page");
			keep.push(u);
		}`, quickjs.EvalGlobal)
	runtime.GC()
	runtime.ReadMemStats(&after)
	wantUncatchable(t, err, false)
	if grown := int64(after.HeapAlloc) - int64(before.HeapAlloc); grown > 16<<20 {
		t.Errorf("Go's heap grew by %d MiB for URLs held by the VM", grown>>20)
	}
}

// TestNoPackageState reads the source: no variable of the package outside a
// function holds what a script gave, or anything that changes. A cache of
// parsed bases, in Go, would belong to no VM's memory limit and outlive its
// VM. What stays is made once and the same for every VM (the Builtins, the
// compiled script, the errors they answer, the hooks of the tests).
func TestNoPackageState(t *testing.T) {
	allowed := map[string]bool{
		"parseBuiltin": true, "canParseBuiltin": true, "setBuiltin": true, "formDecodeBuiltin": true, "formEncodeBuiltin": true,
		"bridge": true, "errTooLong": true, "errNotParts": true, "admitHook": true, "callHook": true, "shim": true, "script": true, "_": true,
	}
	var vars []string
	for _, f := range parseFiles(t, ".") {
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.VAR {
				continue
			}
			for _, s := range g.Specs {
				for _, n := range s.(*ast.ValueSpec).Names {
					if !allowed[n.Name] {
						vars = append(vars, n.Name)
					}
				}
			}
		}
	}
	sort.Strings(vars)
	if len(vars) > 0 {
		t.Errorf("package-level variables that hold state: %v", vars)
	}
}

// TestManyBasesLeaveNothing checks a script that parses many different bases
// leaves nothing in Go.
func TestManyBasesLeaveNothing(t *testing.T) {
	vm := newVM(t)
	var before, after runtime.MemStats
	run := func() {
		evalString(t, vm, `(function () {
			for (var i = 0; i < 4000; i++) {
				URL.canParse("x", "http://h" + i + "/" + "\u0001".repeat(900) + "z");
			}
			return 1;
		})() + ""`)
	}
	run()
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < 3; i++ {
		run()
	}
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&after)
	if grown := int64(after.HeapAlloc) - int64(before.HeapAlloc); grown > 256<<10 {
		t.Errorf("Go's heap grew by %d KiB", grown>>10)
	}
}

// TestSequenceConvertsFirst checks that the pairs of a sequence are all
// converted before the length of any is checked, so a later element that
// throws, or that has a getter, comes first.
func TestSequenceConvertsFirst(t *testing.T) {
	vm := newVM(t)
	got := evalString(t, vm, `(function () {
		function* source() { yield ["one"]; throw new RangeError("conversion continued"); }
		try { new URLSearchParams(source()); return "no error"; } catch (e) { return e.name; }
	})()`)
	if got != "RangeError" {
		t.Errorf("got %q, want RangeError", got)
	}
	got = evalString(t, vm, `(function () {
		var order = "";
		var second = {get 0() { order += "second;"; return "k"; }, get 1() { order += "v;"; return "v"; }, length: 2, [Symbol.iterator]: Array.prototype[Symbol.iterator]};
		try { new URLSearchParams([["one"], second]); } catch (e) { order += e.name; }
		return order;
	})()`)
	if got != "second;v;TypeError" {
		t.Errorf("order = %q, want second;v;TypeError", got)
	}
}

// TestStopAtNextCall checks that no call of a Builtin starts after Interrupt:
// the engine's polls count bytecode and not the time of Go, so a loop around a
// call of a few milliseconds would make dozens of calls before its next poll,
// and the look at the stop flag as each call starts ends the script at the next
// one. The stop is asked inside the fifth call, so the count does not depend
// on the clock.
func TestStopAtNextCall(t *testing.T) {
	vm := newVM(t)
	evalString(t, vm, prepare)
	for _, s := range stuck {
		t.Run(s.name, func(t *testing.T) {
			calls, after := 0, -1
			callHook = func(enter bool) {
				if !enter {
					return
				}
				calls++
				if calls == 5 {
					vm.Interrupt()
					after = 0
				} else if after >= 0 {
					after++
				}
			}
			defer func() { callHook = nil }()
			// A script that calls Go less than five times, which the polls stop,
			// ends at the timeout.
			vm.SetEvalTimeout(time.Second)
			_, err := vm.Eval(quiet(s.js), quickjs.EvalGlobal)
			vm.SetEvalTimeout(0)
			callHook = nil
			if calls < 5 {
				wantUncatchable(t, err, true)
				return
			}
			wantUncatchable(t, err, true)
			if after != 0 {
				t.Errorf("%d calls started after the interrupt", after)
			}
		})
	}
}
