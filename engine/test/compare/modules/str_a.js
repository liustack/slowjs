// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// A module of module_string_names.mjs: bindings exported under names that are
// not identifiers.
var x = 1
var y = 2
let z = 3
export { x as "a-b", y as "with space", z as "\u{1F600}", x as "default" }
export { x as plain }
export var normal = 4
