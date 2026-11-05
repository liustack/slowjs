// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !race

package quickjs // import "github.com/liustack/quickjs"

import "time"

// stopLimit is how long a stop test lets an interrupted loop run on.
const stopLimit = 50 * time.Millisecond
