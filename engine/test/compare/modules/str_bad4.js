// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// A module of module_string_names.mjs: two exports under one string name.
var x = 1
var y = 2
export { x as "dup", y as "dup" }
