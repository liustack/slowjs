// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Arbitrary module namespace names (ES2022): a module can export a binding
// under a string that is not an identifier, with export { x as "a-b" } and
// export * as "name" from, and read one with import { "a-b" as c } and
// export { "a-b" } from. A string name without a from clause, one that is not
// well formed and two exports under one name are syntax errors (upstream
// QuickJS 19d6bd8). The file is what Node 24 prints.
import { "a-b" as ab, "with space" as ws, "\u{1F600}" as smile, default as def, plain } from "./modules/str_a.js"
import * as a from "./modules/str_a.js"
import * as b from "./modules/str_b.js"
const out = []
out.push("values: " + [ab, ws, smile, def, plain].join())
out.push("a keys: " + Object.keys(a).join("|"))
out.push("b keys: " + Object.keys(b).join("|"))
out.push("b values: " + [b["c-d"], b.plain2, b["a-b"], b.default, b["the namespace"] === a, b["the namespace"]["a-b"]].join())
out.push("has: " + ["a-b", "c-d", "the namespace", "nope"].map(k => k in b).join())
out.push("descriptor: " + JSON.stringify(Object.getOwnPropertyDescriptor(a, "a-b")))
const tries = []
for (const name of ["str_bad1", "str_bad2", "str_bad3", "str_bad4"]) {
  tries.push(import("./modules/" + name + ".js").then(m => name + " loads", e => name + " " + e.name))
}
Promise.all(tries).then(r => { globalThis.result = out.join("\n") + "\n" + r.join("\n") })
