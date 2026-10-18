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

var reCellsDecl = regexp.MustCompile(`\n\tvar (cell\d+), (cell\d+) TJSValue ` + regexp.QuoteMeta(cellsInGo))

// In the interpreter of every 64-bit file, the two cells JS_DupValue and
// JS_FreeValue go through are Go variables, no use of them is left in the
// TLS frame, and fixCells leaves a fixed file as it is. The 32-bit files
// keep them.
func TestCellsEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		listed := cellFiles[filepath.Base(fn)]
		out, err := fixCells(b, listed)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(out, b) {
			t.Errorf("%s: fixCells changed a fixed file", fn)
		}
		body, err := funcBody(b, reCallInternal, "_JS_CallInternal")
		if err != nil {
			t.Fatal(err)
		}
		m := reCellsDecl.FindSubmatch(body)
		if !listed {
			if m != nil || bytes.Contains(body, []byte(cellsInGo)) {
				t.Errorf("%s: cells changed in a file that is not listed", fn)
			}
			continue
		}
		if m == nil {
			t.Errorf("%s: no cells declared", fn)
			continue
		}
		// JS_FreeValue's cell, then JS_DupValue's: a store and a load for
		// each expansion.
		for i, name := range m[1:] {
			want := []int{63, 70}[i]
			off := "bp + " + strings.TrimPrefix(string(name), "cell") + ")"
			if bytes.Contains(body, []byte(off)) {
				t.Errorf("%s: %s still used", fn, off)
			}
			if n := bytes.Count(body, append(append([]byte{'\t'}, name...), " = "...)); n != want {
				t.Errorf("%s: %d stores of %s, want %d", fn, n, name, want)
			}
			if n := bytes.Count(body, []byte("uintptr("+string(name)+".Fu.Fuint641)")); n != want {
				t.Errorf("%s: %d loads of %s, want %d", fn, n, name, want)
			}
		}
	}
	if len(cellFiles) != 11 {
		t.Errorf("%d files listed, want the 11 of 64-bit targets", len(cellFiles))
	}
}

const cellsTypes = `
type Tuint64_t = uint64

type TJSValueUnion = struct {
	Ffloat641      [0]float64
	Fptr           [0]uintptr
	Fshort_big_int [0]Tint32_t
	Fuint641       Tuint64_t
}

type TJSValue = struct {
	Fu   TJSValueUnion
	Ftag Tint64_t
}
`

const cellsBefore = cellsTypes + `
func _JS_CallInternal(tls *libc.TLS, caller_ctx uintptr, _func_obj TJSValue, argc int32) (r5 TJSValue) {
	bp := tls.Alloc(96)
	defer tls.Free(96)
	*(*TJSValue)(unsafe.Pointer(bp + 80)) = _func_obj
	var p1, v1, v10 uintptr
	var v9 TJSValue
	var _ /* v at bp+0 */ TJSValue
	var _ /* t at bp+32 */ struct {
		Fu [0]Tuint64_t
		Fd float64
	}
	_, _, _, _ = p1, v1, v10, v9
	v9 = *(*TJSValue)(unsafe.Pointer(bp + 80))
	*(*TJSValue)(unsafe.Pointer(bp + 64)) = v9
	if libc.Uint32FromInt32(int32(v9.Ftag)) >= libc.Uint32FromInt32(EJS_TAG_FIRST) {
		v1 = *(*uintptr)(unsafe.Pointer(bp + 64)) - libc.UintptrFromInt32(1)*4
		goto _1
	_1:
		p1 = v1
		*(*int32)(unsafe.Pointer(p1))++
	}
	*(*float64)(unsafe.Pointer(bp + 32)) = 1
	v9 = *(*TJSValue)(unsafe.Pointer(bp))
	*(*TJSValue)(unsafe.Pointer(bp + 48)) = v9
	if libc.Uint32FromInt32(int32(v9.Ftag)) >= libc.Uint32FromInt32(EJS_TAG_FIRST) {
		v10 = *(*uintptr)(unsafe.Pointer(bp + 48)) - libc.UintptrFromInt32(1)*4
		p1 = v10
	}
	return v9
}
`

