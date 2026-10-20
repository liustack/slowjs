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
	"time"
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

// testdata/regexsplit.js runs regexps of alternatives, groups, quantifiers,
// assertions, lookarounds and back references, with every flag, on a set of
// strings through every API that runs a regexp. testdata/regexsplit.txt is
// what Node gives (and the engine gave the same before lreFirstSet and
// lreSplit), so it must stay as it is.
func TestRegexSplitSemantics(t *testing.T) {
	matchTranscript(t, "regexsplit")
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

		asIs = -2 // lreSkip leaves the start as it is, with the loop
		at0  = -3 // lreSkip runs the match once at position 0
	)
	for _, c := range []struct {
		re     string
		flags  int32
		s      string
		wide   bool // a string of 16-bit units
		cindex int32
		want   int32 // the start, -1 for no match, asIs or at0
		kind   uint8 // how lreRun goes on from a start
	}{
		{"a", 0, "xxa", false, 0, 2, lreRunChar},
		{"a", g, "xxa", false, 1, 2, lreRunChar},
		{"a", g, "xxa", false, 2, 2, lreRunChar},
		{"a", 0, "xxx", false, 0, -1, 0},
		{"a", g, "xxa", false, 3, -1, 0},
		{"a", 0, "", false, 0, -1, 0},
		{"ab", 0, "xab", false, 0, 1, lreRunChar},
		{"[b-d]", 0, "aaac", false, 0, 3, lreRunRange},
		{"[b-d]", 0, "aaae", false, 0, -1, 0},
		{"[<>&]", 0, "xx&", false, 0, 2, lreRunRange},
		{"[^a]", 0, "aab", false, 0, 2, lreRunRange},
		{"[\\s\\S]", 0, "", false, 0, -1, 0},
		{"\\uffff", 0, "a\uffff", true, 0, 1, lreRunChar},
		{"[^a]", 0, "a\uffff", true, 0, 1, lreRunRange},
		{"[\\uff00-\\uffff]", 0, "a\uffff", true, 0, 1, lreRunSet}, // range32: all units from 256 up
		{"中", 0, "ab中", true, 0, 2, lreRunChar},
		{"中", 0, "abc", false, 0, -1, 0},
		{"[一-龥]", 0, "ab中", true, 1, 2, lreRunRange},
		{"^a", 0, "ab", false, 0, at0, 0},
		{"^a", g, "ab", false, 1, -1, 0},
		{"^", g, "", false, 0, at0, 0},
		{"^a", m, "b\na", false, 0, 2, lreRunSet},
		{"a", y, "xxa", false, 0, asIs, 0},
		{"a", u, "xxa", false, 0, asIs, 0},
		{"a", v, "xxa", false, 0, asIs, 0},
		{"a", i, "xxA", false, 0, 2, lreRunSet},
		{"A", i, "xxa", false, 0, 2, lreRunSet},
		{"a", i, "xx\u212a", true, 0, -1, 0},
		{"[a-c]", i, "xxa", false, 0, asIs, 0},
		{"é", i, "xxé", false, 0, asIs, 0},
		{"&", i, "xx&", false, 0, 2, lreRunSet}, // char even when ignoring case
		{"[&<]", i, "xx&", false, 0, asIs, 0},   // range even when ignoring case
		{"a", m, "xxa", false, 0, 2, lreRunChar},
		{"(a)", 0, "xxa", false, 0, 2, lreRunSet},
		{"a|b", 0, "xxb", false, 0, 2, lreRunSet},
		{"a|b", 0, "xxc", false, 0, -1, 0},
		{"a|b", 0, "xx中b", true, 0, 3, lreRunSet},
		{"a|中", 0, "xxx中", true, 0, 3, lreRunSet}, // any unit from 256 up
		{"(?:ab|cd)*e", 0, "xxe", false, 0, 2, lreRunSet},
		{"a?b", 0, "xxb", false, 0, 2, lreRunSet},
		{"a*", 0, "xxa", false, 0, asIs, 0}, // matches the empty string
		{"a|", 0, "xxa", false, 0, asIs, 0},
		{"\\bfoo", 0, "a foo", false, 0, 2, lreRunSet},
		{"(?=a)\\w", 0, "--a", false, 0, 2, lreRunSet},
		{"(?<=a)b", 0, "xab", false, 0, 2, lreRunSet},
		{"(a)\\1", 0, "xxa", false, 0, 2, lreRunSet},
		{"\\1(a)", 0, "xxa", false, 0, asIs, 0},
		{"(?:)a", 0, "xxa", false, 0, 2, lreRunChar},
		{".", 0, "xxa", false, 0, asIs, 0},
		{"\\d", 0, "xx1", false, 0, 2, lreRunRange},
		{"\\w", 0, "-_", false, 0, 1, lreRunRange},
		{"\\s", 0, "a\u3000", true, 0, asIs, 0}, // space
		{"x\\s|a", 0, "-x", false, 0, 1, lreRunSet},
		{strings.Repeat("w|", 600) + "a", 0, "xxa", false, 0, asIs, 0}, // past lreFirstBudget
		{strings.Repeat("w|", 300) + "a", 0, "xxa", false, 0, 2, lreRunSet},
	} {
		name := fmt.Sprintf("/%s/%d on %q from %d", c.re, c.flags, c.s, c.cindex)
		if len(name) > 80 {
			name = name[:80]
		}
		bc := compileRegexp(t, tls, ctx, c.re, c.flags)
		buf, clen, cbufType := regexpInput(t, tls, c.s, c.wide)
		n := uintptr(2 * Xlre_get_capture_count(tls, bc))
		capture := cAlloc(t, tls, n*unsafe.Sizeof(uintptr(0)))
		for k := uintptr(0); k < n; k++ {
			*(*uintptr)(unsafe.Pointer(capture + k*unsafe.Sizeof(uintptr(0)))) = 1
		}
		st := lreSkip(tls, capture, bc, buf, c.cindex, clen, cbufType, ctx)
		want, wantPC, kind := c.want, uintptr(lreSkipMatch), c.kind
		switch want {
		case asIs:
			want, wantPC = c.cindex, MRE_HEADER_LEN
		case at0:
			want = 0
		case -1:
			want, wantPC = 0, 0
		}
		if st.index != want || st.pc != wantPC || st.kind != kind {
			t.Errorf("%s: start %d, code at %d, kind %d, want %d, %d, %d", name, st.index, st.pc, st.kind, want, wantPC, kind)
		}
		for k := uintptr(0); k < n; k++ {
			p := *(*uintptr)(unsafe.Pointer(capture + k*unsafe.Sizeof(uintptr(0))))
			if st.pc == 0 && p != 0 || st.pc != 0 && p != 1 {
				t.Errorf("%s: capture %d is %#x", name, k, p)
			}
		}
		libc.Xfree(tls, capture)
		libc.Xfree(tls, buf)
		Xjs_free(tls, ctx, bc)
	}
}

