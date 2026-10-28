// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine

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

// interruptAfter runs src in v and calls Interrupt from another goroutine
// after d. It returns what Eval returned and how long after Interrupt.
func interruptAfter(v *VM, src string, d time.Duration) (string, error, time.Duration) {
	at := make(chan time.Time, 1)
	timer := time.AfterFunc(d, func() {
		at <- time.Now()
		v.Interrupt()
	})
	defer timer.Stop()
	got, err := v.Eval(src, "stop.js")
	end := time.Now()
	select {
	case t := <-at:
		return got, err, end.Sub(t)
	default:
		return got, err, -1
	}
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
// soon after it, and that the VM runs code again.
func stopsSoftly(t *testing.T, name, setup, src string) {
	t.Helper()
	v := newVM(t, Options{Seed: 1})
	defer closeVM(t, v)
	v.Host().SetKillGrace(-1)
	if setup != "" {
		if _, err := v.Eval(setup, "setup.js"); err != nil {
			t.Fatalf("%s: setup: %v", name, err)
		}
	}
	_, err, after := interruptAfter(v, src, 20*time.Millisecond)
	var je *JSError
	if !errors.As(err, &je) || je.Message != "InternalError: interrupted" || !je.Uncatchable || !strings.Contains(je.Stack, "stop.js") {
		t.Errorf("%s: Eval gave %#v, want the error interrupted", name, err)
		return
	}
	if after < 0 || after > 200*time.Millisecond {
		t.Errorf("%s: stopped %v after Interrupt", name, after)
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
		stopsSoftly(t, c.name, c.setup, c.src)
	}
}

// TestInterruptOneProc: with one P, the goroutine that calls Interrupt runs
// only when the engine's goroutine stops at a poll, as the scheduler asks it
// to after its time slice.
func TestInterruptOneProc(t *testing.T) {
	needPolls(t)
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	for _, c := range endless[:3] {
		stopsSoftly(t, c.name, c.setup, c.src)
	}
}

// TestKill: a script still running when the grace after Interrupt is over is
// killed at the next poll. Eval returns ErrKilled, the VM takes only Close,
// the goroutine has its setting of SetPanicOnFault back, and a new VM runs.
// A grace of 0 kills at the first poll, before the loop reads the flag.
// Array.prototype.lastIndexOf never reads the flag, so it runs until the
// kill: the collection after the setup keeps one from starting while Eval
// compiles, and Interrupt comes once the call runs, since the interpreter
// reads the flag when it calls the script. The call must outlast the
// Interrupt and the grace. Over a dense Array it takes the generic steps of
// the specification, one index at a time: 8 Mi elements take about 200 ms
// on the machine of the measurements, and the Interrupt comes once Eval has
// compiled the call, also under an emulator. (slice, which this test used,
// copies a dense Array in one go and ends in 25 ms.)
func TestKill(t *testing.T) {
	needPolls(t)
	for _, c := range []struct {
		name, setup, src string
		at, grace        time.Duration
	}{
		{"loop", "", "for (;;) {}", 20 * time.Millisecond, 0},
		{"lastIndexOf", "var a = Array(1 << 23).fill(1.5)", "a.lastIndexOf(-1)", 100 * time.Millisecond, 30 * time.Millisecond},
	} {
		v := newVM(t, Options{Seed: 1})
		v.Host().SetKillGrace(c.grace)
		if c.setup != "" {
			if _, err := v.Eval(c.setup, "setup.js"); err != nil {
				t.Fatalf("%s: setup: %v", c.name, err)
			}
			v.RunGC()
		}
		_, err, after := interruptAfter(v, c.src, c.at)
		if !errors.Is(err, ErrKilled) {
			t.Errorf("%s: Eval gave %v, want ErrKilled", c.name, err)
		}
		if after < c.grace || after > c.grace+200*time.Millisecond {
			t.Errorf("%s: killed %v after Interrupt, with a grace of %v", c.name, after, c.grace)
		}
		if !v.Host().Killed() {
			t.Errorf("%s: the host does not say killed", c.name)
		}
		brokenAfter(t, v, c.name+" killed")
	}
}

// TestTimeout: a script that runs past the timeout of the VM stops with the
// error "interrupted", as after Interrupt, and each Eval has the whole
// timeout. One that never reads the flag is killed once the grace after the
// deadline is over.
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
	if _, err := w.Eval("var a = Array(1 << 23).fill(1.5)", "setup.js"); err != nil {
		t.Fatal(err)
	}
	w.RunGC()
	w.SetTimeout(50 * time.Millisecond)
	t0 := time.Now()
	if _, err := w.Eval("for (;;) a.lastIndexOf(-1)", "lastindexof.js"); !errors.Is(err, ErrKilled) {
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
