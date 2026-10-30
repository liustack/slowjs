// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// A module of module_string_names.mjs: names that are not identifiers, read
// and exported again.
export { "a-b" as "c-d", "with space" as plain2, "a-b" } from "./str_a.js"
export * as "the namespace" from "./str_a.js"
export { default } from "./str_a.js"
