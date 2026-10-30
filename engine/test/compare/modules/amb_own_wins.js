// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// A module of module_ambiguous_export.mjs: ns of amb_c.js and a foo under the
// name ns.
export * from "./amb_c.js"
export var ns = 1
