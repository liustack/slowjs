// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Template literals: each substitution is converted to a string right after
// it is evaluated and before the next one is, String.prototype.concat is not
// called, a symbol throws, and the result is the string a + chain gives. Also
// tagged templates, nested ones, strings of 8 and 16 bits and ropes, many
// substitutions, and a check that runs on values made new each time leave
// the heap as it was. The file is what Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? JSON.stringify(r.length > 80 ? r.slice(0, 30) + '...' + r.slice(-30) + '#' + r.length : r) : String(r)))
}
const wide = '日本'
run('plain', () => `abc`)
run('empty', () => ``)
run('one', () => `${1}`)
run('only strings', () => `a${'b'}c`)
run('start', () => `${'a'}bc`)
run('end', () => `ab${'c'}`)
run('adjacent', () => `${'a'}${'b'}${'c'}`)
run('numbers', () => `${1}|${2.5}|${-0}|${NaN}|${1e21}|${0.1 + 0.2}|${2 ** 31}`)
run('others', () => `${true}|${false}|${null}|${undefined}|${10n}|${[1, [2, 3]]}|${{}}|${() => 1}`)
run('symbol', () => `${Symbol('x')}`)
run('symbol later', () => `a${1}b${Symbol('x')}c`)
run('symbol description', () => `${Symbol('x').description}`)
run('wide', () => `a${wide}b${wide}`)
run('wide start', () => `${wide}a`)
run('emoji', () => `x${'\u{1f600}'}y`)
run('rope', () => { const r = 'x'.repeat(9000) + 'y'; return `<${r}>${r}` })
run('long', () => `${'l'.repeat(5000)}${'m'.repeat(5000)}`)
run('nested', () => `a${`b${`c${1}d`}e`}f`)
run('nested in call', () => `a${[1, 2].map(x => `<${x}>`).join('')}b`)
run('expression', () => `${1 + 2}${'a' + 'b'}${[3, 4].length}`)
run('newlines', () => `line1
line2 ${1}
line3`)
run('escapes', () => `\n\tA\x41\${1}\``)
run('dollar', () => `$${1}$$ ${2}$`)
run('raw', () => String.raw`a${1}\n${2}b`)
run('tag', () => { const t = (s, ...v) => JSON.stringify(s) + '|' + JSON.stringify(v) + '|' + JSON.stringify(s.raw); return t`a${1}b${'x'}c` })
run('tag no subst', () => { const t = (s, ...v) => s.length + ',' + v.length; return t`abc` })
run('tag object not converted', () => { const o = { toString() { return 'S' } }; const t = (s, ...v) => v[0] === o; return t`${o}` })
run('tag member', () => { const o = { t(s, ...v) { return this === o && v.join() } }; return o.t`${1}${2}` })
run('order', () => {
  const log = []
  const mk = (n, v) => ({ toString() { log.push('str:' + n); return v } })
  const val = (n, v) => { log.push('eval:' + n); return v }
  const r = `${val(1, mk(1, 'A'))}${val(2, mk(2, 'B'))}${val(3, 3)}`
  return r + ' ' + log.join()
})
run('toString before next substitution', () => {
  const c = { n: 0, toString() { return '' + this.n } }
  return `${c} ${c.n++}`
})
run('date', () => { const d = new Date(2024, 0, 15, 12); return `${d}|${d.setFullYear(2000)}`.indexOf('2024') })
run('toPrimitive hint', () => { const p = { [Symbol.toPrimitive](h) { return h } }; return `${p}` })
run('valueOf not used', () => { const v = { valueOf() { return 1 }, toString() { return 's' } }; return `${v}` })
run('only valueOf', () => { const v = { toString: undefined, valueOf() { return 7 } }; return `${v}` })
run('toString returns object', () => { const v = { toString() { return {} }, valueOf() { return {} } }; return `${v}` })
run('toString throws', () => { const v = { toString() { throw new RangeError('x') } }; return `a${v}b` })
run('throw order', () => {
  const log = []
  const bad = { toString() { log.push('bad'); throw new RangeError('x') } }
  try { return `${bad}${(log.push('next'), 'n')}` } catch (e) { return e.name + ' ' + log.join() }
})
run('concat patched', () => {
  const concat = String.prototype.concat
  String.prototype.concat = function () { return 'concat' }
  try { return `a${1}b${2}` } finally { String.prototype.concat = concat }
})
run('concat removed', () => {
  const concat = String.prototype.concat
  delete String.prototype.concat
  try { return `a${1}b${2}` } finally { String.prototype.concat = concat }
})
run('string proto getter', () => {
  Object.defineProperty(String.prototype, 'probe', { get() { throw new Error('read') }, configurable: true })
  try { return `a${1}b` } finally { delete String.prototype.probe }
})
run('toString patched on number', () => {
  const ts = Number.prototype.toString
  Number.prototype.toString = function () { return 'N' }
  try { return `${1}${2.5}` } finally { Number.prototype.toString = ts }
})
run('many', () => {
  const a = Array.from({ length: 40 }, (_, i) => i)
  return `${a[0]}${a[1]}${a[2]}${a[3]}${a[4]}${a[5]}${a[6]}${a[7]}${a[8]}${a[9]}-${a[10]}${a[11]}${a[12]}${a[13]}${a[14]}${a[15]}${a[16]}${a[17]}${a[18]}${a[19]}-${a[20]}${a[21]}${a[22]}${a[23]}${a[24]}${a[25]}${a[26]}${a[27]}${a[28]}${a[29]}-${a[30]}${a[31]}${a[32]}${a[33]}${a[34]}${a[35]}${a[36]}${a[37]}${a[38]}${a[39]}`
})
run('in loop', () => { let s = ''; for (let i = 0; i < 300; i++) s += `[${i}:${i % 3 ? 'a' : wide}]`; return s })
run('same as plus', () => { let bad = 0; for (const a of ['', 'x', wide, 1, 2.5, null, undefined, true, [1, 2]]) for (const b of ['', 'y', wide, 3, {}]) if (`<${a}|${b}>` !== '<' + String(a) + '|' + String(b) + '>') bad++; return bad })
run('this value', () => { const o = { s: 'S', f() { return `${this.s}-${typeof this}` } }; return o.f() })
run('closure vars', () => { let a = 1; const f = () => `${a}${++a}${a}`; return f() })
run('arguments', () => (function () { return `${arguments.length}${arguments[0]}` })('z', 'y'))
run('label', () => { const k = 'key'; const o = { [`${k}1`]: 1, [`${k}2`]: 2 }; return Object.keys(o).join() })
run('switch', () => { const x = 'b'; switch (`${x}`) { case `a`: return 1; case `b`: return 2 } })
if (typeof memorySize === 'function') {
  const mk = () => {
    let n = 0
    for (let i = 0; i < 300; i++) {
      const o = { toString() { return 'o' + i } }
      n += `a${i}b${o}c${'q'.repeat(i)}${wide}`.length
      try { `a${Symbol()}b${o}` } catch (e) { }
      try { `${o}${{ toString() { throw new Error('t') } }}` } catch (e) { }
    }
    return n
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
