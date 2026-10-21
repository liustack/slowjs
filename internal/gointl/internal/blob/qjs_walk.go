// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blob

// Keys lists a table's keys in order. quickjs's pack tool reads every record
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
