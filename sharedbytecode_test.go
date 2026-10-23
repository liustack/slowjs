// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

// sharedPad is the size of the comment in big, so that its source is far
// larger than anything else a VM of sharedScript holds.
const sharedPad = 256 << 10

// sharedScript has functions of every kind, nested ones among them, one that
// throws two calls down and one that makes a stack, and big, whose source is
// sharedPad bytes of comment.
var sharedScript = `function outer(a, b) {
	// a comment the source keeps
	const inner = (x) => x + a;
	class Point {
		constructor(x) { this.x = x }
		get double() { return this.x * 2 }
		static make() { return new Point(1) }
	}
	return { inner, Point, method() { return b }, async run() { await 0; return a } };
}
function thrower() {
	const f = () => { throw new Error('deep') };
	return f();
}
function stackHere() { return new Error('here').stack }
function big() { /*` + strings.Repeat("x", sharedPad) + `*/ return 1 }
globalThis.shared = { outer, thrower, stackHere, big, o: outer(1, 2) };
`

// sharedProbe is what a VM of sharedScript shows of its functions: their
// text, a stack thrown two calls down and a stack made in a function.
const sharedProbe = `JSON.stringify([
	String(shared.outer), String(shared.o.inner), String(shared.o.Point),
	String(shared.o.method), String(shared.o.run), String(shared.o.Point.make),
	String(Object.getOwnPropertyDescriptor(shared.o.Point.prototype, 'double').get),
	String(shared.thrower), String(shared.big),
	(() => { try { shared.thrower() } catch (e) { return e.stack } })(),
	shared.stackHere(),
	new Function('return 1').toString(),
])`

func compileShared(t testing.TB) []byte {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	code, err := m.Compile(sharedScript, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func newSharedBytecode(t testing.TB, code []byte) *SharedBytecode {
	t.Helper()
	b, err := NewSharedBytecode(code)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A VM that loads shared bytecode shows the same source text, stacks and line
// numbers as one that loads a copy, and holds less memory by at least the
// source of big.
func TestSharedBytecodeSameAsCopy(t *testing.T) {
	code := compileShared(t)
	shared := newSharedBytecode(t, code)
	defer shared.Close()

	copied := newTestVM(t)
	if _, err := copied.EvalBytecode(code); err != nil {
		t.Fatal(err)
	}
	m := newTestVM(t)
	if _, err := m.EvalSharedBytecode(shared); err != nil {
		t.Fatal(err)
	}

	want := evalString(t, copied, sharedProbe)
	if got := evalString(t, m, sharedProbe); got != want {
		t.Fatalf("shared bytecode shows\n%s\nwant\n%s", got, want)
	}
	for _, s := range []string{"a comment the source keeps", "static make() { return new Point(1) }", "at f (<eval>:12:", "at thrower (<eval>:13:", "at stackHere (<eval>:15:", strings.Repeat("x", sharedPad)} {
		if !strings.Contains(want, s) {
			t.Errorf("the probe has no %q", s)
		}
	}

	copied.RunGC()
	m.RunGC()
	if saved := int(copied.MemorySize()) - int(m.MemorySize()); saved < sharedPad {
		t.Errorf("a VM of shared bytecode holds %d bytes less than one of a copy, want at least %d", saved, sharedPad)
	}
}

// The bytes stay while a VM that loaded them is open, after Close of the
// SharedBytecode, and go with the last of them. Loading after Close fails.
func TestSharedBytecodeLifetime(t *testing.T) {
	code := compileShared(t)
	shared := newSharedBytecode(t, code)
	if got := shared.Len(); got != len(code) {
		t.Fatalf("Len %d, want %d", got, len(code))
	}
	want := evalString(t, newTestVMWith(t, code), sharedProbe)

	var vms []*VM
	for range 2 {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.EvalSharedBytecode(shared); err != nil {
			t.Fatal(err)
		}
		vms = append(vms, m)
	}
	if err := shared.Close(); err != nil {
		t.Fatal(err)
	}
	if err := shared.Close(); err != nil {
		t.Fatalf("a second Close: %v", err)
	}
	if shared.p == 0 {
		t.Fatal("Close freed the bytes while VMs that loaded them are open")
	}
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.EvalSharedBytecode(shared); err == nil {
		t.Error("EvalSharedBytecode after Close did not fail")
	}
	m.Close()

	for i, m := range vms {
		if got := evalString(t, m, sharedProbe); got != want {
			t.Errorf("VM %d after Close of the bytecode shows\n%s\nwant\n%s", i, got, want)
		}
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if shared.p != 0 || shared.refs != 0 {
		t.Fatalf("after the last VM closed: p %#x, refs %d", shared.p, shared.refs)
	}
}

func newTestVMWith(t *testing.T, code []byte) *VM {
	t.Helper()
	m := newTestVM(t)
	if _, err := m.EvalBytecode(code); err != nil {
		t.Fatal(err)
	}
	return m
}

// Bytecode that ends inside a source text fails as a copy of it fails.
func TestSharedBytecodeTruncated(t *testing.T) {
	code := compileShared(t)
	at := bytes.Index(code, []byte(strings.Repeat("x", 64)))
	if at < 0 {
		t.Fatal("no source of big in the bytecode")
	}
	short := code[:at+sharedPad/2]

	copied := newTestVM(t)
	_, want := copied.EvalBytecode(short)
	if want == nil {
		t.Fatal("a copy of truncated bytecode loaded")
	}
	shared := newSharedBytecode(t, short)
	defer shared.Close()
	m := newTestVM(t)
	_, err := m.EvalSharedBytecode(shared)
	if err == nil || err.Error() != want.Error() {
		t.Fatalf("got %v, want %v", err, want)
	}
}

// VMs on several goroutines load and read the same bytes at once.
func TestSharedBytecodeConcurrent(t *testing.T) {
	code := compileShared(t)
	shared := newSharedBytecode(t, code)
	want := evalString(t, newTestVMWith(t, code), sharedProbe)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 4 {
				m, err := NewVM()
				if err != nil {
					t.Error(err)
					return
				}
				if _, err := m.EvalSharedBytecode(shared); err != nil {
					t.Error(err)
				} else if v, err := m.Eval(sharedProbe, EvalGlobal); err != nil || v != want {
					t.Errorf("got %v, %v", v, err)
				}
				m.Close()
			}
		})
	}
	wg.Wait()
	if err := shared.Close(); err != nil {
		t.Fatal(err)
	}
	if shared.p != 0 {
		t.Fatal("the bytes stayed after the last VM and Close")
	}
}
