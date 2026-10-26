// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build cengine && !testhooks

package cengine

import "testing"

// TestLifecycleHooks marks the tests that stop the engine's callbacks or make
// the OS refuse a call: they need the hooks that only a build with the tag
// testhooks has.
func TestLifecycleHooks(t *testing.T) {
	t.Skip("the tests that stop callbacks or make the OS refuse a call run with -tags cengine,testhooks")
}
