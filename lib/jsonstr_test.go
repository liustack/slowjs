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

// testdata/jsonstr.js stringifies and parses strings of every kind of
// character, at every length around the runs jsonQuoteRun and
// jsonStringRun copy. testdata/jsonstr.txt is what Node 24 prints for it,
// and the engine before the runs printed the same.
func TestJSONStrings(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("testdata", "jsonstr.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "jsonstr.txt"))
	if err != nil {
		t.Fatal(err)
	}
	compareLines(t, eval(t, "jsonstr.js", string(js)), strings.TrimSuffix(string(want), "\n"))
}

// testdata/jsonstr_errors.js makes JSON.parse throw in and around strings.
// testdata/jsonstr_errors.txt is the transcript of the engine before
// jsonStringRun: the messages and the positions stay the same.
func TestJSONStringErrors(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("testdata", "jsonstr_errors.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "jsonstr_errors.txt"))
	if err != nil {
		t.Fatal(err)
	}
	compareLines(t, eval(t, "jsonstr_errors.js", string(js)), strings.TrimSuffix(string(want), "\n"))
}

// compareLines reports the first lines where got and want differ.
func compareLines(t *testing.T, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
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
			t.Errorf("line %d:\n got %.300s\nwant %.300s", i+1, g, w)
			errs++
		}
	}
}

// jsonBytesCases are UTF-8 texts with each kind of byte jsonUTF8 treats
// apart, valid or not, as the C API's JS_ParseJSON may get them (a string of
// javascript always converts to valid UTF-8, a lone surrogate to its three
// bytes).
var jsonBytesCases = []string{
	"\"a\xc3\xa9b\"", "\"\xc2\x80\xdf\xbf\"", "\"\xc0\x80\"", "\"\xc1\xbf\"", "\"\xe0\x80\x80\"", "\"\xe0\xa0\x80\"",
	"\"\xed\xa0\x80x\"", "\"\xed\xbf\xbf\"", "\"\xef\xbf\xbf\"", "\"\xf0\x8f\xbf\xbf\"", "\"\xf0\x90\x80\x80\"",
	"\"\xf4\x8f\xbf\xbf\"", "\"\xf4\x90\x80\x80\"", "\"\xf5\x80\x80\x80\"", "\"\xf7\xbf\xbf\xbf\"", "\"\xf8\x88\x80\x80\x80\"",
	"\"\xfc\x84\x80\x80\x80\x80\"", "\"\xfe\"", "\"\xff\"", "\"\x80\"", "\"\xbf\"", "\"\xc3\"", "\"\xc3", "\"\xe4\xb8\"",
	"\"\xe4\xb8", "\"\xe4\xb8\xadq\xe4\"", "\"\xc3\xa9\xc3\"", "\"\xc3\xa9\\n\xe4\xb8\xad\"",
	"\"" + strings.Repeat("\xe4\xb8\xad", 70) + "\xc3\"", "\"" + strings.Repeat("\xf0\x9f\x98\x80", 40) + "\"",
	"\"" + strings.Repeat("a\xc3\xa9", 50) + "\xff\"",
}

// jsonBytesWant is what the engine before jsonStringRun gave for
// jsonBytesCases: the code units of the string, or the error and where.
var jsonBytesWant = []string{
	"0: 3: 61 e9 62",
	"1: 2: 80 7ff",
	"2: SyntaxError: Bad UTF-8 sequence at 1:2",
	"3: SyntaxError: Bad UTF-8 sequence at 1:2",
	"4: SyntaxError: Bad UTF-8 sequence at 1:2",
	"5: 1: 800",
	"6: 2: d800 78",
	"7: 1: dfff",
	"8: 1: ffff",
	"9: SyntaxError: Bad UTF-8 sequence at 1:2",
	"10: 2: d800 dc00",
	"11: 2: dbff dfff",
	"12: SyntaxError: Bad UTF-8 sequence at 1:2",
	"13: SyntaxError: Bad UTF-8 sequence at 1:2",
	"14: SyntaxError: Bad UTF-8 sequence at 1:2",
	"15: SyntaxError: Bad UTF-8 sequence at 1:2",
	"16: SyntaxError: Bad UTF-8 sequence at 1:2",
	"17: SyntaxError: Bad UTF-8 sequence at 1:2",
	"18: SyntaxError: Bad UTF-8 sequence at 1:2",
	"19: SyntaxError: Bad UTF-8 sequence at 1:2",
	"20: SyntaxError: Bad UTF-8 sequence at 1:2",
	"21: SyntaxError: Bad UTF-8 sequence at 1:2",
	"22: SyntaxError: Bad UTF-8 sequence at 1:2",
	"23: SyntaxError: Bad UTF-8 sequence at 1:2",
	"24: SyntaxError: Bad UTF-8 sequence at 1:2",
	"25: SyntaxError: Bad UTF-8 sequence at 1:4",
	"26: SyntaxError: Bad UTF-8 sequence at 1:3",
	"27: 3: e9 a 4e2d",
	"28: SyntaxError: Bad UTF-8 sequence at 1:72",
	"29: 80: d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00 d83d de00",
	"30: SyntaxError: Bad UTF-8 sequence at 1:102",
}

