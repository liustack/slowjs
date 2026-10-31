// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Locals that the compiler puts in one slot when no run of the code needs two
// of them at once: blocks one after another, a local read before any write
// (in a branch, in each turn of a loop, in a try), a write nothing reads, a
// try with a catch that reads a local written in the try, a finally that
// reads the local a return took, closures of a loop and nested ones, the
// value of a switch on the stack where a case returns, a generator that
// yields and an async function that awaits in the middle of a range, one
// left suspended, the arguments object in sloppy and strict mode, a direct
// eval and a with block, and a switch of many cases with a var of its own
// each. Each runs on values of every kind. The file is what Node 24 prints,
// and compare.sh also runs it with the slots apart.
const out = []
const show = v => typeof v === 'symbol' ? v.toString() : typeof v === 'function' ? 'function ' + v.name :
  Array.isArray(v) ? '[' + v.map(show).join() + ']' : typeof v === 'object' && v ? JSON.stringify(v) : typeof v === 'bigint' ? v + 'n' : String(v)
const kinds = [undefined, null, true, 0, -1.5, NaN, 12n, 'str', 'made ' + 'x'.repeat(3), Symbol('s'), { a: 1 }, [1, 2], function named() {}]
function blocks(v) { const r = []; { const a = [v, 1]; r.push(a[0]) } { const b = [v, 2]; r.push(b[1]) } { const c = [v]; r.push(c.length) } return r }
function deadWrite(v) { let a = v; let b = [v]; a = 3; return [b[0], a] }
function readFirst(c, v) { var a; if (c) { var b = v; return b } return a }
function loopRead(n, v) { var r = []; for (var i = 0; i < n; i++) { var a; r.push(a === undefined ? 'u' : show(a)); var b = [v, i]; a = b[1] } return r.join() }
function loopRead2(n) { let s = ''; for (let i = 0; i < n; i++) { var t; s += t + ','; t = i } { const u = s.length; s += u } return s }
function tryRead(o) { var r = 'none'; try { const t = o.x.y; r = t } catch (e) { const m = e.constructor.name; return 'caught ' + show(r) + ' ' + m } return show(r) }
function tryRead2(o) { let r = 0; try { r = 1; const t = o.x.y; r = 2; return t } catch (e) { return 'caught at ' + r } }
function fin(v) { let x = [v]; try { return x } finally { x.push('f') } }
function fin2(o) { let x = o.x; try { return x } finally { o.seen = x; x = 9 } }
function fin3(v) { for (let i = 0; i < 2; i++) { const a = [v, i]; try { if (i) return a } finally { const b = a.length; a.push(b) } } }
function loopClosures(n) { const fs = []; for (let i = 0; i < n; i++) { const k = i * 2; fs.push(() => i + k) } { const z = fs.length; fs.push(z) } return fs.slice(0, n).map(h => h()).join() + ',' + fs[n] }
function nested(o) { const a = o.x; { const b = o.y; o.g = () => [a, b] } { const c = o.z; o.h = [c] } { const d = o.w; o.h.push(d) } return show(o.g()) + show(o.h) }
function lowered(o) { { const a = o.x; o.r = [a] } { const b = o.y; o.r.push(b) } const c = o.z; return () => [c, o.r] }
function sw(v) { const a = 'A' + String(v); switch (v) { case 1: { const b = a + 'b'; return b } case 2: { const c = a + 'c'; return c } default: { const d = [a]; return d.length } } }
function* gen(o) { const a = o.x; yield 1; const b = o.y; yield [a, b]; const c = o.z; yield c; { const d = [a, c]; yield d } }
async function af(o) { const a = await o.x; const b = [a, 1]; await null; const c = [b, 2]; return c }
async function hang(o) { const a = [o]; await new Promise(() => {}); return a }
function sloppy(o) { { const a = o.x; o.r = [a] } { const b = o.y; o.r.push(b) } arguments[0] = 'changed'; return arguments.length + show(o.r) }
function strict(o) { 'use strict'; { const a = o.x; o.r = [a] } { const b = o.y; o.r.push(b) } return arguments.length + show(o.r) + show(arguments[0] === o) }
function ev(o) { { const a = o.x; o.r = [a] } { const b = o.y; o.r.push(b) } return eval('show(o.r)') }
function ev2(o) { { var a = o.x } { var b = o.y } return eval('show([a, b])') }
function wi(o) { { const a = o.x; o.r = [a] } { const b = o.y; o.r.push(b) } with (o) { return show(r) } }
function rec(n, v) { { const a = [n, v]; if (n > 0) { const r = rec(n - 1, v); return [a[0], r] } } { const b = 'end'; return b } }
let many = 'function many(o, k) { const out = []; switch (k) {'
for (let i = 0; i < 40; i++) many += ' case ' + i + ': var v' + i + ' = o.p' + i + '; out.push(show(v' + i + '), k); break;'
many += ' } return out.join() }'
const manyFn = new Function('show', many + '; return many')(show)
for (const k of kinds) {
  out.push(show(k) + ': ' + [show(blocks(k)), show(deadWrite(k)), show(readFirst(0, k)), show(readFirst(1, k)), loopRead(3, k),
    tryRead({ x: { y: k } }), tryRead({}), show(tryRead2({ x: { y: k } })), tryRead2({}), show(fin(k)), show(fin3(k)),
    nested({ x: k, y: 1, z: k, w: 2 }), show(lowered({ x: 1, y: k, z: k })()), show([...gen({ x: k, y: 2, z: k })]),
    sloppy({ x: k, y: 1 }), strict({ x: 1, y: k }), ev({ x: k, y: k }), ev2({ x: k, y: 0 }), wi({ x: 0, y: k }),
    show(rec(3, k)), manyFn({ p7: k, p39: 'last' }, 7) + manyFn({ p39: k }, 39)].join(' | '))
}
out.push('loop: ' + loopRead2(4) + ' ' + loopClosures(3))
const o2 = { x: 'v' }
out.push('finally: ' + fin2(o2) + ' ' + o2.seen)
out.push('switch: ' + sw(1) + ' ' + sw(2) + ' ' + sw(3))
af({ x: 'a' }).then(v => out.push('async: ' + show(v)))
hang({ held: 1 })
// a local in its temporal dead zone next to locals that share a slot
try { (o => { { const a = o.x; o.r = a } { const b = o.y; o.r += b } if (o.r) return z; let z = 1; return z })({ x: 1, y: 2 }) } catch (e) { out.push('tdz: ' + e.constructor.name) }
// the value of a local is freed once: runs of all of them leave the heap as it was
if (typeof memorySize === 'function') {
  const all = () => {
    for (const k of kinds) {
      blocks(k); deadWrite(k); readFirst(1, k); loopRead(3, k); tryRead({ x: { y: k } }); tryRead({}); fin(k); fin3(k)
      nested({ x: k, y: 1, z: k, w: 2 }); lowered({ x: 1, y: k, z: k })(); [...gen({ x: k, y: 2, z: k })]
      strict({ x: 1, y: k }); rec(3, k); manyFn({ p7: k }, 7)
    }
    for (let i = 0; i < 20; i++) hang({ i })
  }
  all(); all(); gc()
  all(); all(); gc(); gc()
  const m1 = memorySize(), k1 = memoryBlocks()
  all(); all(); all(); gc(); gc()
  out.push('memory after: ' + heapChange(m1, k1))
} else {
  out.push('memory after: 0')
}
;(() => out.join('\n'))
