// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// fixWorkCharge leaves a fixed file as it is, and puts the change back into
// a file without it, on every target.
func TestWorkChargeIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if n := bytes.Count(b, []byte("_js_charge_interrupts(")); n != len(workCharges) {
			t.Fatalf("%s: _js_charge_interrupts is called %d times, want %d", name, n, len(workCharges))
		}
		got, err := fixWorkCharge(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: a fixed file changed", name)
		}
		bare := unfixWorkCharge(bytes.Clone(b))
		if bytes.Contains(bare, []byte("_js_charge_interrupts(")) {
			t.Fatalf("%s: a change is still there", name)
		}
		if got, err = fixWorkCharge(bare, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: the change put back differs", name)
		}
	}
}

// A change to the functions the work is counted in or on, or to the places
// fixWorkCharge puts the counts, stops it.
func TestWorkChargeRejects(t *testing.T) {
	b := string(engineFile(t))
	bare := string(unfixWorkCharge([]byte(b)))
	parse := workCharges[2]
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"another counter reset", bare, "Finterrupt_counter = int32(MJS_INTERRUPT_COUNTER_INIT)", "Finterrupt_counter = int32(MJS_INTERRUPT_COUNTER_INIT + 1)", "changed (fingerprint"},
		{"another growth", bare, "(*TStringBuffer)(unsafe.Pointer(s)).Fsize * int32(3) / int32(2)", "(*TStringBuffer)(unsafe.Pointer(s)).Fsize * int32(2)", "changed (fingerprint"},
		{"the source counted twice", b, parse.text, parse.text + parse.text, "times, want once"},
		{"the source counted first", b, parse.anchor + parse.text, parse.text + parse.anchor, "is not where fixlib puts it"},
		{"a fourth count", b, "\tstack_size = 0\n\tstack_size_max = 0\n", "\tstack_size = 0\n\tstack_size_max = 0\n\t_js_charge_interrupts(0, 0)\n", "want 3 calls"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := fixWorkCharge([]byte(strings.Replace(c.in, c.old, c.new, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
