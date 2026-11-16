// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// o[k] where k is a string made at run time, which keeps no atom yet: a name
// no object has (the register loop then reads undefined without making an
// atom), and a name some object has, found in the atom table. On plain
// objects and their prototypes, objects without a prototype, arrays,
// arguments, typed arrays (which answer "-0", "Infinity" and the like
// themselves, whatever the prototype holds), String objects, a Proxy on the
// object and on its chain, getters, the global object, wide strings, long
// keys, and a name that some object gets between two reads by the same
// string. Then the same reads made again and again must leave the heap as it
// was. The file is what Node 24 prints.
const out = []
function show(name, f) {
  let r
  try { r = f() } catch (e) { r = 'threw ' + e.constructor.name + ': ' + e.message }
  out.push(name + ': ' + (typeof r === 'string' ? r : JSON.stringify(r)))
}
// a string of these characters, made at run time
function fresh(s) { return s.split('').join('') }
function get(o, k) { return o[k] }
function getAll(o, ks) { const r = []; for (const k of ks) r.push(String(o[k])); return r.join(',') }

const plain = { alpha: 1, beta: 2 }
const child = Object.create(plain)
child.gamma = 3
show('absent', () => [get(plain, fresh('zz_nowhere_1')), get(child, fresh('zz_nowhere_2'))].map(String).join())
show('own and proto', () => [get(plain, fresh('alpha')), get(child, fresh('beta')), get(child, fresh('gamma'))].join())
show('repeated absent', () => { const k = fresh('zz_again'); return [get(plain, k), get(plain, k), get(child, k)].map(String).join() })
show('null proto', () => { const o = Object.create(null); o.x1 = 'x'; return [get(o, fresh('x1')), get(o, fresh('zz_np'))].map(String).join() })
show('added between reads', () => {
  const o = {}, k = fresh('zz_later_key')
  const a = get(o, k)
  o[fresh('zz_later_key')] = 'now'
  const b = get(o, k)
  delete o.zz_later_key
  const c = get(o, k)
  return [a, b, c].map(String).join()
})
show('proto added between reads', () => {
  const base = {}, o = Object.create(base), k = fresh('zz_proto_later')
  const a = get(o, k)
  base.zz_proto_later = 'base'
  return [a, get(o, k), get(o, fresh('zz_proto_later'))].map(String).join()
})
show('getter on proto', () => {
  const log = []
  const base = { get gg_key() { log.push(this === o); return 'g' } }
  const o = Object.create(base)
  return get(o, fresh('gg_key')) + ' ' + log.join()
})
show('Object.prototype', () => {
  Object.prototype.zz_on_object_proto = 'op'
  const r = [get({}, fresh('zz_on_object_proto')), get([], fresh('zz_on_object_proto')), get({}, fresh('zz_not_on_object_proto'))].map(String).join()
  delete Object.prototype.zz_on_object_proto
  return r + ' ' + String(get({}, fresh('zz_on_object_proto')))
})
show('array', () => {
  const a = [10, 20, 30]
  return [get(a, fresh('length')), get(a, fresh('-1')), get(a, fresh('zz_arr')), get(a, fresh('map')) === Array.prototype.map, get(a, fresh('1'))].map(String).join()
})
show('array with own key', () => { const a = [1]; a.extra_key = 'e'; return [get(a, fresh('extra_key')), get(a, fresh('zz_arr2'))].map(String).join() })
show('arguments', () => {
  const args = (function () { return arguments })(1, 2)
  return [get(args, fresh('length')), get(args, fresh('zz_args')), get(args, fresh('0'))].map(String).join()
})
show('typed array', () => {
  const t = new Uint8Array(2)
  const ks = ['-0', 'Infinity', '-Infinity', 'NaN', '-1', 'zz_ta', 'length', 'byteLength']
  return getAll(t, ks.map(fresh))
})
show('typed array, prototype names', () => {
  for (const k of ['-0', 'NaN', 'zz_tap']) Object.prototype[k] = 'op'
  const t = new Float64Array(1)
  const r = getAll(t, ['-0', 'NaN', 'zz_tap', '-Infinity'].map(fresh))
  for (const k of ['-0', 'NaN', 'zz_tap']) delete Object.prototype[k]
  return r
})
show('typed array on the chain', () => {
  const o = Object.create(new Int16Array(3))
  return getAll(o, ['-0', 'Infinity', 'zz_tc', 'BYTES_PER_ELEMENT'].map(fresh))
})
show('String object', () => { const s = new String('abc'); return getAll(s, ['length', 'zz_str', '1', '-0'].map(fresh)) })
show('primitive string', () => getAll('abc', ['length', 'zz_prim', '2'].map(fresh)))
show('proxy', () => {
  const log = []
  const p = new Proxy({ here_key: 'h' }, { get(t, k, r) { log.push(k); return k in t ? t[k] : 'trap:' + k } })
  return getAll(p, ['here_key', 'zz_proxy_absent'].map(fresh)) + ' ' + log.join()
})
show('proxy on the chain', () => {
  const log = []
  const o = Object.create(new Proxy({}, { get(t, k, r) { log.push(k); return 'p:' + k } }))
  o.own_key = 'own'
  return getAll(o, ['own_key', 'zz_proxy_chain'].map(fresh)) + ' ' + log.join()
})
show('global', () => { globalThis.zz_global_name = 'gl'; const r = getAll(globalThis, ['zz_global_name', 'zz_no_global'].map(fresh)); delete globalThis.zz_global_name; return r })
show('wide', () => {
  const o = { 中文: 'cn', é: 'e1' }
  const wideE = ('é中'.slice(0, 1))
  return getAll(o, [fresh('中文'), fresh('中文字'), wideE, fresh('é'), fresh('ée')])
})
show('long', () => {
  const k700 = 'k'.repeat(700), o = {}
  o[k700] = 'long'
  const ks = [fresh(k700), fresh('k'.repeat(701)), fresh('m'.repeat(70000))]
  const vals = ks.map(k => String(o[k]))
  o['m'.repeat(70000)] = 'very'
  return vals.join() + ',' + String(o[fresh('m'.repeat(70000))])
})
show('symbol named the same', () => {
  const s = Symbol.for('zz_symbol_name'), o = { [s]: 'sym' }
  return [get(o, fresh('zz_symbol_name')), get(o, s)].map(String).join()
})
show('keys of a template', () => {
  const scopes = { __scopeDialog: 'dialog' }
  const r = []
  for (const t of ['Dialog', 'Popover', 'Dialog']) r.push(String(scopes[`__scope${t}`]))
  return r.join()
})
show('class names', () => {
  const registered = { 'css-abc123': 'color:red;' }
  const raw = []
  for (const name of 'MuiButton-root css-abc123 MuiButton-text Mui-focused'.split(' ')) {
    if (registered[name] !== undefined) raw.push(registered[name])
  }
  return raw.join()
})

if (typeof memorySize === 'function') {
  const o = { a: 1 }, child2 = Object.create(o), arr = [1, 2]
  const work = () => {
    let n = 0
    for (let i = 0; i < 300; i++) {
      const k = 'zz_heap_' + (i % 50) + '_' + 'x'.repeat(i % 7)
      if (o[k] === undefined) n++
      if (child2[k] === undefined) n++
      if (arr[k] === undefined) n++
      if (o[fresh('a')] === 1) n++
    }
    return n
  }
  work(); work(); gc()
  work(); work(); work(); gc(); gc()
  const m1 = memorySize(), k1 = memoryBlocks()
  work(); work(); work(); gc(); gc()
  out.push('memory after: ' + heapChange(m1, k1))
} else {
  out.push('memory after: 0')
}
out.join('\n')
