// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Symbol(s) of a string that something else still holds (an array, a variable,
// the string literal of digits of a function, which every run of it shares)
// makes the symbol of a copy: the string stays a string, and the symbol is
// freed when nothing but a WeakRef holds it. Symbol.for(s) gives the same
// symbol for the same key, and s stays a string. The file is what
// Node 24 prints, with each check run after a timer: a WeakRef keeps its target
// alive until the job that made it ends, and clearKeptObjects() ends it here.
const out = []
function dead(make) {
  const w = new WeakRef(make())
  clearKeptObjects()
  gc()
  return w.deref() === undefined
}
const s = [String(12) + "3"]
out.push("held by an array: " + dead(() => Symbol(s[0])))
let v = String(45) + "6"
out.push("held by a variable: " + dead(() => Symbol(v)))
function digits() { return Symbol("12") }
out.push("a literal of digits: " + dead(digits) + " " + dead(digits))
out.push("the string after: " + typeof s[0] + " " + s[0] + " " + typeof v + " " + v)
const y = Symbol(s[0])
out.push("description: " + y.description + " " + (Symbol(s[0]) !== y) + " " + (s[0] + "4"))
const k = String(7) + "8"
const g = Symbol.for(k)
out.push("Symbol.for: " + (g === Symbol.for("78")) + " " + Symbol.keyFor(g) + " " + typeof k + " " + (k + "9"))
// The description of a symbol, kept after the symbol is dropped, is a string
// of its own and does not keep the symbol alive either.
const descriptions = []
out.push("held by its description: " + dead(() => { const x = Symbol(String(9) + "0"); descriptions.push(x.description); return x }) + " " + descriptions[0] + " " + (descriptions[0] + "1"))
const z = Symbol(String(5) + "5")
out.push("a description read twice: " + (z.description === z.description) + " " + z.description.length + " " + String(z))
// descriptions of millions of characters, of 16 and 8 bits, copied in steps
const long = "€".repeat(3 << 19) + s[0]
const ls = Symbol(long)
out.push("a long description: " + (ls.description === long) + " " + ls.description.length + " " + Symbol("a".repeat(3 << 20)).description.length + " " + String(ls).length)
out.join("\n")
