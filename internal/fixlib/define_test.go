// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

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
	if len(files) != 5 {
		t.Fatalf("%d generated files, want 5", len(files))
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
		// Without them, fixDefine puts the constants at the end.
		bare := bytes.Replace(cutConstants(bytes.Clone(b), layoutMark), []byte(fmt.Sprintf(defineCall, m[1])), nil, 1)
		end := bytes.Index(b[i:], []byte("\n)\n"))
		want := append(cutConstants(bytes.Clone(b), layoutMark), b[i:i+end+3]...)
		if got, err = fixDefine(bare, name); err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(got, want) {
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

// -define, -backtrace, -regex, -lazyaddr, -inline, -oneframe, -funcshape,
// -recache and -ownkeys print the fingerprints the checks compare with
// defineSites, backtraceSites, regexSites, lazyAddrSites, inlineSites,
// oneFrameSites, funcShapeSites, regexCacheSites and ownKeysSites, so that a
// table updated from them passes.
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
		{"inlineSites", inlineFingerprint, inlineSites[name]},
		{"oneFrameSites", oneFrameFingerprint, oneFrameSites[name]},
		{"funcShapeSites", funcShapeFingerprint, funcShapeSites[name]},
		{"regexCacheSites", regexCacheFingerprint, regexCacheSites[name]},
		{"ownKeysSites", ownKeysFingerprint, ownKeysSites[name]},
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

// -forin, -bitwise, -eq, -lazyaddr, -json, -atomkey, -regex, -funcshape,
// -recache and -ownkeys print the fingerprints even when the table does not
// match the files, which is when they are needed: the check of the table
// must not stop the report.
func TestFingerprintReportIgnoresSites(t *testing.T) {
	hw, err := handWritten(filepath.Join("..", "..", "lib"))
	if err != nil {
		t.Fatal(err)
	}
	fn := filepath.Join("..", "..", "lib", "ccgo_darwin_arm64.go")
	name := filepath.Base(fn)
	for _, c := range []struct {
		name  string
		flag  *bool
		table map[string]string
		fp    func([]byte) (string, error)
	}{
		{"-forin", forIn, forInSites, forInFingerprint},
		{"-bitwise", bitwise, bitwiseSites, bitwiseFingerprint},
		{"-eq", eq, eqSites, eqFingerprint},
		{"-json", jsonF, jsonSites, jsonFingerprint},
		{"-atomkey", atomkey, atomKeySites, atomKeyFingerprint},
		{"-lazyaddr", lazyAddr, lazyAddrSites, lazyAddrFingerprint},
		{"-regex", regex, regexSites, regexFingerprint},
		{"-funcshape", fnShape, funcShapeSites, funcShapeFingerprint},
		{"-recache", recache, regexCacheSites, regexCacheFingerprint},
		{"-ownkeys", ownKeysF, ownKeysSites, ownKeysFingerprint},
	} {
		want := c.table[name]
		*c.flag, c.table[name] = true, "0"
		got, err := reportedFingerprint(fn, hw, c.fp)
		*c.flag, c.table[name] = false, want
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
		} else if got != want {
			t.Errorf("%s: the report gives %s, want %s", c.name, got, want)
		}
	}
}