// lreFirstSet gives the units a match can start with, from both ways of
// every split, and nothing it does not know.
func TestRegexFirstSet(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	for _, c := range []struct {
		re    string
		flags int32
		units string // the units below 256 of the set, in order
		high  bool
	}{
		{"cat|dog|bird", 0, "bcd", false},
		{"(cat)|(?:dog)|[x-z]", 0, "cdxyz", false},
		{"a?b*c", 0, "abc", false},
		{"(?:a|b)?(?:c|d)?e", 0, "abcde", false},
		{"[^\\x00-\\xfe]", 0, "\xff", true},
		{"a|\\u0100", 0, "a", true},
		{"(?<=x)a|(?!y)b", 0, "ab", false},
		{"\\b(?:a|b)\\B$|c", 0, "abc", false},
		{"Ab|c", MLRE_FLAG_IGNORECASE, "ACac", false},
		{"(?i:k)|(?-i:x)", 0, "Kkx", false},
		{"[]|a", 0, "a", false},
	} {
		bc := compileRegexp(t, tls, ctx, c.re, c.flags)
		set, ok := lreFirstSet(bc + lreSkipBody)
		var got []byte
		for k := uint32(0); k < 256; k++ {
			if set.has(k) {
				got = append(got, byte(k))
			}
		}
		if !ok || string(got) != c.units || set.high != c.high {
			t.Errorf("/%s/%d: %v, %q, high %v, want %q, high %v", c.re, c.flags, ok, got, set.high, c.units, c.high)
		}
		Xjs_free(tls, ctx, bc)
	}
}

