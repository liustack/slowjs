// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"os"
	"testing"
)

func TestGoasmRewrite(t *testing.T) {
	const name = "ccgo_linux_amd64.go"
	b, err := os.ReadFile("../../lib/" + name)
	if err != nil {
		t.Fatal(err)
	}
	bare, err := restoreGoasm(b, name)
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := fixGoasm(bare, name)
	if err != nil || !bytes.Equal(fixed, b) {
		t.Fatalf("rewrite differs: %v", err)
	}
	back, err := restoreGoasm(fixed, name)
	if err != nil || !bytes.Equal(back, bare) {
		t.Fatalf("inverse differs: %v", err)
	}
	changed := bytes.Replace(bare, []byte("func _JS_CallInternal("), []byte("func _JS_CallInternalChanged("), 1)
	if _, err := fixGoasm(changed, name); err == nil {
		t.Fatal("accepted changed interpreter")
	}
	corrupt := bytes.Replace(fixed, []byte("return goasmCall(tls, caller_ctx"), []byte("return goasmCall(tls, 0"), 1)
	if _, err := fixGoasm(corrupt, name); err == nil {
		t.Fatal("accepted corrupted hook")
	}
	duplicate := append(append([]byte{}, fixed...), []byte(goasmMark)...)
	if _, err := restoreGoasm(duplicate, name); err == nil {
		t.Fatal("accepted duplicate hook")
	}
	other, err := fixGoasm(bare, "ccgo_darwin_arm64.go")
	if err != nil || !bytes.Equal(other, bare) {
		t.Fatalf("changed fallback target: %v", err)
	}
}
