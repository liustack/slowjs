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
// nothing, whether or not the page ever touches Intl. The formatters hold no
// javascript Value of their own: the shim keeps its state in javascript and
// the host functions trade json strings. But Register hands eight Go
// functions to the VM, and those own libc memory that only Close releases.
//
// The name matches the root package's so that a single -run selects both.
func TestMemgrind2(t *testing.T) {
	// Force libc environ allocation that may otherwise skew the accounting.
	tls := libc.NewTLS()
	tls.Close()

	libc.MemAuditStart()
	for i := range 6 {
		vm, err := quickjs.NewVM()
		if err != nil {
			t.Fatal(err)
		}

		if err := Register(vm, TimeZone("America/New_York")); err != nil {
			vm.Close()
			t.Fatal(err)
		}

		srcs := []string{
			`const d = new Intl.DateTimeFormat('zh-CN', {dateStyle: 'full', timeStyle: 'long'})`,
			`const n = new Intl.NumberFormat('de', {style: 'currency', currency: 'EUR'})`,
			`for (let i = 0; i < 100; i++) { d.format(i * 1e7); n.format(i * 1.5) }`,
			`d.formatToParts(0); n.formatToParts(-1234.5)`,
			`d.resolvedOptions(); n.resolvedOptions()`,
			`new Date(0).toLocaleString('en-US', {timeZone: 'Asia/Tokyo'})`,
			`(1234.5).toLocaleString('en-US')`,
			`Intl.getCanonicalLocales(['en-us', 'DE-de'])`,
			`try { new Intl.DateTimeFormat('en', {timeZone: 'Mars/Olympus'}) } catch (e) { e.message }`,
			`new Intl.DateTimeFormat('fa', {timeZoneName: 'longGeneric'}).format(0)`,
			`new Intl.PluralRules('ru').select(21); new Intl.PluralRules('en', {type: 'ordinal'}).selectRange(1, 2)`,
		}
		switch i % 3 {
		case 1:
			// A page that never touches Intl: the stand-ins and the host
			// functions they hold are all that is there.
			srcs = []string{`1 + 1`}
		case 2:
			// A page whose first touch is a method taken beforehand.
			srcs = []string{`const f = Date.prototype.toLocaleDateString; f.call(new Date(0), 'ja')`}
		}
		for _, src := range srcs {
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
