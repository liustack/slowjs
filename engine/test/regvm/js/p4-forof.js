// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Exceptions and for-of together (engine/regvm-ops-except.h with the window
// ops): a throw in the body passes the catch offset 0 of the loop, which
// closes the iterator, and stops at the catch around the loop; a catch and a
// finally inside the body; a return and a break out of a try in the body;
// and a throw of the iterator's return method while it is closed.
(function () { "use strict";
var log = [];
function note(x) { log.push(x); return x; }
function thr(x) { throw x; }
function iter(n, bad) {
  var i = 0;
  return { next: function () { i++; return { done: i > n, value: i }; },
           return: function () { note("ret" + i); if (bad) throw "badret"; return {}; },
           [Symbol.iterator]: function () { return this; } };
}
function outside(it) {
  var s = 0;
  try { for (var x of it) { s = s + x; if (x === 3) thr(s); } } catch (e) { return e + 100; }
  return s;
}
function inside(it) {
  var s = 0;
  for (var x of it) { try { if (x === 2) thr(x); s = s + x; } catch (e) { s = s + 10; } finally { s = s + 1; } }
  return s;
}
function retIn(it) {
  for (var x of it) { try { if (x === 2) return x; } finally { note("fin" + x); } }
  return 0;
}
function breakIn(it) {
  var s = 0;
  for (var x of it) { try { if (x === 3) break; s = s + x; } catch (e) { s = 0; } }
  return s;
}
function badClose(it) {
  try { for (var x of it) { if (x === 2) thr("body"); } } catch (e) { return e; }
  return 0;
}
function up(it) { for (var x of it) { if (x === 2) thr(x + 40); } return 0; }
function upCatch(it) { try { return up(it); } catch (e) { return e; } }
return function () {
  var r = [], i;
  for (i = 0; i < 3; i++) {
    r.push(outside(iter(5, false)), inside(iter(4, false)), retIn(iter(4, false)),
           breakIn(iter(5, false)), badClose(iter(4, true)), upCatch(iter(4, false)));
  }
  rvExpect(outside, inside, retIn, breakIn, badClose, up, upCatch);
  return r.join(" ") + " | " + log.join(" ");
};
})()
