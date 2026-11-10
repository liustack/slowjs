// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The cases of a switch on strings, which the compiler joins into one opcode
// each (dup_case_atom) and the interpreter runs as a chain, one case after the
// other in one dispatch: cases with a body each (if_false8), cases that share a
// body (if_true8), bodies too long for a jump of one byte (if_false and
// if_true), bodies that fall through, a default among the cases, cases that are
// not string literals in the middle of a chain, and every kind of value. Then
// switches in loops, generators, async functions, try and finally, and the
// errors their bodies throw, with their line and column. The file is what the
// engine printed before the chain, and with the fused opcodes turned off
// (compare.sh runs it both ways).
const out = []
const show = v => typeof v === 'symbol' ? v.toString() : typeof v === 'bigint' ? v + 'n' :
  Array.isArray(v) ? '[' + v.map(show) + ']' : v !== null && typeof v === 'object' ? 'object' :
  Object.is(v, -0) ? '-0' : String(v)
const where = e => String(e.stack).split('\n').slice(0, 3).map(l => l.trim().replace(/\(.*[\\/]/, '(').replace(/^at [^(]*$/, 'at <native>')).join(' / ')
function t(name, f) {
  let r
  try { r = show(f()) } catch (e) { r = (e && e.constructor && e.constructor.name) + ': ' + (e && e.message) + ' @ ' + where(e) }
  out.push(name + ': ' + r)
}

// a body each: each case jumps to the next with if_false8
function each(v) {
  switch (v) {
    case 'alpha': return 1
    case 'beta': return 2
    case 'gamma': return 3
    case 'delta': return 4
    case 'epsilon': return 5
    case '': return 6
    case 'üñí': return 7
    case '__proto__': return 8
    case 'constructor': return 9
    case '1': return 10
    case '-0': return 11
    case 'length': return 12
  }
  return 0
}
// bodies shared by several cases: if_true8 to the body, and the last case of
// each group jumps on with if_false8
function shared(v) {
  switch (v) {
    case 'a': case 'b': case 'c': return 'abc'
    case 'd': case 'e': return 'de'
    case 'f': return 'f'
    case 'g': case 'h': case 'i': case 'j': case 'k': return 'g-k'
  }
  return 'none'
}
// bodies longer than a jump of one byte: if_false and if_true
function long(v) {
  let n = 0
  switch (v) {
    case 'one': case 'uno':
      n += 1; n *= 3; n -= 2; n += 1; n *= 3; n -= 2; n += 1; n *= 3; n -= 2; n += 1; n *= 3; n -= 2
      n += 1; n *= 3; n -= 2; n += 1; n *= 3; n -= 2; n += 1; n *= 3; n -= 2; n += 1; n *= 3; n -= 2
      n += 1; n *= 3; n -= 2; n += 1; n *= 3; n -= 2; n += 1; n *= 3; n -= 2; n += 1; n *= 3; n -= 2
      return 'one ' + n
    case 'two':
      n += 2; n *= 5; n -= 3; n += 2; n *= 5; n -= 3; n += 2; n *= 5; n -= 3; n += 2; n *= 5; n -= 3
      n += 2; n *= 5; n -= 3; n += 2; n *= 5; n -= 3; n += 2; n *= 5; n -= 3; n += 2; n *= 5; n -= 3
      n += 2; n *= 5; n -= 3; n += 2; n *= 5; n -= 3; n += 2; n *= 5; n -= 3; n += 2; n *= 5; n -= 3
      return 'two ' + n
    case 'three': case 'tres': case 'drei':
      n += 7; n *= 2; n -= 1; n += 7; n *= 2; n -= 1; n += 7; n *= 2; n -= 1; n += 7; n *= 2; n -= 1
      n += 7; n *= 2; n -= 1; n += 7; n *= 2; n -= 1; n += 7; n *= 2; n -= 1; n += 7; n *= 2; n -= 1
      n += 7; n *= 2; n -= 1; n += 7; n *= 2; n -= 1; n += 7; n *= 2; n -= 1; n += 7; n *= 2; n -= 1
      return 'three ' + n
    case 'four':
      return 'four'
  }
  return 'none ' + n
}
// many cases that share a body: the first ones jump over more than a jump of
// one byte reaches (if_true)
function wide(v) {
  switch (v) {
    case 'a': case 'b': case 'c': case 'd': case 'e': case 'f': case 'g': case 'h': case 'i': case 'j':
    case 'k': case 'l': case 'one': case 'two': case 'three': case 'four': case 'p': case 'q': case 'r':
      return 'group'
    case 'x': return 'x'
  }
  return 'none'
}
// bodies that fall through to the next, which jumps over the case test
function fall(v) {
  let s = ''
  switch (v) {
    case 'a': s += 'a'
    case 'b': s += 'b'
    case 'c': s += 'c'; break
    case 'd': s += 'd'
    default: s += '*'
    case 'e': s += 'e'
  }
  return s
}
// a default among the cases: the last case jumps to it
function middle(v) {
  switch (v) {
    case 'x': return 'x'
    default: return 'default'
    case 'y': return 'y'
    case 'z': case 'w': return 'zw'
  }
}
// cases that are not string literals in the middle of a chain
const K = 'kay'
let ticks = 0
const getter = { get k() { ticks++; return 'got' } }
function mixed(v) {
  switch (v) {
    case 'p': return 'p'
    case K: return 'K'
    case 'q': return 'q'
    case 1: return 'one'
    case `r`: return 'r'
    case getter.k: return 'getter'
    case 's': case 2.5: case 't': return 'st'
    case null: return 'null'
    case 'u': return 'u'
    case undefined: return 'undefined'
  }
  return 'none'
}
// cases of a number switch around string cases
function numbers(v) {
  switch (v) {
    case 0: return 'zero'
    case 'zero': return 'the string'
    case -0: return 'minus zero'
    case 1n: return 'big'
  }
  return 'none'
}

const rope = 'x'.repeat(3000)
const values = [
  'alpha', 'beta', 'gamma', 'delta', 'epsilon', '', 'üñí', '__proto__', 'constructor', '1', '-0', 'length',
  'zeta', 'alph', 'alphaa', 'ALPHA', 'al' + 'pha', ['ep', 'silon'].join(''), String.fromCharCode(98, 101, 116, 97),
  'ALPHA'.toLowerCase(), 'üñ' + 'í', rope + 'alpha', (rope + 'beta').slice(3000), new String('alpha'),
  { toString() { return 'alpha' } }, 1, '1'.repeat(1), 0, -0, 1.5, null, undefined, true, 10n, Symbol('alpha'),
  Symbol.for('beta'), [], ['alpha'], new Proxy({}, { get(t, k) { ticks++; return () => 'alpha' } }),
  'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'a'.repeat(1) + '',
  'one', 'uno', 'two', 'three', 'tres', 'drei', 'four', 'five', 'tw' + 'o', 'x', 'y', 'z', 'w', 'v',
  'p', 'kay', 'q', 'r', 'got', 's', 't', 2.5, 'u', 'zero', 1n, 2,
]
for (const [name, f] of Object.entries({ each, shared, long, wide, fall, middle, mixed, numbers })) {
  ticks = 0
  const r = values.map(v => { try { return show(f(v)) } catch (e) { return 'threw ' + e.message } })
  out.push(name + ': ' + r.join(' ') + ' (getter read ' + ticks + ' times)')
}

// a chain in a loop, with continue and break, and nested switches
t('loop', () => {
  let s = ''
  for (const v of ['a', 'b', 'stop', 'c', 'a']) {
    switch (v) {
      case 'a': s += 'A'; continue
      case 'b': s += 'B'; break
      case 'stop': s += '|'; break
      default: s += '?'
    }
    s += '.'
  }
  return s
})
t('labeled break', () => {
  let s = ''
  outer: for (const v of ['a', 'b', 'c']) {
    switch (v) {
      case 'a': s += 'a'; break
      case 'b': s += 'b'; break outer
      case 'c': s += 'c'
    }
  }
  return s
})
t('nested', () => {
  const f = (a, b) => {
    switch (a) {
      case 'x':
        switch (b) { case 'p': return 'xp'; case 'q': case 'r': return 'xqr' }
        return 'x?'
      case 'y': case 'z':
        switch (b) { case 'p': return 'yzp' }
        return 'yz?'
    }
    return '??'
  }
  return [f('x', 'p'), f('x', 'r'), f('x', 's'), f('z', 'p'), f('y', 'q'), f('w', 'p')].join(',')
})
t('many evaluations', () => {
  const keys = ['alpha', 'beta', 'gamma', 'delta', 'epsilon', 'zeta', 'eta', '']
  let n = 0
  for (let i = 0; i < 2000; i++) n += each(keys[i & 7]) + shared('abcdefghijklm'[i % 13])
  return n
})

// generators and async functions keep their frame across a yield or an await
function* gen(list) {
  for (const v of list) {
    switch (v) {
      case 'a': yield 'A'; break
      case 'b': case 'c': yield 'BC'; yield 'again'; break
      default: yield '?' + v
    }
  }
}
t('generator', () => [...gen(['a', 'b', 'x', 'c'])].join(','))
t('generator switch on a sent value', () => {
  function* g() {
    for (;;) {
      const v = yield
      switch (v) { case 'stop': return 'stopped'; case 'go': case 'run': continue; default: throw new Error('bad ' + v) }
    }
  }
  const it = g()
  it.next()
  it.next('go')
  it.next('run')
  return show(it.next('stop').value)
})
async function asw(v) {
  switch (await v) {
    case 'a': return 'A ' + (await 1)
    case 'b': case 'c': return 'BC'
  }
  return 'none'
}
asw('a').then(r => out.push('async a: ' + r))
asw(Promise.resolve('c')).then(r => out.push('async c: ' + r))
asw('z').then(r => out.push('async z: ' + r))

// errors thrown in a body, with their line and column
function thrower(v) {
  switch (v) {
    case 'a': return 1
    case 'b': case 'c': return null.x
    case 'd': throw new TypeError('d')
    case 'e': return undefined.y
  }
  return v.z
}
for (const v of ['b', 'c', 'd', 'e', undefined]) t('error ' + show(v), () => thrower(v))
t('try and finally', () => {
  const log = []
  for (const v of ['a', 'b', 'c']) {
    try {
      switch (v) {
        case 'a': log.push('a'); break
        case 'b': case 'z': throw new Error('b')
        case 'c': return log.join('') + ' returned'
      }
    } catch (e) {
      log.push('caught ' + e.message)
    } finally {
      switch (v) { case 'c': log.push('fin c'); break; default: log.push('fin') }
    }
  }
  return log.join(',')
})
t('let in a case', () => {
  switch ('b') {
    case 'a': let x = 1
    case 'b': try { return x } catch (e) { return e.constructor.name }
  }
})
t('the value is evaluated once', () => {
  let n = 0
  const v = () => { n++; return 'c' }
  switch (v()) { case 'a': case 'b': case 'c': case 'd': }
  return n
})
t('cases are evaluated in order', () => {
  const log = []
  const c = s => { log.push(s); return s }
  switch ('b') { case c('a'): case 'x': case c('b'): case c('c'): }
  return log.join('')
})

;(() => out.join("\n"))
