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

// Every target has the front cache wired in.
func TestFrontCacheEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		for _, text := range []string{frontPopHook, frontPushHook, frontOffHook, frontGCHook, frontMallocNew, frontReallocNew} {
			if n := bytes.Count(b, []byte(text)); n != 1 {
				t.Errorf("%s: %d copies of %q", name, n, text)
			}
		}
	}
}

// fixFrontCache leaves a fixed file as it is, and puts the change back into
// a file without it, on every target. restoreFrontCache gives back the text
// ccgo writes.
func TestFrontCacheIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		for _, s := range []string{"frontPop(", "frontPush(", "frontDrain(", "frontRetry(", "frontOff("} {
			if bytes.Contains(b, []byte(s)) {
				t.Fatalf("%s: readEngine leaves a call of %s in", name, s)
			}
		}
		fixed, err := fixFrontCache(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Equal(fixed, b) {
			t.Fatalf("%s: fixFrontCache changed nothing", name)
		}
		again, err := fixFrontCache(bytes.Clone(fixed), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(again, fixed) {
			t.Errorf("%s: a fixed file changed", name)
		}
		back, err := restoreFrontCache(fixed)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(back, b) {
			t.Errorf("%s: restoreFrontCache does not give the text back", name)
		}
	}
}

// A change to what lib/frontcache.go relies on, or a part of the change
// missing, stops fixFrontCache.
func TestFrontCacheRejects(t *testing.T) {
	b := string(engineFile(t))
	fixed, err := fixFrontCache([]byte(b), "ccgo_darwin_arm64.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"another header", b, "b = ptr - uintptr(uint64(libc.UintptrFromInt32(0)+8))\n\tif libc.BoolInt64(!!(libc.Int32FromUint16(*(*Tuint16_t)(unsafe.Pointer(b))) == libc.Int32FromInt32(MFREE_NIL))) != 0 {", "b = ptr - uintptr(uint64(libc.UintptrFromInt32(0)+16))\n\tif libc.BoolInt64(!!(libc.Int32FromUint16(*(*Tuint16_t)(unsafe.Pointer(b))) == libc.Int32FromInt32(MFREE_NIL))) != 0 {", "changed (fingerprint"},
		{"another limit", b, "Fmalloc_size+size > (*TJSMallocState)(unsafe.Pointer(s)).Fmalloc_limit)", "Fmalloc_size+size >= (*TJSMallocState)(unsafe.Pointer(s)).Fmalloc_limit)", "changed (fingerprint"},
		{"another realloc limit", b, "Fmalloc_size+size-old_size > (*TJSMallocState)(unsafe.Pointer(s)).Fmalloc_limit {", "Fmalloc_size+size-old_size >= (*TJSMallocState)(unsafe.Pointer(s)).Fmalloc_limit {", "changed (fingerprint"},
		{"another large block", b, "\t*(*Tuint16_t)(unsafe.Pointer(b)) = uint16(MFREE_NIL)\n\t(*TJSMallocLargeBlockHeader)(unsafe.Pointer(b)).Fheader.Fblock_size_idx = uint8(0xff) /* fail safe */\n\treturn b + 8\n", "\t(*TJSMallocLargeBlockHeader)(unsafe.Pointer(b)).Fheader.Fblock_size_idx = uint8(0xff) /* fail safe */\n\treturn b + 8\n", "changed (fingerprint"},
		{"no pop", string(fixed), frontPopHook, "", "a part of the change"},
		{"no retry", string(fixed), frontMallocNew, frontMallocOld, "a part of the change"},
		{"a call left", string(fixed), frontGCHook, "\tfrontDrain(tls, rt)\n", "a call of frontDrain is left"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := fixFrontCache([]byte(strings.Replace(c.in, c.old, c.new, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
