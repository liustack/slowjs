// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package fmt // import "github.com/liustack/quickjs/fmt"

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/liustack/quickjs"
)

// small sets longArg, longBits and piece so that every string, every
// object and every big int of more than 64 bits is written a part at a
// time, in pieces of a few bytes, as a long one is.
func small(t testing.TB) {
	oldLong, oldBits, oldPiece := longArg, longBits, piece
	longArg, longBits, piece = 0, 64, 5
	t.Cleanup(func() { longArg, longBits, piece = oldLong, oldBits, oldPiece })
}

// asGo checks that the output of each function is that of Go's fmt.
func asGo(t testing.TB, format string, a []any) {
	t.Helper()
	n, read, ok := sizePrintf(format, a, -1)
	if !ok {
		t.Fatalf("sizePrintf(%q) does not fit in no limit", format)
	}
	if got, ok := sprintf(nil, n, read, a); !ok || got != fmt.Sprintf(format, a...) {
		t.Fatalf("sprintf(%q, %#v) is %q, want %q", format, a, got, fmt.Sprintf(format, a...))
	}
}

// The output of printf and sprintf is that of Go's fmt byte for byte, for
// formats with every part fmt reads and arguments of every type the VM
// gives a Go function, and so is that of println and sprint, with short
// arguments and with every string, object and big int of more than 64 bits
// written a part at a time (small).
func TestPrintfAsGo(t *testing.T) {
	formats, tuples := cases(t)
	for _, pieces := range []bool{false, true} {
		t.Run(fmt.Sprint("pieces=", pieces), func(t *testing.T) {
			if pieces {
				small(t)
			}
			for _, format := range formats {
				for _, a := range tuples {
					asGo(t, format, a)
				}
			}
			for _, a := range tuples {
				if got, ok := sprint(nil, sizePrint(a), a); !ok || got != fmt.Sprint(a...) {
					t.Fatalf("sprint(%#v) is %q, want %q", a, got, fmt.Sprint(a...))
				}
				if got, ok := sprintln(nil, sizePrint(a), a); !ok || got != fmt.Sprintln(a...) {
					t.Fatalf("sprintln(%#v) is %q, want %q", a, got, fmt.Sprintln(a...))
				}
			}
		})
	}
}

// The output of sprintf is that of Go's fmt for any format, string and
// integer, with the string, an object of it and a big int made of both,
// written a part at a time, in pieces of 5 bytes.
func FuzzPrintfAsGo(f *testing.F) {
	vm, err := quickjs.NewVM()
	if err != nil {
		f.Fatal(err)
	}
	defer vm.Close()
	if _, err := vm.Eval(`function obj(s) { return {s} }`, quickjs.EvalGlobal); err != nil {
		f.Fatal(err)
	}
	for _, seed := range []string{"%v", "%#q", "%+q", "% #x", "%-12.3s", "%*d", "%[2]v %[1]q", "%.[3]*v", "%#v", "%d", "%#w", "%5[", "%x %X %q"} {
		f.Add(seed, "a\x80é\U0010ffff`\"", 7)
	}
	f.Fuzz(func(t *testing.T, format, s string, n int) {
		if len(format) > 64 || len(s) > 256 {
			return
		}
		o, err := vm.Call("obj", s)
		if err != nil {
			t.Fatal(err)
		}
		b := new(big.Int).Lsh(big.NewInt(int64(n)), uint(len(s)*8))
		b.Add(b, new(big.Int).SetBytes([]byte(s)))
		a := []any{s, n, o, float64(n), s, b}
		small(t)
		asGo(t, format, a)
		if got, ok := sprint(nil, sizePrint(a), a); !ok || got != fmt.Sprint(a...) {
			t.Fatalf("sprint(%#v) is %q, want %q", a, got, fmt.Sprint(a...))
		}
		longArg, longBits, piece = 1<<16, 1<<16, 1<<16
		asGo(t, format, a)
	})
}
