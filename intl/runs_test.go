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
