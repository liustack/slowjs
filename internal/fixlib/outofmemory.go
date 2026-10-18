// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	reThrowOutOfMemory = regexp.MustCompile(`\nfunc XJS_ThrowOutOfMemory\(tls \*libc\.TLS, (\w+) uintptr\) \(r TJSValue\) \{\n`)
	reThrowError2      = regexp.MustCompile(`\nfunc _JS_ThrowError2\(tls \*libc\.TLS, (\w+) uintptr, error_num TJSErrorEnum, fmt uintptr, ap Tva_list, add_backtrace TBOOL\) \(r TJSValue\) \{\n`)
	reCompileRegexp    = regexp.MustCompile(`\nfunc _js_compile_regexp\(tls \*libc\.TLS, (\w+) uintptr, [^\n]*\{\n`)
	reCallName         = regexp.MustCompile(`\b([A-Za-z_][\w.]*)\(`)
	reStringTable      = regexp.MustCompile(`\nvar __ccgo_ts1 = ("[^\n]*")\n`)
)

// throwError2Calls are the functions the end of JS_ThrowError2 calls, from
// building the error object to throwing it, which _js_throw_error2 does in
// the same way.
// The first five are called once each.
var throwError2Calls = []string{
	"XJS_NewObjectProtoClass", "XJS_NewStringLen", "XJS_DefinePropertyValue", "XJS_Throw",
	"_build_backtrace", "__ccgo_up", "int32", "int64", "uint32", "uintptr", "uint64",
}

// fixOutOfMemory makes running out of memory uncatchable, the Go form of
// the same change in lib/internal/patch/quickjs.diff (see
// lib/outofmemory.go):
//
//   - XJS_ThrowOutOfMemory marks the error it throws uncatchable.
//   - _JS_ThrowError2 hands the formatted message to _js_throw_error2.
//   - _js_regexp_exec and _js_regexp_replace throw the out of memory error
//     when the regexp engine runs out of memory, and _js_compile_regexp
//     does when lre_compile does.
func fixOutOfMemory(b []byte) ([]byte, error) {
	strs, err := stringTable(b)
	if err != nil {
		return nil, err
	}

	// JS_ThrowOutOfMemory: uncatchable.
	body, err := funcBody(b, reThrowOutOfMemory, "XJS_ThrowOutOfMemory")
	if err != nil {
		return nil, err
	}
	ctx := string(reThrowOutOfMemory.FindSubmatch(b)[1])
	throw := fmt.Sprintf("\t\tXJS_ThrowInternalError(tls, %s, __ccgo_ts+%d, 0)\n", ctx, strs["out of memory"])
	mark := fmt.Sprintf("\t\tXJS_SetUncatchableException(tls, %s, int32(1))\n", ctx)
	if !bytes.Contains(body, []byte(throw+mark)) {
		if bytes.Count(body, []byte(throw)) != 1 || bytes.Count(body, []byte("XJS_Throw")) != 2 { // its name and the call
			return nil, fmt.Errorf("XJS_ThrowOutOfMemory: want 1 call, of XJS_ThrowInternalError with \"out of memory\"")
		}
		b = bytes.Replace(b, body, bytes.Replace(body, []byte(throw), []byte(throw+mark), 1), 1)
	}

	// JS_ThrowError2: the end in Go.
	if b, err = fixThrowError2(b); err != nil {
		return nil, err
	}

	// The regexp engine running out of memory.
	exec := fmt.Sprintf(", __ccgo_ts+%d, 0)", strs["out of memory in regexp execution"])
	for _, name := range []string{"_js_regexp_exec", "_js_regexp_replace"} {
		re := regexp.MustCompile(`\nfunc ` + name + `\(tls \*libc\.TLS, (\w+) uintptr, [^\n]*\{\n`)
		if body, err = funcBody(b, re, name); err != nil {
			return nil, err
		}
		ctx := string(re.FindSubmatch(b)[1])
		old := "XJS_ThrowInternalError(tls, " + ctx + exec
		switch n := bytes.Count(body, []byte(old)); {
		case n == 1:
			b = bytes.Replace(b, body, bytes.Replace(body, []byte(old), []byte("XJS_ThrowOutOfMemory(tls, "+ctx+")"), 1), 1)
		case n > 1 || !bytes.Contains(body, []byte("XJS_ThrowOutOfMemory(tls, "+ctx+")")):
			return nil, fmt.Errorf("%s: want 1 out of memory error in regexp execution", name)
		}
	}

	if body, err = funcBody(b, reCompileRegexp, "_js_compile_regexp"); err != nil {
		return nil, err
	}
	ctx = string(reCompileRegexp.FindSubmatch(b)[1])
	if !bytes.Contains(body, []byte("_js_regexp_out_of_memory(")) {
		m := regexp.MustCompile(`/\* error_msg at (bp\+\d+) \*/`).FindSubmatch(body)
		if m == nil {
			return nil, fmt.Errorf("_js_compile_regexp: cannot find error_msg")
		}
		msg := string(m[1])
		re := regexp.MustCompile(`\n\tif !\(re_bytecode_buf != 0\) \{\n\t\t(XJS_ThrowSyntaxError\(tls, ` + ctx + fmt.Sprintf(`, __ccgo_ts\+%d, libc\.VaList\(bp\+\d+, `, strs["%s"]) + regexp.QuoteMeta(msg) + `\)\))\n`)
		loc := re.FindAllSubmatchIndex(body, -1)
		if len(loc) != 1 {
			return nil, fmt.Errorf("_js_compile_regexp: want 1 SyntaxError with the message of lre_compile")
		}
		call := string(body[loc[0][2]:loc[0][3]])
		fixed := fmt.Sprintf("\n\tif !(re_bytecode_buf != 0) {\n\t\tif _js_regexp_out_of_memory(%s) {\n\t\t\tXJS_ThrowOutOfMemory(tls, %s)\n\t\t} else {\n\t\t\t%s\n\t\t}\n", msg, ctx, call)
		out := append(append(append([]byte{}, body[:loc[0][0]]...), fixed...), body[loc[0][1]:]...)
		b = bytes.Replace(b, body, out, 1)
	}
	if err := checkOutOfMemoryUses(b, strs); err != nil {
		return nil, err
	}
	return b, nil
}