const cellsAfter = cellsTypes + `
func _JS_CallInternal(tls *libc.TLS, caller_ctx uintptr, _func_obj TJSValue, argc int32) (r5 TJSValue) {
	bp := tls.Alloc(96)
	defer tls.Free(96)
	*(*TJSValue)(unsafe.Pointer(bp + 80)) = _func_obj
	var p1, v1, v10 uintptr
	var v9 TJSValue
	var _ /* v at bp+0 */ TJSValue
	var _ /* t at bp+32 */ struct {
		Fu [0]Tuint64_t
		Fd float64
	}
	var cell48, cell64 TJSValue ` + cellsInGo + `	_, _, _, _ = p1, v1, v10, v9
	v9 = *(*TJSValue)(unsafe.Pointer(bp + 80))
	cell64 = v9
	if libc.Uint32FromInt32(int32(v9.Ftag)) >= libc.Uint32FromInt32(EJS_TAG_FIRST) {
		v1 = uintptr(cell64.Fu.Fuint641) - libc.UintptrFromInt32(1)*4
		goto _1
	_1:
		p1 = v1
		*(*int32)(unsafe.Pointer(p1))++
	}
	*(*float64)(unsafe.Pointer(bp + 32)) = 1
	v9 = *(*TJSValue)(unsafe.Pointer(bp))
	cell48 = v9
	if libc.Uint32FromInt32(int32(v9.Ftag)) >= libc.Uint32FromInt32(EJS_TAG_FIRST) {
		v10 = uintptr(cell48.Fu.Fuint641) - libc.UintptrFromInt32(1)*4
		p1 = v10
	}
	return v9
}
`

// fixCells turns the two cells into Go variables, whatever their offsets,
// leaves named locals and parameter copies alone, and a second run changes
// nothing. A file that is not listed is left as it is.
func TestCellsShape(t *testing.T) {
	withFrameUses(t, map[string]int{})
	out, err := fixCells([]byte(cellsBefore), true)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != cellsAfter {
		t.Fatalf("got\n%s\nwant\n%s", out, cellsAfter)
	}
	again, err := fixCells(out, true)
	if err != nil || !bytes.Equal(again, out) {
		t.Fatalf("fixCells changed a fixed file: %v", err)
	}
	same, err := fixCells([]byte(cellsBefore), false)
	if err != nil || string(same) != cellsBefore {
		t.Fatalf("fixCells changed a file that is not listed: %v", err)
	}
}

// withFrameUses makes uses the other uses of bp that fixCells accepts for
// the length of a test.
func withFrameUses(t *testing.T, uses map[string]int) {
	t.Helper()
	saved := cellsFrameUses
	cellsFrameUses = uses
	t.Cleanup(func() { cellsFrameUses = saved })
}