// Outside unicode mode, lre_canonicalize gives the upper case of an ASCII
// letter, every other ASCII unit as it is, and an ASCII unit for no other
// unit, as Canonicalize of the specification does: a character from 128
// whose upper case is below 128 stays as it is. lreFirstSet relies on it
// for the characters that ignore case.
func TestCanonicalizeASCII(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	for c := uint32(0); c <= 0xffff; c++ {
		got := uint32(Xlre_canonicalize(tls, c, 0))
		want := c
		if c >= 'a' && c <= 'z' {
			want = c - 'a' + 'A'
		}
		if c < 128 && got != want || c >= 128 && got < 128 {
			t.Errorf("lre_canonicalize(%#x) = %#x", c, got)
		}
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
		{"a|b", "x", 3*lreSkipChunk + 1, 1, MLRE_RET_TIMEOUT, 0, 1},
		{"a|b", "x", 3*lreSkipChunk + 1, 0, 0, 0, 3},
		{"a", "\u4e2d", 2*lreSkipChunk + 1, 1, MLRE_RET_TIMEOUT, 0, 1},
		{"a", "x", 2 * lreSkipChunk, 0, 0, 0, 1},
	} {
		name := fmt.Sprintf("/%s/ on %d of %q, interrupt %d", c.re, c.units, c.s, c.stop)
		skipChecks, skipStop = 0, c.stop
		bc := compileRegexp(t, tls, ctx, c.re, 0)
		buf, clen, cbufType := regexpInput(t, tls, strings.Repeat(c.s, c.units), false)
		capture := cAlloc(t, tls, 2*unsafe.Sizeof(uintptr(0)))
		st := lreSkip(tls, capture, bc, buf, 0, clen, cbufType, ctx)
		if st.index != c.start || st.pc != c.pc || skipChecks != c.wantCalls {
			t.Errorf("%s: start %d, code at %d, %d checks, want %d, %d, %d", name, st.index, st.pc, skipChecks, c.start, c.pc, c.wantCalls)
		}
		libc.Xfree(tls, capture)
		libc.Xfree(tls, buf)
		Xjs_free(tls, ctx, bc)
	}
}

// lreSplit steps over at most lreSplitBudget split ops in one call, and
// then gives back the one it is at to be pushed, so a chain of splits that
// leads back to itself, which lre_compile does not write, cannot hold it.
// The bytecode is a split_goto_first that takes itself and pushes a char
// 'z', on a string of 'a'.
func TestRegexSplitBudget(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	bc := cAlloc(t, tls, 8)
	defer libc.Xfree(tls, bc)
	*(*uint8)(unsafe.Pointer(bc)) = EREOP_split_goto_first
	(*Tpacked_u32)(unsafe.Pointer(bc + 1)).Fv = uint32(0xffff_fffb) // -5: back to itself
	*(*uint8)(unsafe.Pointer(bc + 5)) = EREOP_char
	(*Tpacked_u16)(unsafe.Pointer(bc + 6)).Fv = 'z'
	buf, clen, cbufType := regexpInput(t, tls, "a", false)
	defer libc.Xfree(tls, buf)
	s := cAlloc(t, tls, unsafe.Sizeof(TREExecContext{}))
	defer libc.Xfree(tls, s)
	x := (*TREExecContext)(unsafe.Pointer(s))
	x.Fcbuf, x.Fcbuf_end, x.Fcbuf_type = buf, buf+uintptr(clen), cbufType
	done := make(chan [2]uintptr, 1)
	go func() {
		pc, pc1 := lreSplit(tls, s, bc, bc+5, buf)
		done <- [2]uintptr{pc, pc1}
	}()
	select {
	case got := <-done:
		if got != [2]uintptr{bc, bc + 5} {
			t.Errorf("lreSplit gave %d and %d, want the split at 0 and its char at 5", got[0]-bc, got[1]-bc)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("lreSplit did not return")
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
