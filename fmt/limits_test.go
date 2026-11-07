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
	"testing"

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
		"", "abc", "\x00\x01\x7f\x80\xff\U0010ffff\u00e9\u2028 \"'\\`", obj, big1, big.NewInt(0),
		0.0, -1e308, 5e-324, math.NaN(), math.Inf(-1), 3.25, 0, -2147483648, 2147483647, 1114112, 7,
		true, nil, quickjs.Undefined{}, quickjs.Unsupported{},
	}
	verbs := "vsqxXdbocUeEfFgGtpTw%é"
	var formats []string
	for _, f := range []string{"%", "%#", "% ", "%+", "%-", "%0", "%# ", "%+#", "%-# 0"} {
		for _, w := range []string{"", "12", "*", "[2]", "[2]*", "*[1]", "[9]", "[0]", "[x]", "[", "99999999"} {
			for _, p := range []string{"", ".", ".3", ".*", ".[3]*", ".40", ".99999999"} {
				for _, v := range verbs {
					formats = append(formats, f+w+p+string(v))
				}
			}
		}
	}
	formats = append(formats, "", "%", "%!", "%[1]", "%.", "%*", "plain", "%v %v", "%[2]v %v %[1]v", "%[3]*.[2]*[1]f", "%v%v%v%v")
	tuples := [][]any{nil}
	for _, a := range args {
		tuples = append(tuples, []any{a}, []any{a, 7, a}, []any{3, a})
	}
	tuples = append(tuples, args)
	for _, format := range formats {
		for _, a := range tuples {
			if got, bound := len(fmt.Sprintf(format, a...)), sizePrintf(format, a); int64(got) > bound {
				t.Fatalf("sizePrintf(%q, %#v) is %d, under the output of %d bytes", format, a, bound, got)
			}
		}
	}
	for _, a := range tuples {
		if got, bound := max(len(fmt.Sprint(a...)), len(fmt.Sprintln(a...))), sizePrint(a); int64(got) > bound {
			t.Fatalf("sizePrint(%#v) is %d, under the output of %d bytes", a, bound, got)
		}
	}
}
