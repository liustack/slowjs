// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// JSON.parse where it reads keys and plain strings straight from the text
// (json_scan_key, json_key_atom and json_plain_run in quickjs.c) and where it
// does not: keys that are array indexes or not, repeated keys, __proto__,
// keys with escapes or characters of more bytes, whitespace around the colon,
// strings that end on every byte of a word, a reviver, tables of rows with the
// same keys, keys in another order, keys missing or added, more keys than the
// cache holds and keys that share a slot of it, nesting of the same shape,
// and texts longer than a step of the interrupt flag. Errors give only their
// name here, as Node words them differently. The file is what Node 24 prints.
const out = []
function esc(s) {
  let r = ''
  for (const ch of String(s)) {
    const c = ch.codePointAt(0)
    r += c >= 0x20 && c < 0x7f ? ch : '\\u{' + c.toString(16) + '}'
  }
  return r
}
// the own keys of a value, its prototype and its values, all the way down
function dump(v) {
  if (v === null || typeof v !== 'object') return typeof v === 'string' ? '"' + esc(v) + '"' : typeof v + ':' + String(v)
  if (Array.isArray(v)) return '[' + v.map(dump).join(',') + ']'
  const proto = Object.getPrototypeOf(v) === Object.prototype ? '' : '(proto?)'
  return proto + '{' + Reflect.ownKeys(v).map(k => esc(k) + '=' + dump(v[k])).join(',') + '}'
}
// a short hash of a long dump
function hash(s) {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619) >>> 0
  return s.length + '/' + h.toString(16)
}
function run(name, f) {
  let r
  try {
    r = f()
  } catch (e) {
    r = e instanceof Error ? e.constructor.name : 'thrown ' + String(e)
  }
  out.push(name + ': ' + r)
}
const parse = (t, rev) => dump(JSON.parse(t, rev))

// keys that are array indexes, and keys that look like numbers but are not
run('index keys', () => parse('{"b":1,"2":2,"a":3,"1":4,"0":5,"10":6,"01":7,"-0":8,"1.5":9,"":10," 1":11,"1 ":12}'))
run('large index keys', () => parse('{"x":0,"4294967294":1,"4294967295":2,"4294967296":3,"2147483647":4,"2147483648":5,"99999999999":6}'))
run('index keys in rows', () => parse('[{"1":"a","id":"x","0":"b"},{"1":"c","id":"y","0":"d"},{"id":"z","0":"e","1":"f"}]'))
run('index key read', () => { const o = JSON.parse('{"0":"a","123":"b","4294967294":"c","-0":"d","01":"e"}'); return [o[0], o[123], o[4294967294], o['-0'], o['01'], o[1]].join() })

// repeated keys: the last value wins and the key keeps its first place
run('repeated', () => parse('{"a":1,"b":2,"a":3}'))
run('repeated escaped', () => parse('{"name":1,"n\\u0061me":2,"x":3,"name":4}'))
run('repeated index', () => parse('{"1":1,"a":2,"1":3,"0":4}'))
run('repeated in rows', () => parse('[{"a":1,"b":2,"a":3},{"a":4,"b":5},{"b":6,"a":7,"b":8}]'))
run('repeated object value', () => parse('{"a":{"x":1},"a":{"y":2},"b":{"x":3}}'))

// __proto__ is an own property, and the prototype stays Object.prototype
run('proto', () => { const o = JSON.parse('{"__proto__":{"x":1},"y":2}'); return [dump(o), Object.hasOwn(o, '__proto__'), o.x, Object.getPrototypeOf(o) === Object.prototype].join(' ') })
run('proto escaped', () => { const o = JSON.parse('{"__proto\\u005f_":[1],"a":1}'); return [dump(o), Object.getPrototypeOf(o) === Object.prototype].join(' ') })
run('proto in rows', () => { const a = JSON.parse('[{"__proto__":1,"a":2},{"__proto__":{"b":3},"a":4},{"a":5,"__proto__":null}]'); return [dump(a), a.every(o => Object.getPrototypeOf(o) === Object.prototype)].join(' ') })
run('constructor keys', () => parse('{"constructor":1,"toString":2,"hasOwnProperty":3,"valueOf":{"length":4}}'))

