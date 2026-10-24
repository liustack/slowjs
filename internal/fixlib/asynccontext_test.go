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

// fixAsyncContext is the inverse of its exact inverse on every target, and
// stops on a changed function it relies on, a partial or edited change, a
// new store into the handler of a reaction and a new queueing of the job
// that calls the then of a thenable.
func TestAsyncContextRewrite(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		bare, err := restoreAsyncContext(b, name)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(bare, []byte("asyncContextWrap(")) {
			t.Fatalf("%s: restoreAsyncContext left the change", name)
		}
		fixed, err := fixAsyncContext(bare, name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(fixed, b) {
			t.Fatalf("%s: fixAsyncContext does not give the committed file", name)
		}
		if got := bytes.Count(fixed, []byte("asyncContextWrap(")); got != 2 {
			t.Fatalf("%s: asyncContextWrap %d times, want twice", name, got)
		}
		again, err := fixAsyncContext(fixed, name)
		if err != nil || !bytes.Equal(fixed, again) {
			t.Fatalf("%s not idempotent: %v", name, err)
		}

		changed := bytes.Replace(bare, []byte("func _promise_reaction_job(tls *libc.TLS, "), []byte("func _promise_reaction_job(tls *libc.TLS, _ int, "), 1)
		if bytes.Equal(changed, bare) {
			t.Fatalf("%s: the change of the test does not apply", name)
		}
		if _, err := fixAsyncContext(changed, name); err == nil {
			t.Fatal("accepted a changed function")
		}
		partial := bytes.Replace(fixed, []byte("\tthen = asyncContextWrap(tls, ctx2, then)\n"), nil, 1)
		if _, err := fixAsyncContext(partial, name); err == nil {
			t.Fatal("accepted a partial rewrite")
		}
		edited := bytes.Replace(fixed, []byte("asyncContextWrap(tls, ctx2, v6)"), []byte("asyncContextWrap(tls, ctx2, v3)"), 1)
		if _, err := fixAsyncContext(edited, name); err == nil {
			t.Fatal("accepted an edited call")
		}
		store := append(bytes.Clone(bare), "\nfunc _js_new_store(tls *libc.TLS, rd uintptr, v TJSValue) {\n\t(*TJSPromiseReactionData)(unsafe.Pointer(rd)).Fhandler = v\n}\n"...)
		if _, err := fixAsyncContext(store, name); err == nil || !strings.Contains(err.Error(), "stores into the handler") {
			t.Fatalf("%s: accepted a new store into the handler of a reaction: %v", name, err)
		}
		job := append(bytes.Clone(bare), "\nfunc _js_new_job(tls *libc.TLS, ctx uintptr, argv uintptr) {\n\tXJS_EnqueueJob(tls, ctx, __ccgo_fp(_js_promise_resolve_thenable_job), int32(3), argv)\n}\n"...)
		if _, err := fixAsyncContext(job, name); err == nil || !strings.Contains(err.Error(), "thenable job") {
			t.Fatalf("%s: accepted a new queueing of the thenable job: %v", name, err)
		}
	}
}
