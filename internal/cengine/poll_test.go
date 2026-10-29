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
)

func needPolls(t *testing.T) {
	t.Helper()
	if !polls {
		t.Skip("the engine was generated without polls (-poll)")
	}
}

// The scripts of the poll tests: tight loops of the interpreter and of the
// C, each with what it gives. forms has the loops of the interpreter that
// jump back in other ways (do while, continue to a label, a catch, a finally
// block, with), and straight calls a function of 3000 lines with no jump,
// where the compiler puts in OP_nop to count.
var tight = []struct{ name, src, want string }{
	{"loop", `var s = 0; for (var i = 0; i < 3e6; i++) s += i; s`, "4499998500000"},
	{"fib", `function fib(n){return n<2?n:fib(n-1)+fib(n-2)} fib(22)`, "17711"},
	{"forms", `var t = 0; var i = 0; do { t += i } while (++i < 2e5); l: for (var j = 0; j < 1000; j++) { for (var k = 0; k < 100; k++) { if (k == 50) continue l; t++ } } for (var q = 0; q < 2e4; q++) { try { if (q & 1) throw q; t += 1 } catch (e) { t += 2 } finally { t += 3 } } var o = {v: 1}; with (o) { for (var w = 0; w < 1e5; w++) t += v } t`, "20000140000"},
	{"straight", `var f = new Function("x", Array.from({length: 3000}, (_, i) => "x = (x + " + i + ") % 1000003;").join("\n") + " return x"); var t = 0; for (var i = 0; i < 300; i++) t = f(t); t`, "545953"},
	{"typed", `var a = new Int32Array(1 << 22); a[a.length - 1] = 1; a.indexOf(1) + ":" + a.lastIndexOf(0) + ":" + a.includes(2)`, "4194303:4194302:false"},
	{"fill", `var a = new Uint8Array(1 << 23); a.fill(7, 1); a.copyWithin(0, 1); a.copyWithin(2, 0); a[0] + a[a.length - 1] + ":" + a.length`, "14:8388608"},
	{"regexp", `/(a+)+b/.test('a'.repeat(20)) + ":" + "ab12cd34".repeat(1e4).replace(/\d+/g, "#").length`, "false:60000"},
	{"search", `var s = 'a'.repeat(1 << 22); s.indexOf('ab') + ":" + (s + 'b').lastIndexOf('ab')`, "-1:4194303"},
	{"repeat", `'ab'.repeat(1 << 21).length + 'c'.repeat(1 << 22).length`, "8388608"},
	{"sort", `var a = []; for (var i = 0; i < 1 << 17; i++) a.push((i * 7919) % 1000003); a.sort((x, y) => x - y); a[5] + ":" + a[a.length - 1]`, "38:1000000"},
	{"tasort", `var a = new Float64Array(1 << 18); for (var i = 0; i < a.length; i++) a[i] = (i * 7919) % 1000003; a.sort(); a[5] + ":" + a[a.length - 1]`, "23:1000000"},
	{"json", `var j = JSON.stringify(Array.from({length: 1 << 17}, (_, i) => ({i: i}))); JSON.parse(j).length + ":" + j.length`, "131072:1592827"},
}

// TestPollEveryCycle runs the scripts with every cycle polling (an interval
// of 1) and with the default interval: a poll keeps everything the generated
// code holds, so the results are the same, and the polls are many.
func TestPollEveryCycle(t *testing.T) {
	needPolls(t)
	scripts := append([]struct{ name, src, want string }{}, tight...)
	scripts = append(scripts, heavy...)
	for _, c := range scripts {
		var polls [2]int64
		for i, interval := range []int64{0, 1} {
			v := newVM(t, Options{Seed: 1})
			if interval != 0 {
				v.Host().SetPollInterval(interval)
			}
			got, err := v.Eval(c.src, c.name+".js")
			if err != nil || got != c.want {
				t.Errorf("%s, interval %d: %q, %v, want %q", c.name, interval, got, err, c.want)
			}
			polls[i] = v.Host().Counts().Polls
			closeVM(t, v)
		}
		if polls[1] <= polls[0] || polls[1] < 1000 {
			t.Errorf("%s: %d polls with an interval of 1, %d with the default", c.name, polls[1], polls[0])
		}
	}
}

