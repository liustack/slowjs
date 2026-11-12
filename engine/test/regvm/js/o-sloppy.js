// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: delete_var, which only sloppy code has: deleting a global made by
// an assignment (configurable), one made by var (not configurable), one
// that does not exist, a global getter, and a delete with temps owned below
// it. Sloppy functions stay in the old loop until the register loop takes
// them.
var keep = 1;
var out = [];
function id(x) { return x; }
function del() { return delete made; }
function del2() { return delete keep; }
function del3() { return delete nothing_here; }
function del4() { return id(1) + id(2) + (delete made2) + id(3); }
function del5() { return delete getter; }
Object.defineProperty(globalThis, "getter", { get: function () { return 7; }, configurable: true });
for (var i = 0; i < 3; i++) {
  made = i;
  made2 = i;
  out.push(del(), typeof made, del2(), typeof keep, del3(), del4(), typeof made2, del5(), typeof getter);
  Object.defineProperty(globalThis, "getter", { get: function () { return 7; }, configurable: true });
}
rvExpect(del, del2, del3, del4, del5);
out.join(",");
