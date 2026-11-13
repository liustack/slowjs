// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rv-expect-op: DSTART DNEXT
// The record of a destructuring keeps the next method it read from
// %ArrayIteratorPrototype% until it ends, also when a default deletes or
// replaces that next and a collection runs: a WeakRef to it sees it alive
// then, and the reads after the default go on with it, also through the
// Array Iterator a hole makes. Once the destructuring is over, nothing keeps
// a next the prototype no longer has.
"use strict";
(function () {
var AIP = Object.getPrototypeOf([][Symbol.iterator]());
var out = [];
function say(s) { out.push(s); }
function weakNext() { return new WeakRef(AIP.next); }
function alive(w) { return w.deref() !== undefined; }

// the default deletes the next, collects, looks, and puts it back
function deleted() {
  var w = weakNext();
  const [a = (delete AIP.next, clearKeptObjects(), gc(), say("deleted: alive in the default " + alive(w)),
              AIP.next = w.deref(), "a"), b] = [undefined, 2];
  return a + b;
}
say("deleted: " + deleted());

// the default replaces the next with a function that logs, collects, and
// a hole then reads through the Array Iterator with the next of the record
function replaced() {
  var w = weakNext(), a = [undefined, 2];
  a.length = 4;
  const [x = (AIP.next = function () { say("the new next ran"); return { done: true }; },
              clearKeptObjects(), gc(), say("replaced: alive in the default " + alive(w)), "x"),
         y, z = "z", q = "q"] = a;
  AIP.next = w.deref();
  return [x, y, z, q].join();
}
say("replaced: " + replaced());

// replaced for good: once the destructuring is over and the old next is
// collected, the WeakRef is empty, and a destructuring after it calls the
// new next
function gone() {
  var w = weakNext(), saved = [];
  const [x = (AIP.next = function () { saved.push("new"); return { done: true }; }, "x")] = [undefined];
  clearKeptObjects();
  gc();
  say("gone: alive after the destructuring " + alive(w));
  const [p = "p"] = [1];
  return x + p + saved.join();
}
say("gone: " + gone());

rvExpect(deleted, replaced, gone);
return out.join("\n");
})();
