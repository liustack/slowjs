// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff and inject.sh: an instruction with two places to throw that runs
// JS between them, the JS catching every error it sees. A with reads x
// through a getter, and a compare and branch converts its operand with a
// valueOf. The getter and the valueOf run in the register loop, in a run
// of it of their own, and a throw asked for at the second place of the
// instruction (the reading of the interrupt flag after the jump) must
// throw there, not inside the getter or the valueOf, where the catch would
// hide it. The lines below are the places inject.sh checks a run threw at.
// inject-hit: WITH 1
// inject-hit: WITH 2
// inject-hit: JLTF 1
// inject-hit: JLTF 2
(function () {
var out = [];
function getX() { try { return {}; } catch (e) { return "caught in getter"; } }
function read(o) { with (o) { return x; } }
var o = Object.defineProperty({}, "x", { get: getX });
var v = {
  valueOf: function () {
    try { return [1][0]; } catch (e) { return "caught in valueOf"; }
  }
};
function less(a, b) { if (a < b) return "less"; return "not less"; }
function run() {
  try { out.push(typeof read(o)); } catch (e) { out.push(e.name + ": " + e.message); }
  try { out.push(less(v, 2)); } catch (e) { out.push(e.name + ": " + e.message); }
}
run();
run();
run();
console.log(out.join("\n"));
rvExpect(read, getX, less, v.valueOf, run);
})();
