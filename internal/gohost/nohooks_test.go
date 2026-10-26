// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !testhooks

package gohost

import "testing"

// TestRefusedCalls marks the tests that make calls to the OS fail: they need
// the hook that only a build with the tag testhooks has.
func TestRefusedCalls(t *testing.T) {
	t.Skip("the tests that make the OS refuse a call run with -tags testhooks")
}
