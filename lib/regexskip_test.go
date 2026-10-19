// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
	"unsafe"

	"modernc.org/libc"
)

// testdata/regex.js runs a set of regexps on a set of strings through every
// API that runs a regexp. testdata/regex.txt is the transcript of the engine
// before lreSkip, so skipping the positions where a match cannot start must
// leave every line as it was.
func TestRegexSemantics(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("testdata", "regex.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "regex.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := eval(t, "regex.js", string(js))
	if got == strings.TrimSuffix(string(want), "\n") {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(strings.TrimSuffix(string(want), "\n"), "\n")
	errs := 0
	for i := 0; (i < len(gl) || i < len(wl)) && errs < 20; i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Errorf("line %d:\n got %s\nwant %s", i+1, g, w)
			errs++
		}
	}
}

// lreSkip moves the start of a match to the first position where the first
// character can match, gives up when there is none, and leaves the regexps
// it does not know as they are. Every pointer it gets is to C memory, as
// from the engine.
func TestRegexSkip(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	const (
		g = MLRE_FLAG_GLOBAL
		i = MLRE_FLAG_IGNORECASE
		m = MLRE_FLAG_MULTILINE
		u = MLRE_FLAG_UNICODE
		y = MLRE_FLAG_STICKY
		v = MLRE_FLAG_UNICODE_SETS

		asIs = -2 // lreSkip leaves the start as it is
	)
	for _, c := range []struct {
		re     string
		flags  int32
		s      string
		wide   bool // a string of 16-bit units
		cindex int32
		want   int32 // the start, -1 for no match, or asIs
		loop   bool  // whether the match starts with the loop over positions
	}{
		{"a", 0, "xxa", false, 0, 2, true},
		{"a", g, "xxa", false, 1, 2, true},
		{"a", g, "xxa", false, 2, 2, true},
		{"a", 0, "xxx", false, 0, -1, false},
		{"a", g, "xxa", false, 3, -1, false},
		{"a", 0, "", false, 0, -1, false},
		{"ab", 0, "xab", false, 0, 1, true},
		{"[b-d]", 0, "aaac", false, 0, 3, true},
		{"[b-d]", 0, "aaae", false, 0, -1, false},
		{"[<>&]", 0, "xx&", false, 0, 2, true},
		{"[^a]", 0, "aab", false, 0, 2, true},
		{"[\\s\\S]", 0, "", false, 0, -1, false},
		{"\\uffff", 0, "a\uffff", true, 0, 1, true},
		{"[^a]", 0, "a\uffff", true, 0, 1, true},
		{"[\\uff00-\\uffff]", 0, "a\uffff", true, 0, asIs, true}, // range32
		{"中", 0, "ab中", true, 0, 2, true},
		{"中", 0, "abc", false, 0, -1, false},
		{"[一-龥]", 0, "ab中", true, 1, 2, true},
		{"^a", 0, "ab", false, 0, 0, false},
		{"^a", g, "ab", false, 1, -1, false},
		{"^", g, "", false, 0, 0, false},
		{"^a", m, "b\na", false, 0, asIs, true},
		{"a", y, "xxa", false, 0, asIs, false},
		{"a", u, "xxa", false, 0, asIs, true},
		{"a", v, "xxa", false, 0, asIs, true},
		{"a", i, "xxa", false, 0, asIs, true},
		{"[a-c]", i, "xxa", false, 0, asIs, true},
		{"&", i, "xx&", false, 0, asIs, true},    // char even when ignoring case
		{"[&<]", i, "xx&", false, 0, asIs, true}, // range even when ignoring case
		{"a", m, "xxa", false, 0, asIs, true},
		{"(a)", 0, "xxa", false, 0, asIs, true},
		{"a|b", 0, "xxa", false, 0, asIs, true},
		{".", 0, "xxa", false, 0, asIs, true},
		{"\\d", 0, "xx1", false, 0, 2, true},
		{"\\w", 0, "-_", false, 0, 1, true},
		{"\\s", 0, "a\u3000", true, 0, asIs, true}, // space
	} {
		name := fmt.Sprintf("/%s/%d on %q from %d", c.re, c.flags, c.s, c.cindex)
		bc := compileRegexp(t, tls, ctx, c.re, c.flags)
		buf, clen, cbufType := regexpInput(t, tls, c.s, c.wide)
		n := uintptr(2 * Xlre_get_capture_count(tls, bc))
		capture := cAlloc(t, tls, n*unsafe.Sizeof(uintptr(0)))
		for k := uintptr(0); k < n; k++ {
			*(*uintptr)(unsafe.Pointer(capture + k*unsafe.Sizeof(uintptr(0)))) = 1
		}
		start, pc := lreSkip(tls, capture, bc, buf, c.cindex, clen, cbufType, ctx)
		want, wantPC := c.want, uintptr(MRE_HEADER_LEN)
		switch {
		case want == asIs:
			want = c.cindex
		case want < 0:
			want, wantPC = 0, 0
		case !c.loop:
			wantPC = MRE_HEADER_LEN + 11
		}
		if start != want || pc != wantPC {
			t.Errorf("%s: start %d, code at %d, want %d, %d", name, start, pc, want, wantPC)
		}
		for k := uintptr(0); k < n; k++ {
			p := *(*uintptr)(unsafe.Pointer(capture + k*unsafe.Sizeof(uintptr(0))))
			if pc == 0 && p != 0 || pc != 0 && p != 1 {
				t.Errorf("%s: capture %d is %#x", name, k, p)
			}
		}
		libc.Xfree(tls, capture)
		libc.Xfree(tls, buf)
		Xjs_free(tls, ctx, bc)
	}
}

