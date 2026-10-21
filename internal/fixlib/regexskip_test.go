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
		if n := len(reRegexSplitHook.FindAll(b, -1)); n != 1 {
			t.Fatalf("%s: the question to lreSplit %d times", name, n)
		}
		got, err := fixRegexSkip(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: a fixed file changed", name)
		}
		bare := unfixRegexSkip(bytes.Clone(b))
		if bytes.Contains(bare, []byte("lreSkip")) || bytes.Contains(bare, []byte("lreRun")) || bytes.Contains(bare, []byte("lreSplit")) {
			t.Fatalf("%s: a change is still there", name)
		}
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
	bare := string(unfixRegexSkip([]byte(b)))
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"another loop", bare, "libc.Uint32FromInt32(-(libc.Int32FromInt32(5) + libc.Int32FromInt32(1) + libc.Int32FromInt32(5)))", "libc.Uint32FromInt32(-(libc.Int32FromInt32(5) + libc.Int32FromInt32(1) + libc.Int32FromInt32(6)))", "changed (fingerprint"},
		{"another char op", bare, "\t\tif val != c5 {\n\t\t\tgoto no_match\n", "\t\tif val > c5 {\n\t\t\tgoto no_match\n", "changed (fingerprint"},
		{"another line start", bare, "\t\tif opcode == int32(EREOP_line_start) {\n", "\t\tif opcode == int32(EREOP_line_start_m) {\n", "changed (fingerprint"},
		{"another header", bare, "(*TREExecContext)(unsafe.Pointer(s)).Fcapture_count = libc.Int32FromUint8(*(*Tuint8_t)(unsafe.Pointer(bc_buf + 2)))", "(*TREExecContext)(unsafe.Pointer(s)).Fcapture_count = libc.Int32FromUint8(*(*Tuint8_t)(unsafe.Pointer(bc_buf + 3)))", "changed (fingerprint"},
		{"the call elsewhere", b, regexSkipCall + "\tbp := tls.Alloc(320)\n", "\tbp := tls.Alloc(320)\n" + regexSkipCall, "is not where fixlib puts it"},
		{"no run", b, "\tret = int32(" + regexRunNew + ")\n", "\tret = 0\n", "changed (fingerprint"},
		{"another split", bare, "\t\t\tpc1 = pc\n\t\t\tpc = pc + uintptr(libc.Int32FromUint32(val))\n", "\t\t\tpc1 = pc + 1\n\t\t\tpc = pc + uintptr(libc.Int32FromUint32(val))\n", "changed (fingerprint"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := fixRegexSkip([]byte(strings.Replace(c.in, c.old, c.new, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
	// lreFirstSet relies on what lre_canonicalize gives for the ASCII
	// characters.
	for _, c := range []struct{ fn, old, new string }{
		{"Xlre_canonicalize", "c = c - uint32('a') + uint32('A')", "c = c - uint32('a') + uint32('B')"},
		{"_lre_case_folding_entry", ")))[0] >= uint32(128) {", ")))[0] >= uint32(0) {"},
	} {
		fn := c.fn
		body, err := funcBody([]byte(bare), funcHead(fn), fn)
		if err != nil {
			t.Fatal(err)
		}
		changed := strings.Replace(string(body), c.old, c.new, 1)
		if changed == string(body) {
			t.Fatalf("%s: no %q", fn, c.old)
		}
		_, err = fixRegexSkip([]byte(strings.Replace(bare, string(body), changed, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), "changed (fingerprint") {
			t.Errorf("another %s: %v", fn, err)
		}
	}
}

// regexSplitFirst puts the question to lreSplit right after the two ways of
// the split op, and stops when it cannot find the handler, its push or the
// jump to the next op, or finds the question somewhere else or in another
// form, such as one that does not count a step toward the interrupt check.
func TestRegexSplitFirst(t *testing.T) {
	b := engineFile(t)
	body, err := funcBody(b, funcHead(regexMachine), regexMachine)
	if err != nil {
		t.Fatal(err)
	}
	hook := reRegexSplitHook.Find(body)
	if hook == nil || !bytes.Equal(hook, []byte(regexSplitHook("_48"))) {
		t.Fatalf("the question to lreSplit is %q", hook)
	}
	bare := bytes.Replace(body, hook, nil, 1)
	if got, err := regexSplitFirst(bare); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("put back: %v", err)
	}
	split := reRegexSplit.Find(bare)
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"no split", string(bare), string(split), "", "want one handler"},
		{"two splits", string(bare), string(split), string(split) + string(split), "want one handler"},
		{"no push", string(bare), "\t\t*(*uintptr)(unsafe.Pointer(sp)) = pc1\n", "\t\t*(*uintptr)(unsafe.Pointer(sp)) = pc\n", "no longer push"},
		{"another next", string(bare), "\t\tbp = sp\n\t\tgoto _48\n\t_13:", "\t\tbp = sp\n\t\tgoto _49\n\t_13:", "no longer push"},
		{"the question after the push", string(body), string(hook) + "\t\tif libc.BoolInt64(", "\t\tif libc.BoolInt64(", "is not where fixlib puts it"},
		{"a question that does not check", string(body), "\t\t\t\tif _lre_poll_timeout(tls, s) != 0 {\n\t\t\t\t\treturn Tintptr_t(MLRE_RET_TIMEOUT)\n\t\t\t\t}\n\t\t\t\tgoto _48\n", "\t\t\t\tgoto _48\n", "not the one fixlib writes"},
	} {
		in := c.in
		if c.name == "the question after the push" {
			in = strings.Replace(in, "\t\tbp = sp\n\t\tgoto _48\n\t_13:", "\t\tbp = sp\n"+string(hook)+"\t\tgoto _48\n\t_13:", 1)
		}
		if !strings.Contains(in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := regexSplitFirst([]byte(strings.Replace(in, c.old, c.new, 1)))
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
