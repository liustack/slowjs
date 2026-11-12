// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Calls of more arguments than the operand stack holds (JS_CALL_ARGS_ON_STACK
// of engine/quickjs.c, 32): past it the compiler puts the arguments in an
// array and makes the call with OP_apply, as for a spread. Each kind of call
// at 31 to 33 arguments and far past them: a function, a method, an optional
// call, new, super, a direct eval, a tagged template, a spread past the
// limit, a call of 128 arguments whose stack was too deep for the register
// loop, a throw in an argument past the limit, a setter on Array.prototype
// that the array of the arguments must not run, and new and a call of what
// is not a constructor or a function. The functions are made from source,
// each called twice from a driver: the first call converts it, the second
// runs it in the register loop.
(function () {
  "use strict";
  function list(n, f) {
    var r = [], i;
    for (i = 0; i < n; i++)
      r.push(f(i));
    return r.join(", ");
  }
  function nums(n) { return list(n, function (i) { return String(i); }); }
  var fs = [], names = [];
  function add(name, f) { names.push(name); fs.push(f); }
  [31, 32, 33, 40, 128, 300].forEach(function (n) {
    add("call " + n, Function("g", "'use strict'; return g(" + nums(n) + ");"));
    add("method " + n, Function("o", "'use strict'; return o.m(" + nums(n) + ");"));
    add("optional " + n, Function("o", "'use strict'; return [o?.m(" + nums(n) + "), o.none?.(" + nums(n) + ")];"));
    add("new " + n, Function("C", "'use strict'; return new C(" + nums(n) + ").r;"));
    add("eval " + n, Function("x", "'use strict'; var v = 5; return eval('v + x', " + nums(n) + ");"));
    add("tag " + n, Function("t", "'use strict'; var x = 'x'; return t`a" +
      list(n, function (i) { return "${" + (i % 3 ? i : "x") + "}" + i; }) + "`;"));
    add("spread " + n, Function("g", "a", "'use strict'; return g(" + nums(n) + ", ...a, 'end');"));
  });
  add("super 40", Function("B", "'use strict'; class D extends B { constructor() { super(" + nums(40) + "); } } return new D().r;"));
  add("concat 128", Function("'use strict'; var s = 'x'; return s.concat(" +
    list(128, function (i) { return "'" + (i % 10) + "'"; }) + ");"));
  add("throw 40", Function("g", "bad", "'use strict'; return g(" + nums(36) + ", bad(), 37, 38, 39);"));
  add("not a function 40", Function("f", "'use strict'; return f(" + nums(40) + ");"));
  add("not a constructor 40", Function("C", "'use strict'; return new C(" + nums(40) + ");"));
  add("not a constructor 3", Function("C", "'use strict'; return new C(1, 2, 3);"));
  return function () {
    var out = "", k;
    function g() { return arguments.length + ":" + arguments[arguments.length - 1]; }
    var o = { m: function () { return (this === o) + ":" + arguments.length + ":" + arguments[arguments.length - 1]; } };
    function C() { this.r = (new.target === C) + ":" + arguments.length + ":" + arguments[arguments.length - 1]; }
    function t(strs) { return strs.length + ":" + strs.raw[strs.length - 1] + ":" + Object.isFrozen(strs) + ":" + (arguments.length - 1) + ":" + arguments[arguments.length - 1]; }
    function B() { this.r = arguments.length + ":" + arguments[arguments.length - 1]; }
    function bad() { throw new RangeError("bad"); }
    var args = { "call": [g], "method": [o], "optional": [o], "new": [C], "eval": [7], "tag": [t],
      "spread": [g, [41, 42]], "super": [B], "concat": [], "throw": [g, bad],
      "not a function": [{}], "not a constructor": [{}] };
    function at(i) {
      var a = args[names[i].replace(/ [0-9]+$/, "")];
      try { out += names[i] + " " + fs[i].apply(null, a) + "\n"; } catch (e) { out += names[i] + " " + e.name + ": " + e.message + "\n"; }
    }
    // a setter on an index that the array of the arguments defines
    Object.defineProperty(Array.prototype, "35", { set: function () { throw new Error("setter"); }, configurable: true });
    try {
      for (k = 0; k < 2; k++)
        fs.forEach(function (f, i) { at(i); });
      at(names.indexOf("not a constructor 40"));
      out += String(new C(...[1, 2]).r) + "\n";
      try { new ({})(...[1]); } catch (e) { out += e.name + ": " + e.message + "\n"; }
    } finally {
      delete Array.prototype[35];
    }
    rvExpect.apply(null, fs.filter(function (f, i) { return !/^eval/.test(names[i]); }));
    return out;
  };
})()
