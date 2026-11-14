// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func newEvalFileVM(t *testing.T) *VM {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

// evalFileStack runs src under name and returns the stack of the error it
// throws.
func evalFileStack(t *testing.T, m *VM, name, src string) string {
	t.Helper()
	v, err := m.EvalFile(name, src, EvalGlobal)
	if err == nil {
		v.Free()
		t.Fatalf("EvalFile(%q) threw nothing", name)
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("EvalFile(%q): %v", name, err)
	}
	return e.Stack
}

// TestEvalFile: the name is the file of the stack frames of the script and of
// its syntax errors, and the result is the script's value.
func TestEvalFile(t *testing.T) {
	m := newEvalFileVM(t)
	v, err := m.EvalFile("repro.js", "1 + 1", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := v.Any(); err != nil || r != 2 {
		t.Fatalf("got %v, %v", r, err)
	}
	v.Free()

	stack := evalFileStack(t, m, "repro.js", "function f() { throw new Error('x') }\nf()")
	if !strings.Contains(stack, "at f (repro.js:1:") || !strings.Contains(stack, "(repro.js:2:") || strings.Contains(stack, "<eval>:") {
		t.Fatalf("stack %q", stack)
	}
	stack = evalFileStack(t, m, "repro.js", "1;\nvar = ;")
	if !strings.Contains(stack, "repro.js:2:") {
		t.Fatalf("syntax error stack %q", stack)
	}
	// No name is <eval>, as Eval names it.
	stack = evalFileStack(t, m, "", "throw new Error('x')")
	if !strings.Contains(stack, "(<eval>:1:") {
		t.Fatalf("stack %q", stack)
	}
	// Eval still names it <eval>.
	_, err = m.Eval("function g() { throw new Error('x') }\ng()", EvalGlobal)
	var e *Error
	if !errors.As(err, &e) || !strings.Contains(e.Stack, "at g (<eval>:1:") {
		t.Fatalf("Eval: %v", err)
	}
}

// TestEvalFileName: a name keeps one line of the stack per frame and is
// valid UTF-8 of at most maxEvalName bytes, whatever is given.
func TestEvalFileName(t *testing.T) {
	m := newEvalFileVM(t)
	long := strings.Repeat("é", 600) // 1200 bytes
	for _, c := range []struct{ name, want string }{
		{"dir/日本.js", "dir/日本.js"},
		{"a\nb", "a�b"},
		{"a\rb", "a�b"},
		{"a\x00b", "a�b"},
		{"a\tb", "a�b"},
		{"a\x7fb", "a�b"},
		{"a b c", "a�b�c"},
		{"\xffa\xc3", "�a�"},
		{long, strings.Repeat("é", maxEvalName/2)},
		{strings.Repeat("a", maxEvalName-1) + "é", strings.Repeat("a", maxEvalName-1)},
		{strings.Repeat("a", 1<<20), strings.Repeat("a", maxEvalName)},
	} {
		got := cleanEvalName(c.name)
		if got != c.want {
			t.Errorf("cleanEvalName(%.40q) = %.40q, want %.40q", c.name, got, c.want)
			continue
		}
		if len(got) > maxEvalName || !utf8.ValidString(got) {
			t.Errorf("cleanEvalName(%.40q): %d bytes, valid UTF-8 %v", c.name, len(got), utf8.ValidString(got))
		}
		stack := evalFileStack(t, m, c.name, "throw new Error('x')")
		if strings.Count(stack, "\n") != 1 || !strings.Contains(stack, "("+c.want+":1:") {
			t.Errorf("name %.40q: stack %.200q", c.name, stack)
		}
	}
}

// TestEvalFileNameBound: evalSource takes n = len(name)+len(src)+2 bytes, the
// name already cut to maxEvalName, and writes through a slice of exactly n
// bytes, so neither a long name nor a long source writes past what tmp gave,
// on the shadow stack or in the heap. The bytes after the n are left as they
// were.
func TestEvalFileNameBound(t *testing.T) {
	m := newEvalFileVM(t)
	name := cleanEvalName(strings.Repeat("n", 1<<20))
	if len(name) != maxEvalName {
		t.Fatalf("name of %d bytes", len(name))
	}
	for _, size := range []int{0, 100, tmpMax - maxEvalName - 2, tmpMax - maxEvalName - 1, tmpMax} {
		src := strings.Repeat(" ", size)
		n := len(name) + len(src) + 2
		// The shadow stack after m.sp, where a source of up to tmpMax
		// bytes goes, filled with a mark past the n bytes.
		after := bytesAt(m.sp, uintptr(min(n, tmpMax)+64))
		for i := range after {
			after[i] = 0xaa
		}
		var ok bool
		m.outer(func() { ok = checkEvalSource(t, m, name, src, after) })
		if !ok {
			return
		}
	}

	// A source the memory limit has no room for ends with out of memory
	// before anything is written, and the VM goes on.
	m.SetMemoryLimit(1 << 20)
	_, err := m.EvalFile(strings.Repeat("n", 1<<20), "//"+strings.Repeat("x", 4<<20), EvalGlobal)
	var e *Error
	if !errors.As(err, &e) || !e.OutOfMemory {
		t.Fatalf("got %v, want out of memory", err)
	}
	stack := evalFileStack(t, m, strings.Repeat("n", 1<<20), "throw new Error('x')")
	if !strings.Contains(stack, "("+name+":1:") {
		t.Fatalf("stack %.200q", stack)
	}
}

// checkEvalSource runs evalSource inside the engine, as jsEval does, and
// checks the n bytes it wrote and, on the shadow stack, the mark in after
// past them. It reports with t.Errorf, as t.Fatalf would leave the engine
// halfway.
func checkEvalSource(t *testing.T, m *VM, name, src string, after []byte) bool {
	t.Helper()
	n := len(name) + len(src) + 2
	p, _, heap := m.evalSource(name, src)
	if p == 0 {
		t.Errorf("%d bytes: no memory", n)
		return false
	}
	defer m.tmpFree(p, heap)
	b := bytesAt(p, uintptr(n))
	if string(b[:len(name)]) != name || b[len(name)] != 0 || string(b[len(name)+1:n-1]) != src || b[n-1] != 0 {
		t.Errorf("%d bytes: not name, 0, src, 0", n)
		return false
	}
	if heap != (n > tmpMax) {
		t.Errorf("%d bytes: heap %v", n, heap)
		return false
	}
	if !heap {
		for i, c := range after[n:] {
			if c != 0xaa {
				t.Errorf("%d bytes: byte %d after them written", n, i)
				return false
			}
		}
	}
	return true
}

// TestEvalFileSameAsEval: a script gives the same result with a name as with
// none, makes as many Go allocations, and leaves the engine's memory as it
// leaves it with none once its functions are gone: the name costs an atom
// while they live.
func TestEvalFileSameAsEval(t *testing.T) {
	const src = `(function () {
  var a = [];
  for (var i = 0; i < 100; i++) a.push({ i: i, s: "v" + i });
  return JSON.stringify(a.slice(95));
})()`
	run := func(name string) (result string, goAllocs float64, after MemoryUsage) {
		m := newEvalFileVM(t)
		eval := func() string {
			var v Value
			var err error
			if name == "" {
				v, err = m.EvalValue(src, EvalGlobal)
			} else {
				v, err = m.EvalFile(name, src, EvalGlobal)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer v.Free()
			s, err := v.Any()
			if err != nil {
				t.Fatal(err)
			}
			return s.(string)
		}
		result = eval()
		goAllocs = testing.AllocsPerRun(20, func() { eval() })
		m.RunGC()
		return result, goAllocs, m.MemoryUsage()
	}
	r0, a0, u0 := run("")
	for _, name := range []string{"<eval>", "repro.js"} {
		r, a, u := run(name)
		if r != r0 || a != a0 {
			t.Errorf("%s: result %q, %v Go allocations, want %q, %v", name, r, a, r0, a0)
		}
		if u.MallocCount != u0.MallocCount || u.MallocSize != u0.MallocSize || u.AtomCount != u0.AtomCount {
			t.Errorf("%s: %d blocks, %d bytes, %d atoms after, want %d, %d, %d", name, u.MallocCount, u.MallocSize, u.AtomCount, u0.MallocCount, u0.MallocSize, u0.AtomCount)
		}
	}
}
