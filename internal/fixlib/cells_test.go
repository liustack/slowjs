// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
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

// A use of a cell other than a store of a whole JSValue or a load of its
// first word, a third unnamed cell, a missing one, or a JSValue of another
// layout stops fixlib.
func TestCellsRejects(t *testing.T) {
	store := "\t*(*TJSValue)(unsafe.Pointer(bp + 48)) = v9\n"
	for _, c := range []struct{ name, old, new, want string }{
		{"address passed", store, store + "\t_js_free(tls, bp+48)\n", "bp+48: a use other than"},
		{"address kept", store, store + "\tp1 = bp + 48\n", "bp+48: a use other than"},
		{"other load", store, store + "\tv1 = uintptr(*(*int32)(unsafe.Pointer(bp + 48)))\n", "bp+48: a use other than"},
		{"field read", store, store + "\tv1 = uintptr((*(*TJSValue)(unsafe.Pointer(bp + 48))).Ftag)\n", "bp+48: a use other than"},
		{"other store", store, store + "\t*(*uintptr)(unsafe.Pointer(bp + 48)) = v1\n", "bp+48: a use other than"},
		{"store of a load", store, store + "\t*(*TJSValue)(unsafe.Pointer(bp + 48)) = *(*TJSValue)(unsafe.Pointer(bp + 48))\n", "bp+48: a use other than"},
		{"inside a cell", store, store + "\tv1 = *(*uintptr)(unsafe.Pointer(bp + 56))\n", "bp+56 inside the cell bp+48"},
		{"third cell", store, store + "\t*(*TJSValue)(unsafe.Pointer(bp + 88)) = v9\n\tv1 = *(*uintptr)(unsafe.Pointer(bp + 88))\n", "3 cells"},
		{"one cell", "bp + 64)", "bp + 48)", "1 cells"},
		{"no load", "\t\tv10 = *(*uintptr)(unsafe.Pointer(bp + 48)) - libc.UintptrFromInt32(1)*4\n", "", "bp+48: 1 stores and 0 loads"},
		{"name taken", "\tvar v9 TJSValue\n", "\tvar v9, cell48 TJSValue\n", "cell48 already used"},
		{"other layout", "\tFtag Tint64_t\n", "\tFtag Tint32_t\n", "JSValue is not"},
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
