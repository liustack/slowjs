// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// sharedModule is sharedScript as a module that awaits before it sets
// globalThis.shared, as a bundle with top-level await does.
var sharedModule = "await Promise.resolve();\n" + sharedScript
