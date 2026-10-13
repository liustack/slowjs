// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build libc.memgrind

package intl // import "github.com/liustack/quickjs/intl"

import (
	"testing"

	"github.com/liustack/quickjs"
	"modernc.org/libc"
)

// TestMemgrind2 checks that registering Intl and formatting with it leaks
// nothing. The formatters hold no javascript Value of their own - the shim
// keeps its state in javascript and the host functions trade json strings -
// but Register does hand five Go functions to the VM, and those own libc
// memory that only Close releases.
//
// The name matches the root package's so that a single -run selects both.
func TestMemgrind2(t *testing.T) {
	// Force libc environ allocation that may otherwise skew the accounting.
	tls := libc.NewTLS()
	tls.Close()

	libc.MemAuditStart()
	for range 4 {
		vm, err := quickjs.NewVM()
		if err != nil {
			t.Fatal(err)
		}

		if err := Register(vm, TimeZone("America/New_York")); err != nil {
			vm.Close()
			t.Fatal(err)
		}

		for _, src := range []string{
			`const d = new Intl.DateTimeFormat('en-US', {dateStyle: 'full', timeStyle: 'long'})`,
			`const n = new Intl.NumberFormat('en-US', {style: 'currency', currency: 'EUR'})`,
			`for (let i = 0; i < 100; i++) { d.format(i * 1e7); n.format(i * 1.5) }`,
			`d.formatToParts(0); n.formatToParts(-1234.5)`,
			`d.resolvedOptions(); n.resolvedOptions()`,
			`new Date(0).toLocaleString('en-US', {timeZone: 'Asia/Tokyo'})`,
			`(1234.5).toLocaleString('en-US')`,
			`Intl.getCanonicalLocales(['en-us', 'DE-de'])`,
			`try { new Intl.DateTimeFormat('en', {timeZone: 'Mars/Olympus'}) } catch (e) { e.message }`,
		} {
			if _, err := vm.Eval(src, quickjs.EvalGlobal); err != nil {
				vm.Close()
				t.Fatal(src, err)
			}
		}
		if err := vm.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
