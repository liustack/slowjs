// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The host stand-ins of the stand-in mode (setup.go): the first prelude of the program that
// measured the pages of v0.31.1 and before. The bundles of engine/pgo/pages define their own
// console when they load.
globalThis.print = function () {};
globalThis.console = { log() {}, info() {}, warn() {}, error() {}, debug() {} };
globalThis.performance = { now() { return 0; } };
