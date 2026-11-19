// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Each compare and branch of the register loop (engine/regvm-ops-cmp.h):
// <, <=, >, >=, ==, !=, === and !== between two registers and against a
// constant, jumping when false (an if) and when true (a negated compare,
// or the test at the end of a do while), and === against a closure
// variable (JSEQVF). Each compare is written four times, a site for each
// kind of operands: two ints, two numbers that are not both ints, two
// strings, and objects whose valueOf runs JS between the two places of the
// instruction, then the functions run once more on an object whose valueOf
// throws. The script stays under the places inject.sh injects all of, so
// that each place of each site is thrown at. The fast path of two ints (or
// of goasm_eq_fast), the one of two numbers and the slow side read the
// interrupt flag each in a place of its own, and the throw injection throws
// at each.
// inject-hit: JLTF 2
// inject-hit: JLTT 2
// inject-hit: JLTKF 2
// inject-hit: JLTKT 2
// inject-hit: JLTEF 2
// inject-hit: JLTET 2
// inject-hit: JLTEKF 2
// inject-hit: JLTEKT 2
// inject-hit: JGTF 2
// inject-hit: JGTT 2
// inject-hit: JGTKF 2
// inject-hit: JGTKT 2
// inject-hit: JGTEF 2
// inject-hit: JGTET 2
// inject-hit: JGTEKF 2
// inject-hit: JGTEKT 2
// inject-hit: JEQF 2
// inject-hit: JEQT 2
// inject-hit: JEQKF 2
// inject-hit: JEQKT 2
// inject-hit: JSEQF 2
// inject-hit: JSEQT 2
// inject-hit: JSEQKF 2
// inject-hit: JSEQKT 2
// inject-hit: JSEQVF 2
(function () {
  var out = [], log = [];
  function rec(label, f) {
    var r;
    try {
      r = f();
    } catch (e) {
      r = "throws " + e.name + " " + e.message;
    }
    out.push(label + ": " + r + (log.length ? " " + log.join(",") : ""));
    log = [];
  }
  function obj(v) {
    return { valueOf: function () { log.push(v); return v; } };
  }
  var bad = { valueOf: function () { throw new RangeError("valueOf"); } };
  // the operands of a kind: a, b and c of the number n, the second two the
  // same object for objects
  function kinds(n) {
    var o = [obj(n[0]), obj(n[1]), obj(n[2])];
    if (n[1] === n[2])
      o[2] = o[1];
    if (n[0] === n[1])
      o[1] = o[0];
    return [
      n,
      [n[0] + 0.5, n[1] + 0.5, n[2] + 0.5],
      [String(n[0]), String(n[1]), String(n[2])],
      o,
    ];
  }
  // if (a X b): four sites, one a kind
  function ifs(x, k) {
    var s = "var n = 0;";
    for (var i = 0; i < 4; i++)
      s += "if (a" + i + " " + x + " " + (k ? k : "b" + i) + ") n += " + (1 << i) + ";";
    return new Function("a0", "b0", "a1", "b1", "a2", "b2", "a3", "b3", s + "return n;");
  }
  // do v = c; while (v X b): the test, once, on c and b, which it does not
  // hold for
  function loops(x, k) {
    var s = "var v;";
    for (var i = 0; i < 4; i++)
      s += "do v = c" + i + "; while (v " + x + " " + (k ? k : "b" + i) + ");";
    return new Function("b0", "c0", "b1", "c1", "b2", "c2", "b3", "c3", s + "return v;");
  }
  function run(label, f, ks, loop) {
    var args = [], j = loop ? 1 : 0;
    for (var i = 0; i < 4; i++)
      args.push(ks[i][j], ks[i][j + 1]);
    rec(label, function () { return f.apply(null, args); });
    args[j] = bad;
    rec(label + " throws", function () { return f.apply(null, args); });
  }
  // the compares, the operands a, b and c of the do while (not c X b), and
  // whether one is written: only a do while jumps when <, <=, > or >= is
  // true, while an if of != or !== already jumps when == or === is
  var tests = [
    ["<", [2, 3, 4], true], ["<=", [2, 3, 4], true], [">", [4, 3, 2], true],
    [">=", [4, 3, 2], true], ["==", [3, 3, 4], false], ["!=", [2, 3, 3], false],
    ["===", [3, 3, 4], false], ["!==", [2, 3, 3], false],
  ];
  var fs = [];
  for (var t = 0; t < tests.length; t++) {
    var x = tests[t][0], ks = kinds(tests[t][1]), f;
    fs.push(f = ifs(x));
    run("if a " + x + " b", f, ks, false);
    fs.push(f = ifs(x, "3"));
    run("if a " + x + " 3", f, ks, false);
    if (tests[t][2]) {
      fs.push(f = loops(x));
      run("do while c " + x + " b", f, ks, true);
      fs.push(f = loops(x, "3"));
      run("do while c " + x + " 3", f, ks, true);
    }
  }
  // === against a closure variable
  function closure(v) {
    return function (a0, a1, a2, a3) {
      var n = 0;
      if (a0 === v) n += 1;
      if (a1 === v) n += 2;
      if (a2 === v) n += 4;
      if (a3 === v) n += 8;
      return n;
    };
  }
  var o = obj(3);
  rec("closure 3", function () { return closure(3)(3, 3.5, "3", o); });
  rec("closure object", function () { return closure(o)(3, 3.5, "3", o); });
  rec("closure string", function () { return closure("3")(3, 3.5, "3", o); });
  rvExpect.apply(null, fs);
  return out.join("\n");
})()
