// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every 64-bit target calls fieldProto and fieldGoesOn once in get_field
// and once in get_field2, with the fast_array bit of its own
// JS_GetPropertyInternal.
func TestFieldChainEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		want := 0
		if cellFiles[name] {
			want = 2
		}
		for _, call := range []string{"fieldProto(tls, ", "fieldGoesOn(tls, "} {
			if n := bytes.Count(b, []byte(call)); n != want {
				t.Errorf("%s: %d calls of %s, want %d", name, n, call, want)
			}
		}
		if want == 0 {
			if bytes.Contains(b, []byte(fieldLayout)) {
				t.Errorf("%s: constants of fixFieldChain in a 32-bit file", name)
			}
			continue
		}
		off, mask, err := fieldChainConsts(b)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		const wantMask = "0x10"
		if off != "16" || mask != wantMask {
			t.Errorf("%s: fast_array at %s mask %s, want 16 mask %s", name, off, mask, wantMask)
		}
	}
}

// fixFieldChain leaves a fixed file as it is, and puts the change back into
// a file without it, on every target. restoreFieldChain gives back the text
// ccgo writes, byte for byte.
func TestFieldChainIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if bytes.Contains(b, []byte("fieldProto(")) || bytes.Contains(b, []byte(fieldLayout)) {
			t.Fatalf("%s: readEngine leaves the change in", name)
		}
		fixed, err := fixFieldChain(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Equal(fixed, b) != !cellFiles[name] {
			t.Fatalf("%s: fixFieldChain changed the file: %v, want %v", name, !bytes.Equal(fixed, b), cellFiles[name])
		}
		again, err := fixFieldChain(bytes.Clone(fixed), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(again, fixed) {
			t.Errorf("%s: a fixed file changed", name)
		}
		back, err := restoreFieldChain(fixed)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(back, b) {
			t.Errorf("%s: restoreFieldChain does not give the text back", name)
		}
	}
}

// The fast_array bit comes from the file's own JS_GetPropertyInternal.
func TestFieldChainReadsBitField(t *testing.T) {
	b := string(engineFile(t))
	old := "\t\t\t/* exotic behaviors */\n\t\t\tif int32(*(*uint8)(unsafe.Pointer(p2 + 16))&0x10>>4) != 0 {\n"
	if !strings.Contains(b, old) {
		t.Fatalf("no %q", old)
	}
	in := strings.Replace(b, old, "\t\t\t/* exotic behaviors */\n\t\t\tif int32(*(*uint8)(unsafe.Pointer(p2 + 17))&0x4>>2) != 0 {\n", 1)
	out, _, err := planFieldChain([]byte(in), true)
	if err != nil {
		t.Fatal(err)
	}
	off, mask, err := fieldChainConsts(out)
	if err != nil {
		t.Fatal(err)
	}
	if off != "17" || mask != "0x4" {
		t.Errorf("fast_array at %s mask %s, want 17 mask 0x4", off, mask)
	}
}

// A change to a function fieldProto or fieldGoesOn repeats, or to the text
// fixFieldChain reads, stops fixFieldChain.
func TestFieldChainRejects(t *testing.T) {
	b := string(engineFile(t))
	fixed, err := fixFieldChain([]byte(b), "ccgo_darwin_arm64.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"string prototype", b, "\t\tval = *(*TJSValue)(unsafe.Pointer((*TJSContext)(unsafe.Pointer(ctx)).Fclass_proto + uintptr(EJS_CLASS_STRING)*16))\n", "\t\tval = *(*TJSValue)(unsafe.Pointer((*TJSContext)(unsafe.Pointer(ctx)).Fclass_proto + uintptr(EJS_CLASS_STRING_ITERATOR)*16))\n", "changed (fingerprint"},
		{"index atoms", b, "\treturn libc.BoolInt32(v&(libc.Uint32FromUint32(1)<<libc.Int32FromInt32(31)) != uint32(0))\n", "\treturn libc.BoolInt32(v&(libc.Uint32FromUint32(1)<<libc.Int32FromInt32(30)) != uint32(0))\n", "changed (fingerprint"},
		{"typed arrays", b, "\n\t\t\t\t\tif libc.Int32FromUint16((*TJSObject)(unsafe.Pointer(p2)).Fclass_id) >= int32(EJS_CLASS_UINT8C_ARRAY) && libc.Int32FromUint16((*TJSObject)(unsafe.Pointer(p2)).Fclass_id) <= int32(EJS_CLASS_FLOAT64_ARRAY) {\n", "\n\t\t\t\t\tif libc.Int32FromUint16((*TJSObject)(unsafe.Pointer(p2)).Fclass_id) >= int32(EJS_CLASS_UINT8C_ARRAY) && libc.Int32FromUint16((*TJSObject)(unsafe.Pointer(p2)).Fclass_id) <= int32(EJS_CLASS_FLOAT32_ARRAY) {\n", "changed (fingerprint"},
		{"no fast array test", b, "\t\t\t/* exotic behaviors */\n", "", "want one test of a fast array"},
		{"loop", b, "\t\tp3 = *(*uintptr)(unsafe.Pointer(bp + 272))\n\t\tfor {\n", "\t\tp3 = *(*uintptr)(unsafe.Pointer(bp + 272))\n\t\tfor p3 != 0 {\n", "the loop of get_field"},
		{"slow path", b, "\tget_field2_slow_path:\n", "\tget_field2_slow:\n", "no slow path of get_field2"},
		{"a call left", string(fixed), fieldLayout, "\n// another comment\n", "a call of fieldProto or fieldGoesOn is left"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		in := strings.Replace(c.in, c.old, c.new, 1)
		var err error
		if c.in == b {
			_, err = fixFieldChain([]byte(in), "ccgo_darwin_arm64.go")
		} else {
			_, err = restoreFieldChain([]byte(strings.Replace(in, fieldProtoMark, "", 1)))
		}
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
