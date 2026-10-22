// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs

// The collector header layout is checked against every generated target by
// TestEphemeronHeaderBits in internal/fixlib.
const (
	ephemeronTypeMask  = 0xfe
	ephemeronTypeShift = 1
	ephemeronMarkMask  = 0x1
)
