// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// for of and array destructuring over Arrays, where js_for_of_next reads the
// element of a fast Array itself (js_for_of_array_next in quickjs.c): plain
// arrays, holes past the count read through the prototype, an array that
// grows, shrinks, loses its fast form (a deleted element) or is emptied
// while it is walked, a break that closes the iterator, keys() and
// entries(), typed arrays, arguments and strings (not that path), and a
// next or a [Symbol.iterator] replaced on the prototypes, then put back.
(function () { "use strict";
var log = [];
var calls = { n: 0 };
function sum(a) { var s = "", x; for (x of a) s = s + "," + x; return s; }
function first(a) { var x; for (x of a) return x; return "none"; }
function brk(a) { var s = "", x; for (x of a) { if (x === 3) break; s = s + x; } return s; }
function grow(a) { var s = "", x; for (x of a) { if (a.length < 6) a.push(x * 10); s = s + "," + x; } return s; }
function shrink(a) { var s = "", x; for (x of a) { a.pop(); s = s + "," + x; } return s; }
function del(a) { var s = "", x, i = 0; for (x of a) { if (i++ === 1) delete a[3]; s = s + "," + x; } return s; }
function len(a) { var s = "", x, i = 0; for (x of a) { if (i++ === 1) a.length = 2; s = s + "," + x; } return s; }
function longer(a) { var s = "", x, i = 0; for (x of a) { if (i++ === 1) a.length = 6; s = s + "," + x; } return s; }
function destr(a) { var v1, v2, v3, v4; [v1, v2, , v3, ...v4] = a; return [v1, v2, v3, v4.join("+")].join(","); }
function destr2(a) { var [p, q = "dq", ...r] = a; return p + "," + q + "," + r.length; }
function keys(a) { var s = "", x; for (x of a.keys()) s = s + "," + x; return s; }
function entries(a) { var s = "", x; for (x of a.entries()) s = s + "," + x.join(":"); return s; }
function args() { var s = "", x; for (x of arguments) s = s + "," + x; return s; }
function guard(f, a) {
  try { return String(f(a)); } catch (e) { return e.name + ":" + e.message; }
}
function each(tag) {
  var holes = [1, , 3];
  holes.length = 5;
  log.push(tag + " " + [guard(sum, [1, 2, 3, 4, 5]), guard(sum, []), guard(sum, holes), guard(first, [7, 8]),
    guard(brk, [1, 2, 3, 4]), guard(grow, [1, 2, 3]), guard(shrink, [1, 2, 3, 4]), guard(del, [1, 2, 3, 4, 5]),
    guard(len, [1, 2, 3, 4, 5]), guard(longer, [1, 2, 3]), guard(destr, [1, 2, 3, 4, 5, 6, 7]),
    guard(destr, [1]), guard(destr2, ["a"]), guard(destr2, ["a", "b", "c", "d"]),
    guard(keys, [5, 6, 7]), guard(entries, [5, 6]), guard(sum, new Int16Array([3, 4])),
    guard(sum, "xyz"), guard(sum, [{ toString: function () { calls.n = calls.n + 1; return "o"; } }]),
    String(args(4, 5, 6))].join(" | "));
}
each("plain");
Object.defineProperty(Array.prototype, 4, { get: function () { calls.n = calls.n + 1; return "proto4"; }, configurable: true });
each("proto-getter");
delete Array.prototype[4];
var AIP = Object.getPrototypeOf([][Symbol.iterator]());
var next0 = AIP.next;
AIP.next = function () { calls.n = calls.n + 1; var r = next0.call(this); if (!r.done) r.value = "n" + r.value; return r; };
each("next-replaced");
AIP.next = next0;
var it0 = Array.prototype[Symbol.iterator];
Array.prototype[Symbol.iterator] = function* () { calls.n = calls.n + 1; for (var i = 0; i < this.length; i++) yield "g" + this[i]; };
each("iterator-replaced");
Array.prototype[Symbol.iterator] = it0;
each("restored");
log.push("calls " + calls.n);
rvExpect(sum, first, brk, grow, shrink, del, len, longer, destr, destr2, keys, entries, args, each);
return log.join("\n");
})()
