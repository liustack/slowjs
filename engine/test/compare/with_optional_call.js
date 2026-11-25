// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
const out = [], print = (...a) => out.push(a.join(" "))
// An optional call of a name that a with scope may hold keeps the stack
// depth right (upstream QuickJS 1eba383). The call of a with variable pushes
// the object and the function, so the short cut of "?." drops both.
var o = { f: null, g() { return "g"; } };
with (o) { print(String(f?.())); print(g?.()); }
function t() { with (o) { return [f?.(), g?.(), f?.(1, 2)]; } }
print(JSON.stringify(t()));
for (let i = 0; i < 3; i++) with (o) { f?.(); }
function u() { with ({ m() { return this.v; }, v: 5 }) { return [m?.(), (m)?.(), typeof eval?.("1")]; } }
print(JSON.stringify(u()));
print("ok");
out.join("\n")
