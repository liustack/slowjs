// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// fixRegexSkip leaves a fixed file as it is, and puts the change back into a
// file without it, on every target.
func TestRegexSkipIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if n := bytes.Count(b, []byte(regexSkipCall)); n != 1 {
			t.Fatalf("%s: the call of lreSkip %d times", name, n)
		}
		got, err := fixRegexSkip(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: a fixed file changed", name)
		}
		bare := bytes.Replace(bytes.Replace(b, []byte(regexSkipCall), nil, 1), []byte(regexRunNew), []byte(regexRunOld), 1)
		if got, err = fixRegexSkip(bare, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: the change put back differs", name)
		}
	}
}

// A change to the functions lreSkip repeats, or to the places it is
// called from, stops fixRegexSkip.
func TestRegexSkipRejects(t *testing.T) {
	b := string(engineFile(t))
	bare := strings.Replace(strings.Replace(b, regexSkipCall, "", 1), regexRunNew, regexRunOld, 1)
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"another loop", bare, "libc.Uint32FromInt32(-(libc.Int32FromInt32(5) + libc.Int32FromInt32(1) + libc.Int32FromInt32(5)))", "libc.Uint32FromInt32(-(libc.Int32FromInt32(5) + libc.Int32FromInt32(1) + libc.Int32FromInt32(6)))", "changed (fingerprint"},
		{"another char op", bare, "\t\tif val != c5 {\n\t\t\tgoto no_match\n", "\t\tif val > c5 {\n\t\t\tgoto no_match\n", "changed (fingerprint"},
		{"another line start", bare, "\t\tif opcode == int32(EREOP_line_start) {\n", "\t\tif opcode == int32(EREOP_line_start_m) {\n", "changed (fingerprint"},
		{"another header", bare, "(*TREExecContext)(unsafe.Pointer(s)).Fcapture_count = libc.Int32FromUint8(*(*Tuint8_t)(unsafe.Pointer(bc_buf + 2)))", "(*TREExecContext)(unsafe.Pointer(s)).Fcapture_count = libc.Int32FromUint8(*(*Tuint8_t)(unsafe.Pointer(bc_buf + 3)))", "changed (fingerprint"},
		{"the call elsewhere", b, regexSkipCall + "\tbp := tls.Alloc(320)\n", "\tbp := tls.Alloc(320)\n" + regexSkipCall, "is not where fixlib puts it"},
		{"no run", b, "\tret = int32(" + regexRunNew + ")\n", "\tret = 0\n", "changed (fingerprint"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := fixRegexSkip([]byte(strings.Replace(c.in, c.old, c.new, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
