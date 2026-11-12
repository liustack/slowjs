// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package v1

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

// The goldens of the fuzz seeds hold the hash of each program: v1 must make
// the same programs for good. A change here means a change of v1, which the
// goldens cannot follow: put the change into package rvgen instead.
func TestFrozen(t *testing.T) {
	want := map[string]string{
		"core": "9cc46a17f1445b1e6cf72d70b0aca422",
		"full": "63ca17be4a2ed52246a4f1cfcfa56bbf",
		"wide": "6fde6549b924d00578ca86cbd487dea2",
	}
	for _, m := range []string{"core", "full", "wide"} {
		o := Options{Core: m == "core", Wide: m == "wide"}
		h := sha256.New()
		for s := uint64(1); s <= 50; s++ {
			h.Write([]byte(Generate(s, o)))
		}
		if got := fmt.Sprintf("%x", h.Sum(nil)[:16]); got != want[m] {
			t.Errorf("mode %s: the programs of seeds 1 to 50 hash to %s, want %s: v1 changed", m, got, want[m])
		}
	}
}
