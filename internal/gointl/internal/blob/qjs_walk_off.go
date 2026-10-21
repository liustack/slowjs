// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !gointl_walk

package blob

// Walking reports whether the pool parts read are recorded, which only the
// gointl_walk build tag does, for quickjs's pack tool.
const Walking = false

func walked(Shared, int) {}

// Walked is the parts of pool read since StartWalk. Without gointl_walk,
// nothing is recorded.
func Walked(pool []byte) map[int]bool { return nil }

// StartWalk forgets the parts read so far.
func StartWalk() {}
