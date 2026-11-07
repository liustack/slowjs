// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/quickjs/temporal"

import (
	"errors"
	"testing"

	"github.com/liustack/quickjs"
)

// A string argument the VM has no memory to read ends the call with the out
// of memory error: the bridge does not parse it as the empty string. The
// string is a rope of 32 MiB, which reading has to join, in a VM limited to
// 16 MiB.
func TestStringArgumentOutOfMemory(t *testing.T) {
	for _, js := range []string{
		`Temporal.PlainDate.from(s)`,
		`Temporal.Duration.from(s)`,
		`Temporal.PlainDate.from('2024-01-05').toLocaleString(s)`,
	} {
		t.Run(js, func(t *testing.T) {
			vm := newVM(t)
			eval(t, vm, `Temporal.PlainDate.from('2024-01-05').toLocaleString('en'); JSON`)
			vm.SetMemoryLimit(16 << 20)
			_, err := vm.Eval(`
				var r = "ok";
				try {
					let s = "1".repeat(1024);
					for (let i = 0; i < 15; i++) s = s + s;
					`+js+`;
				} catch (e) { r = "caught " + e; }
				r`, quickjs.EvalGlobal)
			var e *quickjs.Error
			if !errors.As(err, &e) || !e.Uncatchable || !e.OutOfMemory {
				t.Fatalf("got %v, want the uncatchable out of memory error", err)
			}
		})
	}
}
