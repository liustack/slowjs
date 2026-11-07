// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package fmt // import "github.com/liustack/quickjs/fmt"

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liustack/quickjs"
)

func newVM(t *testing.T) *quickjs.VM {
	t.Helper()
	vm, err := quickjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vm.Close() })
	g := vm.GlobalObject()
	defer g.Free()
	if err := Register(g.Dup()); err != nil {
		t.Fatal(err)
	}
	return vm
}

// A short format that asks for a long output, in a VM with a memory limit,
// ends the call with the uncatchable out of memory error before Go makes
// the output: "%1000000[1]s" 256 times asked Go for 256 MB in a VM limited
// to 16 MiB, and printf wrote them to stdout. A call whose output fits runs
// as before.
func TestOutputLimit(t *testing.T) {
	vm := newVM(t)
	vm.SetMemoryLimit(16 << 20)
	for _, js := range []string{
		`fmt.sprintf("%1000000[1]s".repeat(256), "")`,
		`fmt.printf("%1000000[1]s".repeat(256), "")`,
		`fmt.sprintf("%[1]*[2]s".repeat(256), 1000000, "")`,
		`fmt.sprintf("%.1000000[1]f".repeat(256), 1.5)`,
		`fmt.sprintf("%[1]q".repeat(16), "\x01".repeat(1 << 20))`,
		`fmt.sprint(...Array(64).fill("x".repeat(1 << 18)))`,
		`fmt.sprintf("%1000000w", 1n << 16384n)`,
		`fmt.sprintf("%#[1]w".repeat(8), "\x01".repeat(1 << 20))`,
		`fmt.sprintf("%#[1]w".repeat(8), JSON.parse('{"a": "' + "\\u0001".repeat(1 << 20) + '"}'))`,
	} {
		t.Run(js, func(t *testing.T) {
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			_, err := vm.Eval(`var r = "ok"; try { `+js+` } catch (e) { r = "caught " + e } r`, quickjs.EvalGlobal)
			runtime.ReadMemStats(&after)
			var e *quickjs.Error
			if !errors.As(err, &e) || !e.Uncatchable || !e.OutOfMemory {
				t.Fatalf("got %v, want the uncatchable out of memory error", err)
			}
			if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 64<<20 {
				t.Fatalf("Go allocated %d MiB in a VM limited to 16 MiB", alloc>>20)
			}
		})
	}
	if got, err := vm.Eval(`fmt.sprintf("%d %d %5.2f|%-4s|", 42, "x".repeat(1 << 20).length, Math.PI, "ab")`, quickjs.EvalGlobal); err != nil || got != "42 1048576  3.14|ab  |" {
		t.Fatalf("got %q, %v", got, err)
	}
}

