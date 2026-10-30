// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import "testing"

// toWellFormed keeps the surrogate pairs and replaces each lone surrogate,
// around the places where its loops read the interrupt flag and in long runs
// of pairs. engine/test/compare/to_well_formed.txt is what Node 24 prints for
// engine/test/compare/to_well_formed.js.
func TestToWellFormedOutput(t *testing.T) {
	checkCompareFile(t, "to_well_formed")
}
