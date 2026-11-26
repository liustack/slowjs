// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
const out = [], print = (...a) => out.push(a.join(" "))
// A property name of a destructuring pattern is followed by ':' (upstream
// QuickJS c9d81f5). A string, a number or a computed name with anything else
// after it is a SyntaxError.
for (const src of ['var {"a" = b} = {a:7}; b', 'var {0 + b} = {0:7}; b', 'var {["a"] ; b} = {a:7}; b', '({"a" ! b} = {a:7}); b',
                   'var {"a" [b]} = {a:[7]}; b', 'var {a: [b]} = {a:[1]}; b', 'var {"a": b} = {a:2}; b', 'var {0: b} = {0:3}; b',
                   'var {["a"]: b} = {a:4}; b', 'var {a = 5} = {}; a', 'var {"a" 1: b} = {a:7}; b']) {
  try { print(JSON.stringify(src), "->", String((0, eval)(src))); } catch (e) { print(JSON.stringify(src), "->", e.name, e.message); }
}
out.join("\n")
