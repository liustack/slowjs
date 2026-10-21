// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// coldSites are the functions whose cold path fixColdFrames moves into a
// function of its own, so that the rest of the TLS frame becomes a Go
// array: the cold function, the frame size and the number of cell uses
// left in lib/ccgo_darwin_arm64.go, and what the cells and the cold path
// are for. Both call TLS.Alloc often in a server render of a React page
// (linux/amd64 profile, 2026-10-19). Each must qualify (see
// fixColdFrames).
var coldSites = map[string]coldSite{
	"___JS_AtomToValue": {"___JS_AtomToValue_cold", 80, 4, "an atom's string or symbol, to read its pointer (JS_DupValue); the cold path writes the digits of an integer atom"},
	"_js_strict_eq2":    {"_js_strict_eq2_cold", 96, 30, "the two operands and float conversions; the cold path compares two BigInts in buffers of the frame"},
}
