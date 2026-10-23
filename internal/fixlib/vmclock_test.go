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

func TestVMClockRewrite(t *testing.T) {
	for _, fn := range libFiles(t) {
		fixed, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		bare, err := restoreVMClock(fixed)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(bare, fixed) {
			t.Fatalf("%s: missing VM clock rewrite", fn)
		}
		again, err := fixVMClock(bare, fn)
		if err != nil || !bytes.Equal(again, fixed) {
			t.Fatalf("%s: round trip differs: %v", fn, err)
		}
		again, err = fixVMClock(fixed, fn)
		if err != nil || !bytes.Equal(again, fixed) {
			t.Fatalf("%s: not idempotent: %v", fn, err)
		}
		for _, bad := range [][]byte{
			bytes.Replace(bare, []byte("_date_now(tls)"), []byte("0"), 1),
			append(bytes.Clone(bare), []byte("\nfunc newClock() { _date_now(tls) }\n")...),
			bytes.Replace(fixed, []byte("dateNowContext("), []byte("otherClock("), 1),
		} {
			if _, err := fixVMClock(bad, fn); err == nil {
				t.Fatalf("%s: accepted changed call sites", fn)
			}
		}
	}
}
