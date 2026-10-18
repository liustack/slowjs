// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
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
	up := bytes.Replace(b, []byte("\t\tXJS_SetUncatchableException(tls, ctx, int32(1))\n"), nil, 1)
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

// The end of JS_ThrowError2 in lib/ccgo_darwin_arm64.go as generated.
const throwError2End = "\tobj = XJS_NewObjectProtoClass(tls, ctx1, **(**TJSValue)(__ccgo_up(ctx1 + 176 + uintptr(error_num)*16)), uint32(EJS_CLASS_ERROR))\n" +
	"\tv1 = int32(libc.BoolInt64(!!(int32(obj.Ftag) == int32(EJS_TAG_EXCEPTION))))\n" +
	"\tgoto _2\n" +
	"_2:\n" +
	"\tif libc.BoolInt64(!!(v1 != 0)) != 0 {\n" +
	"\t\t/* out of memory: throw JS_NULL to avoid recursing */\n" +
	"\t\tobj = TJSValue{\n" +
	"\t\t\tFu:   TJSValueUnion{},\n" +
	"\t\t\tFtag: int64(EJS_TAG_NULL),\n" +
	"\t\t}\n" +
	"\t} else {\n" +
	"\t\tv3 = bp\n" +
	"\t\tv4 = XJS_NewStringLen(tls, ctx1, v3, libc.Xstrlen(tls, v3))\n" +
	"\t\tgoto _5\n" +
	"\t_5:\n" +
	"\t\tXJS_DefinePropertyValue(tls, ctx1, obj, uint32(EJS_ATOM_message), v4, libc.Int32FromInt32(1)<<libc.Int32FromInt32(1)|libc.Int32FromInt32(1)<<libc.Int32FromInt32(0))\n" +
	"\t\tif add_backtrace != 0 {\n" +
	"\t\t\t_build_backtrace(tls, ctx1, obj, libc.UintptrFromInt32(0), 0, 0, 0)\n" +
	"\t\t}\n" +
	"\t}\n" +
	"\tret = XJS_Throw(tls, ctx1, obj)\n" +
	"\treturn ret\n}\n"

