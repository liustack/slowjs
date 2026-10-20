// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The atom type and the length of a JSString on every target: s390x is
// big-endian, so ccgo numbers the bit fields from the other end.
var atomKeyWant = map[bool]atomKeyBits{
	false: {typeOff: "4", typeMask: "0xc0000000", typeShift: "30", lenMask: "0x7fffffff", lenShift: "0"},
	true:  {typeOff: "4", typeMask: "0x3", typeShift: "0", lenMask: "0xfffffffe", lenShift: "1"},
}

// Every target calls atomKey once, at the start of JS_ValueToAtom and only
// for a string, with the bit fields of its own JSString.
func TestAtomKeyEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if n := bytes.Count(b, []byte("atomKey(tls, ")); n != 1 {
			t.Errorf("%s: %d calls of atomKey", name, n)
		}
		head := reAtomKeyHead.FindSubmatchIndex(b)
		if head == nil {
			t.Fatalf("%s: no JS_ValueToAtom", name)
		}
		val := string(b[head[2]:head[3]])
		if !bytes.HasPrefix(b[head[1]:], []byte(atomKeyMark+"\tif jsvTag("+val+") == EJS_TAG_STRING {\n")) {
			t.Errorf("%s: JS_ValueToAtom does not start with the test of a string", name)
		}
		got, err := atomKeyConsts(b)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if want := atomKeyWant[name == "ccgo_linux_s390x.go"]; got != want {
			t.Errorf("%s: bit fields %+v, want %+v", name, got, want)
		}
	}
}

// fixAtomKey leaves a fixed file as it is, and puts the change back into a
// file without it, on every target. restoreAtomKey gives back the text
// ccgo writes, byte for byte.
func TestAtomKeyIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if bytes.Contains(b, []byte("atomKey(")) || bytes.Contains(b, []byte(atomKeyLayout)) {
			t.Fatalf("%s: readEngine leaves the change in", name)
		}
		fixed, err := fixAtomKey(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Equal(fixed, b) {
			t.Fatalf("%s: fixAtomKey changed nothing", name)
		}
		again, err := fixAtomKey(bytes.Clone(fixed), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(again, fixed) {
			t.Errorf("%s: a fixed file changed", name)
		}
		back, err := restoreAtomKey(fixed)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(back, b) {
			t.Errorf("%s: restoreAtomKey does not give the text back", name)
		}
	}
}

// The bit fields come from the file's own __JS_NewAtom and is_num_string:
// other masks there give other constants.
func TestAtomKeyReadsBitFields(t *testing.T) {
	b := string(engineFile(t))
	for _, c := range []struct {
		name, old, new string
		want           atomKeyBits
	}{
		{"atom type", "*(*uint32)(unsafe.Pointer(str + 4))&0xc0000000>>30)) == atom_type {", "*(*uint32)(unsafe.Pointer(str + 8))&0x3>>0)) == atom_type {",
			atomKeyBits{typeOff: "8", typeMask: "0x3", typeShift: "0", lenMask: "0x7fffffff", lenShift: "0"}},
		{"length", "\tlen1 = int32(*(*uint32)(unsafe.Pointer(p + 0)) & 0x7fffffff >> 0)\n\tif len1 == 0 || len1 > int32(10) {\n", "\tlen1 = int32(*(*uint32)(unsafe.Pointer(p + 0)) & 0xfffffffe >> 1)\n\tif len1 == 0 || len1 > int32(10) {\n",
			atomKeyBits{typeOff: "4", typeMask: "0xc0000000", typeShift: "30", lenMask: "0xfffffffe", lenShift: "1"}},
	} {
		if !strings.Contains(b, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		out, _, err := planAtomKey([]byte(strings.Replace(b, c.old, c.new, 1)))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got, err := atomKeyConsts(out)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// A change to a function atomKey repeats, or to the text fixAtomKey reads,
// stops fixAtomKey.
func TestAtomKeyRejects(t *testing.T) {
	b := string(engineFile(t))
	fixed, err := fixAtomKey([]byte(b), "ccgo_darwin_arm64.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"atom index", b, "\t\tfor p1 != p {\n", "\t\tfor p1 != p && p1 != 0 {\n", "changed (fingerprint"},
		{"index strings", b, "\t\t\t\tif n64>>libc.Int32FromInt32(32) != uint64(0) {\n", "\t\t\t\tif n64>>libc.Int32FromInt32(31) != uint64(0) {\n", "changed (fingerprint"},
		{"constant atoms", b, "return libc.BoolInt32(libc.Int32FromUint32(v) < int32(EJS_ATOM_END))", "return libc.BoolInt32(libc.Int32FromUint32(v) <= int32(EJS_ATOM_END))", "changed (fingerprint"},
		{"property key", b, "\treturn _JS_ToStringInternal(tls, ctx, val, int32(MTRUE))\n", "\treturn _JS_ToStringInternal(tls, ctx, val, int32(MFALSE))\n", "changed (fingerprint"},
		{"symbol path", b, "\t\t\tatom = XJS_DupAtom(tls, ctx, _js_get_atom_index(tls, (*TJSContext)(unsafe.Pointer(ctx)).Frt, p))\n", "\t\t\tatom = _js_get_atom_index(tls, (*TJSContext)(unsafe.Pointer(ctx)).Frt, p)\n", "changed (fingerprint"},
		{"another head", b, "func XJS_ValueToAtom(tls *libc.TLS, ctx uintptr, _val TJSValue) (r TJSAtom) {", "func XJS_ValueToAtom(tls *libc.TLS, ctx uintptr, _val TJSValue) (atom TJSAtom) {", "want one function XJS_ValueToAtom"},
		{"no atom string case", b, "\t\t\t/* str is the atom, return its index */\n", "", "want one test of an atom string"},
		{"another length", b, "\tlen1 = int32(*(*uint32)(unsafe.Pointer(p + 0)) & 0x7fffffff >> 0)\n\tif len1 == 0 || len1 > int32(10) {\n", "\tlen1 = int32((*TJSString)(unsafe.Pointer(p)).Flen)\n\tif len1 == 0 || len1 > int32(10) {\n", "want one length of a string"},
		{"a call left", string(fixed), atomKeyLayout, "\n// another comment\n", "a call of atomKey is left"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		in := strings.Replace(c.in, c.old, c.new, 1)
		var err error
		if c.in == b {
			_, err = fixAtomKey([]byte(in), "ccgo_darwin_arm64.go")
		} else {
			_, err = restoreAtomKey([]byte(strings.Replace(in, atomKeyMark, "", 1)))
		}
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
