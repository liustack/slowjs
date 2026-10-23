// Copyright (c) 2026 The slowjs Authors.
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

// fixSharedDebug is the inverse of its exact inverse on every target, and
// stops on a changed function it relies on, a partial or edited change, a new
// function that uses the source text or line table, and a use of the bit
// shared_debug takes.
func TestSharedDebugRewrite(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		bare, err := restoreSharedDebug(b, name)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range sharedDebugCalls {
			if bytes.Contains(bare, []byte(call)) {
				t.Fatalf("%s: restoreSharedDebug left %s", name, call)
			}
		}
		fixed, err := fixSharedDebug(bare, name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(fixed, b) {
			t.Fatalf("%s: fixSharedDebug does not give the committed file", name)
		}
		for call, want := range map[string]int{"readSharedDebug(": 1, "debugAlloc(": 2, "debugGetBuf(": 2, "debugFree(": 2, "debugShared(": 2} {
			if got := strings.Count(string(fixed), call); got != want {
				t.Fatalf("%s: %s %d times, want %d", name, call, got, want)
			}
		}
		again, err := fixSharedDebug(fixed, name)
		if err != nil || !bytes.Equal(fixed, again) {
			t.Fatalf("%s not idempotent: %v", name, err)
		}

		changed := bytes.Replace(bare, []byte("func _find_line_num(tls *libc.TLS, "), []byte("func _find_line_num(tls *libc.TLS, _ int, "), 1)
		if bytes.Equal(changed, bare) {
			t.Fatalf("%s: the change of the test does not apply", name)
		}
		if _, err := fixSharedDebug(changed, name); err == nil {
			t.Fatal("accepted a changed function")
		}
		partial := bytes.Replace(fixed, []byte(sharedDebugHook), nil, 1)
		if _, err := fixSharedDebug(partial, name); err == nil {
			t.Fatal("accepted a partial rewrite")
		}
		edited := bytes.Replace(fixed, []byte("debugFree(tls, rt1, b, "), []byte("debugFree(tls, rt1, 0, "), 1)
		if _, err := fixSharedDebug(edited, name); err == nil {
			t.Fatal("accepted an edited call")
		}
		for _, use := range []string{
			"\treturn (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fsource\n",
			"\tXjs_free_rt(tls, rt, (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fpc2line_buf)\n\treturn 0\n",
			"\treturn *(*uintptr)(unsafe.Pointer(b+96+24))\n",
		} {
			user := append(bytes.Clone(bare), "\nfunc _js_new_user(tls *libc.TLS, rt uintptr, b uintptr) uintptr {\n"+use+"}\n"...)
			if _, err := fixSharedDebug(user, name); err == nil || !strings.Contains(err.Error(), "_js_new_user") {
				t.Fatalf("%s: accepted a new function that uses the source or line table: %v", name, err)
			}
		}
		for _, bit := range []string{
			"\treturn int32(*(*uint8)(unsafe.Pointer(b + 18))&0x20>>5)\n",
			"\tlibc.SetBitFieldPtr8Uint8(b+18, libc.Uint8FromInt32(1), 5, 0x20)\n\treturn 0\n",
		} {
			user := append(bytes.Clone(bare), "\nfunc _js_bit_user(tls *libc.TLS, b uintptr) int32 {\n"+bit+"}\n"...)
			if _, err := fixSharedDebug(user, name); err == nil || !strings.Contains(err.Error(), "shared_debug") {
				t.Fatalf("%s: accepted a use of the bit shared_debug takes: %v", name, err)
			}
		}
	}
}