// TestGCInTightLoops runs the tight loops while another goroutine keeps
// starting collections. Each collection waits for the engine's goroutine to
// reach a poll or a function prologue: no wait comes near what one of these
// loops takes without polls, and the results do not change.
func TestGCInTightLoops(t *testing.T) {
	needPolls(t)
	stop := make(chan struct{})
	done := make(chan struct{})
	var longest time.Duration
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				t0 := time.Now()
				runtime.GC()
				longest = max(longest, time.Since(t0))
			}
		}
	}()
	for _, c := range tight {
		v := newVM(t, Options{Seed: 1})
		got, err := v.Eval(c.src, c.name+".js")
		if err != nil || got != c.want {
			t.Errorf("%s with collections going on: %q, %v, want %q", c.name, got, err, c.want)
		}
		closeVM(t, v)
	}
	close(stop)
	<-done
	if longest > 500*time.Millisecond {
		t.Errorf("a collection took %v", longest)
	}
}

// interruptRunning runs src in v and calls Interrupt from another goroutine
// once the engine has polled 10 times in it on top of the polls of a full
// collection (gc), the polls of the RunGC the caller ran after its setup.
// JS_RunGC leaves the threshold of the next collection where it was, so the
// first allocation of the Eval may start one more, which takes about as many
// polls, before src runs; an Interrupt in it stops the script before it
// starts. A poll comes every 10000 ticks and src compiles in far fewer, so by
// then src runs, and the entry into the engine, which clears an Interrupt
// that came before it, is behind. It returns what Eval returned and how long
// after Interrupt, or -1 when Eval returned first.
func interruptRunning(v *VM, src string, gc int64) (string, error, time.Duration) {
	type result struct {
		got string
		err error
	}
	done := make(chan result, 1)
	before := v.Host().Polls()
	go func() {
		got, err := v.Eval(src, "stop.js")
		done <- result{got, err}
	}()
	for v.Host().Polls()-before < gc+10 {
		select {
		case r := <-done:
			return r.got, r.err, -1
		default:
			time.Sleep(20 * time.Microsecond)
		}
	}
	at := time.Now()
	v.Interrupt()
	r := <-done
	return r.got, r.err, time.Since(at)
}

// runGC runs a full collection in v and returns its polls.
func runGC(v *VM) int64 {
	before := v.Host().Polls()
	v.RunGC()
	return v.Host().Polls() - before
}

// The scripts Interrupt stops where the engine reads its interrupt flag.
var endless = []struct{ name, setup, src string }{
	{"loop", "", "for (;;) {}"},
	{"finally", "", "l: do { for (;;) { try { continue l } finally { } } } while (1)"},
	{"catch", "", "for (;;) { try { throw 1 } catch (e) {} }"},
	{"straight", "var f = new Function(Array.from({length: 3000}, (_, i) => 'var a' + i + ' = ' + i + ';').join('\\n'))", "for (;;) f()"},
	{"regexp", "", "/(a+)+b/.test('a'.repeat(60))"},
	{"sort", "var a = []; for (var i = 0; i < 1 << 18; i++) a.push((i * 7919) % 1000003)", "for (;;) a.slice().sort((x, y) => x - y)"},
	{"search", "var s = 'a'.repeat(1 << 24)", "for (;;) s.indexOf('ab')"},
	{"typed", "var ta = new Int32Array(1 << 24)", "for (;;) ta.indexOf(1)"},
	{"json", "var j = '[' + Array(1 << 20).fill(1).join() + ']'", "for (;;) JSON.parse(j)"},
	{"repeat", "", "for (;;) 'ab'.repeat(1 << 24)"},
	{"tasort", "var f = new Float64Array(1 << 23); for (var i = 0; i < f.length; i++) f[i] = (i * 7919) % 1000003", "for (;;) f.sort()"},
}

