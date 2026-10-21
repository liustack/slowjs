// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build libc.memgrind

package temporal // import "github.com/liustack/quickjs/temporal"

import (
	"testing"

	"github.com/liustack/quickjs"
	"modernc.org/libc"
)

// TestMemgrind3 checks that registering Temporal and using it leaks
// nothing, whether or not the page ever touches it: the bridge trades
// numbers and strings, and every array it makes is freed.
//
// The name matches the pattern of the root package's and intl's so that a
// single -run selects all of them.
func TestMemgrind3(t *testing.T) {
	tls := libc.NewTLS()
	tls.Close()

	probe := probeScript()
	libc.MemAuditStart()
	for i := range 6 {
		vm, err := quickjs.NewVM()
		if err != nil {
			t.Fatal(err)
		}

		if err := Register(vm); err != nil {
			vm.Close()
			t.Fatal(err)
		}

		srcs := []string{probe, `for (let i = 0; i < 50; i++) Temporal.ZonedDateTime.from('2024-03-10T01:30[America/New_York]').add({hours: i}).until('2025-01-01T00:00[America/New_York]', {largestUnit: 'year'}).toString()`}
		switch i % 3 {
		case 1:
			srcs = []string{`1 + 1`}
		case 2:
			srcs = []string{`const f = Date.prototype.toTemporalInstant; f.call(new Date(0)).toString()`}
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
