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

// Every generated file reads memory through unsafe.Pointer only: the one
// __ccgo_up left is its definition, and fixUp leaves a fixed file as it is.
func TestUpEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		if n := bytes.Count(b, []byte("__ccgo_up(")); n != 1 {
			t.Errorf("%s: %d __ccgo_up, want only the definition", fn, n)
			continue
		}
		out, err := fixUp(b)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(out, b) {
			t.Errorf("%s: fixUp changed a fixed file", fn)
		}
	}
}

const upDef = "\nfunc __ccgo_up(n uintptr) unsafe.Pointer {\n\treturn unsafe.Pointer(&n)\n}\n"

const upBefore = `package p

import "unsafe"

func f(bp, p uintptr) int64 {
	**(**uintptr)(__ccgo_up(bp + 8)) = p
	(**(**[2]TJSValue)(__ccgo_up(bp + 16)))[1] = **(**TJSValue)(__ccgo_up(p))
	q := *(**TJSValue)(__ccgo_up(p + 16))
	_ = q
	**(**int32)(__ccgo_up(**(**uintptr)(__ccgo_up(bp + 8)) + uintptr(**(**int32)(__ccgo_up(bp)))*4)) = 1
	return (*(**TJSValue)(__ccgo_up(bp + 16))).Ftag
}
` + upDef

const upAfter = `package p

import "unsafe"

func f(bp, p uintptr) int64 {
	*(*uintptr)(unsafe.Pointer(bp + 8)) = p
	(*(*[2]TJSValue)(unsafe.Pointer(bp + 16)))[1] = *(*TJSValue)(unsafe.Pointer(p))
	q := (*TJSValue)(unsafe.Pointer(p + 16))
	_ = q
	*(*int32)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(bp + 8)) + uintptr(*(*int32)(unsafe.Pointer(bp)))*4)) = 1
	return ((*TJSValue)(unsafe.Pointer(bp + 16))).Ftag
}
` + upDef

// fixUp rewrites each conversion through __ccgo_up, nested ones too, in one
// run, and a second run changes nothing.
func TestUpShapes(t *testing.T) {
	out, err := fixUp([]byte(upBefore))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != upAfter {
		t.Fatalf("got\n%s\nwant\n%s", out, upAfter)
	}
	again, err := fixUp(out)
	if err != nil || !bytes.Equal(again, out) {
		t.Fatalf("fixUp changed a fixed file: %v", err)
	}
}

// Any other use of __ccgo_up, or a definition that is missing or does
// something else, stops fixlib.
func TestUpRejects(t *testing.T) {
	for _, c := range []struct{ name, old, new, want string }{
		{"pointer kept", "\tq := *(**TJSValue)(__ccgo_up(p + 16))\n", "\tq := __ccgo_up(p + 16)\n", "__ccgo_up used as"},
		{"single star", "\tq := *(**TJSValue)(__ccgo_up(p + 16))\n", "\tq := (*TJSValue)(__ccgo_up(p + 16))\n", "__ccgo_up used as"},
		{"passed on", "\tq := *(**TJSValue)(__ccgo_up(p + 16))\n", "\tq := g(__ccgo_up)\n", "__ccgo_up used as"},
		{"no definition", upDef, "\n", "want 1 definition of __ccgo_up"},
		{"other definition", "\treturn unsafe.Pointer(&n)\n", "\treturn unsafe.Pointer(n)\n", "__ccgo_up changed"},
	} {
		if !strings.Contains(upBefore, c.old) {
			t.Fatalf("%s: %q not in the input", c.name, c.old)
		}
		_, err := fixUp([]byte(strings.Replace(upBefore, c.old, c.new, 1)))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

// All the changes together leave a fixed file as it is: the ones before
// fixUp take what it wrote.
func TestEngineFixedFile(t *testing.T) {
	hw, err := handWritten(filepath.Join("..", "..", "lib"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		out, err := fixEngine(b, hw, filepath.Base(fn))
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(out, b) {
			t.Errorf("%s: the changes changed a fixed file", fn)
		}
	}
}
