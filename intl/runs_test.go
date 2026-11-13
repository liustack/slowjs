// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/stopsweep"
	"github.com/liustack/quickjs/internal/vmhook"
)

// uncuttable are calls whose input has one piece no Builtin can cut into
// smaller ones, about N code points long, set up by the first string and
// made by the second: a run of a script a dictionary divides as a whole,
// and a run of N/4 accents put in order by combining class.
var uncuttable = []struct{ name, setup, call string }{
	{"a run of Thai",
		`var s = 'สวัสดีครับ'.repeat(N / 10), g = new Intl.Segmenter('th', {granularity: 'word'})`,
		`g.segment(s).containing(0).segment`},
	{"a run of Khmer",
		`var s = 'ភាសាខ្មែរ'.repeat(N / 9), g = new Intl.Segmenter('km', {granularity: 'word'})`,
		`g.segment(s).containing(0).segment`},
	{"a run of Chinese",
		`var s = '我们今天去北京大学看书'.repeat(N / 11), g = new Intl.Segmenter('zh', {granularity: 'word'})`,
		`g.segment(s).containing(0).segment`},
	{"a run of Japanese with katakana",
		`var s = '東京タワーに行きました'.repeat(N / 11), g = new Intl.Segmenter('ja', {granularity: 'word'})`,
		`g.segment(s).containing(0).segment`},
	{"a run of halfwidth katakana, which NFKC changes",
		`var s = 'ｶﾀｶﾅﾃｽﾄ'.repeat(N / 7), g = new Intl.Segmenter('ja', {granularity: 'word'})`,
		`g.segment(s).containing(0).segment`},
	{"a Thai word and a long run of a mark",
		`var s = 'ภาษา' + 'ั'.repeat(N), g = new Intl.Segmenter('th', {granularity: 'word'})`,
		`g.segment(s).containing(0).segment`},
	{"a Thai word and a long run of what is no word",
		`var s = 'ภาษา' + 'ๆ'.repeat(N), g = new Intl.Segmenter('th', {granularity: 'word'})`,
		`g.segment(s).containing(0).segment`},
	{"a Han character and a long run of halfwidth voiced marks",
		`var s = '中' + 'ﾞ'.repeat(N), g = new Intl.Segmenter('ja', {granularity: 'word'})`,
		`g.segment(s).containing(0).segment`},
	{"a run of accents of two classes",
		`var a = 'a' + '̖́'.repeat(N / 8) + '̀', b = 'b' + '̖́'.repeat(N / 8) + '̂', c = new Intl.Collator('en')`,
		`c.compare(a, b)`},
}

// The work on one long piece reads vmhook.Stop at least once per
// workPerStop units of it. Before, a run of Thai, Chinese or accents was
// worked on whole with no check.
func TestUncuttableRunsCheck(t *testing.T) {
	const n = 1 << 20
	for _, c := range uncuttable {
		t.Run(c.name, func(t *testing.T) {
			vm := newVM(t)
			setUp(t, vm, c.setup, n)
			s, err := measureStops(t, vm, c.call)
			if err != nil {
				t.Fatalf("%s: %v", c.call, err)
			}
			if s.stops < n/workPerStop {
				t.Errorf("%s read vmhook.Stop %d times for %d code points, want at least %d", c.call, s.stops, n, n/workPerStop)
			}
		})
	}
}

// The eval timeout ends the work on one long piece within the grace of the
// kill after it, and the VM goes on. The timeout comes after the text is
// read, in the work on the piece: with no check there, each call ran
// 0.5 to 14.5 s on an Apple M4, and ended with no error.
func TestUncuttableRunsTimeout(t *testing.T) {
	const (
		n       = 8 << 20
		timeout = 100 * time.Millisecond
		grace   = 50 * time.Millisecond
	)
	for _, c := range uncuttable {
		t.Run(c.name, func(t *testing.T) {
			vm := newVM(t)
			setUp(t, vm, c.setup, n)
			if err := vm.SetEvalTimeout(timeout); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			_, err := vm.Eval(c.call, quickjs.EvalGlobal)
			took := time.Since(start)
			if errors.Is(err, quickjs.ErrKilled) {
				t.Fatalf("killed after %v, want the interrupt", took)
			}
			var e *quickjs.Error
			if !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
				t.Fatalf("got %v after %v, want interrupted", err, took)
			}
			if took > timeout+grace {
				t.Errorf("stopped after %v, with a timeout of %v", took, timeout)
			}
			t.Logf("stopped after %v", took)
			if err := vm.SetEvalTimeout(0); err != nil {
				t.Fatal(err)
			}
			if v, err := vm.Eval(`1+1`, quickjs.EvalGlobal); err != nil || v != 2 {
				t.Fatalf("the VM after the stop: 1+1 got %v, %v", v, err)
			}
		})
	}
}

