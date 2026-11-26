// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
const out = [], print = (...a) => out.push(a.join(" "))
// An assignment to a name inside a with statement makes a reference that the
// with object may hold. When the name is a const declared inside the with
// statement, the assignment still reads it, evaluates its right side and
// writes, and only the write throws: a logical assignment that does not write
// returns the value. Each line shows the result or the name of the error, the
// variable after it and how many times the right side ran, in the code of a
// direct eval, a class, an arrow function, a function and the with block
// itself, for a const and a let starting at 0 and at 1.
const ops = [
  "x ||= (n++, 2)", "x &&= (n++, 2)", "x ??= (n++, 2)", "x = (n++, 2)", "x += (n++, 2)",
  "x++", "++x", "[x] = (n++, [2])", "({ a: x } = (n++, { a: 2 }))",
  "for (x of (n++, [2])) ;", "for (x in (n++, { k: 1 })) ;",
];
const places = {
  "eval": (b) => "with ({}) { return eval(" + JSON.stringify("(() => { " + b + " })()") + "); }",
  "class": (b) => "with ({}) { class C { static run() { " + b + " } } return C.run(); }",
  "arrow": (b) => "with ({}) { return (() => { " + b + " })(); }",
  "function": (b) => "with ({}) { return (function () { " + b + " })(); }",
  "block": (b) => "with ({}) { " + b + " }",
};
for (const place in places) {
  for (const decl of ["const", "let"]) {
    for (const init of [0, 1]) {
      for (const op of ops) {
        const body = decl + " x = " + init + "; let n = 0, r; try { " + (op.startsWith("for") ? op + " r = \"done\";" : "r = (" + op + ");") + " } catch (e) { r = e.name; } return [r, x, n];";
        let r;
        try { r = JSON.stringify(Function(places[place](body))()); } catch (e) { r = e.name; }
        print(place, decl, init, op, r);
      }
    }
  }
}
// The same in a function with many names, which the compiler looks up through
// an index, from the function itself and from an arrow function inside it.
const many = Array.from({ length: 40 }, (_, i) => "v" + i + " = " + i).join(", ");
for (const place of ["arrow", "function"]) {
  for (const init of [0, 1]) {
    for (const op of ["x ||= (n++, 2)", "x = (n++, 2)", "x++"]) {
      for (const inner of [false, true]) {
        const run = inner ? "(() => (" + op + "))()" : "(" + op + ")";
        const body = "const " + many + "; const x = " + init + "; let n = 0, r; try { r = " + run + "; } catch (e) { r = e.name; } return [r, x, n];";
        let r;
        try { r = JSON.stringify(Function(places[place](body))()); } catch (e) { r = e.name; }
        print(place, "many", init, op, inner ? "inner" : "here", r);
      }
    }
  }
}
out.join("\n")
