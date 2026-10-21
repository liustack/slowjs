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

// The interpreter counts down the interrupt counter in its own body on
// every target, and calls the poll only when the counter runs out.
func TestPollEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		body, err := funcBody(b, reCallInternal, "_JS_CallInternal")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if n := bytes.Count(body, []byte("_js_poll_interrupts(tls, ")) - bytes.Count(body, []byte("___js_poll_interrupts(tls, ")); n != 0 {
			t.Errorf("%s: _JS_CallInternal calls _js_poll_interrupts %d times", name, n)
		}
		if n := bytes.Count(body, []byte("pollDue(")); n != pollCount {
			t.Errorf("%s: _JS_CallInternal counts down %d times, want %d", name, n, pollCount)
		}
	}
}

// fixPoll leaves a fixed file as it is, and puts the change back into a
// file without it, on every target. restorePoll gives back the text
// ccgo writes.
func TestPollIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if bytes.Contains(b, []byte("pollDue(")) {
			t.Fatalf("%s: readEngine leaves the countdown in", name)
		}
		fixed, err := fixPoll(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Equal(fixed, b) {
			t.Fatalf("%s: fixPoll changed nothing", name)
		}
		again, err := fixPoll(bytes.Clone(fixed), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(again, fixed) {
			t.Errorf("%s: a fixed file changed", name)
		}
		back, err := restorePoll(fixed)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(back, b) {
			t.Errorf("%s: restorePoll does not give the text back", name)
		}
	}
}

// A change to js_poll_interrupts, whose countdown pollDue repeats, or to
// the number of polls of the interpreter stops fixPoll.
func TestPollRejects(t *testing.T) {
	b := string(engineFile(t))
	for _, c := range []struct {
		name, old, new, err string
	}{
		{"another counter", "\tv2 = ctx + 424\n", "\tv2 = ctx + 428\n", "changed (fingerprint"},
		{"another threshold", "if libc.BoolInt64(!!(v1 <= libc.Int32FromInt32(0))) != 0 {\n\t\treturn ___js_poll_interrupts(tls, ctx)", "if libc.BoolInt64(!!(v1 < libc.Int32FromInt32(0))) != 0 {\n\t\treturn ___js_poll_interrupts(tls, ctx)", "changed (fingerprint"},
		{"one poll less", "\tif _js_poll_interrupts(tls, caller_ctx) != 0 {\n", "\tif 0 != 0 {\n", "want 8 polls"},
		{"a countdown already there", "\tif _js_poll_interrupts(tls, caller_ctx) != 0 {\n", "\tif pollDue(caller_ctx+424) && ___js_poll_interrupts(tls, caller_ctx) != 0 {\n", "want 8 polls"},
	} {
		if !strings.Contains(b, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := fixPoll([]byte(strings.Replace(b, c.old, c.new, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