// JSON.parse of UTF-8 that is not valid gives the same strings, errors and
// positions as before jsonStringRun.
func TestJSONStringBytes(t *testing.T) {
	compareLines(t, strings.Join(jsonBytes(t), "\n"), strings.Join(jsonBytesWant, "\n"))
}

// jsonBytes parses each of jsonBytesCases with JS_ParseJSON.
func jsonBytes(t *testing.T) []string {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)
	eval1(t, tls, ctx, `globalThis.hex = function (s) { var o = []; for (var i = 0; i < s.length; i++) o.push(s.charCodeAt(i).toString(16)); return s.length + ': ' + o.join(' '); }; 0`)
	name, _ := libc.CString("v")
	defer libc.Xfree(tls, name)
	fn, _ := libc.CString("<bytes>")
	defer libc.Xfree(tls, fn)
	var out []string
	for i, c := range jsonBytesCases {
		buf, _ := libc.CString(c)
		v := XJS_ParseJSON(tls, ctx, buf, Tsize_t(len(c)), fn)
		libc.Xfree(tls, buf)
		show := "hex(v)"
		if tag(v) == EJS_TAG_EXCEPTION {
			v = XJS_GetException(tls, ctx)
			show = "v.name + ': ' + v.message + ' at ' + v.lineNumber + ':' + v.columnNumber"
		}
		g := XJS_GetGlobalObject(tls, ctx)
		XJS_SetPropertyStr(tls, ctx, g, name, v)
		XFreeValue(tls, ctx, g)
		r := eval1(t, tls, ctx, show)
		out = append(out, fmt.Sprintf("%d: %s", i, r))
	}
	return out
}

// When the memory runs out while JSON.stringify or JSON.parse copies a run
// of a string, they throw the out of memory error, and with the memory
// back they give the right string. Each case calls a function, made
// beforehand, with more and more memory, until it returns true.
func TestJSONStringsOutOfMemory(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	eval1(t, tls, ctx, `globalThis.narrow = 'ab\u00e9'.repeat(40000) + '"';
globalThis.wide = '\u6f22x'.repeat(40000) + '\ud83d\ude00\n';
globalThis.qn = JSON.stringify(narrow);
globalThis.qw = JSON.stringify(wide);
globalThis.ascii = 'x'.repeat(120000);
globalThis.qa = JSON.stringify(ascii); 0`)
	ms := &(*TJSRuntime)(unsafe.Pointer(rt)).Fmalloc_ctx.Fmalloc_state
	for _, c := range []string{"() => JSON.stringify(narrow) === qn", "() => JSON.stringify(wide) === qw",
		"() => JSON.parse(qn) === narrow", "() => JSON.parse(qw) === wide", "() => JSON.parse(qa) === ascii"} {
		fn := eval0(t, tls, ctx, c)
		if tag(fn) != EJS_TAG_OBJECT {
			t.Fatalf("%s: tag %d", c, tag(fn))
		}
		ooms := 0
		for extra := Tsize_t(0); ; extra += 4093 {
			if extra > 4<<20 {
				t.Fatalf("%s: does not return true with 4 MiB more", c)
			}
			XJS_SetMemoryLimit(tls, rt, ms.Fmalloc_size+extra)
			r := XJS_Call(tls, ctx, fn, jsUndefined(), 0, 0)
			XJS_SetMemoryLimit(tls, rt, ^Tsize_t(0))
			if tag(r) == EJS_TAG_EXCEPTION {
				if XJS_IsUncatchableException(tls, ctx) == 0 {
					t.Fatalf("%s: with %d bytes more: %s", c, extra, toString(t, tls, ctx, r))
				}
				XJS_SetUncatchableException(tls, ctx, 0)
				XFreeValue(tls, ctx, XJS_GetException(tls, ctx))
				ooms++
				continue
			}
			if tag(r) != EJS_TAG_BOOL || jsvToInt32(r) != 1 {
				t.Fatalf("%s: with %d bytes more: tag %d, want true", c, extra, tag(r))
			}
			break
		}
		XFreeValue(tls, ctx, fn)
		t.Logf("%s: ran out of memory %d times", c, ooms)
		if ooms < 10 {
			t.Errorf("%s: ran out of memory %d times, want the copies to run out", c, ooms)
		}
	}
}