// A use of a cell other than a store of a whole JSValue or a load of its
// first word, a third unnamed cell, a missing one, a JSValue of another
// layout, or a use of the frame that may reach a cell some other way stops
// fixlib.
func TestCellsRejects(t *testing.T) {
	withFrameUses(t, map[string]int{})
	store := "\t*(*TJSValue)(unsafe.Pointer(bp + 48)) = v9\n"
	for _, c := range []struct{ name, old, new, want string }{
		{"address passed", store, store + "\t_js_free(tls, bp+48)\n", "_js_free(bp+48): inside the cell bp+48"},
		{"address kept", store, store + "\tp1 = bp + 48\n", "p1 = bp + 48: inside the cell bp+48"},
		{"other load", store, store + "\tv1 = uintptr(*(*int32)(unsafe.Pointer(bp + 48)))\n", "bp+48: a use other than"},
		{"field read", store, store + "\tv1 = uintptr((*(*TJSValue)(unsafe.Pointer(bp + 48))).Ftag)\n", "bp+48: a use other than"},
		{"other store", store, store + "\t*(*uintptr)(unsafe.Pointer(bp + 48)) = v1\n", "bp+48: a use other than"},
		{"store of a load", store, store + "\t*(*TJSValue)(unsafe.Pointer(bp + 48)) = *(*TJSValue)(unsafe.Pointer(bp + 48))\n", "bp+48: a use other than"},
		{"inside a cell", store, store + "\tv1 = *(*uintptr)(unsafe.Pointer(bp + 56))\n", "bp+56 inside the cell bp+48"},
		{"third cell", store, store + "\t*(*TJSValue)(unsafe.Pointer(bp + 16)) = v9\n\tv1 = *(*uintptr)(unsafe.Pointer(bp + 16))\n", "3 cells"},
		{"one cell", "bp + 64)", "bp + 48)", "1 cells"},
		{"no load", "\t\tv10 = *(*uintptr)(unsafe.Pointer(bp + 48)) - libc.UintptrFromInt32(1)*4\n", "", "bp+48: 1 stores and 0 loads"},
		{"name taken", "\tvar v9 TJSValue\n", "\tvar v9, cell48 TJSValue\n", "cell48 already used"},
		{"other layout", "\tFtag Tint64_t\n", "\tFtag Tint32_t\n", "JSValue is not"},
		{"wider union", "\tFptr           [0]uintptr\n", "\tFptr           uintptr\n", "JSValue is not"},
		{"sum of offsets", store, store + "\tv1 = *(*uintptr)(unsafe.Pointer(bp + 32 + 16))\n", "bp + 32 + 16: an address in the frame"},
		{"converted offset", store, store + "\tv1 = *(*uintptr)(unsafe.Pointer(bp + uintptr(48)))\n", "bp + uintptr(48): an address in the frame"},
		{"frame passed", store, store + "\tlibc.Xmemset(tls, bp, 0, 80)\n", "libc.Xmemset(bp): a use of bp"},
		{"offset passed", store, store + "\t_g(tls, bp+uintptr(48))\n", "_g(bp+uintptr(48)): a use of bp"},
		{"frame kept", store, store + "\tv1 = bp\n", "v1 = bp: a use of bp"},
		{"wide read", store, store + "\tv9 = *(*TJSValue)(unsafe.Pointer(bp + 40))\n", "bp+40 reaches into the cell bp+48"},
		{"wide read of a local", "\t*(*float64)(unsafe.Pointer(bp + 32)) = 1\n", "\t*(*[3]TJSValue)(unsafe.Pointer(bp + 32)) = [3]TJSValue{}\n", "bp+32 reaches into the cell bp+48"},
		{"pointer of unknown size", store, store + "\tv1 = uintptr((*TFoo)(unsafe.Pointer(bp + 40)).Fx)\n", "bp+40: a TFoo, whose size is not known"},
		{"not a pointer", store, store + "\tv1 = uintptr(unsafe.Pointer(bp + 40))\n", "bp + 40: an address in the frame"},
	} {
		if !strings.Contains(cellsBefore, c.old) {
			t.Fatalf("%s: %q not in the input", c.name, c.old)
		}
		_, err := fixCells([]byte(strings.ReplaceAll(cellsBefore, c.old, c.new)), true)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

// A use of bp that cellsFrameUses lists is accepted as many times as it
// lists it: one more, one less or one it does not list stops fixlib.
func TestCellsFrameUses(t *testing.T) {
	use := "\t_g(tls, bp+16)\n"
	in := strings.Replace(cellsBefore, "\treturn v9\n", use+"\treturn v9\n", 1)
	withFrameUses(t, map[string]int{"_g(bp+16)": 1})
	if _, err := fixCells([]byte(in), true); err != nil {
		t.Fatalf("listed use: %v", err)
	}
	for _, c := range []struct {
		name string
		uses map[string]int
		in   string
		want string
	}{
		{"not listed", map[string]int{}, in, "_g(bp+16): a use of bp"},
		{"once more", map[string]int{"_g(bp+16)": 1}, strings.Replace(in, use, use+use, 1), "_g(bp+16): a use of bp"},
		{"gone", map[string]int{"_g(bp+16)": 1, "_h(bp+16)": 1}, in, "_h(bp+16): listed in cellsFrameUses 1 times, found 0"},
		{"inside a cell", map[string]int{"_g(bp+56)": 1}, strings.Replace(in, "bp+16", "bp+56", 1), "_g(bp+56): inside the cell bp+48"},
	} {
		withFrameUses(t, c.uses)
		_, err := fixCells([]byte(c.in), true)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

// In a fixed file, any use of the frame where the cells were stops fixlib:
// it would read or write bytes nothing writes or reads any more.
func TestCellsRejectsFixed(t *testing.T) {
	withFrameUses(t, map[string]int{})
	at := "\tcell48 = v9\n"
	for _, c := range []struct{ name, new, want string }{
		{"load left", at + "\tv1 = *(*uintptr)(unsafe.Pointer(bp + 48))\n", "bp+48: the cell bp+48 is a Go variable"},
		{"store left", at + "\t*(*TJSValue)(unsafe.Pointer(bp + 64)) = v9\n", "cells declared and others [64] still in the frame"},
		{"inside", at + "\tv1 = *(*uintptr)(unsafe.Pointer(bp + 72))\n", "bp+72: the cell bp+64 is a Go variable"},
		{"wide read", at + "\tv9 = *(*TJSValue)(unsafe.Pointer(bp + 40))\n", "bp+40: the cell bp+48 is a Go variable"},
		{"sum of offsets", at + "\tv1 = *(*uintptr)(unsafe.Pointer(bp + 32 + 16))\n", "bp + 32 + 16: an address in the frame"},
		{"frame passed", at + "\tlibc.Xmemset(tls, bp, 0, 80)\n", "libc.Xmemset(bp): a use of bp"},
		{"new cell", at + "\t*(*TJSValue)(unsafe.Pointer(bp + 16)) = v9\n\tv1 = *(*uintptr)(unsafe.Pointer(bp + 16))\n", "cells declared and others [16] still in the frame"},
	} {
		if !strings.Contains(cellsAfter, at) {
			t.Fatalf("%s: %q not in the fixed input", c.name, at)
		}
		_, err := fixCells([]byte(strings.Replace(cellsAfter, at, c.new, 1)), true)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

// -cells prints cellsFrameUses as the files have it, with the named local
// each use starts in, so that a new local in the interpreter is an update
// to read and paste.
func TestCellsReport(t *testing.T) {
	hw, err := handWritten(filepath.Join("..", "..", "lib"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := cellsReport(libFiles(t), hw)
	if err != nil {
		t.Fatal(err)
	}
	for key, n := range cellsFrameUses {
		if !strings.Contains(out, fmt.Sprintf("\t%q: %d,", key, n)) {
			t.Errorf("no %q: %d in\n%s", key, n, out)
		}
	}
	for _, want := range []string{
		`"_find_own_property(bp+288)": 1, // pr at bp+288`,
		`"_js_add_slow(bp+480+uintptr(2)*16)": 1, // ops at bp+480`,
		`"libc.VaList(bp+1256)": 2, // 8 bytes past val at bp+1232`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in\n%s", want, out)
		}
	}

	// A use that differs between the files stops it.
	dir := t.TempDir()
	var files []string
	for i, name := range []string{"ccgo_darwin_arm64.go", "ccgo_linux_amd64.go"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "lib", name))
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			b = bytes.Replace(b, []byte("libc.VaList(bp+1256"), []byte("libc.VaList(bp+1264"), 1)
		}
		fn := filepath.Join(dir, name)
		if err := os.WriteFile(fn, b, 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, fn)
	}
	if _, err := cellsReport(files, hw); err == nil {
		t.Error("files with other uses went through")
	}
}
