// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build race

package whatwgurl

// raceEnabled is true when the race detector is on, which makes the loops of Go
// five to twenty times slower: the tests of how long Go works between two
// questions to the stop function measure that time, and only mean something
// without it.
const raceEnabled = true