// fixThrowError2 takes the end as generated, and stops when the end builds
// an error that _js_throw_error2 does not: other property flags, another
// message, other backtrace flags, a call more or less.
func TestOutOfMemoryRejectsOtherError(t *testing.T) {
	b := engineFile(t)
	call := "\treturn _js_throw_error2(tls, ctx1, error_num, bp, add_backtrace)\n}\n"
	if bytes.Count(b, []byte(call)) != 1 {
		t.Fatal("cannot find the call of _js_throw_error2")
	}
	up := strings.Replace(string(b), call, throwError2End, 1)
	out, err := fixThrowError2([]byte(up))
	if err != nil || !bytes.Equal(out, b) {
		t.Fatalf("fixThrowError2 does not give the fixed file back: %v", err)
	}
	for _, c := range []struct{ name, old, new string }{
		{"other flags", "libc.Int32FromInt32(1)<<libc.Int32FromInt32(0))\n", "libc.Int32FromInt32(1)<<libc.Int32FromInt32(0)|libc.Int32FromInt32(1)<<libc.Int32FromInt32(2))\n"},
		{"another message", "\t\tv3 = bp\n", "\t\tv3 = fmt\n"},
		{"other backtrace flags", "libc.UintptrFromInt32(0), 0, 0, 0)", "libc.UintptrFromInt32(0), 0, 0, 1)"},
		{"another class", "uint32(EJS_CLASS_ERROR))\n", "uint32(EJS_CLASS_OBJECT))\n"},
		{"a second property", "\t\tif add_backtrace != 0 {\n", "\t\tXJS_DefinePropertyValue(tls, ctx1, obj, uint32(EJS_ATOM_name), v4, libc.Int32FromInt32(1)<<libc.Int32FromInt32(1)|libc.Int32FromInt32(1)<<libc.Int32FromInt32(0))\n\t\tif add_backtrace != 0 {\n"},
		{"no backtrace", "\t\t\t_build_backtrace(tls, ctx1, obj, libc.UintptrFromInt32(0), 0, 0, 0)\n", ""},
	} {
		if !strings.Contains(throwError2End, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		changed := strings.Replace(up, throwError2End, strings.Replace(throwError2End, c.old, c.new, 1), 1)
		if _, err := fixThrowError2([]byte(changed)); err == nil {
			t.Errorf("%s went through", c.name)
		}
	}
}

// A new place that throws the regexp engine's out of memory error, uses the
// "out of memory" message or calls the regexp engine stops fixlib: it would
// throw a catchable error for running out of memory.
func TestOutOfMemoryRejectsNewUse(t *testing.T) {
	b := engineFile(t)
	strs, err := stringTable(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, body string }{
		{"regexp error", fmt.Sprintf("\tXJS_ThrowInternalError(tls, ctx, __ccgo_ts+%d, 0)\n", strs["out of memory in regexp execution"])},
		{"message", fmt.Sprintf("\tXJS_ThrowRangeError(tls, ctx, __ccgo_ts+%d, 0)\n", strs["out of memory"])},
		{"lre_exec", "\tXlre_exec(tls, 0, 0, 0, 0, 0, 0)\n"},
		{"lre_compile", "\tXlre_compile(tls, 0, 0, 0, 0, 0, 0)\n"},
	} {
		f := "\nfunc _js_new_use(tls *libc.TLS, ctx uintptr) {\n" + c.body + "}\n\nfunc _js_regexp_finalizer("
		_, err := fixOutOfMemory(bytes.Replace(b, []byte("\nfunc _js_regexp_finalizer("), []byte(f), 1))
		if err == nil || !strings.Contains(err.Error(), "_js_new_use") {
			t.Errorf("%s: got %v, want the new function named", c.name, err)
		}
	}
}

// The checks of fixOutOfMemory hold for a fixed file too: a second throw
// in JS_ThrowOutOfMemory, or a second out of memory error in a regexp
// function, stops fixlib.
func TestOutOfMemoryChecksFixedFile(t *testing.T) {
	b := engineFile(t)
	for _, c := range []struct{ head, at, text string }{
		{"func XJS_ThrowOutOfMemory(", "\treturn TJSValue{", "\tXJS_ThrowRangeError(tls, ctx, 0, 0)\n"},
		{"func _js_regexp_exec(", "\t\t\t\tXJS_ThrowOutOfMemory(tls, ctx3)\n", "\t\t\t\tXJS_ThrowOutOfMemory(tls, ctx3)\n"},
	} {
		if _, err := fixOutOfMemory(mutate(t, b, c.head, c.at, c.text)); err == nil {
			t.Errorf("%s: %q more went through", c.head, strings.TrimSpace(c.text))
		}
	}
}

// _js_compile_regexp counts as fixed only with fixlib's whole test of the
// message: the name of the helper elsewhere in it is not enough.
func TestOutOfMemoryRejectsRegexpTextElsewhere(t *testing.T) {
	b := engineFile(t)
	wrapped := "\t\tif _js_regexp_out_of_memory(bp+12) {\n\t\t\tXJS_ThrowOutOfMemory(tls, ctx1)\n\t\t} else {\n\t\t\tXJS_ThrowSyntaxError(tls, ctx1, __ccgo_ts+12059, libc.VaList(bp+88, bp+12))\n\t\t}\n"
	if bytes.Count(b, []byte(wrapped)) != 1 {
		t.Fatal("no fixed test of the message in _js_compile_regexp")
	}
	b = bytes.Replace(b, []byte(wrapped), []byte("\t\tXJS_ThrowSyntaxError(tls, ctx1, __ccgo_ts+12059, libc.VaList(bp+88, bp+12))\n"), 1)
	if _, err := fixOutOfMemory(bytes.Clone(b)); err != nil {
		t.Fatalf("the file with the test taken out: %v", err)
	}
	b = mutate(t, b, "func _js_compile_regexp(", "\tre_bytecode_buf = ", "\t// _js_regexp_out_of_memory(bp+12)\n")
	if _, err := fixOutOfMemory(b); err == nil {
		t.Error("the name of the helper in a comment counted as the test")
	}
}
