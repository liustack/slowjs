// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reltimedata

import "github.com/liustack/quickjs/internal/gointl/internal/blob"

// Walk reads all of a locale's data, the fields Decode leaves in the pool
// included, so that quickjs's pack tool learns which parts of the pool the
// locale reaches.
func Walk(data []byte, pool blob.Shared) error {
	l, err := Decode(data, pool)
	if err != nil {
		return err
	}
	for i := range l.fields {
		if err := l.pool.Read(l.fields[i], func(r *blob.Reader) { var f Field; decodeField(r, &f) }); err != nil {
			return err
		}
	}
	return nil
}
