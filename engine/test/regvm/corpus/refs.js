// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: reference counts in converted code made visible: objects made,
// passed, returned and dropped in converted functions must be freed when the
// last reference goes (a WeakRef no longer reaches them after a collection)
// and must stay while one is held. A count too high keeps the object alive, a
// count too low frees it early (a sanitizer build reports it). A WeakRef
// keeps its target to the end of the job (KeptObjects of the specification):
// settle() ends it as the end of a job does (clearKeptObjects of the test
// host) and collects, and each case checks its own count, so that a count
// too high fails the script with one loop as with two.
(function () { "use strict";
if (typeof clearKeptObjects !== "function" || typeof gc !== "function")
  throw new Error("refs.js needs gc and clearKeptObjects of the test host (jsrun, rvdiff)");
function settle() { clearKeptObjects(); gc(); }
var out = [];
function p(x) { out.push(String(x)); }
function mk(n) { return { n: n, inner: { s: "v" + n } }; }
function pass(o) { return o; }
function drop(o) { o.n; return 1; }
function twice(o) { return pass(pass(o)); }
function field(o) { return o.inner; }
function keep(holder, o) { holder.o = o; return holder; }
function chain(n) { var a = mk(n), b = mk(n + 1); a.next = b; return a.next.inner.s; }
function extra(a) { return a.n; }
function missing(a, b, c) { if (c === undefined) return a; return b; }
function method(o) { return o.get(); }
function run(name, f) {
  var refs = [];
  var held = [];
  for (var i = 0; i < 4; i++) {
    var r = f(i, held);
    refs.push(new WeakRef(r));
  }
  settle();
  var alive = 0;
  for (var j = 0; j < refs.length; j++) if (refs[j].deref() !== undefined) alive++;
  p(name + " alive " + alive + " held " + held.length);
  if (alive !== held.length)
    throw new Error("refs: " + name + " leaves " + alive + " alive, " + held.length + " held");
}
run("pass", function (i) { var o = mk(i); pass(o); return o; });
run("drop", function (i) { var o = mk(i); drop(o); drop(o); return o; });
run("twice", function (i) { var o = mk(i); twice(o); return o; });
run("field", function (i) { var o = mk(i); field(o); return field(o); });
run("keep", function (i, held) { var o = mk(i); held.push(keep({}, o)); return o; });
run("chain", function (i) { var o = mk(i); chain(i); return o; });
run("extra", function (i) { var o = mk(i), q = mk(i); extra(o, q, q, o); return q; });
run("missing", function (i) { var o = mk(i); missing(o); missing(o, o); return o; });
run("method", function (i) { var o = mk(i); o.get = function () { return this.inner; }; method(o); return o.inner; });
run("throw", function (i) { var o = mk(i); try { extra(null, o); } catch (e) { } return o; });
run("callback", function (i) { var o = mk(i); [o, o].map(pass); return o; });
// a reference a converted frame lets go of is gone then, before the frame
// ends: each object is asked after, inside the frame that held it
function late(i) {
  var o = mk(i), w = new WeakRef(o);
  pass(o);
  o = null;
  settle();
  return w.deref() === undefined ? "gone" : "held";
}
var lates = [];
for (var k = 0; k < 4; k++) lates.push(late(k));
p("late " + lates.join(","));
// a string made in converted code and kept by a closure that is not
function cat(a, b) { return a + b; }
var s = cat(cat("a", "b"), cat("c", String(1)));
p(s + " " + s.length);
gc();
return out.join("\n");
})()
