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
	withUpSites(t, map[string]int{})
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
	withUpSites(t, map[string]int{})
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

// withUpSites makes sites the statements whose order fixUp accepts for the
// length of a test.
func withUpSites(t *testing.T, sites map[string]int) {
	t.Helper()
	saved := upSites
	upSites = sites
	t.Cleanup(func() { upSites = saved })
}

const upOrderBefore = `package p

import "unsafe"

func f(tls *libc.TLS, s, p uintptr, i int32) int32 {
	**(**TJSValue)(__ccgo_up((*TBuf)(unsafe.Pointer(s)).Fbuf + uintptr(i)*16)) = _grow(tls, s)
	**(**int32)(__ccgo_up(p + 8)) = _grow(tls, s)
	_grow(tls, **(**uintptr)(__ccgo_up((*TBuf)(unsafe.Pointer(s)).Fbuf + 8)))
	if int32(**(**int8)(__ccgo_up(**(**uintptr)(__ccgo_up(p))))) == int32('.') && _digit(tls, libc.Int32FromInt32(1)) < 10 {
		return libc.Int32FromInt32(**(**int32)(__ccgo_up((*TBuf)(unsafe.Pointer(s)).Fbuf)))
	}
	return 0
}
` + upDef

// A statement where fixUp drops the order between a read in the address of
// a conversion and a call after it must be one of upSites, as many times as
// it lists it. A read before the call in the same argument list, a
// conversion of an address without a read, and a call of a libc
// conversion helper do not count.
func TestUpOrder(t *testing.T) {
	sites := map[string]int{
		"f: (*TBuf)(unsafe.Pointer(s)).Fbuf + uintptr(i)*N -> _grow": 1,
		"f: *(*uintptr)(unsafe.Pointer(p)) -> _digit":                1,
	}
	withUpSites(t, sites)
	if _, err := fixUp([]byte(upOrderBefore)); err != nil {
		t.Fatalf("listed statements: %v", err)
	}
	got, err := upOrder([]byte(upOrderBefore))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(sites) {
		t.Fatalf("got %v, want %v", got, sites)
	}
	for k, n := range sites {
		if got[k] != n {
			t.Errorf("%s: %d, want %d", k, got[k], n)
		}
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"new statement", "\t_grow(tls, **(**uintptr)", "\t**(**uintptr)(__ccgo_up((*TBuf)(unsafe.Pointer(s)).Fbuf + 24)) = _shrink(tls, s)\n\t_grow(tls, **(**uintptr)",
			"f: (*TBuf)(unsafe.Pointer(s)).Fbuf + N -> _shrink: not in upSites"},
		{"once more", "\t_grow(tls, **(**uintptr)", "\t**(**TJSValue)(__ccgo_up((*TBuf)(unsafe.Pointer(s)).Fbuf + uintptr(i)*16)) = _grow(tls, s)\n\t_grow(tls, **(**uintptr)",
			"f: (*TBuf)(unsafe.Pointer(s)).Fbuf + uintptr(i)*N -> _grow: 2 times, upSites has 1"},
		{"gone", "&& _digit(tls, libc.Int32FromInt32(1)) < 10", "", "f: *(*uintptr)(unsafe.Pointer(p)) -> _digit: 0 times, upSites has 1"},
		{"other call", "= _grow(tls, s)\n\t**(**int32)", "= _shrink(tls, s)\n\t**(**int32)", "f: (*TBuf)(unsafe.Pointer(s)).Fbuf + uintptr(i)*N -> _shrink: not in upSites"},
		{"in a return", "\treturn 0\n", "\treturn **(**int32)(__ccgo_up((*TBuf)(unsafe.Pointer(s)).Fbuf)) + _grow(tls, s)\n", "f: (*TBuf)(unsafe.Pointer(s)).Fbuf -> _grow: not in upSites"},
		{"in an argument", "\treturn 0\n", "\t_put(tls, **(**int32)(__ccgo_up((*TBuf)(unsafe.Pointer(s)).Fbuf)), _grow(tls, s))\n\treturn 0\n", "f: (*TBuf)(unsafe.Pointer(s)).Fbuf -> _grow: not in upSites"},
		{"nested", "\treturn 0\n", "\treturn **(**int32)(__ccgo_up(**(**uintptr)(__ccgo_up(*(*uintptr)(unsafe.Pointer(s)) + 8)) + uintptr(_grow(tls, s))))\n", "f: *(*uintptr)(unsafe.Pointer(s)) + N -> _grow: not in upSites"},
	} {
		if !strings.Contains(upOrderBefore, c.old) {
			t.Fatalf("%s: %q not in the input", c.name, c.old)
		}
		_, err := fixUp([]byte(strings.Replace(upOrderBefore, c.old, c.new, 1)))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

// Every statement of upSites is in each generated file, with the reads of
// its key and the calls after them, so the list is not stale. Which
// statements fixUp changes can only be told from the files as ccgo writes
// them, so upOrder runs on them when fixlib takes a new release.
func TestUpSitesInEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		for key := range upSites {
			fname, rest, ok := strings.Cut(key, ": ")
			reads, calls, ok2 := strings.Cut(rest, " -> ")
			if !ok || !ok2 {
				t.Fatalf("%q: not a key of upSites", key)
			}
			body, err := funcBody(b, funcHead(fname), fname)
			if err != nil {
				t.Errorf("%s: %s: %v", fn, key, err)
				continue
			}
			norm := upNorm(string(body))
			for _, part := range append(strings.Split(reads, " ; "), strings.Fields(calls)...) {
				if !strings.Contains(norm, part) {
					t.Errorf("%s: %s: no %q in %s", fn, key, part, fname)
				}
			}
		}
	}
}
