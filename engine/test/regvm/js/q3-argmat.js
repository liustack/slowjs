// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A strict function that reads arguments.length or arguments[i] and also
// lets the object go (to apply, to a variable, to a call, as this of a
// call) makes it at the read that lets it go (ARGMAT,
// engine/regvm-ops-args.h), or never when that read does not run: the
// object has the values of entry wherever it is made, the same object
// comes from every later read of the call, and a change made through it is
// seen by ARGLEN and ARGGET after.
// inject-hit: ARGMAT 1
(function () {
  "use strict";
  var out = [];
  function rec(label, f) {
    var r;
    try {
      r = f();
    } catch (e) {
      r = "throws " + e.name + " " + e.message;
    }
    out.push(label + ": " + r);
  }

  // clsx's shape: apply only when there are more than six
  function lu() {
    var r = "";
    for (var i = 0; i < arguments.length; i++)
      r += (r ? " " : "") + arguments[i];
    return r;
  }
  function W(a, b, c, d, e, f) {
    if (arguments.length > 6)
      return lu.apply(null, arguments);
    return [a, b, c, d, e, f].filter(Boolean).join(" ");
  }
  rec("cold", function () { return W("x") + " | " + W("x", "y", 0, "z"); });
  rec("taken", function () { return W(1, 2, 3, 4, 5, 6, 7, 8); });

  // made after the function changed its locals: the object has the
  // arguments of entry, whatever the locals hold
  function locals(a, b) {
    var n = arguments.length, x = a, y = b;
    x = "changed"; y = x + "!";
    var o = arguments;
    return [n, x, y, o[0], o[1], o.length, arguments[0], arguments.length].join(",");
  }
  rec("locals", function () { return locals("p", "q") + " | " + locals("p") + " | " + locals(1, 2, 3); });

  // inside a try, with a throw between a read and the escape
  function inTry(a) {
    var r = [arguments.length, arguments[0]];
    try {
      if (a === "throw")
        throw new Error("between");
      r.push("no throw");
    } catch (e) {
      r.push(e.message);
      r.push(Array.prototype.join.call(arguments, "+"));
    } finally {
      r.push(arguments[1], arguments.length);
    }
    return r.join(",");
  }
  rec("try", function () { return inTry("throw", "b") + " | " + inTry("ok", "b"); });

  // an escape on the nth pass of a loop: one object, the one of entry
  function loop(a, b, c) {
    var kept = null, same = true, r = [];
    for (var i = 0; i < arguments.length; i++) {
      r.push(arguments[i]);
      if (i === 1)
        kept = arguments;
      else if (kept !== null)
        same = same && kept === arguments;
    }
    return r.join("") + " " + (kept ? kept.length + ":" + kept[2] : "none") + " " + same;
  }
  rec("loop", function () { return loop("a", "b", "c", "d") + " | " + loop("a") + " | " + loop(); });

  // recursion: the frames that let the object go make it, the others not
  function rec2(n) {
    var r = n + ":" + arguments.length;
    if (n % 2)
      r += "@" + Object.keys(arguments).join(".");
    if (n > 0)
      r += " " + rec2(n - 1, "x", "y");
    return r + "/" + arguments[arguments.length - 1];
  }
  rec("recursion", function () { return rec2(4, "first"); });

  // a change through the object that left is seen by the reads after
  function change(o) { o[0] = "changed"; o.length = 7; o[5] = "five"; }
  function changed(a) {
    var r = [arguments.length, arguments[0]];
    change(arguments);
    r.push(arguments.length, arguments[0], arguments[5], a);
    return r.join(",");
  }
  rec("changed", function () { return changed("orig", 2); });

  // the same object at every escape, and the object as this of a call
  function same(a) {
    var x = arguments, y = arguments;
    return (x === y) + " " + arguments.length + " " + Object.prototype.toString.call(arguments) +
           " " + typeof arguments + " " + (arguments instanceof Object);
  }
  rec("same", function () { return same(1, 2); });
  function asThis(a) {
    var n = arguments.length;
    return n + " " + arguments.hasOwnProperty(0) + " " + arguments.hasOwnProperty(n);
  }
  rec("asThis", function () { return asThis("a") + " | " + asThis(); });

  // a spread of the arguments, and callee, which throws in strict code
  function spread(a) {
    if (arguments.length === 0)
      return "none";
    return Math.max(...arguments) + " " + [...arguments].length;
  }
  rec("spread", function () { return spread(3, 9, 4) + " | " + spread(); });
  function callee(a) {
    var n = arguments.length;
    return n + " " + arguments.callee;
  }
  rec("callee", function () { return callee(1); });
  return out.join("\n");
})();
