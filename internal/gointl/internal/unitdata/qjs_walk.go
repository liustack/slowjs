// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package unitdata

import (
	"fmt"

	"github.com/liustack/quickjs/internal/gointl/internal/blob"
)

// Walk reads all of a locale's data, the tables Decode leaves in the pool
// included, so that quickjs's pack tool learns which parts of the pool the
// locale reaches.
func Walk(data []byte, pool blob.Shared) error {
	l, err := Decode(data, pool)
	if err != nil {
		return err
	}
	for i := range l.Widths {
		w := &l.Widths[i]
		for _, name := range w.units.Keys() {
			if _, ok := w.Unit(name); !ok {
				return fmt.Errorf("unitdata: unit %s does not read", name)
			}
		}
	}
	return nil
}
