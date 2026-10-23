// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"fmt"
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

// readAfter reads the generated file fn of lib/ as the final change named
// change leaves it, with the final changes made after it undone.
func readAfter(fn, change string) ([]byte, error) {
	b, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}
	for i := len(finalChanges) - 1; i >= 0; i-- {
		if finalChanges[i].name == change {
			return b, nil
		}
		if b, err = finalChanges[i].restore(b, filepath.Base(fn)); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("no final change %s", change)
}
