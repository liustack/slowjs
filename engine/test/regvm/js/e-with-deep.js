// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// WITH 2 (the poll after a found name) with a stack over 32 temps at the
// target, after a getter ran: the temps 32 and up must be in its pcmap mask
// inject-hit: WITH 2
function g() { return arguments.length; }
function f(o) { with (o) return g({},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{},{}, x); }
var o = { get x() { return {}; } };
var r = 0;
for (var i = 0; i < 3; i++) r += f(o);
rvExpect(f);
