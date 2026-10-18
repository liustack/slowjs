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
	"strings"
	"testing"
)

// Every generated file gives a new runtime the Go port's stack limit and
// throws V8's RangeError for running out of stack, in the parser too, and
// fixStack leaves a fixed file as it is.
func TestStackEveryFile(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "lib", "ccgo_*.go"))
	if err != nil || len(files) == 0 {
		t.Fatal(files, err)
	}
	for _, fn := range files {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		out, err := fixStack(b)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(out, b) {
			t.Fatalf("%s: fixStack changed a fixed file", fn)
		}
		body, err := funcBody(out, reNewRuntime2, "XJS_NewRuntime2")
		if err != nil || !bytes.Contains(body, []byte("\t(*TJSRuntime)(unsafe.Pointer(rt)).Fstack_size = 0\n")) {
			t.Errorf("%s: a new runtime does not start with the default stack limit (%v)", fn, err)
		}
		strs, err := stringTable(out)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if m := reStackOverflow.FindSubmatch(out); m == nil || string(m[1]) != "XJS_ThrowRangeError" || string(m[2]) != fmt.Sprintf("__ccgo_ts+%d", strs[stackOverflowMessage]) {
			t.Errorf("%s: running out of stack is not a RangeError of %q", fn, stackOverflowMessage)
		}
		for _, name := range []string{"_next_token", "_json_next_token"} {
			re := regexp.MustCompile(`\nfunc ` + name + `\(`)
			body, err := funcBody(out, re, name)
			if err != nil || !bytes.Contains(body, []byte(parseStackOverflow)) || reParseStackError.Match(body) {
				t.Errorf("%s: %s does not throw the RangeError when it runs out of stack (%v)", fn, name, err)
			}
		}
	}
}

// undoStack takes a fixed generated file back to the shape fixStack takes:
// the engine's stack size, an InternalError of "stack overflow", the
// syntax error in the tokenizers, and no message of V8's in __ccgo_ts1.
func undoStack(t *testing.T, b []byte) []byte {
	t.Helper()
	strs, err := stringTable(b)
	if err != nil {
		t.Fatal(err)
	}
	up := bytes.Replace(b, []byte("\t(*TJSRuntime)(unsafe.Pointer(rt)).Fstack_size = 0\n"), []byte("\t(*TJSRuntime)(unsafe.Pointer(rt)).Fstack_size = libc.Uint64FromInt32(libc.Int32FromInt32(1024) * libc.Int32FromInt32(1024))\n"), 1)
	m := reStackOverflow.FindSubmatchIndex(up)
	if m == nil {
		t.Fatal("cannot find _JS_ThrowStackOverflow")
	}
	old := fmt.Sprintf("__ccgo_ts+%d", strs["stack overflow"])
	up = append(append(append(append(append([]byte{}, up[:m[2]]...), "XJS_ThrowInternalError"...), up[m[3]:m[4]]...), old...), up[m[5]:]...)
	parse := "\t\treturn _js_parse_error(tls, s, " + old + ", 0)\n"
	if n := bytes.Count(up, []byte(parseStackOverflow)); n != 2 {
		t.Fatalf("found the tokenizers' stack overflow %d times, want 2", n)
	}
	up = bytes.ReplaceAll(up, []byte(parseStackOverflow), []byte(parse))
	tail := []byte(stackOverflowMessage + `\x00"` + "\n")
	if bytes.Count(up, tail) != 1 {
		t.Fatal("the message is not at the end of __ccgo_ts1")
	}
	return bytes.Replace(up, tail, []byte(`"`+"\n"), 1)
}

// fixStack takes a generated file before the change to one after it.
func TestStackFromUpstream(t *testing.T) {
	b := engineFile(t)
	up := undoStack(t, b)
	if strs, err := stringTable(up); err != nil || strs[stackOverflowMessage] != 0 {
		t.Fatalf("the message is still in __ccgo_ts1 (%v)", err)
	}
	if bytes.Equal(up, b) {
		t.Fatal("nothing undone")
	}

	out, err := fixStack(up)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, b) {
		t.Fatal("fixStack does not give the fixed file back")
	}
}

// A new runtime whose stack size is not the 1 MB default stops fixlib.
func TestStackRejectsOtherDefault(t *testing.T) {
	b := engineFile(t)
	up := bytes.Replace(b, []byte("\t(*TJSRuntime)(unsafe.Pointer(rt)).Fstack_size = 0\n"), []byte("\t(*TJSRuntime)(unsafe.Pointer(rt)).Fstack_size = libc.Uint64FromInt32(libc.Int32FromInt32(256) * libc.Int32FromInt32(256))\n"), 1)
	_, err := fixStack(up)
	if err == nil || !strings.Contains(err.Error(), "not the 1 MB default") {
		t.Fatalf("got %v, want the other default named", err)
	}
}

// A second setting of the stack size right after the first stops fixlib:
// the last one wins, and fixStack would only set the first to 0.
func TestStackRejectsSecondDefault(t *testing.T) {
	line := []byte("\t(*TJSRuntime)(unsafe.Pointer(rt)).Fstack_size = 0\n")
	b := bytes.Replace(engineFile(t), line, append(append([]byte{}, line...), "\t(*TJSRuntime)(unsafe.Pointer(rt)).Fstack_size = uint64(1024 * 1024)\n"...), 1)
	if _, err := fixStack(b); err == nil {
		t.Fatal("a second setting of the stack size went through")
	}
}

// A new use of the "stack overflow" message outside the regexp parser stops
// fixlib: it would throw something other than the RangeError.
func TestStackRejectsNewOverflowError(t *testing.T) {
	b := engineFile(t)
	strs, err := stringTable(b)
	if err != nil {
		t.Fatal(err)
	}
	f := fmt.Sprintf("\nfunc _js_new_check(tls *libc.TLS, ctx uintptr) {\n\tXJS_ThrowInternalError(tls, ctx, __ccgo_ts+%d, 0)\n}\n\nfunc _js_regexp_finalizer(", strs["stack overflow"])
	_, err = fixStack(bytes.Replace(b, []byte("\nfunc _js_regexp_finalizer("), []byte(f), 1))
	if err == nil || !strings.Contains(err.Error(), "_js_new_check") {
		t.Fatalf("got %v, want the new function named", err)
	}
}

// A tokenizer whose stack check is gone, with a throw of the RangeError
// left for some other test, stops fixlib: only the whole check counts as
// made.
func TestStackRejectsOtherCheck(t *testing.T) {
	b := engineFile(t)
	body, err := funcBody(b, reNextToken, "_next_token")
	if err != nil {
		t.Fatal(err)
	}
	other := regexp.MustCompile(`if _js_check_stack_overflow\([^\n]*\) != 0 \{\n\t\t_JS_ThrowStackOverflow`).ReplaceAll(body, []byte("if _other_check(tls) != 0 {\n\t\t_JS_ThrowStackOverflow"))
	if bytes.Equal(other, body) {
		t.Fatal("cannot find the stack check")
	}
	if _, err := fixStack(bytes.Replace(b, body, other, 1)); err == nil {
		t.Fatal("a tokenizer without its stack check went through")
	}
}