// jsonTestVM is a runtime and a context for the tests that call the runs
// directly.
type jsonTestVM struct {
	t   *testing.T
	tls *libc.TLS
	rt  uintptr
	ctx uintptr
}

func newJSONTestVM(t *testing.T) *jsonTestVM {
	tls := libc.NewTLS()
	rt := XJS_NewRuntime(tls)
	v := &jsonTestVM{t, tls, rt, XJS_NewContext(tls, rt)}
	t.Cleanup(func() {
		XJS_FreeContext(tls, v.ctx)
		XJS_FreeRuntime(tls, rt)
		tls.Close()
	})
	return v
}

// buffer returns a StringBuffer with room for size characters, 16-bit or
// not, holding pre characters 'x', and a function that frees it.
func (v *jsonTestVM) buffer(size, pre int, wide bool) (uintptr, func()) {
	b := libc.Xmalloc(v.tls, Tsize_t(unsafe.Sizeof(TStringBuffer{})))
	w := int32(0)
	if wide {
		w = 1
	}
	if _string_buffer_init2(v.tls, v.ctx, b, int32(size), w) != 0 {
		v.t.Fatal("string_buffer_init2 failed")
	}
	for range pre {
		if _string_buffer_putc(v.tls, b, 'x') != 0 {
			v.t.Fatal("string_buffer_putc failed")
		}
	}
	return b, func() { _string_buffer_free(v.tls, b); libc.Xfree(v.tls, b) }
}

// units returns the code units b holds, and fails the test when b holds
// more than its room.
func (v *jsonTestVM) units(b uintptr, what string) []uint16 {
	sb := (*TStringBuffer)(unsafe.Pointer(b))
	if sb.Flen1 > sb.Fsize {
		v.t.Fatalf("%s: the buffer holds %d characters, more than its room of %d", what, sb.Flen1, sb.Fsize)
	}
	out := make([]uint16, sb.Flen1)
	for i := range out {
		if sb.Fis_wide_char != 0 {
			out[i] = *(*uint16)(unsafe.Pointer(sb.Fstr + jsonStrData + uintptr(i)*2))
		} else {
			out[i] = uint16(*(*byte)(unsafe.Pointer(sb.Fstr + jsonStrData + uintptr(i))))
		}
	}
	return out
}

// outOfMemory reports whether the context has the out of memory error
// pending, and clears it.
func (v *jsonTestVM) outOfMemory() bool {
	if XJS_HasException(v.tls, v.ctx) == 0 || XJS_IsUncatchableException(v.tls, v.ctx) == 0 {
		return false
	}
	XJS_SetUncatchableException(v.tls, v.ctx, 0)
	XFreeValue(v.tls, v.ctx, XJS_GetException(v.tls, v.ctx))
	return true
}

func xs(n int) string { return strings.Repeat("x", n) }

// jsonRunTexts are the strings of the run tests: ASCII, Latin-1, wide
// characters and a surrogate pair, at the lengths around the room of
// the buffers.
func jsonRunTexts() []string {
	var out []string
	for _, n := range []int{0, 1, 2, 7, 8, 9, 15, 16, 17, 30, 31, 32, 33, 34, 47, 48, 49, 63, 64, 65, 66, 100, 300} {
		out = append(out, strings.Repeat("a", n), strings.Repeat("\u00e9", n), "ab"+strings.Repeat("\u6f22", n),
			strings.Repeat("a", n)+"\u00e9", strings.Repeat("a", n)+"\u6f22"+strings.Repeat("b", n), strings.Repeat("a", n)+"\U0001f600")
	}
	return out
}

