// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// A module of module_string_names.mjs: a string export name that is not
// well formed is a syntax error.
var x = 1
export { x as "\uD800" }
