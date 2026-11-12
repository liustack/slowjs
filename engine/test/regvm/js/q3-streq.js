// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The equality of two strings of one length that are not the same string,
// compared in goasm_eq_fast (engine/quickjs.c) when both are flat and shorter
// than a step, by the compares into a bool and the compare jumps, as
// registers and as constants: an 8-bit string against a 16-bit one of the
// same text (RegExp.escape writes a 16-bit string for a 16-bit argument,
// though what it writes for U+2028 is ASCII), two 16-bit strings, strings
// that differ at the first or the last character, a rope against a flat
// string, two atoms, an atom against a string that is not one, and pairs of
// a step less one and of a step, where the step pair is left to the slow
// side, which reads the interrupt flag as it compares.
(function () {
  "use strict";
  var out = [];
  function seq(a, b) { return a === b; }
  function sne(a, b) { return a !== b; }
  function eq(a, b) { return a == b; }
  function ne(a, b) { return a != b; }
  function jseq(a, b) { if (a === b) return "T"; return "F"; }
  function jne(a, b) { if (a != b) return "T"; return "F"; }
  function all(name, a, b) {
    out.push(name + " " + a.length + "," + b.length + ": " +
             [seq(a, b), sne(a, b), eq(a, b), ne(a, b), jseq(a, b), jne(a, b),
              seq(b, a), eq(b, a)].map(function (x) { return x === true ? 1 : x === false ? 0 : x; }).join(""));
  }
  function cat(a, b) { return a + b; }

  // an 8-bit string and a 16-bit one with the same text
  var w = RegExp.escape(" "), n = cat("\\u", "2028");
  all("8/16 same", w, n);
  all("8/16 same long", cat(RegExp.escape("_ "), "abcdef"), cat("_\\u2028", "abcdef"));
  // the same, against a constant (SEQK and the others)
  out.push("k " + [w === "\\u2028", w !== "\\u2028", w == "\\u2028", w != "\\u2028",
                   w === "\\u2029", w == "\\u2029"].join(","));
  // an 8-bit string and a 16-bit one that differ
  all("8/16 last", cat("abc", "é"), cat("abc", "Ā"));
  all("8/16 first", cat("é", "abc"), cat("Ā", "abc"));
  // two 16-bit strings
  all("16/16 same", cat("Ā", "abc"), cat("Āa", "bc"));
  all("16/16 last", cat("Ā", "abc"), cat("Ā", "abd"));
  // two 8-bit strings
  all("8/8 same", cat("ab", "cd"), cat("a", "bcd"));
  all("8/8 first", cat("xb", "cd"), cat("a", "bcd"));
  all("8/8 last", cat("ab", "cd"), cat("a", "bce"));
  all("empty", cat("", ""), "");

  // a rope (a join of more than 512 characters) against a flat string
  var half = "0123456789abcdef".repeat(40);
  var rope = cat(half, half), flat = half.padEnd(1280, half);
  all("rope/flat", rope, flat);
  all("rope/rope", rope, cat(half, half));
  all("rope/flat differ", rope, half.padEnd(1279, half).padEnd(1280, "!"));

  // two atoms: property names
  var ks = Object.keys({ abc: 1, abd: 2, xyz: 3 });
  all("atom/atom", ks[0], ks[1]);
  all("atom/atom same", ks[0], Object.keys({ abc: 0 })[0]);
  // an atom against a string that is not one
  all("atom/str", ks[0], cat("ab", "c"));
  all("atom/str differ", ks[2], cat("xy", "a"));

  // a step less one: compared at once. A step: the slow side, by steps
  var step = 65536;
  var x = "x";
  all("step-1 same", x.repeat(step - 1), x.repeat(step - 1));
  all("step-1 last", x.repeat(step - 1), x.repeat(step - 2).padEnd(step - 1, "y"));
  all("step same", x.repeat(step), x.repeat(step));
  all("step last", x.repeat(step), x.repeat(step - 1).padEnd(step, "y"));
  all("step 16 same", "\u0100".repeat(step), "\u0100".repeat(step));
  all("step 8/16 differ", x.repeat(step), "\u0100".repeat(step));

  // a loop of compares, as a switch over strings makes them
  var names = ["div", "span", "a", "button", "input", "svg", "path"];
  var count = 0;
  for (var i = 0; i < 700; i++) {
    var t = cat(names[i % 7].slice(0, 1), names[i % 7].slice(1));
    if (t === "button") count += 1;
    else if (t === "span") count += 10;
    else if (t == "svg") count += 100;
  }
  out.push("loop " + count);
  return out.join("\n");
})();
