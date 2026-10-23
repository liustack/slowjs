// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package namedata

import "github.com/liustack/slowjs/internal/gointl/internal/blob"

// Walk reads all of a locale's data, the tables Decode leaves in the pool
// included, so that slowjs's pack tool learns which parts of the pool the
// locale reaches.
func Walk(data []byte, pool blob.Shared) error {
	l, err := Decode(data, pool)
	if err != nil {
		return err
	}
	for i, set := range l.sets {
		for _, code := range set.Keys() {
			// The set's own record is read first. A name it leaves empty
			// is looked for in the wider sets, which are walked anyway.
			l.Lookup(i/Widths, i%Widths, code)
		}
	}
	return nil
}