// stopsSoftly checks that Interrupt stopped src with the error "interrupted"
// soon after it, before gohost.KillPolls polls saw the request, with the kill
// grace given, and that the VM runs code again.
func stopsSoftly(t *testing.T, name, setup, src string, grace time.Duration) {
	t.Helper()
	v := newVM(t, Options{Seed: 1})
	defer closeVM(t, v)
	v.Host().SetKillGrace(grace)
	if setup != "" {
		if _, err := v.Eval(setup, "setup.js"); err != nil {
			t.Fatalf("%s: setup: %v", name, err)
		}
	}
	_, err, _ := interruptRunning(v, src, runGC(v))
	var je *JSError
	if !errors.As(err, &je) || je.Message != "InternalError: interrupted" || !je.Uncatchable || !strings.Contains(je.Stack, "stop.js") {
		t.Errorf("%s: Eval gave %#v, want the error interrupted", name, err)
		return
	}
	if n := v.Host().StopPolls(); n > gohost.KillPolls {
		t.Errorf("%s: stopped %d polls after Interrupt, want at most %d", name, n, gohost.KillPolls)
	}
	// The next call runs, and its exceptions are caught again.
	if got, err := v.Eval(`[1, 2, 3].map(x => x * 2).join() + (() => { try { null.x } catch (e) { return " caught" } })()`, "after.js"); err != nil || got != "2,4,6 caught" {
		t.Errorf("%s: after the interrupt: %q, %v", name, got, err)
	}
}

// TestInterrupt: Interrupt stops each endless script with the error
// "interrupted", which the engine throws where it reads the flag the next
// poll set, and the VM goes on.
func TestInterrupt(t *testing.T) {
	needPolls(t)
	for _, c := range endless {
		stopsSoftly(t, c.name, c.setup, c.src, -1)
	}
}

// TestInterruptOneProc: with one P, the goroutine that calls Interrupt runs
// only when the engine's goroutine stops at a poll, as the scheduler asks it
// to after its time slice.
func TestInterruptOneProc(t *testing.T) {
	needPolls(t)
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	for _, c := range endless[:3] {
		stopsSoftly(t, c.name, c.setup, c.src, -1)
	}
}

// TestKill: a script still running when the grace after Interrupt is over,
// and gohost.KillPolls polls after the one that set the flag, is killed at
// the poll. Eval returns ErrKilled, the VM takes only Close, the goroutine has
// its setting of SetPanicOnFault back, and a new VM runs. A grace of 0 kills
// once the polls have come, and only then.
// Array.prototype.lastIndexOf never reads the flag, so it runs until the
// kill. Interrupt comes once the call runs (see interruptRunning), since the
// interpreter would read a flag set before it calls lastIndexOf and stop the
// script there. The call must outlast the Interrupt and the grace on any
// machine, so it goes over an array-like object of 2^31-1 indices with none
// of them set: the generic steps of the specification, one index at a time,
// take a minute or more, where a dense Array of 8 Mi elements took 200 ms on
// the Linux machine of the first measurements and ended before the kill on a
// Mac.
// (slice, which this test used before, copies a dense Array in one go and
// ends in 25 ms.)
func TestKill(t *testing.T) {
	needPolls(t)
	for _, c := range []struct {
		name, setup, src string
		grace            time.Duration
	}{
		{"lastIndexOf", "var a = { length: 2 ** 31 - 1 }", "Array.prototype.lastIndexOf.call(a, -1)", 30 * time.Millisecond},
		{"lastIndexOf, a grace of 0", "var a = { length: 2 ** 31 - 1 }", "Array.prototype.lastIndexOf.call(a, -1)", 0},
	} {
		v := newVM(t, Options{Seed: 1})
		v.Host().SetKillGrace(c.grace)
		if c.setup != "" {
			if _, err := v.Eval(c.setup, "setup.js"); err != nil {
				t.Fatalf("%s: setup: %v", c.name, err)
			}
		}
		_, err, after := interruptRunning(v, c.src, runGC(v))
		if !errors.Is(err, ErrKilled) {
			t.Errorf("%s: Eval gave %v, want ErrKilled", c.name, err)
		}
		if after < c.grace {
			t.Errorf("%s: killed %v after Interrupt, before the grace of %v was over", c.name, after, c.grace)
		}
		if n := v.Host().StopPolls(); n < gohost.KillPolls+1 || c.grace == 0 && n != gohost.KillPolls+1 {
			t.Errorf("%s: killed at poll %d after Interrupt, with a grace of %v", c.name, n, c.grace)
		}
		if !v.Host().Killed() {
			t.Errorf("%s: the host does not say killed", c.name)
		}
		brokenAfter(t, v, c.name+" killed")
	}
}

