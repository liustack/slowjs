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
// A "/" after these starts a regexp.
t("after if", "(a = `${(() => { if (true) /[)]/.test('x'); return 1 })()}`) => a");
t("after if plain", "(a = (() => { if (true) /[)]/.test('x'); return 1 })()) => a");
t("after while", "(a = (() => { while (false) /[)]/; return 2 })()) => a");
t("after for", "(a = (() => { for (;false;) /[)]/; return 3 })()) => a");
t("after for in", "(a = (() => { for (var k in {}) /[)]/; return 4 })()) => a");
t("after for of", "(a = (() => { for (var k of []) /[)]/; return 5 })()) => a");
t("after with", "(a = (() => { with ({}) /[)]/; return 6 })()) => a");
t("after do while", "(a = (() => { do ; while (false) /[)]/.test(''); return 7 })()) => a");
t("after else", "(a = (() => { if (false) ; else /[)]/; return 8 })()) => a");
t("after block", "(a = (() => { { } /[)]/.test(''); return 9 })()) => a");
t("after if block", "(a = (() => { if (true) { } /[)]/.test(''); return 10 })()) => a");
t("after else block", "(a = (() => { if (true) { } else { } /[)]/.test(''); return 11 })()) => a");
t("after try", "(a = (() => { try { } catch (e) { } finally { } /[)]/.test(''); return 12 })()) => a");
t("after catch", "(a = (() => { try { } catch { } /[)]/.test(''); return 13 })()) => a");
t("after switch", "(a = (() => { switch (1) { case 1: } /[)]/.test(''); return 14 })()) => a");
t("after case", "(a = (() => { switch (1) { case 1: /[)]/.test('') } return 15 })()) => a");
t("after case block", "(a = (() => { switch (1) { case 1 ? 1 : 0: { } /[)]/.test('') } return 16 })()) => a");
t("after label block", "(a = (() => { l: { } /[)]/.test(''); return 17 })()) => a");
t("after function", "(a = (() => { function g() { } /[)]/.test(''); return 18 })()) => a");
t("after async function", "(a = (() => { async function g() { } /[)]/.test(''); return 19 })()) => a");
t("after generator", "(a = (() => { function* g() { } /[)]/.test(''); return 20 })()) => a");
t("after class", "(a = (() => { class C { m() { } } /[)]/.test(''); return 21 })()) => a");
t("after class extends", "(a = (() => { class C extends Object { } /[)]/.test(''); return 22 })()) => a");
t("after arrow body", "(a = (() => { var f = () => { }\n/[)]/.test(''); return 23 })()) => a");
t("after object line", "(a = (() => { var x = 1\n{ }\n/[)]/.test(''); return 24 })()) => a");
t("after function line", "(a = (() => { var x = 1\nfunction g() { }\n/[)]/.test(''); return 25 })()) => a");
t("after return line", "(a = String((() => { return\n{ }\n/[)]/ })())) => a");
t("after prefix ++", "(a = (() => { var r = 0; ++/[)]/.lastIndex; return 26 })()) => a");
t("after yield", "(a = (function* () { yield /[)]/ })().next().value.source) => a");
t("after await", "(a = (async () => await /[)]/)() instanceof Promise) => a");
t("after static block", "(a = (() => { class C { static { { } /[)]/.test('') } } return 27 })()) => a");
t("after async line", "(a = (() => { var x = 1\nasync function f() { } /[)]/; return 28 })()) => a");
t("after async generator line", "(a = (() => { var x = 1\nasync function* g() { } /[)]/; return 29 })()) => a");
t("after call line", "(a = (() => { f()\n{ } /[)]/; function f() { } return 30 })()) => a");
t("after label line", "(a = (() => { 0\nl: { } /[)]/; return 31 })()) => a");
t("after class declaration", "(a = (() => { class C extends (class { }) { } /[)]/; return 32 })()) => a");
t("after member break line", "(a = (() => { var x = { break: 0 }; x.break\n{ } /[)]/; return 33 })()) => a");
t("after var line", "(a = () => { var x\n/[)]/; return 34 }) => a()");
t("after labels in ternaries", "(a = () => { " + "1?".repeat(300) + "1" + ":0".repeat(300) + "; l: { } /[)]/; return 35 }) => a()");
// yield and await read as in the function they are in.
t("yield in an arrow", "var yield = 8; (a = function* () { return x => yield / 2 }) => a().next().value()");
t("await in an arrow", "var await = 8; (a = (async () => (b => await / 2))) => typeof a");
t("await in a function", "var await = 8; (a = (() => { function f() { return await / 2 } return f() })()) => a");
t("yield in a generator method", "(a = { *if() { yield /[)]/ } }) => a.if().next().value.source");
t("await in an async method", "(a = { async if() { await /[)]/ } }) => typeof a.if");
t("await in a computed async method", "(a = { async [(() => 'm')()]() { await /[)]/ } }) => typeof a.m");
t("await after a concise async arrow", "var await = 8; (a = true ? async x => x : await / 2) => typeof a");
t("await in a template", "var await = 8; (a = `${async x => x}${await / 2}`) => a.endsWith('4')");
t("yield in a block", "var yield = 8; (a = () => { 2 * f()\n{ yield / 2 } function f() { return 2 } return 36 }) => a()");
// An async arrow function whose parameter is named async, of or yield: its
// body is async, so the "/" after await starts a regexp.
t("async arrow named async", "(a = async async => await /[)]/) => typeof a(1)");
t("async arrow named of", "(a = async of => await /[)]/) => typeof a(1)");
t("async arrow named yield", "(a = async yield => await /[)]/) => typeof a(1)");
t("async arrow named async in a for", "for (var a = async async => await /[)]/; ;) break; typeof a(1)");
t("async arrow named async in an async function", "async function make() { return (f = async async => await /[)]/) => f } typeof make");
// An assignment in the heritage of a class in a parameter list gives the
// parameters a scope of their own, so a closure made there reads the x
// outside, not the var x of the body.
t("assignment in a class heritage", "let x = 1, read; (function ({[class extends (read = () => x, Object) {}]: value}) { var x = 2; })({}); read()");
// "/" after a class expression divides.
t("class expression", "(x = class extends (class { }) { } / 2) => String(x)");
// Brackets nested deeper than the scan once followed.
t("nested 300", "(a = " + "(".repeat(300) + "37" + ")".repeat(300) + ") => a");
t("nested 255", "(x = " + "(".repeat(255) + "38" + ")".repeat(255) + ") => x");
out.join("\n")
