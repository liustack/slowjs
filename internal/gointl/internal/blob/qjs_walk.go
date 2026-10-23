// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package blob

// Keys lists a table's keys in order. slowjs's pack tool reads every record
// of a table with them, to learn which parts of a pool a locale's data
// reaches.
func (t Table) Keys() []string {
	keys := make([]string, t.Len())
	for i := range keys {
		k, _ := t.pool.part(t.number(i, 0))
		keys[i] = string(k)
	}
	return keys
}