// checkOutOfMemoryUses checks that no other function throws the regexp
// engine's out of memory error, uses the "out of memory" message or calls
// the regexp engine: each would throw a catchable error for running out of
// memory. _re_parse_out_of_memory gives lre_compile the message
// _js_regexp_out_of_memory looks for.
func checkOutOfMemoryUses(b []byte, strs map[string]int) error {
	str := func(s string) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf(`__ccgo_ts\+%d\b`, strs[s]))
	}
	for _, c := range []struct {
		what string
		re   *regexp.Regexp
		want map[string]int
	}{
		{"the out of memory error of regexp execution", str("out of memory in regexp execution"), nil},
		{`the "out of memory" message`, str("out of memory"), map[string]int{"XJS_ThrowOutOfMemory": 1, "_re_parse_out_of_memory": 1}},
		{"lre_exec", regexp.MustCompile(`\bXlre_exec\(`), map[string]int{"_js_regexp_exec": 1, "_js_regexp_replace": 1}},
		{"lre_compile", regexp.MustCompile(`\bXlre_compile\(`), map[string]int{"_js_compile_regexp": 1}},
	} {
		if diff := diffCounts(countPerFunc(b, c.re), c.want); len(diff) != 0 {
			return fmt.Errorf("uses of %s differ: %s: running out of memory there must throw XJS_ThrowOutOfMemory", c.what, strings.Join(diff, ", "))
		}
	}
	return nil
}

