// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
const out = [], print = (...a) => out.push(a.join(" "))
// A function holds at most 65533 block scopes (upstream QuickJS a6da4a3). The
// number is written in 16 bits in the code, so one more is a compile error
// and not wrong code. Closures made before and after many empty blocks still
// see their own variable.
function make(n, tail) {
  let body = "var fs = [];\n";
  body += "{ let x = 'first'; fs.push(() => x); }\n{ let y = 'second'; fs.push(() => y); }\n";
  for (let i = 0; i < n; i++) body += "{ 0; }\n";
  body += "{ let z = 'late1'; fs.push(() => z); }\n{ let w = 'late2'; fs.push(() => w); }\n";
  body += tail;
  return body + "return fs.map(f => f());";
}
function run(label, body) {
  try { print(label, JSON.stringify(new Function(body)())); } catch (e) { print(label, e.name, e.message); }
}
run("65000 blocks", make(65000, ""));
run("65540 blocks", make(65540, ""));
// the limit as such: the body scope and the 4 blocks above are 5 scopes of the 65533 left
for (const n of [65520, 65527, 65528, 65529, 65530]) run(n + " blocks", make(n, ""));
run("inner function", "return (function () {" + make(65540, "") + "})();");
run("class and loops", make(65530, "for (let i = 0; i < 2; i++) { fs.push(() => i); }\nclass C { #p = 1; static s = 2; }\ntry { throw 1 } catch (e) { fs.push(() => e); }\n"));
// Each kind of statement that pushes a scope, entered with the count of scopes
// at its limit: it compiles or gives the one error, and never anything else.
const kinds = {
  block: "{ let a = 1; }",
  forof: "for (const a of [1]) { }",
  forin: "for (const a in {x: 1}) { }",
  forloop: "for (let a = 0; a < 1; a++) { }",
  ifdecl: "if (1) function g() { }",
  switch_: "switch (1) { case 1: let a = 1; }",
  catchvar: "try { throw 1 } catch (e) { }",
  with_: "with ({}) { }",
  class_: "class C { #p = 1; m() { } }",
  class_extends: "class C extends Object { }",
  class_expr: "(class { static { } })",
  arrow: "(() => { })",
  param_expr: "(function (a = 1) { })",
};
for (const [name, stmt] of Object.entries(kinds)) {
  const seen = [];
  for (let n = 65522; n <= 65533; n++) {
    let body = "";
    for (let i = 0; i < n; i++) body += "{ 0; }\n";
    let r;
    try { new Function(body + stmt); r = "ok"; } catch (e) { r = e.name + " " + e.message; }
    if (seen.length === 0 || seen[seen.length - 1][1] !== r) seen.push([n, r]);
  }
  print(name, JSON.stringify(seen));
}
out.join("\n")
