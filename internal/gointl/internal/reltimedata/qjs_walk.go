// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package reltimedata

import "github.com/liustack/slowjs/internal/gointl/internal/blob"

// Walk reads all of a locale's data, the fields Decode leaves in the pool
// included, so that slowjs's pack tool learns which parts of the pool the
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
