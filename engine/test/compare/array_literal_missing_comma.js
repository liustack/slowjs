// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
const out = [], print = (...a) => out.push(a.join(" "))
// An array literal needs a comma between elements, also after a hole and past
// 32 elements (upstream QuickJS c4b449c).
const big = Array.from({ length: 40 }, (_, i) => i);
for (const src of ["[1 2]", "[, 1 2]", "[, 1 2 3]", "[" + big.join(",") + " 99]", "[, 1, 2]", "[" + big.join(",") + ",]",
                   "[" + big.join(",") + ",,7]", "[,]", "[1,,]", "[,,1 ]", "[1 ,, 2 3]"]) {
  try { print(JSON.stringify(src).slice(0, 40), "->", JSON.stringify((0, eval)(src))); } catch (e) { print(JSON.stringify(src).slice(0, 40), "->", e.name, e.message); }
}
out.join("\n")
