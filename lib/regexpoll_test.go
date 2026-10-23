// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"strings"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

var (
	rePollCalls int   // the calls of rePollInterrupt
	rePollStop  int32 // what rePollInterrupt returns
)

func rePollInterrupt(tls *libc.TLS, rt, opaque uintptr) int32 {
	rePollCalls++
	return rePollStop
}

// lreCompile compiles re with lre_compile for ctx and returns the error
// message, empty when it compiled.
func lreCompile(t *testing.T, tls *libc.TLS, ctx uintptr, re string) string {
	t.Helper()
	src, err := libc.CString(re)
	if err != nil {
		t.Fatal(err)
	}
	defer libc.Xfree(tls, src)
	const msgSize = 64
	n := cAlloc(t, tls, unsafe.Sizeof(int32(0)))
	defer libc.Xfree(tls, n)
	msg := cAlloc(t, tls, msgSize)
	defer libc.Xfree(tls, msg)
	bc := Xlre_compile(tls, n, msg, msgSize, src, Tsize_t(len(re)), 0, ctx)
	if bc == 0 {
		return libc.GoString(msg)
	}
	Xjs_free(tls, ctx, bc)
	return ""
}

// The regexp parser counts a step toward the interrupt counter of the
// context for each term, alternative and atom of a class, and one more for
// each 256 bytes of code or ranges it moves, so the interrupt handler sees
// a compile in proportion to its work. When the handler asks to stop, the
// compile fails with the message js_compile_regexp throws the interrupt
// for, and a script cannot catch it.
func TestRegexCompilePoll(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)
	XJS_SetInterruptHandler(tls, rt, funcPointer(rePollInterrupt), 0)
	counter := &(*TJSContext)(unsafe.Pointer(ctx)).Finterrupt_counter

	// The steps of small patterns, which move less than 256 bytes.
	const full = 1 << 30
	for _, c := range []struct {
		re    string
		steps int32
	}{
		{"a", 1},
		{"abc", 3},
		{"a|b|c", 5},     // 3 terms, 2 alternatives
		{"(a|b|c)+d", 7}, // the group, 3 terms in it, 2 alternatives, d
		{"[abc]", 4},     // the class and its 3 atoms
		{"[a-z0-9_]", 4}, // the class and its 3 atoms
		{"(?<=abc)x", 8}, // the lookbehind, 3 terms moved back, x
		{"(?:" + strings.Repeat("ab", 40) + ")", 81},
	} {
		rePollCalls, *counter = 0, full
		if msg := lreCompile(t, tls, ctx, c.re); msg != "" {
			t.Fatalf("/%s/: %s", c.re, msg)
		}
		if got := full - *counter; got != c.steps || rePollCalls != 0 {
			t.Errorf("/%s/: %d steps, %d calls of the handler, want %d, 0", c.re, got, rePollCalls, c.steps)
		}
	}

	// Moving code counts too: each alternative moves the code of those
	// before it, and each atom of a class copies the ranges before it.
	for _, c := range []struct {
		name, re string
		min      int32
	}{
		// 1000 alternatives of 3 bytes of code, a split and a goto each:
		// the k-th moves 13k bytes.
		{"alternatives", strings.TrimSuffix(strings.Repeat("a|", 1000), "|"), 1000 * 999 * 13 / 2 / 256},
		// 1000 atoms of a class, each a range of 8 bytes apart from the
		// others: the k-th copies 8k bytes.
		{"class", "[" + string(classAtoms(1000)) + "]", 1000 * 999 * 8 / 2 / 256},
		// 1000 terms in a lookbehind, each of 3 bytes: the k-th moves 3k.
		{"lookbehind", "(?<=" + strings.Repeat("a", 1000) + ")", 1000 * 999 * 3 / 2 / 256},
	} {
		rePollCalls, *counter = 0, full
		if msg := lreCompile(t, tls, ctx, c.re); msg != "" {
			t.Fatalf("%s: %s", c.name, msg)
		}
		// Each step drops what it moves below 256 bytes: 0.9 of the bytes.
		if got := full - *counter; got < c.min*9/10 || got > 2*c.min+4000 {
			t.Errorf("%s: %d steps, want at least %d", c.name, got, c.min)
		}
	}

	// The handler is called once in JS_INTERRUPT_COUNTER_INIT steps, and a
	// handler that asks to stop fails the compile at once, with the
	// message of an interrupt.
	long := strings.Repeat("(?:a|b|c)+d", 5000) // 35000 steps
	rePollCalls, rePollStop, *counter = 0, 0, MJS_INTERRUPT_COUNTER_INIT
	if msg := lreCompile(t, tls, ctx, long); msg != "" || rePollCalls != 3 {
		t.Errorf("a compile of 35000 steps: %q, %d calls of the handler, want none, 3", msg, rePollCalls)
	}
	rePollCalls, rePollStop, *counter = 0, 1, 2
	if msg := lreCompile(t, tls, ctx, long); msg != "interrupted" || rePollCalls != 1 || *counter != MJS_INTERRUPT_COUNTER_INIT {
		t.Errorf("an interrupt: %q, %d calls of the handler, counter %d, want interrupted, 1, %d", msg, rePollCalls, *counter, MJS_INTERRUPT_COUNTER_INIT)
	}

	// A script gets the interrupt, uncatchable, from new RegExp and from a
	// regexp literal the parser compiles, and the handler was called once,
	// in the compile.
	for _, js := range []string{
		"try { new RegExp('" + long + "'); 'compiled' } catch (e) { 'caught' }",
		"try { /" + long + "/; 'compiled' } catch (e) { 'caught' }",
		"try { eval('/" + long + "/'); 'compiled' } catch (e) { 'caught' }",
	} {
		rePollCalls, rePollStop, *counter = 0, 1, MJS_INTERRUPT_COUNTER_INIT
		got := eval1(t, tls, ctx, js)
		if got != "<exception=InternalError: interrupted>" || rePollCalls != 1 {
			t.Errorf("%.40s: %s, %d calls of the handler, want the interrupt, 1", js, got, rePollCalls)
		}
	}
	rePollStop = 0
	if got := eval1(t, tls, ctx, "String(new RegExp('"+long+"').test('ad'.repeat(5000)))"); got != "true" {
		t.Errorf("after: %s", got)
	}
}

// classAtoms returns n characters no two of which are next to each other,
// so that each is a range of its own in a class.
func classAtoms(n int) []rune {
	r := make([]rune, n)
	for i := range r {
		r[i] = rune(0x4e00 + 2*i)
	}
	return r
}

// A lookup in the regexp cache counts a step for each 32 bytes of the
// pattern it hashes and compares, so a loop that takes a long pattern from
// the cache reaches the interrupt check as soon as one that compiles a
// short one.
func TestRegexCacheCountsSteps(t *testing.T) {
	v := newReVM(t, 16, 1<<20)
	defer v.close()
	counter := &(*TJSContext)(unsafe.Pointer(v.ctx)).Finterrupt_counter
	for _, c := range []struct {
		pattern string
		steps   int32
	}{
		{"'a'.repeat(32000)", 1000},
		{"'\u4e2d'.repeat(32000)", 2000}, // 16-bit characters
	} {
		v.run("var p = " + c.pattern + "; new RegExp(p); 0")
		*counter = 1 << 30
		v.run("new RegExp(p); 0")
		// The script itself polls a few times.
		if got := 1<<30 - *counter; got < c.steps || got > c.steps+20 {
			t.Errorf("%s from the cache: %d steps, want %d", c.pattern, got, c.steps)
		}
	}
}
