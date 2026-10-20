// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// The two slow functions of every 64-bit target try bitwiseNumbers at their
// entry, and those of the 32-bit targets do not.
// fixBitwise leaves a fixed file as it is, and puts the change back into a
// file without it.
func TestBitwiseIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		want := 0
		if cellFiles[name] {
			want = 2
		}
		if n := len(reBitwiseEntry.FindAll(b, -1)); n != want {
			t.Fatalf("%s: bitwiseNumbers is called %d times, want %d", name, n, want)
		}
		got, err := fixBitwise(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: a fixed file changed", name)
		}
		bare := unfixBitwise(bytes.Clone(b))
		if bytes.Contains(bare, []byte("bitwiseNumbers")) {
			t.Fatalf("%s: the call is still there", name)
		}
		if got, err = fixBitwise(bare, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: the change put back differs", name)
		}
	}
}

// A change to the functions bitwiseNumbers repeats, or to the calls of the
// slow paths in the interpreter, stops fixBitwise.
func TestBitwiseRejects(t *testing.T) {
	b := string(unfixBitwise(engineFile(t)))
	for _, c := range []struct {
		name, fn, old, new, err string
	}{
		{"another shift", "_js_binary_logic_slow", "& uint32(0x1f)", "& uint32(0x3f)", "changed (fingerprint"},
		{"another uint32", "_js_shr_slow", ">> (", ">> 1 >> (", "changed (fingerprint"},
		{"another rounding", "_JS_ToInt32Free", "libc.Int32FromInt32(1023)+libc.Int32FromInt32(30)+libc.Int32FromInt32(53)", "libc.Int32FromInt32(1023)+libc.Int32FromInt32(30)+libc.Int32FromInt32(54)", "changed (fingerprint"},
		{"another number", "_JS_ToNumberHintFree", "case uint32(EJS_TAG_FLOAT64):", "case uint32(EJS_TAG_BOOL):", "changed (fingerprint"},
		{"another call", "_JS_CallInternal", "if _js_shr_slow(tls, ctx10, sp) != 0 {", "if _js_shr_slow(tls, ctx10, sp) < 0 {", "want 5 calls"},
	} {
		body, err := funcBody([]byte(b), funcHead(c.fn), c.fn)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), c.old) {
			t.Fatalf("%s: no %q in %s", c.name, c.old, c.fn)
		}
		_, err = fixBitwise([]byte(strings.Replace(b, string(body), strings.Replace(string(body), c.old, c.new, 1), 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
