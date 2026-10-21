// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"os"
	"path/filepath"
)

// readNoDefer reads the generated file fn of lib/ as fixNoDefer leaves it,
// before the final changes.
func readNoDefer(fn string) ([]byte, error) {
	b, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}
	return restoreFinal(b, filepath.Base(fn))
}
