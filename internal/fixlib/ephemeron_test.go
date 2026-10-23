// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestEphemeronRewrite(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readAfter(fn, "fixEphemeron")
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		bare, err := restoreEphemeron(b, name)
		if err != nil {
			t.Fatal(err)
		}
		fixed, err := fixEphemeron(bare, name)
		if err != nil {
			t.Fatal(err)
		}
		again, err := fixEphemeron(fixed, name)
		if err != nil || !bytes.Equal(fixed, again) {
			t.Fatalf("%s not idempotent: %v", name, err)
		}
		back, err := restoreEphemeron(fixed, name)
		if err != nil || !bytes.Equal(back, bare) {
			t.Fatalf("%s inverse: %v", name, err)
		}
		changed := bytes.Replace(bare, []byte("func _gc_scan(tls"), []byte("func _gc_scan_changed(tls"), 1)
		if _, err := fixEphemeron(changed, name); err == nil {
			t.Fatal("accepted changed collector")
		}
		partial := bytes.Replace(fixed, []byte(ephemeronHooks[0].text), nil, 1)
		if _, err := fixEphemeron(partial, name); err == nil {
			t.Fatal("accepted partial rewrite")
		}
		corrupt := bytes.Replace(fixed, []byte("if gcEphemeron(tls, rt, remove_weak_objects)"), []byte("if gcEphemeron(tls, rt, 0)"), 1)
		if _, err := fixEphemeron(corrupt, name); err == nil {
			t.Fatal("accepted edited hook")
		}
	}
}

// Compile-only cross-target checks cannot notice reversed C bitfields.
func TestEphemeronHeaderBits(t *testing.T) {
	for _, fn := range libFiles(t) {
		constants, err := os.ReadFile(filepath.Join("..", "..", "lib", "ephemeronbits.go"))
		if err != nil {
			t.Fatal(err)
		}
		values := map[string]string{}
		for _, m := range regexp.MustCompile(`ephemeron(TypeMask|TypeShift|MarkMask)\s*=\s*(0x[0-9a-f]+|[0-9]+)`).FindAllSubmatch(constants, -1) {
			values[string(m[1])] = string(m[2])
		}
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		mark, err := funcBody(b, funcHead("_mark_children"), "_mark_children")
		if err != nil {
			t.Fatal(err)
		}
		if len(values) != 3 || !bytes.Contains(mark, []byte("& "+values["TypeMask"]+" >> "+values["TypeShift"])) {
			t.Fatalf("%s: GC type bits differ from ephemeronbits.go", fn)
		}
		scan, err := funcBody(b, funcHead("_gc_scan"), "_gc_scan")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(scan, []byte(", "+values["MarkMask"]+") /* reset the mark")) {
			t.Fatalf("%s: GC mark bit differs from ephemeronbits.go", fn)
		}
	}
}
