// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Calls that return what they call, from functions with no local variable:
// the call becomes a tail call, which leaves the function and its arguments
// on the stack, so the stack is full when the frame frees its values. Each
// size runs: an even and an odd number of values, arguments the frame copies
// (fewer given than declared), plain calls and method calls, values that are
// objects, a call that throws, and deep chains of such calls. The objects
// passed are freed with the frames: the heap is as it was after them.
const out = []
const log = s => out.push(s)
const id = x => x
const sum = (...a) => a.reduce((s, v) => s + (typeof v === 'object' ? v.n : v), 0)
const err = f => { try { return String(f()) } catch (e) { return e.constructor.name + ': ' + e.message } }

// two values left: the function and one argument
function call1(g) { return g(1) }
function callArg(g, x) { return g(x) }
// four: the function and three arguments
function call3(g, a, b) { return g(a, b, 1) }
// three: the function and two arguments
function call2(g, a) { return g(a, 2) }
// declared two, given one: the frame copies the two arguments, and two
// values are on the stack, four in all
function fewer(g, x) { return g(1) }
// the method calls: this, the function, then the arguments
const o = { n: 10, m(...a) { return this.n + sum(...a) }, z() { return this.n } }
function method0(o) { return o.z() }
function method1(o) { return o.m(1) }
function method2(o, x) { return o.m(x, 2) }
function method2Fewer(o, x) { return o.m(1, 2) }

log('call1: ' + call1(id) + ',' + call1(sum))
log('callArg: ' + callArg(id, 7) + ',' + callArg(sum, { n: 5 }) + ',' + callArg(x => x.n, { n: 6 }))
log('call3: ' + call3(sum, 1, { n: 2 }) + ',' + call3(sum, 'a', 'b'))
log('call2: ' + call2(sum, { n: 3 }) + ',' + call2(sum, 3))
log('fewer: ' + fewer(id) + ',' + fewer(sum))
log('method0: ' + method0(o) + ',' + method0({ z() { return 'own' } }))
log('method1: ' + method1(o))
log('method2: ' + method2(o, { n: 4 }) + ',' + method2(o, 4))
log('method2 fewer: ' + method2Fewer(o))
log('throws: ' + err(() => call1(() => { throw new Error('in callee') })) + ' | ' +
  err(() => method0({ z() { throw new TypeError('in method') } })) + ' | ' +
  err(() => call1(5)) + ' | ' + err(() => method0({})))

// chains of tail calls, each from a frame with a full stack
function down(g, n) { if (n === 0) return g(n); return down(g, n - 1, 0) }
function step(k) { return k.next(k, 0) }
let depth = 0
const k = { next(k) { return ++depth < 2000 ? step(k) : depth } }
log('chains: ' + down(id, 1000) + ',' + step(k))

// many calls with objects as the values left on the stack. After a warm-up,
// the heap is as it was (two collections before each reading, as in
// strict_equal.js)
const loop = () => {
  let total = 0
  for (let i = 0; i < 20000; i++) {
    const x = { n: i & 7 }
    total += callArg(y => y.n, x) + method2(o, x) + call3(sum, x, x) + method0(o)
  }
  return total
}
loop(); gc(); gc()
const m1 = memorySize(), k1 = memoryBlocks()
const total = loop()
gc(); gc()
// read before the line of the log is made, which takes blocks of its own
const change = heapChange(m1, k1)
log('loop: ' + total + ' heap ' + change)
out.join('\n')
