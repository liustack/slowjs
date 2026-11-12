// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: sloppy functions in the register loop. The this of a sloppy
// function is coerced (undefined and null to the global object, a
// primitive to its wrapper), that of a strict one is not: undefined, null,
// a number, a string, a boolean, a symbol and an object, given by a plain
// call, a method call in the loop and Function.prototype.call from C. A
// store decides whether to throw by the mode of the function that makes it,
// not by that of the frame the loop was entered with: a strict function
// calls a sloppy one in the loop and the other way round, for a store to an
// undeclared variable, to a read-only property, to a read-only element, and
// a delete of a non-configurable property. This file is sloppy, the strict
// functions say "use strict" in their bodies. The functions sN and tN are
// the ones the register loop takes, each runs several times, so that its
// later calls run in the register loop. ck catches what they throw.
(function () {
var out = [];
function p(x) { out.push(String(x)); }
function ck(f, a, b) {
  try { return "ok " + f(a, b); } catch (e) { return e.name + ": " + e.message; }
}
var G = this;
var sym = Symbol("s");
var frozen = Object.freeze({ x: 1 });
var ro = Object.defineProperty({}, "x", { value: 1, writable: false, configurable: false });
var farr = Object.freeze([1, 2]);

// this, sloppy and strict
function sThis() { return this === G ? "global" : typeof this + ":" + (this === null ? "null" : String(this === undefined ? "u" : this.valueOf === undefined ? "?" : typeof this.valueOf())); }
function tThis() { "use strict"; return this === G ? "global" : this === null ? "null" : this === undefined ? "undefined" : typeof this; }
function sKind() { return this instanceof Number ? "Number" : this instanceof String ? "String" : this instanceof Boolean ? "Boolean" : this instanceof Symbol ? "Symbol" : this === G ? "global" : typeof this; }
// a plain call and a method call made in the register loop
function sPlain() { return sThis() + " " + sKind(); }
function tPlain() { "use strict"; return tThis() + " " + sThis(); }
Number.prototype.sM = sKind; String.prototype.sM = sKind; Boolean.prototype.sM = sKind; Symbol.prototype.sM = sKind;
Number.prototype.tM = tThis; String.prototype.tM = tThis; Boolean.prototype.tM = tThis; Symbol.prototype.tM = tThis;
function sMeth(v) { return v.sM() + " " + v.tM(); }
function tMeth(v) { "use strict"; return v.sM() + " " + v.tM(); }

// stores, each made by its own function: the s ones are sloppy, the t ones
// strict
function sUndecl(v) { undeclaredW1 = v; return v; }
function tUndecl(v) { "use strict"; undeclaredW1 = v; return v; }
function sRO(o, v) { o.x = v; return o.x; }
function tRO(o, v) { "use strict"; o.x = v; return o.x; }
function sEl(a, v) { a[0] = v; return a[0]; }
function tEl(a, v) { "use strict"; a[0] = v; return a[0]; }
function sDel(o) { return delete o.x; }
function tDel(o) { "use strict"; return delete o.x; }
// a strict caller of the sloppy stores, a sloppy caller of the strict ones,
// and a chain strict, sloppy, strict
function tCallsS(k, v) {
  "use strict";
  if (k === 0) return sUndecl(v);
  if (k === 1) return sRO(frozen, v);
  if (k === 2) return sRO(ro, v);
  if (k === 3) return sEl(farr, v);
  return 0;
}
function sCallsT(k, v) {
  if (k === 0) return tUndecl(v);
  if (k === 1) return tRO(frozen, v);
  if (k === 2) return tRO(ro, v);
  if (k === 3) return tEl(farr, v);
  return 0;
}
function sMid(k, v) { return sCallsT(k, v); }
function tChain(k, v) { "use strict"; return sMid(k, v) + 1; }
function tDelS(o) { "use strict"; return sDel(o); }
function sDelT(o) { return tDel(o); }

var vals = [undefined, null, 5, "str", true, sym, { o: 1 }];
var names = ["undefined", "null", "number", "string", "boolean", "symbol", "object"];
var i, k;
for (k = 0; k < 3; k++) {
  p("plain: " + sPlain() + " | " + tPlain());
  for (i = 0; i < vals.length; i++) {
    p(names[i] + " call: " + sThis.call(vals[i]) + " / " + sKind.call(vals[i]) + " / " + tThis.call(vals[i]));
    if (i >= 2 && i <= 5) p(names[i] + " method: " + ck(sMeth, vals[i]) + " / " + ck(tMeth, vals[i]));
  }
  for (i = 0; i < 4; i++) {
    p("strict calls sloppy " + i + ": " + ck(tCallsS, i, k));
    delete G.undeclaredW1;
    p("sloppy calls strict " + i + ": " + ck(sCallsT, i, k));
    delete G.undeclaredW1;
    p("strict, sloppy, strict " + i + ": " + ck(tChain, i, k));
    delete G.undeclaredW1;
  }
  p("delete: " + ck(tDelS, ro) + " / " + ck(sDelT, ro) + " / " + ck(sDel, { x: 1 }) + " / " + ck(tDel, { x: 1 }));
}
rvExpect(sThis, tThis, sKind, sPlain, tPlain, sMeth, tMeth, sUndecl, tUndecl, sRO, tRO, sEl, tEl,
  tCallsS, sCallsT, sMid, tChain);
rvExpect(sDel, tDel, tDelS, sDelT);
return out.join("\n");
})()
