// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// A module of module_string_names.mjs: a string export name without from is
// a syntax error.
var x = 1
export { x as "ok" }
export { "bad" }
