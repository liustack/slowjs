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
	"strings"
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
// to 16 MiB, and printf wrote them to stdout. A call whose budget fits runs
// as before, and the budget can be far above the output.
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

// What the output of a call makes Go's fmt allocate, counted whole, as none
// of it may be collected before the call returns, is at most its budget, for
// empty and short outputs under a few GOMAXPROCS, and for long outputs of
// each kind: the buffer grown by append, from nothing and from what each
// length makes it, and by padding, the verbs that make their bytes in a
// slice of their own, quoting a string and writing a float, and the digits
// of big ints. "%[1]s" 33 times of 64 KiB made Go allocate 6.8 times the
// bound, and "%[1]q" of control characters up to 10.9 times, more than the
// 6 times they were checked against while the test made the collector run
// often and measured what the heap held at once. What Go allocates to put
// the pool of printers in the runtime's list of pools, which the budget
// does not count, is small here, as the test program has few pools.
func TestOutputAllocatedUnderBudget(t *testing.T) {
	s := strings.Repeat("\x01", 1<<20)
	b := new(big.Int).Lsh(big.NewInt(1), 1<<20)
	type call struct {
		format string
		a      []any
	}
	calls := []call{
		{"%1000000s", []any{""}},
		{strings.Repeat("%1000000[1]s", 4), []any{""}},
		{"%s%1000000s%s%999999s", []any{s, "", s, ""}},
		{"%[1]q%[1]q", []any{s}},
		{"%+q", []any{s}},
		{"%#q", []any{strings.Repeat("x", 1<<20)}},
		{"%# x", []any{s}},
		{"%.1000000f", []any{1.5}},
		{"%1000000.1000000e", []any{1.5}},
		{"%d %b %x %o %v", []any{b, b, b, b, b}},
		{"%1000000d %s", []any{b, b}},
		{"%1000w", []any{b}},
		{"%#[1]w%#[1]v", []any{s}},
		{strings.Repeat("%v", 1<<20), nil},
		{strings.Repeat("x", 1<<20) + "%1000000s", []any{s}},
		{"%1000000d%[1]*d", []any{1000000}},
		{"", nil},
		{"x", nil},
		{"%s", []any{"x"}},
	}
	// The lengths where the buffer grows by append or by padding, and
	// where Go rounds what it allocates up the most.
	for _, n := range []int{255, 256, 1000, 32 << 10, 32<<10 + 1, 40 << 10, 64 << 10, 100000, 1 << 20} {
		x := strings.Repeat("x", n)
		for _, k := range []int{1, 2, 3, 5, 9, 17, 33, 65} {
			if n*k > 8<<20 {
				break
			}
			calls = append(calls,
				call{strings.Repeat("%[1]s", k), []any{x}},
				call{strings.Repeat("%[1]q", k), []any{s[:n]}},
				call{"%s" + strings.Repeat("%[2]*[1]s", k), []any{x, n + 1}},
				call{strings.Repeat("%[1]s%[2]*[3]s", k), []any{x, n / 4, ""}},
			)
		}
	}
	// A short output makes Go's fmt allocate its printer, and the slots of
	// its pool, which grow with GOMAXPROCS.
	for _, procs := range []int{1, 2, 8, 64} {
		old := runtime.GOMAXPROCS(procs)
		for _, c := range []call{{"", nil}, {"x", nil}, {"%d", []any{1}}} {
			bound, read, _ := sizePrintf(c.format, c.a, -1)
			if alloc := allocatedDuring(func() { _ = fmt.Sprintf(read, c.a...) }); alloc > budget(bound, 0) {
				t.Errorf("%q with GOMAXPROCS %d: Go allocated %d bytes, more than the %d of the bound of %d", c.format, procs, alloc, budget(bound, 0), bound)
			}
		}
		runtime.GOMAXPROCS(old)
	}
	most := 0.0
	for _, c := range calls {
		bound, read, ok := sizePrintf(c.format, c.a, -1)
		if !ok {
			t.Fatal(c.format)
		}
		var out string
		alloc := allocatedDuring(func() { out = fmt.Sprintf(read, c.a...) })
		if bound >= 4<<10 {
			most = max(most, float64(alloc)/float64(bound))
		}
		if alloc > budget(bound, 0) {
			t.Errorf("%.40q: Go allocated %d bytes for %d, more than the %d of the bound of %d", c.format, alloc, len(out), budget(bound, 0), bound)
		}
	}
	t.Logf("on outputs of 4 KiB or more, Go allocated at most %.1f times the bound", most)
	for _, a := range [][]any{{s, s, b}, {s, 1.5, "x"}} {
		bound := sizePrint(a)
		for _, f := range []func(...any) string{fmt.Sprint, fmt.Sprintln} {
			if alloc := allocatedDuring(func() { _ = f(a...) }); alloc > budget(bound, 0) {
				t.Errorf("Go allocated %d bytes, more than the %d of the bound of %d", alloc, budget(bound, 0), bound)
			}
		}
	}
}

