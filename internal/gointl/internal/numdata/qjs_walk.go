// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package numdata

import (
	"fmt"

	"github.com/liustack/quickjs/internal/gointl/internal/blob"
)

// Walk reads all of a locale's data, the currencies Decode leaves in the pool
// included, so that quickjs's pack tool learns which parts of the pool the
// locale reaches.
func Walk(data []byte, pool blob.Shared) error {
	l, err := Decode(data, pool)
	if err != nil {
		return err
	}
	for _, code := range l.currencies.Keys() {
		if _, ok := l.Currency(code); !ok {
			return fmt.Errorf("numdata: currency %s does not read", code)
		}
	}
	return nil
}
