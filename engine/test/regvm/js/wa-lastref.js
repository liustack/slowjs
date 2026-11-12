// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// A call with extra arguments whose frame holds the last reference to its
// function (engine/regvm-run.h, rv_free_extra): the function comes from an
// indirect eval, so no constant table keeps its bytecode, and box.pop()
// hands the call its only reference. The return frees the frame's slots,
// the function with its register code among them, and only then the extra
// arguments: on each way out of a frame, a return of a temp, a local, a
// constant and undefined, the return of a function with captured locals, an
// exception caught one frame and two frames up, a function that reads its
// arguments, rest, and a call from C (whose caller keeps a reference). The
// ASan build finds a read of the freed code.
(function () { "use strict";
var log = [];
var srcs = {
    temp: 'return a + 1;',
    local: 'var x = a; return x;',
    konst: 'return 7;',
    undef: 'a;',
    closure: 'var x = a; function g() { return x; } return g() + 1;',
    thrower: 'throw new Error("t" + a);',
    args: 'return arguments.length + a;',
    rest: 'return arguments[1];',
};
// each source at a line of its own past the end of this file, so that the
// throw-injection test (inject.sh), which knows a function by its line and
// column, tells them apart
var geval = eval;
function at(k, fn) { return geval("\n".repeat(100 + 2 * k) + '"use strict"; (' + fn + ")"); }
function make(name) { return at(Object.keys(srcs).indexOf(name), "function (a) { " + srcs[name] + " }"); }
function restFn() { return at(30, "function (a, ...r) { return r.length + a; }"); }
// one function of each source, called three times in a try each, so that it
// converts even when inject.sh stops one call, which rvExpect checks below
var probes = [];
for (var name in srcs) probes.push(make(name));
probes.push(restFn());
probes.forEach(function (f) { try { f(1); } catch (e) {} try { f(1, 2); } catch (e) {} try { f(1, 2, 3); } catch (e) {} });

// the function converts at its first call, the second runs in the loop with
// the only reference in the call
function lastBox(box) { return box.pop()(1, {}, [2], "x"); }
function last(f) { var box = [f]; f = null; return lastBox(box); }
function once(name) { var f = make(name); try { f(0); } catch (e) {} return f; }
function plain(name) { return last(once(name)); }
function caught(name) { try { return last(once(name)); } catch (e) { return "c:" + e.message; } }
function mid(box) { return lastBox(box); }
function caught2(name) { try { var box = [once(name)]; return mid(box); } catch (e) { return "c2:" + e.message; } }
function rest() { var box = [restFn()]; box[0](0); return lastBox(box); }
// a C caller keeps its own reference for the whole call: the entry with
// extra arguments
function fromC(name) { var f = once(name), box = [f]; f = null; return Reflect.apply(box.pop(), undefined, [1, {}, 2]); }

function run() {
    var out = [];
    for (var name in srcs)
        if (name !== "thrower") out.push(name + "=" + plain(name));
    out.push(caught("thrower"), caught2("thrower"), "rest=" + rest());
    out.push("c:" + fromC("args"), "c:" + fromC("temp"));
    return out.join(",");
}
log.push(run());
log.push(run());
log.push(run());
rvExpect.apply(null, probes);
rvExpect(lastBox, last, once, plain, caught, mid, caught2, rest, fromC, run);
return log.join("\n");
})()
