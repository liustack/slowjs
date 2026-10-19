// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// fixDefine leaves a fixed file as it is, and puts the call and the bit
// fields back into a file without them, on every target.
func TestDefineIdempotent(t *testing.T) {
	files := libFiles(t)
	if len(files) != 13 {
		t.Fatalf("%d generated files, want 13", len(files))
	}
	for _, fn := range files {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		got, err := fixDefine(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: a second run changes the file", fn)
		}
		if n := bytes.Count(got, []byte("objlitDefine(tls, ")); n != 1 {
			t.Errorf("%s: %d calls of objlitDefine", fn, n)
		}

		m := reDefineValueHead.FindSubmatch(b)
		i := bytes.Index(b, []byte(layoutMark))
		if m == nil || i < 0 {
			t.Fatalf("%s: no call or no bit fields", fn)
		}
		// The layout constants end the file.
		bare := bytes.Replace(b[:i:i], []byte(fmt.Sprintf(defineCall, m[1])), nil, 1)
		if got, err = fixDefine(bare, name); err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: fixDefine does not restore the change", fn)
		}
	}
}

// fixDefine stops when what objlitDefine stands for changes.
func TestDefineRejects(t *testing.T) {
	fn := filepath.Join("..", "..", "lib", "ccgo_darwin_arm64.go")
	b, err := readEngine(fn)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"a changed JS_CreateProperty", "/* add a new property or modify an existing exotic one */", "/* changed */", "changed (fingerprint"},
		{"a changed add_property", "/* try to find an existing shape */", "/* changed */", "changed (fingerprint"},
		{"a changed find_own_property", "func _find_own_property(tls *libc.TLS, ppr uintptr, p uintptr, atom TJSAtom) (r uintptr) {\n", "func _find_own_property(tls *libc.TLS, ppr uintptr, p uintptr, atom TJSAtom) (r uintptr) {\n\t// changed\n", "changed (fingerprint"},
		{"a new signature", "func XJS_DefinePropertyValue(tls *libc.TLS, ctx1 uintptr, this_obj TJSValue, prop TJSAtom, val TJSValue, flags int32) (r int32) {", "func XJS_DefinePropertyValue(tls *libc.TLS, ctx1 uintptr, this_obj TJSValue, prop TJSAtom, val TJSValue, flags uint32) (r int32) {", "signature"},
		{"moved bit fields", "libc.SetBitFieldPtr8Uint8(p+16, libc.Uint8FromInt32(MTRUE), 1, 0x2)", "libc.SetBitFieldPtr8Uint8(p+17, libc.Uint8FromInt32(MTRUE), 1, 0x2)", "bit field 1"},
		// The two lines keep their bits, so JS_NewObjectFromShape alone
		// would make extensible 0x4.
		{"swapped bit fields",
			"\tlibc.SetBitFieldPtr8Uint8(p+16, libc.Uint8FromInt32(MTRUE), 1, 0x2)\n\tlibc.SetBitFieldPtr8Uint8(p+16, libc.Uint8FromInt32(0), 2, 0x4)\n",
			"\tlibc.SetBitFieldPtr8Uint8(p+16, libc.Uint8FromInt32(0), 2, 0x4)\n\tlibc.SetBitFieldPtr8Uint8(p+16, libc.Uint8FromInt32(MTRUE), 1, 0x2)\n",
			"extensible"},
	} {
		if bytes.Count(b, []byte(c.old)) != 1 {
			t.Fatalf("%s: want %q once in the file", c.name, c.old)
		}
		_, err := fixDefine(bytes.Replace(bytes.Clone(b), []byte(c.old), []byte(c.new), 1), filepath.Base(fn))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}

// -define, -backtrace, -regex and -lazyaddr print the fingerprints the
// checks compare with defineSites, backtraceSites, regexSites and
// lazyAddrSites, so that a table updated from them passes.
func TestFingerprintReportMatchesSites(t *testing.T) {
	hw, err := handWritten(filepath.Join("..", "..", "lib"))
	if err != nil {
		t.Fatal(err)
	}
	fn := filepath.Join("..", "..", "lib", "ccgo_darwin_arm64.go")
	name := filepath.Base(fn)
	for _, c := range []struct {
		table string
		fp    func([]byte) (string, error)
		want  string
	}{
		{"defineSites", defineFingerprint, defineSites[name]},
		{"backtraceSites", backtraceFingerprint, backtraceSites[name]},
		{"regexSites", regexFingerprint, regexSites[name]},
		{"lazyAddrSites", lazyAddrFingerprint, lazyAddrSites[name]},
	} {
		got, err := reportedFingerprint(fn, hw, c.fp)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%s: the report gives %s, the table has %s", c.table, got, c.want)
		}
	}
}