// allocatedDuring runs f and returns the bytes Go allocated while it ran,
// with nothing in the pool of fmt's printers, as after a collection.
func allocatedDuring(f func()) int64 {
	runtime.GC()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return int64(after.TotalAlloc - before.TotalAlloc)
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

// %T writes the name of the type of its argument, and %p the address an
// object or a big int is at, so neither is longer for a longer argument: in
// a VM with 2 MiB left under its memory limit, "%T" of a string of 1 MiB
// writes "string", where the bound counted the string and ended the call.
// Their bound is their width and precision and a few bytes, and what they
// write is in it.
func TestTypeAndPointer(t *testing.T) {
	vm := newVM(t)
	if _, err := vm.Eval(`var s = "x".repeat(1 << 20), o = {a: s}, b = 1n << 1048000n; 1`, quickjs.EvalGlobal); err != nil {
		t.Fatal(err)
	}
	vm.SetMemoryLimit(vm.MemorySize() + 2<<20)
	for _, c := range []struct{ js, want string }{
		{`fmt.sprintf("%T", s)`, "string"},
		{`fmt.sprintf("%12T|%-12.3T|", s, b)`, "      string|*bi         |"},
		{`fmt.sprintf("%T %T", o, b)`, "*quickjs.Object *big.Int"},
		{`fmt.sprintf("%p", o).slice(0, 2)`, "0x"},
		{`fmt.sprintf("%#p", b).length < 20`, "true"},
		{`fmt.sprintf("%.1000000T", 0)`, "int"},
		{`fmt.sprintf("%.*T|%-*.*T|", 1000000, s, 6, 1000000, b)`, "string|*big.Int|"},
	} {
		got, err := vm.Eval(c.js, quickjs.EvalGlobal)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: got %v, %v, want %q", c.js, got, err, c.want)
		}
	}
	s := strings.Repeat("x", 1<<20)
	obj, err := vm.Eval(`({a: 1})`, quickjs.EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	b := new(big.Int).Lsh(big.NewInt(1), 1<<20)
	for _, c := range []struct {
		format string
		a      any
	}{
		{"%T", s}, {"%T", b}, {"%-20T", nil}, {"%T", quickjs.Undefined{}}, {"%p", b}, {"%#20.30p", b}, {"%p", obj}, {"%.1000000T", 1}, {"%20.1000000T", s},
	} {
		bound, _, _ := sizePrintf(c.format, []any{c.a}, -1)
		if out := fmt.Sprintf(c.format, c.a); int64(len(out)) > bound || bound > 256 {
			t.Errorf("%s of %T: a bound of %d for %d bytes", c.format, c.a, bound, len(out))
		}
	}
}

// A precision makes an output longer only where Go's fmt pads to it
// (precGrows). For every verb, with and without '#', and every kind of
// argument the VM gives, the bound of the verb with a precision of 1000000
// holds what it writes, and stays short when that is short: "%.1000000t"
// of true writes "true", and "%.1000000f" of NaN "NaN", where the bound
// counted the precision and a call with less than 16 MB left ended. In a
// VM with 2 MiB left under its memory
// limit, the short outputs are made, and the long ones end the call.
func TestPrecision(t *testing.T) {
	vm := newVM(t)
	obj, err := vm.Eval(`({a: "b"})`, quickjs.EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	args := []any{
		"ab", obj, big.NewInt(5), new(big.Int).Lsh(big.NewInt(1), 200), 1.5, 0.1, 5e-324,
		-math.MaxFloat64, math.NaN(), math.Inf(1), math.Inf(-1), 7, -7, true, nil, quickjs.Undefined{},
		quickjs.Unsupported{},
	}
	const short = 4 << 10
	for _, verbs := range []string{"ABCDEFGHIJKLMNOPQRSTUVWXYZ", "abcdefghijklmnopqrstuvwxyz"} {
		for _, verb := range verbs {
			for _, flags := range []string{"", "#", "+", "# "} {
				format := "%" + flags + ".1000000" + string(verb)
				for _, a := range args {
					out := fmt.Sprintf(format, a)
					bound, _, _ := sizePrintf(format, []any{a}, -1)
					if int64(len(out)) > bound {
						t.Errorf("%s of %T: a bound of %d under %d bytes", format, a, bound, len(out))
					}
					if len(out) < short && bound >= short {
						t.Errorf("%s of %T: a bound of %d for %d bytes", format, a, bound, len(out))
					}
				}
			}
		}
	}
	if _, err := vm.Eval(`var b = 5n, o = {a: "b"}; 1`, quickjs.EvalGlobal); err != nil {
		t.Fatal(err)
	}
	vm.SetMemoryLimit(vm.MemorySize() + 2<<20)
	for _, c := range []struct{ js, want string }{
		{`fmt.sprintf("%.1000000t", true)`, "true"},
		{`fmt.sprintf("%.1000000v", true)`, "true"},
		{`fmt.sprintf("%.1000000c", 65)`, "A"},
		{`fmt.sprintf("%.1000000q", 65)`, "'A'"},
		{`fmt.sprintf("%.1000000s|%.1000000q|%.1000000x", "ab", "ab", "ab")`, `ab|"ab"|6162`},
		{`fmt.sprintf("%.1000000v|%.1000000d", o, o)`, `{"a":"b"}|&{%!d(string={"a":"b"})}`},
		{`fmt.sprintf("%.1000000g|%.1000000v|%#.1000000v|%.1000000b", 1.5, 1.5, 1.5, 1.5)`, "1.5|1.5|1.5|6755399441055744p-52"},
		{`fmt.sprintf("%.1000000f|%.1000000e|%#.1000000g|%#.1000000x", NaN, Infinity, NaN, -Infinity)`, "NaN|+Inf|NaN|-Inf"},
		{`fmt.sprintf("%.1000000c|%.1000000t", b, null)`, "%!c(big.Int=5)|%!t(<nil>)"},
		{`fmt.sprintf("%.1000000v|%.1000000d", undefined, undefined)`, "undefined|{}"},
	} {
		got, err := vm.Eval(c.js, quickjs.EvalGlobal)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: got %v, %v, want %q", c.js, got, err, c.want)
		}
	}
	for _, js := range []string{
		`fmt.sprintf("%.1000000d", 1)`,
		`fmt.sprintf("%.1000000U", 65)`,
		`fmt.sprintf("%.1000000z", 1)`,
		`fmt.sprintf("%.1000000e", 1.5)`,
		`fmt.sprintf("%.1000000x", 1.5)`,
		`fmt.sprintf("%#.1000000g", 1.5)`,
		`fmt.sprintf("%#.1000000z", 1.5)`,
		`fmt.sprintf("%.1000000s", b)`,
		`fmt.sprintf("%.1000000p", o)`,
	} {
		_, err := vm.Eval(`var r = "ok"; try { `+js+` } catch (e) { r = "caught " + e } r`, quickjs.EvalGlobal)
		var e *quickjs.Error
		if !errors.As(err, &e) || !e.Uncatchable || !e.OutOfMemory {
			t.Errorf("%s: got %v, want the uncatchable out of memory error", js, err)
		}
	}
}
