// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !race

package url

// raceEnabled is false: the loops of Go run at the speed that the tests of
// the time between two questions to the stop function measure.
const raceEnabled = false
