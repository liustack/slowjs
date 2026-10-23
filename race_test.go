// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build race

package slowjs // import "github.com/liustack/slowjs"

import "time"

// stopLimit is how long a stop test lets an interrupted loop run on. The race
// detector makes every engine step several times slower, and the interrupt
// check counts steps, so the check comes later in wall time.
const stopLimit = 500 * time.Millisecond
