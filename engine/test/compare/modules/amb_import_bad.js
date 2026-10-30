// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// A module of module_ambiguous_export.mjs: a module that imports foo from a
// module where it is ambiguous.
import { foo } from "./amb_two_foo.js"
export var seen = foo
