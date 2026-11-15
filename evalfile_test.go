// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"strings"
	"testing"
	"time"
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
		{"a\nb", "a\uFFFDb"},
		{"a\rb", "a\uFFFDb"},
		{"a\x00b", "a\uFFFDb"},
		{"a\tb", "a\uFFFDb"},
		{"a\x7fb", "a\uFFFDb"},
		{"a\u2028b\u2029c", "a\uFFFDb\uFFFDc"},
		{"\xffa\xc3", "\uFFFDa\uFFFD"},
		{long, strings.Repeat("é", maxEvalName/2)},
		{strings.Repeat("a", maxEvalName-1) + "é", strings.Repeat("a", maxEvalName-1)},
		{strings.Repeat("a", 1<<20), strings.Repeat("a", maxEvalName)},
		// C1 controls
		{"a\u0080b\u0085c\u009fd", "a�b�c�d"},
		// a cut inside a character of 3 bytes and of 4
		{strings.Repeat("a", maxEvalName-2) + "日", strings.Repeat("a", maxEvalName-2)},
		{strings.Repeat("a", maxEvalName-3) + "😀", strings.Repeat("a", maxEvalName-3)},
		// a cut next to a replaced character: one that fits, then one
		// that does not
		{strings.Repeat("a", maxEvalName-4) + "\n" + "é", strings.Repeat("a", maxEvalName-4) + "�"},
		{strings.Repeat("a", maxEvalName-2) + "\n", strings.Repeat("a", maxEvalName-2)},
		{strings.Repeat("a", maxEvalName-1) + "\xff", strings.Repeat("a", maxEvalName-1)},
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
// on the shadow stack or in the heap. On the shadow stack the bytes after the
// n are checked to be left as they were. In the heap they cannot be: the
// block comes from the allocator only once n is asked for, so no mark can be
// put past it first. What guards it there is the same slice of n bytes,
// written by copy and by the two stores at len(name) and n-1, both below n.
//
// This checks the writes, not what JS_Eval reads. The engine reads the name
// with strlen, which reads whole aligned words (engine/libc/string/strlen.c),
// so up to 7 bytes past the 0 byte of the name, though never past the word
// that holds it. Those bytes are inside what tmp reserved, which is n rounded
// up: on the shadow stack to 16 bytes (the call starts at AlignSP(p+n), and
// p is the shadow pointer, 16-byte aligned), in the heap to 8 (JS_MALLOC_ALIGN
// of engine/quickjs.c for a small block, ALIGN_SIZE of engine/tlsf.c for a
// large one, and blocks start on 8 bytes). TestEvalSourceRounding checks the
// shadow stack for the smallest n, 3.
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

// TestEvalFileModuleName: a module's name is its identity, so EvalFile
// refuses a module name the cleaning would change, before it copies
// anything, where it changes a name of a script for its stack. Two names
// that would clean to the same name would otherwise make one module.
func TestEvalFileModuleName(t *testing.T) {
	m := newEvalFileVM(t)
	prefix := strings.Repeat("m", maxEvalName)
	for _, name := range []string{"a\nb", "a\rb", "a\x00b", "\xffa", "a\u2028b", prefix + "1", prefix + "2"} {
		v, err := m.EvalFile(name, `export const x = 1`, EvalModule)
		if err == nil {
			v.Free()
			t.Errorf("module %.40q: no error", name)
			continue
		}
		var e *Error
		if errors.As(err, &e) || !strings.Contains(err.Error(), "module name") {
			t.Errorf("module %.40q: %v", name, err)
		}
	}
	// The same names run as scripts, cleaned.
	for _, name := range []string{"a\nb", prefix + "1"} {
		v, err := m.EvalFile(name, `1`, EvalGlobal)
		if err != nil {
			t.Fatalf("script %.40q: %v", name, err)
		}
		v.Free()
	}
	// A name the cleaning keeps is the module's name.
	for _, c := range [][2]string{{"dir/日本.mjs", `export const x = 7`}, {"main.mjs", `import { x } from "dir/日本.mjs"; globalThis.got = x`}} {
		v, err := m.EvalFile(c[0], c[1], EvalModule)
		if err != nil {
			t.Fatal(err)
		}
		v.Free()
	}
	if got, err := m.Eval(`got`, EvalGlobal); err != nil || got != 7 {
		t.Fatalf("imported %v, %v", got, err)
	}
}

