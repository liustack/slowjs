// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"testing"
	"time"
)

// The eval timeout stops the move of a large block within a few polls: the
// heap copies a block that moves a MiB at a time with a read of the
// interrupt flag before each step, and a stop leaves the old block where it
// was, with its content, and ends the call with the interrupt, not with the
// out of memory error of a failed resize, and the views made before the call
// follow the block of a buffer that is kept. The bytes an ArrayBuffer gains are
// cleared a MiB at a time the same way. A resize to a quarter moves the block
// out of its chunk on every system, and a resize back moves it again where
// the host cannot remap a chunk (darwin, windows). Before, the copy of 64 MiB
// and the clear of 192 MiB ran in one go, for hundreds of polls past the
// request.
func TestArrayBufferMoveStops(t *testing.T) {
	const setup = `var n = 1 << 28, b = new ArrayBuffer(n, {maxByteLength: n}); new Uint8Array(b).fill(7); ` +
		`var vb = b, fv = new Uint8Array(b, 0, 16), tv = new Uint8Array(b), dv = new DataView(b)`
	// what each stop must leave: the buffer at one of its two lengths, with
	// the bytes it keeps through both, and the views made before: they follow
	// the block of a buffer that is kept, or have nothing when a transfer
	// detached it
	const after = `[b.byteLength === n || b.byteLength === n >> 2, new Uint8Array(b)[0], new Uint8Array(b)[(n >> 2) - 1], b.detached, ` +
		`vb === b ? tv.length === b.byteLength && dv.byteLength === b.byteLength && fv.length === 16 && fv[0] === 7 && ` +
		`tv[(n >> 2) - 1] === 7 && dv.getUint8(2) === 7 && (tv[1] = 9, new Uint8Array(b)[1] === 9) && b.maxByteLength === n : ` +
		`vb.detached && tv.length === 0 && fv.length === 0].join()`
	const want = "true,7,7,false,true"
	for _, c := range []struct{ name, call string }{
		{"resize", `(b.resize(n >> 2), b.resize(n))`},
		{"transfer", `(b = b.transfer(n >> 2), b = b.transfer(n))`},
	} {
		t.Run(c.name, func(t *testing.T) {
			vm := func() *VM {
				m, err := NewVM()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := m.Eval(setup+`; 0`, EvalGlobal); err != nil {
					m.Close()
					t.Fatal(err)
				}
				return m
			}
			// one call, then a stop at each sixth of it
			m := vm()
			start := time.Now()
			_, err := m.Eval(c.call+`; 0`, EvalGlobal)
			call := time.Since(start)
			if err == nil {
				var v any
				v, err = m.Eval(after, EvalGlobal)
				if err == nil && v != want {
					t.Fatalf("after one call: got %v, want %s", v, want)
				}
			}
			m.Close()
			if err != nil {
				t.Fatal(err)
			}
			for k := 1; k <= 5; k++ {
				m := vm()
				if err := m.SetEvalTimeout(max(call*time.Duration(k)/6, time.Millisecond)); err != nil {
					t.Fatal(err)
				}
				start := time.Now()
				_, err := m.Eval(`for (;;) `+c.call, EvalGlobal)
				took := time.Since(start)
				polls := m.h.StopPolls()
				if errors.Is(err, ErrKilled) {
					m.Close()
					t.Fatalf("killed after %v of a call of %v, want the interrupt", took, call)
				}
				var e *Error
				if !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
					m.Close()
					t.Fatalf("got %v after %v, want interrupted", err, took)
				}
				if polls > copyStopPolls {
					t.Errorf("%d polls saw the request to stop, want at most %d (stopped %v after the start, in a call of %v)", polls, copyStopPolls, took, call)
				}
				t.Logf("stopped after %v of a call of %v, %d polls saw the request", took, call, polls)
				if err := m.SetEvalTimeout(0); err != nil {
					t.Fatal(err)
				}
				if v, err := m.Eval(after, EvalGlobal); err != nil || v != want {
					m.Close()
					t.Fatalf("the buffer after the stop: got %v, %v, want %s", v, err, want)
				}
				if _, err := m.Eval(c.call+`; 0`, EvalGlobal); err != nil {
					m.Close()
					t.Fatalf("a call after the stop: %v", err)
				}
				m.Close()
			}
		})
	}
}
