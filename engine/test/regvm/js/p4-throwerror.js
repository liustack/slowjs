// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// throw_error in the register loop (THROWE): an assignment to a const throws
// a TypeError, caught a frame up and then left uncaught, with the stack of
// each. Each function runs three times, so that it runs in the register loop
// from its second call.
(function () { "use strict";
function throwRO(x) { const c = 1; c = 2; return c; }
function throwROArg(x) { const c = x; if (x > 1) c = x + 1; return c; }
function caller(f, x) { try { return f(x); } catch (e) { return e; } }
function stack(f, x) { try { return f(x); } catch (e) { return e.stack; } }
return function () {
  var r = [], i;
  for (i = 0; i < 3; i++) {
    r.push(String(caller(throwRO, 0)), caller(throwROArg, 1), String(caller(throwROArg, 2)),
           stack(throwRO, 0), stack(throwROArg, 5));
  }
  rvExpect(throwRO, throwROArg, caller, stack);
  r.push(throwROArg(3));
  return r.join("\n");
};
})()
