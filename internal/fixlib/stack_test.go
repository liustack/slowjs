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
