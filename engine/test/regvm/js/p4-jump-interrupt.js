// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// An interrupt read at a conditional jump is named where the old loop names
// it: it reads the flag once it has jumped or not, so the stack has the line
// before the target of a jump taken, and the line of the branch otherwise.
// Each case runs as a job: arm(1) (stopAfterPolls of jsrun) sets the flag at
// its next reading, which is the jump after it, and the interrupt ends the
// job with its stack. A compare that throws by itself is named at the
// compare.
(function () { "use strict";
function nop() {}
function ifTruthy(arm, x) {
  arm(1);
  if (x) {
    return 10;
  }
  return 20;
}
function ifNot(arm, x) {
  arm(1);
  if (!x) {
    return 10;
  }
  return 20;
}
function ifObject(arm, o) {
  arm(1);
  if (o.a) {
    return 10;
  }
  return 20;
}
function ifLess(arm, x, y) {
  arm(1);
  if (x < y) {
    return 10;
  }
  return 20;
}
function ifLessK(arm, x) {
  arm(1);
  if (x <= 3) {
    return 10;
  }
  return 20;
}
function ifStrict(arm, x) {
  arm(1);
  if (x === "a") {
    return 10;
  }
  return 20;
}
function ifNotEq(arm, x, y) {
  arm(1);
  if (x != y) {
    return 10;
  }
  return 20;
}
function ifNull(arm, x) {
  arm(1);
  if (x == null) {
    return 10;
  }
  return 20;
}
function ifUndefined(arm, x) {
  arm(1);
  if (x === undefined) {
    return 10;
  }
  return 20;
}
function ifType(arm, x) {
  arm(1);
  if (typeof x === "function") {
    return 10;
  }
  return 20;
}
function andAlso(arm, o) {
  arm(1);
  var r = o.a &&
    o.b;
  return r;
}
function orElse(arm, o) {
  arm(1);
  var r = o.a ||
    o.b;
  return r;
}
function cases(arm, x) {
  arm(1);
  switch (x) {
  case "one":
    return 1;
  case "two":
    return 2;
  case "three":
    return 3;
  default:
    return 0;
  }
}
function loopHead(arm, n) {
  var s = 0;
  arm(1);
  while (n > 0) {
    s = s + n;
    n = n - 1;
  }
  return s;
}
function loopTail(arm, n) {
  var s = 0;
  do {
    s = s + n;
    n = n - 1;
    arm(n == 1 ? 1 : 0);
  } while (n > 0);
  return s;
}
// the compare throws by itself, with a temp owned under it
function cmpThrows(o, bad) {
  try {
    if (o.a + (bad < 3)) {
      return 1;
    }
    return 2;
  } catch (e) {
    return String(e.stack).split("\n")[1];
  }
}
var bad = { valueOf: function () { throw new Error("no value"); } };
var o1 = { a: 1, b: 2 }, o0 = { a: 0, b: 0 };
var runs = [
  [ifTruthy, 1], [ifTruthy, 0], [ifTruthy, o1], [ifTruthy, ""],
  [ifNot, 1], [ifNot, 0],
  [ifObject, o1], [ifObject, o0],
  [ifLess, 1, 2], [ifLess, 2, 1], [ifLess, "a", "b"], [ifLess, 2.5, 1.5],
  [ifLessK, 1], [ifLessK, 7], [ifLessK, "9"],
  [ifStrict, "a"], [ifStrict, "b"], [ifStrict, 1],
  [ifNotEq, 1, 2], [ifNotEq, 1, 1], [ifNotEq, "1", 1],
  [ifNull, null], [ifNull, 0],
  [ifUndefined, undefined], [ifUndefined, 0],
  [ifType, nop], [ifType, 0],
  [andAlso, o1], [andAlso, o0],
  [orElse, o1], [orElse, o0],
  [cases, "one"], [cases, "two"], [cases, "three"], [cases, "four"], [cases, 1],
  [loopHead, 3], [loopHead, 0],
  [loopTail, 3],
];
var out = [], i, k;
// each form runs twice with nothing stopped, and then in the register loop
for (k = 0; k < 2; k++) {
  for (i = 0; i < runs.length; i++)
    out.push(runs[i][0](nop, runs[i][1], runs[i][2]));
  out.push(cmpThrows(o1, 1), cmpThrows(o1, bad));
}
rvExpect(ifTruthy, ifNot, ifObject, ifLess, ifLessK, ifStrict, ifNotEq, ifNull, ifUndefined,
         ifType, andAlso, orElse, cases, loopHead, loopTail, cmpThrows);
function later(r) {
  Promise.resolve().then(function () {
    print("not stopped: " + r[0].name + " " + r[0](stopAfterPolls, r[1], r[2]));
  });
}
for (i = 0; i < runs.length; i++)
  later(runs[i]);
return out.join(" ");
})()
