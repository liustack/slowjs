// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
const out = [], print = (...a) => out.push(a.join(" "))
// Before the parser reads an arrow function parameter list, a destructuring
// target or the head of a for, it scans ahead over the tokens to their closing
// parenthesis or bracket, and has to tell a "/" that divides from one that
// starts a regexp without parsing. A wrong guess swallows the rest of the line
// as a regexp, or reads a ")" or "]" inside a regexp as a token, and then a
// valid program is a SyntaxError. Each case runs in an indirect eval, so that
// one that fails prints its error and the next ones still run.
function t(name, src) {
  let r;
  try { r = (0, eval)(src); r = typeof r === "function" ? r() : r; r = JSON.stringify(r); }
  catch (e) { r = e.name + ": " + e.message; }
  print(name, r);
}
var o8 = { of: 8, default: 8, new: 8, delete: 8, in: 8, typeof: 8, return: 8, class: 8, function: 8, if: 8, for: 8, while: 8, yield: 8, await: 8, let: 8, async: 8 };
// A "/" after these divides.
t("of in template", "(a = `${({of:8}).of / 2}`) => a");
t("property of", "(a = o8.of / 2) => a");
t("property of /=", "var o = {of: 8}; (a = (o.of /= 2, o.of)) => a");
t("optional of", "(a = o8?.of / 2) => a");
t("variable of", "var of = 6; (a = of / 2) => a");
t("for of variable", "(a = (() => { var of = [8], r; for (var x of of / 1 ? of : []) r = x; return r / 2 })()) => a");
for (const k of ["default", "new", "delete", "in", "typeof", "return", "class", "function", "if", "for", "while", "let", "async", "yield", "await"])
  t("property " + k, "(a = o8." + k + " / 2) => a");
t("async arrow", "typeof (async (a = o8.of / 2) => a)");
t("postfix ++", "var i = 8; (a = i++ / 2) => a");
t("object literal", "(a = {v: 8}.v / 2, b = {} / 1) => [a, b]");
t("function expression", "(a = function () {} / 1, b = function f() { return 8 }() / 2) => [a, b]");
t("class expression", "(a = class {} / 1, b = class extends Object { static v = 8 }.v / 2) => [a, b]");
t("ternary object", "(a = 1 ? {} : {} / 1, b = (0 ? 1 : {v: 8}).v / 2) => [a, b]");
t("label and object", "(a = (() => { l: { var x = {v: 8}.v / 2 } return x })()) => a");
t("call", "(a = Math.abs(8) / 2) => a");
t("index", "(a = [8][0] / 2) => a");
t("template", "(a = `8` / 2) => a");
t("for head of", "var r; for (const x of [o8.of / 2]) r = x; r");
t("for destructuring", "var r; for (const [x = o8.of / 2] of [[]]) r = x; r");
t("for let of", "var r; for (let of of [/[)]/.source]) r = of; r");
t("for of of", "var of; for (of of [/[)]/.source]); of");
out.join("\n")
