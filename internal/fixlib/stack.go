// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

var (
	reNewRuntime2      = regexp.MustCompile(`\nfunc XJS_NewRuntime2\(tls \*libc\.TLS, [^\n]*\{\n`)
	reDefaultStackSize = regexp.MustCompile(`\n\t\(\*TJSRuntime\)\(unsafe\.Pointer\(rt\)\)\.Fstack_size = ([^\n]+)\n`)
	reStackOverflow    = regexp.MustCompile(`\nfunc _JS_ThrowStackOverflow\(tls \*libc\.TLS, ctx uintptr\) \(r TJSValue\) \{\n\treturn (XJS_Throw\w+Error)\(tls, ctx, (__ccgo_ts\+\d+), 0\)\n\}\n`)
	reNextToken        = regexp.MustCompile(`\nfunc _next_token\(tls \*libc\.TLS, s uintptr\) \(r int32\) \{\n`)
	reJSONNextToken    = regexp.MustCompile(`\nfunc _json_next_token\(tls \*libc\.TLS, s uintptr\) \(r int32\) \{\n`)
	reParseStackError  = regexp.MustCompile(`\n\tif _js_check_stack_overflow\(tls, \(\*TJSContext\)\(unsafe\.Pointer\(\(\*TJSParseState\)\(unsafe\.Pointer\(s\)\)\.Fctx\)\)\.Frt, uint(?:32|64)\(0\)\) != 0 \{\n(\t\treturn _js_parse_error\(tls, s, __ccgo_ts\+(\d+), 0\)\n)\t\}\n`)
)

// stackOverflowMessage is the message of running out of stack, worded as
// in V8.
const stackOverflowMessage = "Maximum call stack size exceeded"

// parseStackOverflow is what the tokenizers do when they run out of stack.
const parseStackOverflow = "\t\t_JS_ThrowStackOverflow(tls, (*TJSParseState)(unsafe.Pointer(s)).Fctx)\n\t\treturn -1\n"

// fixStack makes the stack limit of a new runtime the Go port's default,
// and running out of stack a RangeError, the Go form of the same change in
// lib/internal/patch/quickjs.diff:
//
//   - XJS_NewRuntime2 sets the runtime's stack size to 0. The engine's
//     default is a size in bytes, 1 MB, but the Go port's stack check
//     (_js_check_stack_overflow in lib/libquickjs.go) counts TLS stack
//     slots and reads 0 as its own default, MaxStackSlots. A million slots
//     is far more than the goroutine's stack holds, so recursing without
//     end used to kill the process with a Go stack overflow.
//   - _JS_ThrowStackOverflow throws a RangeError, as browsers do, rather
//     than an InternalError, with V8's message: "Maximum call stack size
//     exceeded" rather than "stack overflow". ccgo keeps each C string
//     literal once in __ccgo_ts1, and "stack overflow" stays there for the
//     regexp parser, so the new message goes at the end of the table, where
//     it moves no other string.
//   - The tokenizers of the parser and of JSON.parse (_next_token and
//     _json_next_token) throw that same error when they run out of stack,
//     as V8's parser does, rather than the syntax error "stack overflow".
func fixStack(b []byte) ([]byte, error) {
	body, err := funcBody(b, reNewRuntime2, "XJS_NewRuntime2")
	if err != nil {
		return nil, err
	}
	m := reDefaultStackSize.FindAllSubmatchIndex(body, -1)
	if len(m) != 1 {
		return nil, fmt.Errorf("XJS_NewRuntime2: want 1 setting of the stack size, found %d", len(m))
	}
	if rhs := body[m[0][2]:m[0][3]]; !bytes.Equal(rhs, []byte("0")) {
		if !bytes.Contains(rhs, []byte("1024")) {
			return nil, fmt.Errorf("XJS_NewRuntime2: the stack size is set to %s, not the 1 MB default", rhs)
		}
		fixed := append(append(append([]byte{}, body[:m[0][2]]...), '0'), body[m[0][3]:]...)
		b = bytes.Replace(b, body, fixed, 1)
	}

	strs, err := stringTable(b)
	if err != nil {
		return nil, err
	}
	old := strs["stack overflow"]
	if old <= 0 {
		return nil, fmt.Errorf("__ccgo_ts1 does not have %q once", "stack overflow")
	}
	msg, ok := strs[stackOverflowMessage]
	if !ok {
		if b, msg, err = appendString(b, stackOverflowMessage); err != nil {
			return nil, err
		}
	} else if msg <= 0 {
		return nil, fmt.Errorf("__ccgo_ts1 has %q more than once", stackOverflowMessage)
	}
	want := fmt.Sprintf("__ccgo_ts+%d", msg)

	loc := reStackOverflow.FindAllSubmatchIndex(b, -1)
	if len(loc) != 1 {
		return nil, fmt.Errorf("_JS_ThrowStackOverflow: cannot find it as one throw of a message")
	}
	throw, str := string(b[loc[0][2]:loc[0][3]]), string(b[loc[0][4]:loc[0][5]])
	if throw != "XJS_ThrowRangeError" || str != want {
		if throw != "XJS_ThrowInternalError" || str != fmt.Sprintf("__ccgo_ts+%d", old) {
			return nil, fmt.Errorf("_JS_ThrowStackOverflow: throws %s of %s, want an XJS_ThrowInternalError of %q", throw, str, "stack overflow")
		}
		b = append(append(append(append(append([]byte{}, b[:loc[0][2]]...), "XJS_ThrowRangeError"...), b[loc[0][3]:loc[0][4]]...), want...), b[loc[0][5]:]...)
	}

	for _, fn := range []struct {
		name string
		re   *regexp.Regexp
	}{{"_next_token", reNextToken}, {"_json_next_token", reJSONNextToken}} {
		body, err := funcBody(b, fn.re, fn.name)
		if err != nil {
			return nil, err
		}
		m := reParseStackError.FindAllSubmatchIndex(body, -1)
		if len(m) > 1 {
			return nil, fmt.Errorf("%s: want 1 stack check, found %d", fn.name, len(m))
		}
		if len(m) == 0 {
			if !bytes.Contains(body, []byte(" != 0 {\n"+parseStackOverflow+"\t}\n")) {
				return nil, fmt.Errorf("%s: cannot find the stack check", fn.name)
			}
			continue
		}
		if s := string(body[m[0][4]:m[0][5]]); s != fmt.Sprint(old) {
			return nil, fmt.Errorf("%s: the stack check throws __ccgo_ts+%s, not %q", fn.name, s, "stack overflow")
		}
		fixed := append(append(append([]byte{}, body[:m[0][2]]...), parseStackOverflow...), body[m[0][3]:]...)
		b = bytes.Replace(b, body, fixed, 1)
	}
	return b, nil
}

// appendString adds a C string to the end of __ccgo_ts1, where it moves no
// other string, and returns its offset.
func appendString(b []byte, str string) ([]byte, int, error) {
	m := reStringTable.FindSubmatchIndex(b)
	if m == nil {
		return nil, 0, fmt.Errorf("cannot find __ccgo_ts1")
	}
	table, err := strconv.Unquote(string(b[m[2]:m[3]]))
	if err != nil {
		return nil, 0, fmt.Errorf("__ccgo_ts1: %v", err)
	}
	if strconv.Quote(str) != `"`+str+`"` {
		return nil, 0, fmt.Errorf("%q needs escapes in __ccgo_ts1", str)
	}
	end := m[3] - 1 // the closing quote
	out := append(append(append([]byte{}, b[:end]...), str+`\x00`...), b[end:]...)
	return out, len(table), nil
}
