// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// arguments.length and arguments[key] in sloppy functions with a simple
// parameter list, whose arguments object is mapped: an index below the
// count and the parameters reads the parameter as it is now, a write to a
// parameter shows in the object and a write to the object in the
// parameter, an index past the parameters reads the argument passed. Each
// function is made with Function, so that it stays sloppy when the whole
// file is run as strict code. Every kind of key, fewer, as many and more
// arguments than parameters, a getter on Object.prototype for a missing
// index that keeps the object and writes to it, delete, defineProperty,
// length redefined, callee, the object kept or iterated, and the same
// function on the stack twice. The file is what Node 24 prints.
const out = []
function show(v) {
  switch (typeof v) {
  case 'symbol': return 'symbol'
  case 'function': return 'function'
  case 'object': return v === null ? 'null' : 'object'
  case 'number': return Object.is(v, -0) ? '-0' : String(v)
  default: return typeof v + ':' + String(v)
  }
}
function rec(label, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name + ' ' + e.message }
  out.push(label + ': ' + r)
}
const S = (params, body) => Function('show', 'sym', 'keyObj', 'return function (' + params + ') {' + body + '}')(show, sym, keyObj)
const sym = Symbol('k')
const keyObj = { toString() { return '1' } }

const keys = S('a, b', `
  var r = [];
  r.push(arguments.length);
  r.push(show(arguments[0]), show(arguments[1]), show(arguments[2]));
  r.push(show(arguments[arguments.length]), show(arguments[arguments.length + 1]));
  r.push(show(arguments[-1]), show(arguments[1.0]), show(arguments[-0]), show(arguments[0.5]));
  r.push(show(arguments["0"]), show(arguments["1"]), show(arguments["length"]));
  r.push(show(arguments[sym]), show(arguments[keyObj]), show(arguments[Symbol.iterator]));
  r.push(show(arguments[NaN]), show(arguments[2147483648]), show(arguments[4294967295]));
  r.push(arguments.length);
  return r.join(" ");`)
rec('keys 0', () => keys())
rec('keys 1', () => keys('x'))
rec('keys 2', () => keys('x', 2))
rec('keys 3', () => keys('x', 2, null))
rec('keys 5', () => keys(1, 2, 3, 4, 5))
const noParams = S('', `
  var r = [arguments.length];
  for (var i = 0; i < arguments.length + 1; i++) r.push(show(arguments[i]));
  return r.join(" ");`)
rec('no parameters 0', () => noParams())
rec('no parameters 3', () => noParams('p', 'q', 'r'))

// a write to a parameter shows in the object, below the count of arguments
const writes = S('a, b, c', `
  var r = [show(arguments[0]), show(arguments[1]), show(arguments[2])];
  a = "A"; b = "B"; c = "C";
  r.push(show(arguments[0]), show(arguments[1]), show(arguments[2]), show(arguments[3]), arguments.length);
  r.push(a, b, c);
  return r.join(" ");`)
rec('writes 0', () => writes())
rec('writes 1', () => writes(1))
rec('writes 2', () => writes(1, 2))
rec('writes 4', () => writes(1, 2, 3, 4))
const incr = S('n', `
  var r = [];
  for (var i = 0; i < 3; i++) { n++; r.push(arguments[0]); }
  return r.join(" ") + " " + arguments.length;`)
rec('increments', () => incr(10))
rec('increments, no argument', () => incr())
// a write to the object shows in the parameter
const objWrites = S('a, b', `
  var r = [show(arguments[0])];
  arguments[0] = "w0";
  arguments[1] = "w1";
  arguments[3] = "w3";
  r.push(show(a), show(b), show(arguments[0]), show(arguments[1]), show(arguments[3]), arguments.length);
  a = "back";
  r.push(show(arguments[0]));
  return r.join(" ");`)
rec('object writes 1', () => objWrites('x'))
rec('object writes 2', () => objWrites('x', 'y'))
rec('object writes 4', () => objWrites('x', 'y', 'z', 'q'))
// a parameter captured by a closure that writes it
const closure = S('a', `
  var set = function (v) { a = v; };
  var r = [show(arguments[0])];
  set("closure");
  r.push(show(arguments[0]), show(a), arguments.length);
  return r.join(" ");`)
rec('closure writes 1', () => closure('orig'))
rec('closure writes 0', () => closure())

// a getter on Object.prototype for a missing index: it gets the object,
// keeps it and writes to it, and the parameter follows
const kept = []
const missing = S('a', `
  var r = show(arguments[3]);
  r += " " + show(arguments[0]) + " " + show(a) + " " + show(arguments[1]) + " " + arguments.length;
  return r + " " + show(arguments[5]);`)
