// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// String.prototype.concat with every kind of this value and argument (strings
// of 8 and 16 bits, empty ones, ropes, numbers, objects, symbols, BigInts),
// the order in which toString is called, an exception in the middle, and
// many arguments. Template literals are not involved. Then a check that runs
// on values made new each time leave the heap as it was. The file is what
// Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? JSON.stringify(r.length > 80 ? r.slice(0, 30) + '...' + r.slice(-30) + '#' + r.length : r) : String(r)))
}
const concat = (t, ...a) => String.prototype.concat.apply(t, a)
function bigRope() { return 'x'.repeat(9000) + 'yz' }
const wide = '日本'
const objs = () => {
  const log = []
  return [log, { toString() { log.push('a'); return 'A' } }, { toString() { log.push('b'); return 'B' }, valueOf() { log.push('vB'); return 'V' } }, { [Symbol.toPrimitive](h) { log.push('p:' + h); return 'P' } }]
}
run('none', () => concat('abc'))
run('empty this', () => concat(''))
run('empty args', () => concat('', ''))
run('one', () => concat('a', 'b'))
run('many', () => concat('a', 'b', 'c', 'd', 'e', 'f'))
run('numbers', () => concat('n', 1, 2.5, -0, NaN, Infinity, 1e21, 0.1 + 0.2))
run('bool null undefined', () => concat('x', true, false, null, undefined))
run('bigint', () => concat('x', 1n, -2n, 2n ** 70n))
run('this number', () => concat(12, 3))
run('this bool', () => concat(true, 'x'))
run('this object', () => concat({ toString() { return 'T' } }, 'x'))
run('this array', () => concat([1, 2], [3], []))
run('this null', () => concat(null, 'x'))
run('this undefined', () => concat(undefined))
run('this symbol', () => concat(Symbol(), 'x'))
run('arg symbol', () => concat('x', Symbol()))
run('arg symbol later', () => concat('x', 'y', 1, Symbol('s'), 'z'))
run('wide this', () => concat(wide, 'a'))
run('wide arg', () => concat('a', wide))
run('wide both', () => concat(wide, wide, 'x', wide))
run('wide empty', () => concat('', wide, ''))
run('emoji', () => concat('a', '\u{1f600}', 'b'))
run('rope this', () => concat(bigRope(), 'tail'))
run('rope arg', () => concat('head', bigRope()))
run('rope both', () => concat(bigRope(), bigRope()))
run('rope wide', () => concat(bigRope(), wide))
run('long', () => concat('l'.repeat(10000), 'm'.repeat(10000), 'n'))
run('result is a string', () => typeof concat(new String('a'), new String('b')))
run('string objects', () => concat(new String('a'), new String('b'), new Number(1)))
run('order', () => { const [log, a, b, p] = objs(); const r = concat(a, b, p, 5); return r + ' ' + log.join() })
run('order this first', () => { const [log, a, b] = objs(); const r = concat('x', a, b); return r + ' ' + log.join() })
run('exception middle', () => {
  const [log, a, b] = objs()
  const bad = { toString() { log.push('bad'); throw new RangeError('x') } }
  try { concat('x', a, bad, b) } catch (e) { return e.name + ' ' + log.join() }
})
run('exception this', () => {
  const log = []
  const bad = { toString() { log.push('bad'); throw new RangeError('x') } }
  try { concat(bad, { toString() { log.push('later'); return 'l' } }) } catch (e) { return e.name + ' ' + log.join() }
})
run('arg changes this', () => {
  const o = { n: 1, toString() { return 'o' + this.n } }
  return concat(o, { toString() { o.n = 2; return 'x' } }, o)
})
run('many args', () => concat('', ...Array.from({ length: 300 }, (_, i) => i % 7 ? String(i) : i)))
run('count', () => concat('a', 'b').length + concat('a').length)
run('length property', () => String.prototype.concat.length + ' ' + String.prototype.concat.name)
run('call', () => 'abc'.concat('d', 'e'))
run('call number', () => (5).toString().concat(6))
run('new copy', () => { const a = 'same'; const b = a.concat(); return (a === b) + ' ' + (a.concat('') === a) })
run('keeps this', () => { const a = 'q'.repeat(100); const b = a.concat('r'); return a.length + ' ' + b.length + ' ' + a.slice(-1) + b.slice(-1) })
run('index after', () => { const s = 'ab'.concat('cd', 'ef'); return s[0] + s[3] + s[5] + s.indexOf('de') + s.charCodeAt(2) })
run('wide index', () => { const s = wide.concat('a', wide); return s.length + ' ' + s.charCodeAt(0) + ' ' + s.charCodeAt(3) + ' ' + s.charCodeAt(4) })
// equal to the results of +
{
  const parts = ['', 'a', wide, '1', '\u{1f600}', 'long'.repeat(200)]
  let bad = 0, n = 0
  for (const a of parts) for (const b of parts) for (const c of parts) { n++; if (concat(a, b, c) !== a + b + c) bad++ }
  out.push('same as plus: ' + n + ' ' + bad)
}
if (typeof memorySize === 'function') {
  const mk = () => {
    const x = []
    for (let i = 0; i < 200; i++) {
      const o = { toString() { return 'o' + i } }
      x.push(concat('p' + i, o, i, bigRope(), wide, 'q'.repeat(i)))
      try { concat('p', Symbol(), o) } catch (e) { }
      try { concat(o, { toString() { throw new Error('t') } }) } catch (e) { }
    }
    return x.length
  }
  mk(); mk(); gc()
  mk(); mk(); mk(); gc()
  const m1 = memorySize()
  mk(); mk(); mk(); gc()
  out.push('memory after: ' + (memorySize() - m1))
} else {
  out.push('memory after: 0')
}
out.join('\n')
