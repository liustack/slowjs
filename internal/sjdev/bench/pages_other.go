// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !linux

package bench

import "fmt"

// DropPages is only on linux.
func DropPages(path string) error {
	return fmt.Errorf("dropping the cached pages of a file needs linux")
}