rec('getter for a missing index', () => {
  Object.defineProperty(Object.prototype, 3, {
    configurable: true,
    get() {
      kept.push(this)
      this[0] = 'written'
      this[5] = 'five'
      Object.defineProperty(this, 'length', { value: 42 })
      return 'got ' + typeof this + ' ' + Object.prototype.toString.call(this)
    }
  })
  try { return missing('a', 'b') } finally { delete Object.prototype[3] }
})
rec('kept', () => {
  const a = kept[0]
  return kept.length + ' ' + a.length + ' ' + a[0] + ' ' + a[1] + ' ' + a[5] + ' ' + Object.prototype.toString.call(a)
})

// delete, defineProperty and length, then reads
const deletes = S('a, b', `
  var r = [];
  delete arguments[0];
  r.push(show(arguments[0]), show(a), arguments.length);
  a = "after";
  r.push(show(arguments[0]), show(arguments[1]));
  return r.join(" ");`)
rec('delete', () => deletes('x', 'y'))
const defines = S('a', `
  var r = [];
  Object.defineProperty(arguments, 0, { value: "defined" });
  r.push(show(arguments[0]), show(a));
  a = "param";
  r.push(show(arguments[0]), show(a));
  Object.defineProperty(arguments, 0, { writable: false });
  a = "param2";
  r.push(show(arguments[0]), show(a));
  return r.join(" ");`)
rec('defineProperty', () => defines('x'))
const lengthSet = S('a', `
  arguments.length = 1;
  var r = [arguments.length, show(arguments[0]), show(arguments[1])];
  arguments.length = 5;
  r.push(arguments.length, show(arguments[4]));
  return r.join(" ");`)
rec('length set', () => lengthSet('x', 'y'))
const callee = S('a', `
  var c = arguments.callee;
  return (typeof c) + " " + arguments.length + " " + show(arguments[0]);`)
rec('callee', () => callee('x'))
const iterated = S('a', `
  var r = [];
  a = "A";
  for (var v of arguments) r.push(show(v));
  return r.join(" ") + " " + arguments.length;`)
rec('iterated', () => iterated('x', 'y'))
const sliced = S('a', `
  a = "A";
  return [].slice.call(arguments).join(",") + " " + arguments.length;`)
rec('sliced', () => sliced('x', 'y', 'z'))

// the same function on the stack twice: an outer call that built its
// object and an inner one that did not
const twice = S('depth, tag', `
  var r = [];
  if (depth === 0) r.push(show(arguments[7]));
  if (depth < 2) r.push(twice(depth + 1, tag + depth));
  r.push(depth + ":" + arguments.length + ":" + show(arguments[1]) + ":" + show(arguments[2]));
  if (depth === 2) r.push(show(arguments["2"]));
  return r.join(" ");`)
globalThis.twice = twice
rec('twice', () => twice(0, 't'))
rec('twice extra', () => twice(0, 't', 'e'))

// sums, the createElement pattern, many arguments
const sum = S('a, b', `
  var t = 0;
  for (var i = 0; i < arguments.length; i++) t += arguments[i];
  return t + "/" + arguments.length + "/" + show(arguments[arguments.length - 1]);`)
rec('sum none', () => sum())
rec('sum fewer', () => sum(1))
rec('sum more', () => sum(1, 2, 3, 4))
const long = []
for (let i = 0; i < 3000; i++) long.push(i)
rec('sum apply', () => sum.apply(null, long))
const children = S('type, props', `
  var n = arguments.length - 2, kids;
  if (n === 1) kids = arguments[2];
  else if (n > 1) { kids = Array(n); for (var c = 0; c < n; c++) kids[c] = arguments[c + 2]; }
  return type + " " + JSON.stringify(kids);`)
rec('children 0', () => children('div', null))
rec('children 1', () => children('div', null, 'a'))
rec('children 3', () => children('div', null, 'a', 1, [2]))
// a throw from a getter on the slow read, and reads after it
const throwing = S('a', `
  var r = [];
  try { r.push(show(arguments[4])); } catch (e) { r.push("caught " + e.message); }
  r.push(arguments.length, show(arguments[0]), show(arguments[4]));
  return r.join(" ");`)
rec('throwing getter', () => {
  let n = 0
  Object.defineProperty(Object.prototype, 4, {
    configurable: true,
    get() { if (n++ === 0) throw new Error('first ' + typeof this); return 'second' }
  })
  try { return throwing('only') } finally { delete Object.prototype[4] }
})
// a parameter name given twice: the name is the last parameter, the first
// index keeps what was passed
const twiceNamed = S('a, a', `
  a = 5;
  return [show(arguments[0]), show(arguments[1]), arguments.length].join(" ");`)
rec('twice 2', () => twiceNamed(1, 2))
rec('twice 1', () => twiceNamed(1))
rec('twice 3', () => twiceNamed(1, 2, 3))
out.join('\n')