// TestEvalModuleSameName: what is true of two modules given the same name,
// by EvalFile as by Eval, which names every module <eval>: both run, and
// import.meta and an import of that name find the first.
func TestEvalModuleSameName(t *testing.T) {
	for _, name := range []string{"", "m.js"} {
		m := newEvalFileVM(t)
		eval := func(src string) {
			t.Helper()
			var v Value
			var err error
			if name == "" {
				v, err = m.EvalValue(src, EvalModule)
			} else {
				v, err = m.EvalFile(name, src, EvalModule)
			}
			if err != nil {
				t.Fatalf("%q: %v", name, err)
			}
			v.Free()
		}
		eval(`export const x = 1; globalThis.ran = ["first"]; globalThis.firstMeta = import.meta`)
		eval(`export const x = 2; ran.push("second"); globalThis.sameMeta = import.meta === firstMeta`)
		modName := name
		if modName == "" {
			modName = evalName
		}
		eval(`import { x } from "` + modName + `"; globalThis.imported = x`)
		got, err := m.Eval(`JSON.stringify([ran, sameMeta, imported])`, EvalGlobal)
		if err != nil {
			t.Fatal(err)
		}
		if got != `[["first","second"],true,1]` {
			t.Errorf("name %q: %v", name, got)
		}
	}
}

// TestEvalFileStops: a call of EvalFile whose source took a block of the
// heap (longer than tmpMax) and that the eval timeout or Interrupt stops
// gives the block back, and the VM goes on.
func TestEvalFileStops(t *testing.T) {
	m := newEvalFileVM(t)
	src := strings.Repeat(" ", tmpMax) + "for (;;) {}"
	for _, how := range []string{"timeout", "interrupt"} {
		m.RunGC()
		before := m.MemoryUsage()
		if how == "timeout" {
			if err := m.SetEvalTimeout(50 * time.Millisecond); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := m.SetEvalTimeout(0); err != nil {
				t.Fatal(err)
			}
			timer := time.AfterFunc(50*time.Millisecond, m.Interrupt)
			defer timer.Stop()
		}
		_, err := m.EvalFile("loop.js", src, EvalGlobal)
		var e *Error
		if !errors.As(err, &e) || !e.Interrupted {
			t.Fatalf("%s: got %v, want interrupted", how, err)
		}
		m.RunGC()
		if after := m.MemoryUsage(); after.MallocCount != before.MallocCount || after.MallocSize != before.MallocSize {
			t.Errorf("%s: %d blocks, %d bytes after, %d, %d before", how, after.MallocCount, after.MallocSize, before.MallocCount, before.MallocSize)
		}
		stack := evalFileStack(t, m, "after.js", "throw new Error('x')")
		if !strings.Contains(stack, "(after.js:1:") {
			t.Fatalf("%s: stack %q", how, stack)
		}
	}
}

// TestEvalSourceRounding: for the smallest source, a name of one byte and
// no source (n = 3), the shadow stack reserves 16 bytes from a 16-byte
// boundary, so the word strlen reads around the 0 byte of the name is
// reserved (see TestEvalFileNameBound).
func TestEvalSourceRounding(t *testing.T) {
	m := newEvalFileVM(t)
	m.outer(func() {
		p, sp, heap := m.evalSource("a", "")
		defer m.tmpFree(p, heap)
		if heap || p%16 != 0 || sp != p+16 {
			t.Errorf("p %#x, call at %#x, heap %v: want 16 bytes from a 16-byte boundary", p, sp, heap)
		}
		if b := bytesAt(p, 3); string(b) != "a\x00\x00" {
			t.Errorf("bytes %q", b)
		}
	})
}