// lreSkip checks for an interrupt after each lreSkipChunk units it looks
// at, as the backtracking machine does every few thousand steps, and then
// answers as the machine does: lre_exec returns LRE_RET_TIMEOUT.
func TestRegexSkipInterrupt(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)
	XJS_SetInterruptHandler(tls, rt, funcPointer(skipInterrupt), 0)

	for _, c := range []struct {
		re        string
		s         string
		units     int
		stop      int32
		start     int32
		pc        uintptr
		wantCalls int
	}{
		{"a", "x", lreSkipChunk, 1, 0, 0, 0},
		{"a", "x", lreSkipChunk + 1, 1, MLRE_RET_TIMEOUT, 0, 1},
		{"a", "x", 3*lreSkipChunk + 1, 0, 0, 0, 3},
		{"[ab]", "x", 3*lreSkipChunk + 1, 1, MLRE_RET_TIMEOUT, 0, 1},
		{"a", "\u4e2d", 2*lreSkipChunk + 1, 1, MLRE_RET_TIMEOUT, 0, 1},
		{"a", "x", 2 * lreSkipChunk, 0, 0, 0, 1},
	} {
		name := fmt.Sprintf("/%s/ on %d of %q, interrupt %d", c.re, c.units, c.s, c.stop)
		skipChecks, skipStop = 0, c.stop
		bc := compileRegexp(t, tls, ctx, c.re, 0)
		buf, clen, cbufType := regexpInput(t, tls, strings.Repeat(c.s, c.units), false)
		capture := cAlloc(t, tls, 2*unsafe.Sizeof(uintptr(0)))
		start, pc := lreSkip(tls, capture, bc, buf, 0, clen, cbufType, ctx)
		if start != c.start || pc != c.pc || skipChecks != c.wantCalls {
			t.Errorf("%s: start %d, code at %d, %d checks, want %d, %d, %d", name, start, pc, skipChecks, c.start, c.pc, c.wantCalls)
		}
		libc.Xfree(tls, capture)
		libc.Xfree(tls, buf)
		Xjs_free(tls, ctx, bc)
	}
}

var (
	skipChecks int   // the calls of skipInterrupt
	skipStop   int32 // what skipInterrupt returns
)

// skipInterrupt is the interrupt handler of TestRegexSkipInterrupt. It is
// a function of the package, not a closure: the engine holds it as a
// uintptr, which does not keep a closure alive.
func skipInterrupt(tls *libc.TLS, rt, opaque uintptr) int32 {
	skipChecks++
	return skipStop
}

// funcPointer returns f, a function of the package, as the engine holds a
// pointer to a C function.
func funcPointer(f any) uintptr {
	type iface [2]uintptr
	return (*iface)(unsafe.Pointer(&f))[1]
}

// cAlloc returns n bytes of C memory, freed with libc.Xfree.
func cAlloc(t *testing.T, tls *libc.TLS, n uintptr) uintptr {
	t.Helper()
	p := libc.Xcalloc(tls, 1, Tsize_t(max(n, 1)))
	if p == 0 {
		t.Fatal("out of memory")
	}
	return p
}

// compileRegexp returns the bytecode of /re/flags. What it passes to the
// engine is in C memory, as the engine requires.
func compileRegexp(t *testing.T, tls *libc.TLS, ctx uintptr, re string, flags int32) uintptr {
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
	bc := Xlre_compile(tls, n, msg, msgSize, src, Tsize_t(len(re)), flags, ctx)
	if bc == 0 {
		t.Fatalf("/%s/: %s", re, libc.GoString(msg))
	}
	return bc
}

// regexpInput returns s in C memory as the engine holds a string: 8-bit
// units when every character fits, 16-bit units otherwise or when wide is
// set. It is freed with libc.Xfree.
func regexpInput(t *testing.T, tls *libc.TLS, s string, wide bool) (buf uintptr, clen, cbufType int32) {
	t.Helper()
	units := utf16.Encode([]rune(s))
	for _, c := range units {
		wide = wide || c > 0xff
	}
	if !wide {
		buf = cAlloc(t, tls, uintptr(len(units)+1))
		for k, c := range units {
			*(*byte)(unsafe.Pointer(buf + uintptr(k))) = byte(c)
		}
		return buf, int32(len(units)), 0
	}
	buf = cAlloc(t, tls, 2*uintptr(len(units))+2)
	for k, c := range units {
		*(*uint16)(unsafe.Pointer(buf + 2*uintptr(k))) = c
	}
	return buf, int32(len(units)), 1
}
