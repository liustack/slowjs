// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// unfixRegexPollFile gives b, a generated file, without the change of
// fixRegexPoll.
func unfixRegexPollFile(t *testing.T, b []byte) []byte {
	t.Helper()
	for _, name := range []string{"_re_parse_alternative", "_re_parse_disjunction", "_re_parse_nested_class"} {
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			t.Fatal(err)
		}
		b = bytes.Replace(b, body, unfixRegexPoll(bytes.Clone(body)), 1)
	}
	return regexp.MustCompile(`\} else if _js_regexp_interrupted\(bp\+\d+\) \{\n[^\n]*\n[^\n]*\n\t\t`).ReplaceAll(b, nil)
}

// fixRegexPoll leaves a fixed file as it is, and puts the change back into
// a file without it, on every target.
func TestRegexPollIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if n := bytes.Count(b, []byte("_re_poll_interrupts(")); n != 4 {
			t.Fatalf("%s: _re_poll_interrupts is called %d times, want 4", name, n)
		}
		if n := bytes.Count(b, []byte("_js_regexp_interrupted(")); n != 1 {
			t.Fatalf("%s: _js_regexp_interrupted is called %d times, want 1", name, n)
		}
		got, err := fixRegexPoll(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: a fixed file changed", name)
		}
		bare := unfixRegexPollFile(t, bytes.Clone(b))
		if bytes.Contains(bare, []byte("_re_poll_interrupts(")) || bytes.Contains(bare, []byte("_js_regexp_interrupted(")) {
			t.Fatalf("%s: a change is still there", name)
		}
		if got, err = fixRegexPoll(bare, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: the change put back differs", name)
		}
	}
}

// A change to the functions the steps are counted in or on, or to the
// places fixRegexPoll puts them, stops it.
func TestRegexPollRejects(t *testing.T) {
	b := string(engineFile(t))
	bare := string(unfixRegexPollFile(t, []byte(b)))
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"another counter reset", bare, "Finterrupt_counter = int32(MJS_INTERRUPT_COUNTER_INIT)", "Finterrupt_counter = int32(MJS_INTERRUPT_COUNTER_INIT + 1)", "changed (fingerprint"},
		{"another handler call", bare, "return libc.BoolInt32((*TJSRuntime)(unsafe.Pointer(rt)).Finterrupt_handler != 0 &&", "return libc.BoolInt32((*TJSRuntime)(unsafe.Pointer(rt)).Finterrupt_handler == 0 &&", "changed (fingerprint"},
		{"another split", bare, "if _dbuf_insert(tls, s, start, int32(5)) != 0 {", "if _dbuf_insert(tls, s, start, int32(6)) != 0 {", "changed (fingerprint"},
		{"another message buffer", bare, "libc.X__builtin___vsnprintf_chk(tls, s+168,", "libc.X__builtin___vsnprintf_chk(tls, s+160,", "changed (fingerprint"},
		{"a term polled twice", b, pollTerm("s"), pollTerm("s") + pollTerm("s"), "times, want once"},
		{"the term polled after the call", b, pollTerm("s") + "\t\tret = _re_parse_term(tls, s, is_backward_dir)\n", "\t\tret = _re_parse_term(tls, s, is_backward_dir)\n" + pollTerm("s"), "is not where fixlib puts it"},
		{"a fifth step", b, "\tstack_size = 0\n\tstack_size_max = 0\n", "\tstack_size = 0\n\tstack_size_max = 0\n\t_ = _re_poll_interrupts(tls, 0, 0)\n", "want 4 calls"},
		{"the branch after the SyntaxError", b, "\t\t} else if _js_regexp_interrupted(bp+12) {\n", "\t\t} else if _js_regexp_interrupted(bp+12) {\n\t\t\t_ = 0\n", "is not where fixlib puts it"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := fixRegexPoll([]byte(strings.Replace(c.in, c.old, c.new, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