// TestNoKillBeforeThePolls: with a grace of 0, Interrupt still stops each
// endless script with the error "interrupted" and kills none: each reads the
// flag, or frees what it made, well within gohost.KillPolls polls of it being
// set.
func TestNoKillBeforeThePolls(t *testing.T) {
	needPolls(t)
	for _, c := range endless {
		stopsSoftly(t, c.name, c.setup, c.src, 0)
	}
}

// TestTimeout: a script that runs past the timeout of the VM stops with the
// error "interrupted", as after Interrupt, and each Eval has the whole
// timeout. One that never reads the flag is killed once the grace after the
// deadline is over. The call that never reads it must outlast the timeout and
// the grace on any machine, so it is the one TestKill uses: lastIndexOf over
// an array-like object of 2^31-1 indices, which takes a minute or more. A
// dense Array of 8 Mi elements took 77 ms on an AMD EPYC 9V45 and ended before
// the kill.
func TestTimeout(t *testing.T) {
	needPolls(t)
	v := newVM(t, Options{Seed: 1})
	defer closeVM(t, v)
	v.Host().SetKillGrace(-1)
	v.SetTimeout(50 * time.Millisecond)
	for i := range 2 {
		t0 := time.Now()
		_, err := v.Eval("for (;;) {}", "stop.js")
		var je *JSError
		if !errors.As(err, &je) || je.Message != "InternalError: interrupted" || !je.Uncatchable {
			t.Fatalf("round %d: Eval gave %#v, want the error interrupted", i, err)
		}
		if d := time.Since(t0); d < 50*time.Millisecond || d > 500*time.Millisecond {
			t.Errorf("round %d: stopped after %v, with a timeout of 50 ms", i, d)
		}
	}
	if got, err := v.Eval("1 + 1", "after.js"); err != nil || got != "2" {
		t.Fatalf("after the timeouts: %q, %v", got, err)
	}

	w := newVM(t, Options{Seed: 1})
	w.Host().SetKillGrace(30 * time.Millisecond)
	if _, err := w.Eval("var a = { length: 2 ** 31 - 1 }", "setup.js"); err != nil {
		t.Fatal(err)
	}
	w.RunGC()
	w.SetTimeout(50 * time.Millisecond)
	t0 := time.Now()
	if _, err := w.Eval("Array.prototype.lastIndexOf.call(a, -1)", "lastindexof.js"); !errors.Is(err, ErrKilled) {
		t.Errorf("an endless lastIndexOf: Eval gave %v, want ErrKilled", err)
	}
	if d := time.Since(t0); d < 80*time.Millisecond || d > 2*time.Second {
		t.Errorf("killed after %v, with a timeout of 50 ms and a grace of 30 ms", d)
	}
	brokenAfter(t, w, "killed after its timeout")
}

// TestNoKillOutsideScripts: an Interrupt while the VM collects, which runs
// no script, sets the flag and kills nothing, even with a grace of 0 and a
// poll on every cycle. The next Eval starts afresh.
func TestNoKillOutsideScripts(t *testing.T) {
	needPolls(t)
	v := newVM(t, Options{Seed: 1})
	defer closeVM(t, v)
	if _, err := v.Eval("globalThis.a = Array.from({ length: 300000 }, (_, i) => ({ i, s: 'x' + i }))", "setup.js"); err != nil {
		t.Fatal(err)
	}
	v.Host().SetKillGrace(0)
	v.Host().SetPollInterval(1)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				v.Interrupt()
			}
		}
	}()
	before := v.Host().Counts().Polls
	for range 3 {
		v.RunGC()
	}
	interrupted := v.Host().Interrupted()
	v.Usage()
	close(stop)
	<-done
	if polls := v.Host().Counts().Polls - before; polls == 0 {
		t.Fatal("the collections did not poll")
	}
	if !interrupted {
		t.Fatal("no Interrupt came during the collections")
	}
	if v.Host().Broken() || v.Host().Killed() {
		t.Fatalf("a collection was killed: broken %v, killed %v", v.Host().Broken(), v.Host().Killed())
	}
	v.Host().SetKillGrace(gohost.DefaultKillGrace)
	if got, err := v.Eval("a.length", "after.js"); err != nil || got != "300000" {
		t.Fatalf("after the collections: %q, %v", got, err)
	}
}
