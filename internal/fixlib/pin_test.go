// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// The interpreter of every listed file keeps each value of pinVars in its
// stack frame, and fixPin leaves a fixed file as it is. The other files
// are not changed.
func TestPinEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		listed := pinFiles[filepath.Base(fn)]
		out, err := fixPin(b, listed)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(out, b) {
			t.Errorf("%s: fixPin changed a fixed file", fn)
		}
		body, err := funcBody(b, reCallInternal, "_JS_CallInternal")
		if err != nil {
			t.Fatal(err)
		}
		if n := bytes.Count(body, []byte(pinInGo)); n != map[bool]int{true: 1, false: 0}[listed] {
			t.Errorf("%s: %d lines that keep values in the stack frame", fn, n)
		}
		if listed && !bytes.Contains(body, []byte(" = &this_obj1, &new_target, &argc, ")) {
			t.Errorf("%s: the parameters are not kept", fn)
		}
	}
	if len(pinFiles) != 3 || len(pinVars) != 15 {
		t.Errorf("%d files and %d values, want 3 and 15", len(pinFiles), len(pinVars))
	}
}

const pinBefore = `
func _JS_CallInternal(tls *libc.TLS, caller_ctx uintptr, _func_obj TJSValue, this_obj1 TJSValue, new_target TJSValue, argc int32, argv uintptr, flags int32) (r5 TJSValue) {
	var arg_buf, b1, ctx10, local_buf, rt, sf, stack_buf, var_buf, var_refs uintptr
	var v9 TJSValue
	_, _, _, _, _, _, _, _, _, _ = arg_buf, b1, ctx10, local_buf, rt, sf, stack_buf, var_buf, var_refs, v9
	rt = (*TJSContext)(unsafe.Pointer(caller_ctx)).Frt
	goto _1
_1:
	return v9
}
`

// fixPin puts one line right after the declarations, and a second run
// changes nothing. A file that is not listed is left as it is.
func TestPinShape(t *testing.T) {
	out, err := fixPin([]byte(pinBefore), true)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(pinBefore, "v9\n\trt = ", "v9\n\t_, _, _, _, _, _, _, _, _, _, _, _, _, _, _ = &this_obj1, &new_target, &argc, &argv, &caller_ctx, &flags, &rt, &ctx10, &b1, &sf, &var_buf, &arg_buf, &var_refs, &local_buf, &stack_buf"+pinInGo+"\trt = ", 1)
	if string(out) != want {
		t.Fatalf("got\n%s\nwant\n%s", out, want)
	}
	again, err := fixPin(out, true)
	if err != nil || !bytes.Equal(again, out) {
		t.Fatalf("fixPin changed a fixed file: %v", err)
	}
	same, err := fixPin([]byte(pinBefore), false)
	if err != nil || string(same) != pinBefore {
		t.Fatalf("fixPin changed a file that is not listed: %v", err)
	}
}

// A value of pinVars that the interpreter no longer has, or a line that
// keeps other values, stops fixlib.
func TestPinRejects(t *testing.T) {
	for _, c := range []struct{ name, old, new, want string }{
		{"local gone", "var_refs uintptr", "var_refs2 uintptr", "no var_refs to keep"},
		{"parameter gone", "new_target TJSValue", "new_target1 TJSValue", "no new_target to keep"},
		{"other line", "\trt = ", "\t_ = &v9" + pinInGo + "\trt = ", "other values kept"},
	} {
		if !strings.Contains(pinBefore, c.old) {
			t.Fatalf("%s: %q not in the input", c.name, c.old)
		}
		_, err := fixPin([]byte(strings.Replace(pinBefore, c.old, c.new, 1)), true)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}