// Every run of JSON.parse, into a buffer of every room and kind, leaves
// the characters of the string in it as string_buffer_putc would, stops
// at the quote and never fills the buffer past its room.
func TestJSONStringRunStaysInBuffer(t *testing.T) {
	v := newJSONTestVM(t)
	for _, size := range []int{1, 2, 3, 8, 16, 31, 32, 33, 64} {
		for _, pre := range []int{0, 1, size - 1, size} {
			for _, wide := range []bool{false, true} {
				for _, text := range jsonRunTexts() {
					what := fmt.Sprintf("size %d, %d before, wide %v, %+q", size, pre, wide, text)
					b, free := v.buffer(size, pre, wide)
					src, _ := libc.CString(text + "\"")
					end := src + uintptr(len(text)) + 1
					got := jsonStringRun(v.tls, b, src, end, '"')
					if got != src+uintptr(len(text)) {
						t.Fatalf("%s: stops %d bytes on, want %d", what, int64(got)-int64(src), len(text))
					}
					want := append(utf16.Encode([]rune(xs(pre))), utf16.Encode([]rune(text))...)
					if g := v.units(b, what); string(utf16.Decode(g)) != string(utf16.Decode(want)) || len(g) != len(want) {
						t.Fatalf("%s: holds %x, want %x", what, g, want)
					}
					libc.Xfree(v.tls, src)
					free()
				}
			}
		}
	}
}

// Every run of JSON.stringify, from a string of 8 or 16 bits into a buffer
// of every room and kind, leaves the characters up to the first one to
// escape in it and never fills the buffer past its room.
func TestJSONQuoteRunStaysInBuffer(t *testing.T) {
	v := newJSONTestVM(t)
	pi := libc.Xmalloc(v.tls, 4)
	defer libc.Xfree(v.tls, pi)
	for _, size := range []int{1, 2, 3, 8, 16, 31, 32, 33, 64} {
		for _, pre := range []int{0, 1, size - 1, size} {
			for _, wide := range []bool{false, true} {
				for _, text := range jsonRunTexts() {
					for _, tail := range []string{"", "\"z", "\n"} {
						what := fmt.Sprintf("size %d, %d before, wide %v, %+q", size, pre, wide, text+tail)
						cs, _ := libc.CString(text + tail)
						str := XJS_NewStringLen(v.tls, v.ctx, cs, Tsize_t(len(text+tail)))
						libc.Xfree(v.tls, cs)
						if tag(str) != EJS_TAG_STRING {
							t.Fatalf("%s: tag %d", what, tag(str))
						}
						b, free := v.buffer(size, pre, wide)
						*(*int32)(unsafe.Pointer(pi)) = 0
						if jsonQuoteRun(v.tls, b, jsvPtr(str), pi) != 0 {
							t.Fatalf("%s: jsonQuoteRun failed", what)
						}
						// The run stops at the surrogate pair, which the loop writes.
						kept := text
						if i := strings.IndexRune(kept, 0x1f600); i >= 0 {
							kept = kept[:i]
						}
						want := append(utf16.Encode([]rune(xs(pre))), utf16.Encode([]rune(kept))...)
						if got := *(*int32)(unsafe.Pointer(pi)); int(got) != len(utf16.Encode([]rune(kept))) {
							t.Fatalf("%s: stops at %d, want %d", what, got, len(utf16.Encode([]rune(kept))))
						}
						if g := v.units(b, what); string(utf16.Decode(g)) != string(utf16.Decode(want)) || len(g) != len(want) {
							t.Fatalf("%s: holds %x, want %x", what, g, want)
						}
						free()
						XFreeValue(v.tls, v.ctx, str)
					}
				}
			}
		}
	}
}