// A segmenter stopped in a long run finds the same segments when it is
// asked again: the stop drops its cursor, which the next call makes again.
func TestSegmenterAfterStop(t *testing.T) {
	vm := newVM(t)
	eval(t, vm, `var s = 'สวัสดีครับ'.repeat(800000) + ' end', segs = new Intl.Segmenter('th', {granularity: 'word'}).segment(s); 0`)
	if err := vm.SetEvalTimeout(20 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Eval(`segs.containing(0)`, quickjs.EvalGlobal); err == nil {
		t.Fatal("the first call was not stopped")
	}
	if err := vm.SetEvalTimeout(0); err != nil {
		t.Fatal(err)
	}
	got := eval(t, vm, `[segs.containing(0).segment, segs.containing(6).segment, segs.containing(s.length - 1).segment].join('|')`)
	if got != "สวัสดี|ครับ|end" {
		t.Errorf("segments after the stop: %v", got)
	}
}

// A long run of Chinese or Japanese takes Go memory in proportion to it
// until it is divided, and the call ends with the out of memory error when
// the VM has no room for that. Go takes about 150 bytes a code unit to
// divide a run of Chinese, 1.2 GiB for this one of 8.8 Mi, which went past
// the memory limit of 64 MiB with no error.
func TestCJKRunRoom(t *testing.T) {
	vm := newVM(t)
	eval(t, vm, `var s = '我们今天去北京大学看书'.repeat(800000), g = new Intl.Segmenter('zh', {granularity: 'word'}); g.segment('中文').containing(0); 0`)
	runtime.GC()
	vm.SetMemoryLimit(vm.MemorySize() + 64<<20)
	_, err := vm.Eval(`g.segment(s).containing(0)`, quickjs.EvalGlobal)
	if !isOutOfMemory(err) {
		t.Fatalf("got %v, want the out of memory error", err)
	}
	if got := eval(t, vm, `g.segment('我们今天去北京').containing(0).segment`); got != "我们" {
		t.Errorf("after the error: %v", got)
	}
}

// longRead are calls whose work on a long text, after reading it, is short:
// a Segments object reads its whole text before the first segment, and
// finds the first ones in the first piece, and a ListFormat reads its items
// and only the first characters of each.
var longRead = []struct{ name, setup, call string }{
	{"a list of one long item",
		`var l = ['x'.repeat(N), 'y'], f = new Intl.ListFormat('en')`,
		`f.format(l).length`},
	{"the parts of a list of one long item in Thai, with characters past the BMP",
		`var l = ['สวัสดี😀'.repeat(N / 8), 'y'], f = new Intl.ListFormat('th')`,
		`f.formatToParts(l).length`},
	{"the first word of a long text",
		`var s = 'hello world '.repeat(N / 12), g = new Intl.Segmenter('en', {granularity: 'word'})`,
		`g.segment(s).containing(0).segment`},
	{"the first grapheme of a long text in Thai",
		`var s = 'สวัสดี ครับ '.repeat(N / 11), g = new Intl.Segmenter('th')`,
		`g.segment(s)[Symbol.iterator]().next().value.segment`},
}

// Reading the text of a Segments object reads vmhook.Stop at least once per
// workPerStop units of it, as the work on it does. Before, it was read as
// WTF-8 and turned back into UTF-16 in one pass, with no check, which held
// the eval timeout 19 to 40 ms on an Apple M4 for a text of 8 Mi units, and
// three to five times that on the Intel Macs of GitHub.
func TestLongReadStops(t *testing.T) {
	const n = 2 << 20
	for _, c := range longRead {
		t.Run(c.name, func(t *testing.T) {
			vm := newVM(t)
			setUp(t, vm, c.setup, n)
			s, err := measureStops(t, vm, c.call)
			if err != nil {
				t.Fatalf("%s: %v", c.call, err)
			}
			if s.stops < n/workPerStop {
				t.Errorf("%s read vmhook.Stop %d times for %d units, want at least %d", c.call, s.stops, n, n/workPerStop)
			}
		})
	}
}

// An Interrupt that comes while the text is read ends the call with the
// interrupt at the next read of vmhook.Stop, and the call returns at once.
func TestLongReadInterrupt(t *testing.T) {
	const n = 2 << 20
	const at = n / workPerStop / 2
	for _, c := range longRead {
		t.Run(c.name, func(t *testing.T) {
			vm := newVM(t)
			setUp(t, vm, c.setup, n)
			stops := 0
			stop := vmhook.Stop
			vmhook.Stop = func(v any) bool {
				if stops++; stops == at {
					vm.Interrupt()
				}
				return stop(v)
			}
			defer func() { vmhook.Stop = stop }()
			_, err := vm.Eval(c.call, quickjs.EvalGlobal)
			var e *quickjs.Error
			if !errors.As(err, &e) || !e.Interrupted {
				t.Fatalf("%s, interrupted at read %d of vmhook.Stop: got %v after %d reads, want the interrupt", c.call, at, err, stops)
			}
			if stops != at {
				t.Errorf("%s read vmhook.Stop %d times, and the interrupt came at read %d", c.call, stops, at)
			}
		})
	}
}

// Each long call stops within the grace of stopsweep at every eval timeout
// from 2 ms to the time it takes whole, so that a part of it with no check
// shows wherever it lies.
func TestLongCallsStopAtEveryTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("times each call at many timeouts")
	}
	const n = 8 << 20
	for _, c := range longRead {
		t.Run(c.name, func(t *testing.T) {
			vm := newVM(t)
			setUp(t, vm, c.setup, n)
			stopsweep.Sweep(t, vm, c.call, n, workPerStop)
		})
	}
	for _, c := range swept {
		t.Run(c.name, func(t *testing.T) {
			vm := newVM(t)
			setUp(t, vm, c.setup, c.n)
			stopsweep.Sweep(t, vm, c.call, c.n, workPerStop)
		})
	}
}

