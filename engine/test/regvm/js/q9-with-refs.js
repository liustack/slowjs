// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// References made inside with to an argument (make_arg_ref) and to a
// variable of the function around (make_var_ref_ref), which OLD runs
// (engine/regvm-ops-old.h), then read and written with get_ref_value and
// put_ref_value: an update, a compound assignment, a destructuring target,
// with the object of the with that has the name, that has not, and whose
// setter throws. This file is sloppy.
(function () {
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
  function argRef(o, x) {
    with (o) {
      x += 1;
      x++;
      [x] = [x * 2];
    }
    return x + "/" + o.x;
  }
  function outer(o) {
    var v = 1;
    function inner() {
      with (o) {
        v += 1;
        v++;
        ({ v } = { v: v * 3 });
      }
      return v;
    }
    return inner() + "," + inner() + "/" + v + "/" + o.v;
  }
  function setterThrows(i) {
    return { set x(v) { throw new RangeError("set x " + v + " " + i); }, get x() { return i; } };
  }
  for (var i = 0; i < 4; i++) {
    rec("arg none", function () { return argRef({}, i); });
    rec("arg own", function () { return argRef({ x: 10 }, i); });
    rec("arg setter throws", function () { return argRef(setterThrows(i), i); });
    rec("var none", function () { return outer({}); });
    rec("var own", function () { return outer({ v: 5 }); });
    rec("var setter throws", function () {
      return outer({ set v(n) { throw new RangeError("set v " + n); }, get v() { return 2; } });
    });
  }
  rvExpect(argRef, outer);
  return out.join("\n");
})()