// A run that needs more memory than is left fails as string_buffer_putc
// does, with the out of memory error: one that grows an 8-bit buffer, and
// a first wide character that widens a buffer with room left, for both
// runs.
func TestJSONRunsOutOfMemory(t *testing.T) {
	v := newJSONTestVM(t)
	pi := libc.Xmalloc(v.tls, 4)
	defer libc.Xfree(v.tls, pi)
	for _, c := range []struct {
		name, text string
		pre        int
	}{
		{"grow", strings.Repeat("a", 100), 0},
		{"widen", "\u6f22", 2},
	} {
		for _, quote := range []bool{false, true} {
			what := fmt.Sprintf("%s, quote %v", c.name, quote)
			b, free := v.buffer(32, c.pre, false)
			cs, _ := libc.CString(c.text + "\"")
			str := XJS_NewStringLen(v.tls, v.ctx, cs, Tsize_t(len(c.text)))
			*(*int32)(unsafe.Pointer(pi)) = 0
			blocks := fillMemory(v.tls, v.rt)
			var failed bool
			if quote {
				failed = jsonQuoteRun(v.tls, b, jsvPtr(str), pi) != 0
			} else {
				failed = jsonStringRun(v.tls, b, cs, cs+uintptr(len(c.text))+1, '"') == 0
			}
			for _, list := range blocks {
				for _, p := range list {
					Xjs_free_rt(v.tls, v.rt, p)
				}
			}
			XJS_SetMemoryLimit(v.tls, v.rt, ^Tsize_t(0))
			if !failed {
				t.Errorf("%s: the run did not fail", what)
			}
			if !v.outOfMemory() {
				t.Errorf("%s: no out of memory error", what)
			}
			if (*TStringBuffer)(unsafe.Pointer(b)).Ferror_status == 0 {
				t.Errorf("%s: the buffer has no error", what)
			}
			free()
			XFreeValue(v.tls, v.ctx, str)
			libc.Xfree(v.tls, cs)
		}
	}
}

// An ASCII run of JSON.parse ends MJS_STRING_LEN_MAX+1 bytes on at the
// latest, so its length fits an int32.
func TestJSONRunLimit(t *testing.T) {
	p := uintptr(0x1000)
	cases := []struct{ end, want uintptr }{
		{p, p}, {p + 1, p + 1}, {p + MJS_STRING_LEN_MAX, p + MJS_STRING_LEN_MAX},
		{p + MJS_STRING_LEN_MAX + 1, p + MJS_STRING_LEN_MAX + 1}, {p + MJS_STRING_LEN_MAX + 2, p + MJS_STRING_LEN_MAX + 1},
		{p + 3<<30, p + MJS_STRING_LEN_MAX + 1},
	}
	if unsafe.Sizeof(p) == 8 {
		sh := uint(32)
		cases = append(cases, struct{ end, want uintptr }{p + 1<<sh - 1, p + MJS_STRING_LEN_MAX + 1}, struct{ end, want uintptr }{p + 1<<sh + 5, p + MJS_STRING_LEN_MAX + 1})
	}
	for _, c := range cases {
		if got := jsonRunLimit(p, c.end); got != c.want {
			t.Errorf("jsonRunLimit(%#x, %#x) = %#x, want %#x", p, c.end, got, c.want)
		}
	}
}

// A run that would make the string longer than MJS_STRING_LEN_MAX throws
// "string too long", as string_buffer_putc did. The buffer is made to look
// that long, so no gigabyte of input is needed.
func TestJSONRunTooLong(t *testing.T) {
	v := newJSONTestVM(t)
	for _, wide := range []bool{false, true} {
		b, free := v.buffer(32, 0, wide)
		sb := (*TStringBuffer)(unsafe.Pointer(b))
		sb.Flen1, sb.Fsize = MJS_STRING_LEN_MAX-3, MJS_STRING_LEN_MAX-3
		text := strings.Repeat("a", 10)
		cs, _ := libc.CString(text + "\"")
		if got := jsonStringRun(v.tls, b, cs, cs+uintptr(len(text))+1, '"'); got != 0 {
			t.Errorf("wide %v: the run did not fail", wide)
		}
		libc.Xfree(v.tls, cs)
		e := XJS_GetException(v.tls, v.ctx)
		m := XToCString(v.tls, v.ctx, e)
		if got := libc.GoString(m); got != "InternalError: string too long" {
			t.Errorf("wide %v: %s, want InternalError: string too long", wide, got)
		}
		XJS_FreeCString(v.tls, v.ctx, m)
		XFreeValue(v.tls, v.ctx, e)
		free()
	}
}