// The bound on the output of each function is at least the output, for
// formats with every part fmt reads and arguments of every type the VM
// gives a Go function.
func TestSizeAtLeastOutput(t *testing.T) {
	vm := newVM(t)
	obj, err := vm.Eval(`({a: "\x01\u00ff\ud800", b: [1, 2.5, null]})`, quickjs.EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	big1, _ := new(big.Int).SetString("-123456789012345678901234567890123456789", 10)
	args := []any{
		"", "abc", "\x00\x01\x7f\x80\xff\U0010ffff\u00e9\u2028 \"'\\`", strings.Repeat("\x01", 100), obj, big1, big.NewInt(0),
		new(big.Int).Lsh(big.NewInt(1), 1000), new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(3), 2000)),
		0.0, -1e308, 5e-324, math.NaN(), math.Inf(-1), 3.25, 0, -2147483648, 2147483647, 1114112, 7,
		true, nil, quickjs.Undefined{}, quickjs.Unsupported{},
	}
	verbs := "vsqxXdbocUeEfFgGtpTw%é"
	var formats []string
	for _, f := range []string{"%", "%#", "% ", "%+", "%-", "%0", "%# ", "%+#", "%-# 0"} {
		for _, w := range []string{"", "12", "1000", "*", "[2]", "[2]*", "*[1]", "[9]", "[0]", "[x]", "[", "[x", "99999999"} {
			for _, p := range []string{"", ".", ".3", ".*", ".[3]*", ".40", ".99999999"} {
				for _, v := range verbs {
					formats = append(formats, f+w+p+string(v))
				}
			}
		}
	}
	formats = append(formats, "", "%", "%!", "%[1]", "%.", "%*", "plain", "%v %v", "%[2]v %v %[1]v", "%[3]*.[2]*[1]f", "%v%v%v%v",
		"%[", "%[x%[x%[x", "%[x]%[x", "%.[x%[3", "[%[", "%-[%[", "%[2]*[x", "%[1][%[", "%[x]x[%[2]v[", "%[%]", "%.[*[d", "%[1.[", "x]%[", "%[]%[")
	tuples := [][]any{nil}
	for _, a := range args {
		tuples = append(tuples, []any{a}, []any{a, 7, a}, []any{3, a})
	}
	tuples = append(tuples, args)
	for _, format := range formats {
		for _, a := range tuples {
			out := fmt.Sprintf(format, a...)
			bound, read, ok := sizePrintf(format, a, -1)
			if !ok {
				t.Fatalf("sizePrintf(%q) does not fit in no limit", format)
			}
			if int64(len(out)) > bound {
				t.Fatalf("sizePrintf(%q, %#v) is %d, under the output of %d bytes", format, a, bound, len(out))
			}
			if again := fmt.Sprintf(read, a...); again != out {
				t.Fatalf("sizePrintf(%q, %#v) reads it as %q, which makes %q, not %q", format, a, read, again, out)
			}
		}
	}
	for _, a := range tuples {
		if got, bound := max(len(fmt.Sprint(a...)), len(fmt.Sprintln(a...))), sizePrint(a); int64(got) > bound {
			t.Fatalf("sizePrint(%#v) is %d, under the output of %d bytes", a, bound, got)
		}
	}
}