// keys with escapes, characters of more bytes and lone surrogates
run('escaped keys', () => parse('{"\\u0041":1,"\\"":2,"a\\\\b":3,"\\/":4,"\\n\\t":5,"a\\u0000b":6}'))
run('wide keys', () => parse('{"\u00e9":1,"\u4e2d\u6587":2,"a\u00e9":3,"\ud83d\ude00":4,"\\ud83d\\ude00":5}'))
run('surrogate keys', () => parse('{"\\ud800":1,"\\udc00":2,"\\ud800x":3,"x\\udfff":4,"\\ud800\\ud800":5}'))
run('surrogate keys equal', () => { const o = JSON.parse('{"\\ud83d\\ude00":1,"\ud83d\ude00":2}'); return Object.keys(o).length + ' ' + o['\ud83d\ude00'] })
run('keys of every plain byte', () => { let k = ''; for (let c = 0x20; c < 0x7f; c++) if (c !== 0x22 && c !== 0x5c) k += String.fromCharCode(c); const o = JSON.parse('{"' + k + '":1,"' + k.slice(1) + '":2,"' + k.slice(0, -1) + '":3}'); return Object.keys(o).map(esc).join('|') })
run('del in key', () => parse('{"a\u007fb":1,"\u007f":2}'))

// whitespace around the colon and between members
run('whitespace', () => parse('{ "a" : 1 , "b"\n:\t2,\r\n"c"\r:3,"d":\n\n4 }'))
run('no colon', () => parse('{"a" 1}'))
run('two colons', () => parse('{"a"::1}'))
run('colon only', () => parse('{"a":}'))
run('comma at the end', () => parse('{"a":1,}'))
run('key not closed', () => parse('{"abc'))
run('key at the end', () => parse('{"abc"'))
run('key and colon at the end', () => parse('{"abc":'))
run('control in key', () => parse('{"a\u0001":1}'))
run('tab in key', () => parse('{"a\tb":1}'))
run('bad escape in key', () => parse('{"a\\x":1}'))
run('short escape in key', () => parse('{"a\\u00":1}'))
run('bare key', () => parse('{a:1}'))
run('single quoted key', () => parse("{'a':1}"))
run('number key', () => parse('{1:1}'))
run('missing comma', () => parse('{"a":1 "b":2}'))
run('data after', () => parse('{"a":1}x'))
run('vertical tab', () => parse('{"a":1,\u000b"b":2}'))

// strings that end on every byte of a word, and that go on with an escape or a character of more bytes
run('string lengths', () => { const r = []; for (let n = 0; n <= 20; n++) { const s = 'abcdefghijklmnopqrstu'.slice(0, n); const v = JSON.parse('["' + s + '","' + s + '\\n","' + s + '\u00e9","' + s + '\u4e2d"]'); r.push(v.map(x => x.length).join('.')) } return r.join(' ') })
run('string boundaries', () => parse('[" ","!","#","[","]","~","\u007f","a\\"b","\\\\"]'))
run('empty strings', () => parse('{"":"","a":"","":"x"}'))
run('string not closed', () => parse('["abcdefghijklmnop'))
run('control in string', () => parse('["abcdefghij\u001fk"]'))

// a reviver sees the keys in order, with the source of the primitives
run('reviver', () => {
  const seen = []
  const v = JSON.parse('[{"id":"a","n":1,"0":true},{"id":"b","n":2,"0":false}]', function (k, v) {
    seen.push(esc(k) + (typeof v !== 'object' ? '=' + String(v) : ''))
    return typeof v === 'number' ? v * 10 : v
  })
  return dump(v) + ' ' + seen.join(',')
})
run('reviver deletes', () => parse('{"a":1,"b":2,"c":{"a":3,"b":4}}', (k, v) => k === 'a' ? undefined : v))
run('reviver repeated', () => parse('{"a":1,"a":2,"b":3}', (k, v) => typeof v === 'number' ? v + 100 : v))