// swept are more long calls TestLongCallsStopAtEveryTimeout sweeps, each with
// an input of n units that keeps the sweep short. With no check, a case
// mapping of Greek of 8 Mi units ran 0.75 s past a timeout of 25 ms on an
// Apple M4.
var swept = []struct {
	name, setup, call string
	n                 int
}{
	{"toLocaleUpperCase in Turkish",
		`var s = 'istanbul '.repeat(N / 9)`, `s.toLocaleUpperCase('tr').length`, 8 << 20},
	{"toLocaleLowerCase in Turkish of I and marks",
		`var s = 'I' + '̣'.repeat(N - 1)`, `s.toLocaleLowerCase('tr').length`, 2 << 20},
	{"toLocaleUpperCase in Lithuanian",
		`var s = 'i̇'.repeat(N / 2)`, `s.toLocaleUpperCase('lt').length`, 8 << 20},
	{"toLocaleLowerCase in Lithuanian",
		`var s = 'ÌÍĨ '.repeat(N / 4)`, `s.toLocaleLowerCase('lt').length`, 2 << 20},
	{"toLocaleUpperCase in Greek",
		`var s = 'άέή'.repeat(N / 3)`, `s.toLocaleUpperCase('el').length`, 1 << 20},
}

// A dropped cursor is not kept: no slot of the list holds it after.
func TestCursorDropClears(t *testing.T) {
	v := &vmCursors{next: 1}
	a, b := &cursor{id: 1}, &cursor{id: 2}
	v.add(a)
	v.add(b)
	v.drop(a)
	v.drop(b)
	for i, c := range v.list[:cap(v.list)] {
		if c != nil {
			t.Errorf("slot %d holds cursor %d", i, c.id)
		}
	}
}
