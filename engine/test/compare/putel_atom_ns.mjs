// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// o[k] = v on a module namespace and on an object whose prototype is one, with
// a key that is an atom (from Object.keys or a literal) and the same key made
// again as a string that is not an atom: a namespace takes no key and changes
// none (a TypeError in a module, which is strict code), and an object whose
// prototype is a namespace gets the key as its own. The file is what Node 24
// prints.
import * as a from "./modules/str_a.js"
function put(o, k, v) { o[k] = v }
const fresh = k => typeof k === "string" ? (" " + k).slice(1) : k
const out = []
const keys = [...Object.keys(a), "plain", "nope", "zz", "-0", "1e3", "length", "__proto__", "then", Symbol.toStringTag]
for (const k of keys) {
  const line = []
  for (const key of [k, fresh(k)]) {
    try { put(a, key, 9); line.push("ok") } catch (e) { line.push(e.name) }
    const under = Object.create(a)
    try { put(under, key, 9); line.push("under ok " + Object.keys(under).join("+")) } catch (e) { line.push("under " + e.name) }
  }
  out.push(String(k) + ": " + line.join(", "))
}
out.push("values: " + [a.plain, a["a-b"], a.normal, "nope" in a, Object.isExtensible(a)].join())
globalThis.result = out.join("\n")
