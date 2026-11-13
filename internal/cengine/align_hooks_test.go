// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package cengine

import (
	"strings"
	"testing"
)

// TestCheckSP: in a test build a shadow pointer off 16 bytes stops the call
// into the engine before it runs, and one on 16 passes.
func TestCheckSP(t *testing.T) {
	CheckSP(0)
	CheckSP(1 << 40)
	for _, sp := range []uintptr{8, 1<<40 + 4, 1<<40 + 24} {
		func() {
			defer func() {
				r := recover()
				if s, _ := r.(string); !strings.Contains(s, "not on 16 bytes") {
					t.Errorf("CheckSP(%#x): %v, want a panic", sp, r)
				}
			}()
			CheckSP(sp)
		}()
	}
}
