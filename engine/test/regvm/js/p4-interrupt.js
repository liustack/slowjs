// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// An uncatchable exception (an interrupt, stopAfterPolls of jsrun) thrown in
// a loop of the register loop passes every catch and finally of its frame
// and of the frames below it, with temps owned in each, and leaves through
// the entry. Were one of them to catch it, the function would return and the
// output would be the log, not the exception.
(function () { "use strict";
var log = [];
function note(x) { log.push(x); return x; }
function spin(lim) {
  var n = 0;
  try {
    try { while (lim) { n++; } } catch (e) { return note(-1); } finally { note(-2); }
  } catch (e2) { return note(-3); }
  return n;
}
function outer(o, lim) {
  try { return o.a + spin(lim) + o.b; } catch (e) { return note(-4); } finally { note(-5); }
}
function under(o, lim) {
  try { return o.b + outer(o, lim); } finally { note(-6); }
}
var o = { a: 1, b: 2 };
return function () {
  note(under(o, 0));
  note(under(o, 0));
  rvExpect(spin, outer, under);
  stopAfterPolls(100);
  note(under(o, 1));
  return log.join(" ");
};
})()
