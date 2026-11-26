// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// Function.prototype.toString of a function with no source (a built-in, a
// bound function) runs no code of the script: it takes the own name property
// when that is a string held as a value, and no name otherwise, so a getter
// put there is not called and cannot throw. The file is what the engine
// prints. Node 24 prints the same but for the lines marked "Node:", where it
// gives the name the built-in was made with, which the engine does not keep,
// and for a bound function, which it never names.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + JSON.stringify(r))
}
// V8 writes [native code] on the line of the name, and the engine on lines
// of their own
const str = f => Function.prototype.toString.call(f).replace(/\s+/g, ' ')
run('plain', () => str(parseInt))
run('getter of a getter', () => str(Object.getOwnPropertyDescriptor(Map.prototype, 'size').get))
run('symbol name', () => str(RegExp.prototype[Symbol.split]))
run('constructor', () => str(Array))
// Node: function abs() { [native code] } 0
run('getter', () => {
  let n = 0
  Object.defineProperty(Math.abs, 'name', { get() { n++; return 'changed' }, configurable: true })
  return str(Math.abs) + ' ' + n
})
// Node: function max() { [native code] }
run('getter that throws', () => {
  Object.defineProperty(Math.max, 'name', { get() { throw new RangeError('x') }, configurable: true })
  return str(Math.max)
})
// Node: function min() { [native code] }
run('value', () => { Object.defineProperty(Math.min, 'name', { value: 'other' }); return str(Math.min) })
// Node: function sqrt() { [native code] }
run('deleted', () => { delete Math.sqrt.name; return str(Math.sqrt) })
// Node: function cbrt() { [native code] }
run('not a string', () => { Object.defineProperty(Math.cbrt, 'name', { value: 42 }); return str(Math.cbrt) })
// Node: function () { [native code] }
run('bound', () => str(function f() { }.bind(null)))
run('bound with a getter that throws', () => {
  const b = function f() { }.bind(null)
  Object.defineProperty(b, 'name', { get() { throw new RangeError('x') } })
  return str(b)
})
run('proxy', () => str(new Proxy(Math.floor, { get() { throw new RangeError('x') } })))
out.join('\n')