// fixThrowError2 replaces the end of _JS_ThrowError2, from building the
// error object on, with a call of _js_throw_error2.
func fixThrowError2(b []byte) ([]byte, error) {
	body, err := funcBody(b, reThrowError2, "_JS_ThrowError2")
	if err != nil {
		return nil, err
	}
	ctx := string(reThrowError2.FindSubmatch(b)[1])
	call := fmt.Sprintf("\treturn _js_throw_error2(tls, %s, error_num, bp, add_backtrace)\n}\n", ctx)
	if bytes.HasSuffix(body, []byte(call)) {
		return b, nil
	}

	if !regexp.MustCompile(`\n\tvar _ /\* buf at bp\+0 \*/ \[256\]u?int8\n`).Match(body) {
		return nil, fmt.Errorf("_JS_ThrowError2: buf is not at bp+0")
	}
	start := bytes.Index(body, []byte("\tobj = XJS_NewObjectProtoClass("))
	end := []byte(fmt.Sprintf("\tret = XJS_Throw(tls, %s, obj)\n\treturn ret\n}\n", ctx))
	if start < 0 || !bytes.HasSuffix(body, end) {
		return nil, fmt.Errorf("_JS_ThrowError2: cannot find the end that builds and throws the error")
	}
	tail := body[start:]
	// The end does what _js_throw_error2 does: no call more, and the
	// fallback for an object it cannot build.
	for _, m := range reCallName.FindAllSubmatch(tail, -1) {
		if !slices.Contains(throwError2Calls, string(m[1])) && !strings.HasPrefix(string(m[1]), "libc.") {
			return nil, fmt.Errorf("_JS_ThrowError2: the end calls %s", m[1])
		}
	}
	calls := map[string]int{}
	for _, m := range reCallName.FindAllSubmatch(tail, -1) {
		calls[string(m[1])]++
	}
	for _, name := range throwError2Calls[:5] {
		if calls[name] != 1 {
			return nil, fmt.Errorf("_JS_ThrowError2: the end calls %s %d times, want once", name, calls[name])
		}
	}
	// Each call as _js_throw_error2 makes it: an Error of the context's
	// prototype for error_num, the message from buf, writable and
	// configurable, and a backtrace without a file name.
	c := regexp.QuoteMeta(ctx)
	msg := regexp.MustCompile(`\n\t+(\w+) = bp\n\t+(\w+) = XJS_NewStringLen\(tls, ` + c + `, (\w+), libc\.Xstrlen\(tls, (\w+)\)\)\n`).FindSubmatch(tail)
	if msg == nil || string(msg[1]) != string(msg[3]) || string(msg[1]) != string(msg[4]) {
		return nil, fmt.Errorf("_JS_ThrowError2: the end makes the message some other way than from buf")
	}
	for _, want := range []string{
		`\tobj = XJS_NewObjectProtoClass\(tls, ` + c + `, ` + reDeref(`TJSValue`, c+` \+ \d+ \+ uintptr\(error_num\)\*\d+`) + `, uint32\(EJS_CLASS_ERROR\)\)\n`,
		`\n\t+/\* out of memory: throw JS_NULL to avoid recursing \*/\n`,
		`\n\t+XJS_DefinePropertyValue\(tls, ` + c + `, obj, uint32\(EJS_ATOM_message\), ` + string(msg[2]) + `, libc\.Int32FromInt32\(1\)<<libc\.Int32FromInt32\(1\)\|libc\.Int32FromInt32\(1\)<<libc\.Int32FromInt32\(0\)\)\n`,
		`\n\t+if add_backtrace != 0 \{\n\t+_build_backtrace\(tls, ` + c + `, obj, libc\.UintptrFromInt32\(0\), 0, 0, 0\)\n\t+\}\n`,
	} {
		if !regexp.MustCompile(want).Match(tail) {
			return nil, fmt.Errorf("_JS_ThrowError2: the end does not match %s: compare it with _js_throw_error2", want)
		}
	}
	fixed := append(append([]byte{}, body[:start]...), call...)
	return bytes.Replace(b, body, fixed, 1), nil
}

// stringTable returns the offset in __ccgo_ts of each C string literal of
// the file that occurs there once.
func stringTable(b []byte) (map[string]int, error) {
	m := reStringTable.FindSubmatch(b)
	if m == nil {
		return nil, fmt.Errorf("cannot find __ccgo_ts1")
	}
	s, err := strconv.Unquote(string(m[1]))
	if err != nil {
		return nil, fmt.Errorf("__ccgo_ts1: %v", err)
	}
	strs := map[string]int{}
	off := 0
	for _, str := range strings.SplitAfter(s, "\x00") {
		lit, ok := strings.CutSuffix(str, "\x00")
		if _, dup := strs[lit]; dup {
			strs[lit] = -1
		} else if ok {
			strs[lit] = off
		}
		off += len(str)
	}
	for _, want := range []string{"out of memory", "out of memory in regexp execution", "%s"} {
		if strs[want] <= 0 {
			return nil, fmt.Errorf("__ccgo_ts1 does not have %q once", want)
		}
	}
	return strs, nil
}
