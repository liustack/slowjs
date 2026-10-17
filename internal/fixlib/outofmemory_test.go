// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Every generated file makes running out of memory uncatchable, and
// fixOutOfMemory leaves a fixed file as it is.
func TestOutOfMemoryEveryFile(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "lib", "ccgo_*.go"))
	if err != nil || len(files) == 0 {
		t.Fatal(files, err)
	}
	for _, fn := range files {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		out, err := fixOutOfMemory(b)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(out, b) {
			t.Fatalf("%s: fixOutOfMemory changed a fixed file", fn)
		}
		body, err := funcBody(out, reThrowOutOfMemory, "XJS_ThrowOutOfMemory")
		if err != nil || !bytes.Contains(body, []byte("XJS_SetUncatchableException(")) {
			t.Errorf("%s: JS_ThrowOutOfMemory does not make its error uncatchable (%v)", fn, err)
		}
		for _, c := range []struct {
			s string
			n int
		}{
			{"return _js_throw_error2(tls, ", 1},
			{"_js_regexp_out_of_memory(", 1},
			{"/* out of memory: throw JS_NULL to avoid recursing */", 0},
		} {
			if n := bytes.Count(out, []byte(c.s)); n != c.n {
				t.Errorf("%s: %d of %q, want %d", fn, n, c.s, c.n)
			}
		}
	}
}

// fixOutOfMemory takes a generated file before the change to one after it.
func TestOutOfMemoryFromUpstream(t *testing.T) {
	b := engineFile(t)
	strs, err := stringTable(b)
	if err != nil {
		t.Fatal(err)
	}

	// Undo the change in XJS_ThrowOutOfMemory and the regexp functions.
	up := bytes.Replace(b, []byte("\t\tXJS_SetUncatchableException(tls, ctx, int32(MTRUE))\n"), nil, 1)
	exec := []byte("XJS_ThrowInternalError(tls, ctx3, __ccgo_ts+" + strconv.Itoa(strs["out of memory in regexp execution"]) + ", 0)")
	re := regexp.MustCompile(`\nfunc _js_regexp_exec\(`)
	body, err := funcBody(up, re, "_js_regexp_exec")
	if err != nil {
		t.Fatal(err)
	}
	up = bytes.Replace(up, body, bytes.Replace(body, []byte("XJS_ThrowOutOfMemory(tls, ctx3)"), exec, 1), 1)
	if bytes.Equal(up, b) {
		t.Fatal("nothing undone")
	}

	out, err := fixOutOfMemory(up)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, b) {
		t.Fatal("fixOutOfMemory does not give the fixed file back")
	}
}

// A new call in the end of JS_ThrowError2 stops fixlib.
func TestOutOfMemoryRejectsNewThrowError2(t *testing.T) {
	b := engineFile(t)
	// Put the upstream end back with one call more.
	call := []byte("\treturn _js_throw_error2(tls, ctx1, error_num, bp, add_backtrace)\n}\n")
	end := []byte("\tobj = XJS_NewObjectProtoClass(tls, ctx1, proto, uint32(EJS_CLASS_ERROR))\n\t/* out of memory: throw JS_NULL to avoid recursing */\n\tv4 = XJS_NewStringLen(tls, ctx1, v3, libc.Xstrlen(tls, v3))\n\tXJS_DefinePropertyValue(tls, ctx1, obj, uint32(EJS_ATOM_message), v4, 3)\n\t_js_new_call(tls, ctx1)\n\tif add_backtrace != 0 {\n\t}\n\tret = XJS_Throw(tls, ctx1, obj)\n\treturn ret\n}\n")
	if bytes.Count(b, call) != 1 {
		t.Fatal("cannot find the call of _js_throw_error2")
	}
	_, err := fixOutOfMemory(bytes.Replace(b, call, end, 1))
	if err == nil || !strings.Contains(err.Error(), "_js_new_call") {
		t.Fatalf("got %v, want the new call named", err)
	}
}
