// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"strings"
	"testing"
)

// sharedModule is sharedScript as a module that awaits before it sets
// globalThis.shared, as a bundle with top-level await does.
var sharedModule = "await Promise.resolve();\n" + sharedScript

// A module loads from shared bytecode as a script does: its top-level await
// finishes before EvalSharedBytecode returns, and the VM shows the same source
// text, stacks and line numbers as one that loads a copy, and holds less
// memory by at least the source of big.
func TestSharedBytecodeModule(t *testing.T) {
	code := moduleBytecode(t, sharedModule)
	shared := newSharedBytecode(t, code)
	defer shared.Close()

	copied := newTestVM(t)
	if _, err := copied.EvalBytecode(code); err != nil {
		t.Fatal(err)
	}
	m := newTestVM(t)
	if _, err := m.EvalSharedBytecode(shared); err != nil {
		t.Fatal(err)
	}

	want := evalString(t, copied, sharedProbe)
	if got := evalString(t, m, sharedProbe); got != want {
		t.Fatalf("shared bytecode shows\n%s\nwant\n%s", got, want)
	}
	for _, s := range []string{"a comment the source keeps", "at f (<eval>:13:", "at thrower (<eval>:14:", "at stackHere (<eval>:16:", strings.Repeat("x", sharedPad)} {
		if !strings.Contains(want, s) {
			t.Errorf("the probe has no %q:\n%s", s, want)
		}
	}

	copied.RunGC()
	m.RunGC()
	if saved := int(copied.MemorySize()) - int(m.MemorySize()); saved < sharedPad {
		t.Errorf("a VM of shared bytecode holds %d bytes less than one of a copy, want at least %d", saved, sharedPad)
	}
}
