// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package beastjs // import "github.com/liustack/beastjs"

// sharedModule is sharedScript as a module that awaits before it sets
// globalThis.shared, as a bundle with top-level await does.
var sharedModule = "await Promise.resolve();\n" + sharedScript