// A format with many a '[' and no ']' after them makes its output in time
// linear in its length, where fmt takes time quadratic in it: "%[x" a
// million times took Go minutes.
func TestUnclosedIndexes(t *testing.T) {
	vm := newVM(t)
	vm.SetEvalTimeout(10 * time.Second)
	start := time.Now()
	got, err := vm.Eval(`fmt.sprintf("%[x".repeat(1000000))`, quickjs.EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Repeat("%!x(BADINDEX)", 1000000); got != want {
		t.Fatalf("got %d bytes, want %d of %%!x(BADINDEX)", len(fmt.Sprint(got)), len(want))
	}
	t.Logf("in %v", time.Since(start))
}

// A format with many a '[' and no ']' after them, in a VM that has less
// left under its memory limit than the format sizePrintf hands fmt, ends
// with the out of memory error before Go makes that format.
func TestUnclosedIndexesRoom(t *testing.T) {
	vm := newVM(t)
	if _, err := vm.Eval(`var f = "%[x".repeat(1 << 20); 1`, quickjs.EvalGlobal); err != nil {
		t.Fatal(err)
	}
	vm.SetMemoryLimit(vm.MemorySize() + 1<<20)
	_, err := vm.Eval(`var r = "ok"; try { fmt.sprintf(f) } catch (e) { r = "caught " + e } r`, quickjs.EvalGlobal)
	var e *quickjs.Error
	if !errors.As(err, &e) || !e.Uncatchable || !e.OutOfMemory {
		t.Fatalf("got %v, want the uncatchable out of memory error", err)
	}
	if _, _, ok := sizePrintf("%[x%[x", nil, 6); ok {
		t.Fatal("a format of 6 bytes that grows fits in 6 bytes")
	}
	if _, read, ok := sizePrintf("%[x%[x", nil, 64); !ok || read != "%[]x%[x" {
		t.Fatalf("got %q, %v", read, ok)
	}
}

// What Go holds at once to make the output of a call is at most held times
// the bound on its output: the most the heap held above what it held before,
// with the collector run often, for long outputs of each kind, the buffer
// grown by append and by padding, and the verbs that make their bytes in a
// slice of their own, quoting a string and writing a float.
func TestHeldUnderNeed(t *testing.T) {
	s := strings.Repeat("\x01", 1<<20)
	b := new(big.Int).Lsh(big.NewInt(1), 1<<20)
	for _, c := range []struct {
		format string
		a      []any
	}{
		{"%1000000s", []any{""}},
		{strings.Repeat("%1000000[1]s", 4), []any{""}},
		{"%s%1000000s%s%999999s", []any{s, "", s, ""}},
		{"%[1]q%[1]q", []any{s}},
		{"%+q", []any{s}},
		{"%# x", []any{s}},
		{"%.1000000f", []any{1.5}},
		{"%1000000.1000000e", []any{1.5}},
		{"%d %b %x", []any{b, b, b}},
		{"%1000w", []any{b}},
		{"%#[1]w%#[1]v", []any{s}},
		{strings.Repeat("%v", 1<<20), nil},
		{strings.Repeat("x", 1<<20) + "%1000000s", []any{s}},
	} {
		bound, read, ok := sizePrintf(c.format, c.a, -1)
		if !ok {
			t.Fatal(c.format)
		}
		var out string
		h := heldDuring(func() { out = fmt.Sprintf(read, c.a...) })
		t.Logf("%.40q: %d bytes, bound %d, held %d", c.format, len(out), bound, h)
		if h > held*bound {
			t.Fatalf("%.40q: Go held %d bytes, more than %d times the bound of %d", c.format, h, held, bound)
		}
	}
	for _, a := range [][]any{{s, s, b}, {s, 1.5, "x"}} {
		bound := sizePrint(a)
		h := heldDuring(func() { _ = fmt.Sprintln(a...) })
		if h > held*bound {
			t.Fatalf("sprintln: Go held %d bytes, more than %d times the bound of %d", h, held, bound)
		}
	}
}

// heldDuring runs f and returns the most bytes the heap held at once while
// it ran, above what it held before, with GOGC at 10.
func heldDuring(f func()) int64 {
	defer debug.SetGCPercent(debug.SetGCPercent(10))
	runtime.GC()
	objects := func() int64 {
		s := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
		metrics.Read(s)
		return int64(s[0].Value.Uint64())
	}
	base := objects()
	var peak atomic.Int64
	done := make(chan struct{})
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		for {
			if h := objects() - base; h > peak.Load() {
				peak.Store(h)
			}
			select {
			case <-done:
				return
			case <-time.After(50 * time.Microsecond):
			}
		}
	}()
	f()
	close(done)
	<-sampled
	return max(peak.Load(), objects()-base)
}

// Every format of up to 6 bytes of "%[]1x*." makes the same output as the
// format sizePrintf hands fmt, under its bound, with no argument, one int,
// and a string and an int, and that format grows by no more than sizePrintf
// makes room for.
func TestShortFormats(t *testing.T) {
	const alphabet = "%[]1x*."
	tuples := [][]any{nil, {1}, {"ab", 2}}
	var walk func(format string)
	walk = func(format string) {
		for _, a := range tuples {
			out := fmt.Sprintf(format, a...)
			bound, read, ok := sizePrintf(format, a, -1)
			if !ok {
				t.Fatalf("sizePrintf(%q) does not fit in no limit", format)
			}
			if int64(len(out)) > bound {
				t.Fatalf("sizePrintf(%q, %#v) is %d, under the output of %d bytes", format, a, bound, len(out))
			}
			if again := fmt.Sprintf(read, a...); again != out {
				t.Fatalf("sizePrintf(%q, %#v) reads it as %q, which makes %q, not %q", format, a, read, again, out)
			}
			if read != format {
				first := 0
				for read[first] == format[first] {
					first++
				}
				if len(read) > len(format)+(len(format)-first+1)/2+2 {
					t.Fatalf("sizePrintf(%q) grows it to %q", format, read)
				}
			}
		}
		if len(format) < 6 {
			for i := range len(alphabet) {
				walk(format + alphabet[i:i+1])
			}
		}
	}
	walk("")
}