// tables: rows of the same keys, in another order, with keys missing or added
function table(n, row) {
  const parts = []
  for (let i = 0; i < n; i++) parts.push(row(i))
  return '[' + parts.join(',') + ']'
}
const t1 = table(2000, i => '{"id":"' + i.toString(16).padStart(8, '0') + '-aaaa-bbbb-cccc","name":"n' + i + '","price":' + i * 1.5 + ',"ok":' + (i % 2 === 0) + ',"tag":null}')
run('table', () => { const a = JSON.parse(t1); return a.length + ' ' + dump(a[0]) + ' ' + dump(a[1999]) + ' ' + hash(dump(a)) })
const t2 = table(1000, i => { const k = ['"a":' + i, '"bb":"' + i + '"', '"ccc":[' + i + ']', '"dddd":{"x":' + i + '}']; for (let j = 0; j < i % 4; j++) k.push(k.shift()); return '{' + k.join(',') + '}' })
run('table in turns', () => { const a = JSON.parse(t2); return dump(a.slice(0, 5)) + ' ' + hash(dump(a)) })
const t3 = table(1000, i => { const k = []; if (i % 3) k.push('"id":' + i); k.push('"name":"r' + i + '"'); if (i % 5 === 0) k.push('"extra' + (i % 7) + '":' + i); if (i % 7 === 0) k.push('"id":"again"'); return '{' + k.join(',') + '}' })
run('table with gaps', () => { const a = JSON.parse(t3); return dump(a.slice(0, 8)) + ' ' + hash(dump(a)) })
const t4 = table(300, i => { const k = []; for (let j = 0; j < 100; j++) k.push('"k' + ((i + j * 7) % 150) + '":' + j); return '{' + k.join(',') + '}' })
run('table of many keys', () => { const a = JSON.parse(t4); return Object.keys(a[0]).slice(0, 12).join() + ' ' + hash(dump(a)) })
const t5 = table(500, i => i % 2 ? '{"abcde":' + i + ',"abcxe":"' + i + '","abcde":-' + i + '}' : '{"abcxe":' + i + ',"abcde":"' + i + '"}')
run('table of keys that share a slot', () => { const a = JSON.parse(t5); return dump(a.slice(0, 4)) + ' ' + hash(dump(a)) })
const t6 = table(400, i => '{"node":{"node":{"node":{"leaf":' + i + ',"kids":[{"node":null,"leaf":' + (i + 1) + '}]}}},"leaf":"' + i + '"}')
run('nesting of the same shape', () => { const a = JSON.parse(t6); return dump(a[399]) + ' ' + hash(dump(a)) })
run('deep nesting of the same shape', () => { let t = '1'; for (let i = 0; i < 400; i++) t = '{"a":' + t + ',"b":' + i + '}'; let o = JSON.parse(t), d = 0; while (typeof o === 'object') { o = o.a; d++ } return d + ' ' + o })
run('deep arrays', () => { const t = '['.repeat(400) + '"x"' + ']'.repeat(400); let o = JSON.parse(t), d = 0; while (Array.isArray(o)) { o = o[0]; d++ } return d + ' ' + o })
const t7 = table(1000, i => '{"id":"' + i + '","\u540d\u524d":"\u5c71\u7530' + i + '","note":"caf\u00e9 ' + i + '","emoji":"\ud83d\ude00' + i + '"}')
run('table of wide text', () => { const a = JSON.parse(t7); return dump(a[7]) + ' ' + hash(dump(a)) })
const t8 = table(1000, i => '{"id":' + i + ',"name":"\u00e9t\u00e9 ' + i + '","n":"a' + i + '"}')
run('table of latin-1 text', () => { const a = JSON.parse(t8); return dump(a[7]) + ' ' + hash(dump(a)) })
run('table reviver', () => { let n = 0; const a = JSON.parse(t1, (k, v) => (n++, v)); return n + ' ' + hash(dump(a)) })
run('table error at the end', () => parse(t1.slice(0, -2)))

// texts longer than a step of 65536 characters, of 8 bits with and without
// characters above 0x7f, and of 16 bits, with strings across the steps
function long(fill) {
  const parts = []
  for (let i = 0; i < 9000; i++) parts.push('{"k":"' + fill(i) + '","i":' + i + '}')
  return '[' + parts.join(',') + ']'
}
for (const [name, fill] of [['ascii', i => 'x'.repeat(i % 23)], ['latin-1', i => 'x'.repeat(i % 23) + (i % 3 ? '' : '\u00e9')], ['wide', i => 'x'.repeat(i % 23) + (i % 3 ? '' : '\u4e2d')]]) {
  const t = long(fill)
  run('long ' + name, () => { const a = JSON.parse(t); return t.length + ' ' + a.length + ' ' + hash(dump(a)) })
}
run('long string', () => { const s = 'ab'.repeat(40000) + '\u00e9' + 'cd'.repeat(40000); const v = JSON.parse('{"' + s + '":"' + s + '","k":1}'); return Object.keys(v).map(k => k.length).join() + ' ' + hash(v[s]) })

// JSON.rawJSON parses its text the same way
run('rawJSON', () => [JSON.stringify({ a: JSON.rawJSON('"abc"'), b: JSON.rawJSON('12') }), JSON.isRawJSON(JSON.rawJSON('"x"'))].join(' '))

;(() => out.join('\n'))
