// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// typeof compared with a string, which the compiler turns into one opcode:
// ==, ===, != and !== with each name of a type and with strings that are
// none, and < and > with "u", as values and as the test of a branch, on a
// value of every type. The file is what the engine printed before the
// opcode, and with it turned off (compare.sh runs it both ways).
const out = []
const values = [
  ['undefined', undefined], ['null', null], ['true', true], ['0', 0], ['1.5', 1.5], ['NaN', NaN],
  ['10n', 10n], ['string', 'str'], ['empty string', ''], ['symbol', Symbol('s')], ['object', {}],
  ['array', []], ['function', function () {}], ['arrow', () => 1], ['class', class {}],
  ['async function', async function () {}], ['generator', function* () {}], ['proxy of a function', new Proxy(function () {}, {})],
  ['proxy of an object', new Proxy({}, {})], ['String object', new String('s')], ['Date', new Date(0)],
  ['regexp', /x/], ['bound', function () {}.bind(null)], ['Math', Math], ['globalThis', globalThis],
]
const tests = {
  'eq string': v => typeof v == 'string', 'seq string': v => typeof v === 'string',
  'ne string': v => typeof v != 'string', 'sne string': v => typeof v !== 'string',
  'eq object': v => typeof v == `object`, 'sne object': v => typeof v !== 'object',
  'eq number': v => typeof v == 'number', 'eq boolean': v => typeof v == 'boolean',
  'eq symbol': v => typeof v == 'symbol', 'sne symbol': v => typeof v !== 'symbol',
  'eq bigint': v => typeof v == 'bigint', 'eq undefined': v => typeof v == 'undefined',
  'sne undefined': v => typeof v !== 'undefined', 'eq function': v => typeof v == 'function',
  'ne function': v => typeof v != 'function',
  'lt u': v => typeof v < 'u', 'gt u': v => typeof v > 'u', 'lt u template': v => typeof v < `u`,
  'le u': v => typeof v <= 'u', 'lt v': v => typeof v < 'v', 'lt t': v => typeof v < 't',
  'eq not a type': v => typeof v == 'array', 'eq empty': v => typeof v == '', 'eq String': v => typeof v === 'String',
  'eq strin': v => typeof v == 'strin', 'eq number as a number': v => typeof v == 1,
  'if string': v => { if (typeof v == 'string') return 'yes'; return 'no' },
  'if not string': v => { if (typeof v != 'string') return 'yes'; return 'no' },
  'if not object': v => { if (typeof v !== 'object') return 'yes'; else return 'no' },
  'if lt u': v => { if (typeof v < 'u') return 'yes'; return 'no' },
  'if gt u': v => { if (typeof v > 'u') return 'yes'; return 'no' },
  'and': v => typeof v == 'object' && v !== null,
  'or': v => typeof v == 'string' || typeof v == 'number',
  'not': v => !(typeof v == 'string'),
  'ternary': v => typeof v != 'object' ? 'a' : 'b',
  'while': v => { let n = 0; while (typeof v != 'number' && n < 3) n++; return n },
  'member': v => typeof (v && v.constructor) == 'function',
  'reverse': v => 'string' == typeof v,
}
for (const [name, v] of values)
  out.push(name + ': ' + Object.entries(tests).map(([k, f]) => k + '=' + f(v)).join(' '))
// a global that does not exist, and one in its temporal dead zone
out.push('missing global: ' + (typeof notDefinedAnywhere < 'u') + ' ' + (typeof notDefinedAnywhere == 'undefined') + ' ' + (typeof notDefinedAnywhere != 'object'))
try { (() => { const r = typeof later == 'number'; let later = 1; return r })() } catch (e) { out.push('tdz: ' + e.constructor.name + ': ' + e.message) }
out.join('\n')
