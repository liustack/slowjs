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

// The constants fixJSONStrings writes on every target: s390x is
// big-endian, so ccgo numbers the bit fields from the other end.
var jsonLayoutWant = map[bool]string{
	false: "const (\n\tjsonStrLenOff    = 0\n\tjsonStrLenMask   = 0x7fffffff\n\tjsonStrLenShift  = 0\n\tjsonStrWideOff   = 0\n\tjsonStrWideMask  = 0x80000000\n\tjsonStrWideShift = 31\n\tjsonStrData      = 12\n)\n",
	true:  "const (\n\tjsonStrLenOff    = 0\n\tjsonStrLenMask   = 0xfffffffe\n\tjsonStrLenShift  = 1\n\tjsonStrWideOff   = 0\n\tjsonStrWideMask  = 0x1\n\tjsonStrWideShift = 0\n\tjsonStrData      = 12\n)\n",
}

// On every target, JS_ToQuotedString and json_parse_string each call their
// run once, at the start of the loop over the characters, and the file has
// the layout of its own JSString. fixJSONStrings leaves a fixed file as it
// is and puts the change back into a file without it, and
// restoreJSONStrings gives back the text without it, byte for byte.
func TestJSONStringsEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		for _, c := range []struct{ fn, call string }{
			{"_JS_ToQuotedString", "\tfor {\n" + jsonQuoteMark + "\t\tif jsonQuoteRun(tls, b, p1, bp"},
			{"_json_parse_string", "\tfor {\n" + jsonParseCall + "\t\tif p >= "},
		} {
			body, err := funcBody(b, funcHead(c.fn), c.fn)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if n := bytes.Count(body, []byte("Run(tls, ")); n != 1 || !bytes.Contains(body, []byte(c.call)) {
				t.Errorf("%s: %s: %d calls of a run, want one at the start of the loop", name, c.fn, n)
			}
		}
		if !bytes.Contains(b, []byte(jsonLayout+jsonLayoutWant[name == "ccgo_linux_s390x.go"])) {
			t.Errorf("%s: not the layout of a JSString on this target", name)
		}

		again, err := fixJSONStrings(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(again, b) {
			t.Errorf("%s: a fixed file changed", name)
		}
		plain, err := restoreJSONStrings(bytes.Clone(b))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Contains(plain, []byte("jsonQuoteRun")) || bytes.Contains(plain, []byte("jsonStringRun")) || bytes.Contains(plain, []byte("jsonStr")) {
			t.Fatalf("%s: restoreJSONStrings leaves part of the change", name)
		}
		fixed, err := fixJSONStrings(bytes.Clone(plain), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(fixed, b) {
			t.Errorf("%s: fixJSONStrings of the text without the change does not give the file", name)
		}
	}
}

// A change to a function the runs repeat, or to the text fixJSONStrings
// reads, stops it.
func TestJSONStringsRejects(t *testing.T) {
	fixed := string(engineFile(t))
	plainB, err := restoreJSONStrings([]byte(fixed))
	if err != nil {
		t.Fatal(err)
	}
	plain := string(plainB)
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"kept characters", plain, "\t\tif v15 = c1 < uint32(32); !v15 {\n", "\t\tif v15 = c1 < uint32(31); !v15 {\n", "changed (fingerprint"},
		{"getc", plain, "func _string_getc(tls *libc.TLS, p uintptr, pidx uintptr) (r int32) {\n", "func _string_getc(tls *libc.TLS, p uintptr, pidx uintptr) (r int32) {\n\t_ = 0\n", "changed (fingerprint"},
		{"UTF-8", plain, "\tif l > max_len-int32(1) {\n", "\tif l > max_len {\n", "changed (fingerprint"},
		{"write16", plain, "func _string_buffer_write16(tls *libc.TLS, s uintptr, p uintptr, len1 int32) (r int32) {\n", "func _string_buffer_write16(tls *libc.TLS, s uintptr, p uintptr, len1 int32) (r int32) {\n\t_ = 0\n", "changed (fingerprint"},
		{"another quote loop", plain, "\t\tc1 = libc.Uint32FromInt32(_string_getc(tls, p1, bp+32))\n", "\t\tc1 = libc.Uint32FromInt32(_string_getc(tls, p1, bp+32)) + 0\n", "_JS_ToQuotedString: want one loop"},
		{"another index", plain, "\t\tc1 = libc.Uint32FromInt32(_string_getc(tls, p1, bp+32))\n", "\t\tc1 = libc.Uint32FromInt32(_string_getc(tls, p1, bp+40))\n", "string_getc moves it at bp+40"},
		{"another parse loop", plain, "\t\t\tgoto end_of_input\n\t\t}\n\t\tv2 = p\n", "\t\t\tgoto end_of_input\n\t\t}\n\t\tv2 = p + 0\n", "_json_parse_string: want one loop"},
		{"another wide flag", plain, "\tif int32(Tuint8_t(*(*uint32)(unsafe.Pointer(p + 0))&0x80000000>>31)) != 0 {\n\t\tv1 = libc.Int32FromUint16(*(*Tuint16_t)(unsafe.Pointer(p + 12 + uintptr(idx)*2)))\n", "\tif int32(Tuint8_t((*TJSString)(unsafe.Pointer(p)).Fis_wide_char)) != 0 {\n\t\tv1 = libc.Int32FromUint16(*(*Tuint16_t)(unsafe.Pointer(p + 12 + uintptr(idx)*2)))\n", "string_get: want one read"},
		{"a call left", fixed, jsonParseCall, jsonParseMark + "\t\tif p = jsonStringRun(tls, b, p, end, sep); p == 0 {\n\t\t\tgoto fail\n\t\t}\n", "a call of jsonQuoteRun or jsonStringRun is left"},
		{"another quote call", fixed, "\t\tif jsonQuoteRun(tls, b, p1, bp+32) != 0 {\n", "\t\tif jsonQuoteRun(tls, b, p, bp+32) != 0 {\n", "not what fixJSONStrings writes"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		in := strings.Replace(c.in, c.old, c.new, 1)
		var err error
		if c.in == plain {
			_, err = fixJSONStrings([]byte(in), "ccgo_darwin_arm64.go")
		} else {
			_, err = restoreJSONStrings([]byte(in))
		}
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
