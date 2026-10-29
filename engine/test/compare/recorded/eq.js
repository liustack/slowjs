// ===, !==, == and != on every pair of kinds of values, each made fresh for
// each comparison so that both operands are freed, and on one value beside
// itself. The strings come as atoms, as built strings, as ropes, as wide
// strings and as the descriptions of symbols, which are atoms of another
// type with the text of a string atom.
var obj = {}, sym = Symbol('s'), fn = function () {};
var make = [
  function () { return undefined; },
  function () { return null; },
  function () { return true; },
  function () { return false; },
  function () { return 0; },
  function () { return -0; },
  function () { return 1; },
  function () { return 1.5; },
  function () { return NaN; },
  function () { return 0n; },
  function () { return 1n; },
  function () { return 2n ** 70n; },
  function () { return ''; },
  function () { return 'foo'; },
  function () { return 'fo' + 'o'.repeat(1); },
  function () { return ['f', 'o', 'o'].join(''); },
  function () { return 'bar'; },
  function () { return 'fooo'.slice(0, 3); },
  function () { return '1'; },
  function () { return '1.5'; },
  function () { return 'true'; },
  function () { return '[object Object]'; },
  function () { return 'x'.repeat(300) + 'foo'; },
  function () { return 'x'.repeat(300) + 'fo' + 'o'; },
  function () { return 'Āfoo'.slice(1); },
  function () { return 'é漢'; },
  function () { return 'é' + '漢'; },
  function () { return Symbol('foo').description; },
  function () { return Symbol.for('foo').description; },
  function () { return Object.keys({ foo: 1 })[0]; },
  function () { return JSON.parse('"foo"'); },
  function () { return typeof fn; },
  function () { return 'function'; },
  function () { return sym; },
  function () { return Symbol('s'); },
  function () { return Symbol.iterator; },
  function () { return obj; },
  function () { return {}; },
  function () { return [1]; },
  function () { return [1, 5]; },
  function () { return fn; },
  function () { return { valueOf: function () { return 1; } }; },
  function () { return { toString: function () { return 'foo'; } }; },
  function () { return new String('foo'); },
  function () { return Object(sym); },
];
var out = [];
function b(x) { return x ? 'T' : 'F'; }
for (var i = 0; i < make.length; i++) {
  var row = [];
  for (var j = 0; j < make.length; j++) {
    row.push(b(make[i]() === make[j]()) + b(make[i]() !== make[j]()) + b(make[i]() == make[j]()) + b(make[i]() != make[j]()));
  }
  var v = make[i]();
  row.push(b(v === v) + b(v !== v) + b(v == v) + b(v != v));
  out.push(i + ' ' + row.join(' '));
}
// The forms React and its bundles use, in loops, with the result kept in
// variables and in branches.
var n = 0, kinds = [null, undefined, 0, '', 'div', 'span', fn, obj, sym, true];
for (var k = 0; k < 200; k++) {
  var x = kinds[k % kinds.length], y = kinds[(k * 7) % kinds.length];
  if (null != x) n += 1;
  if (void 0 === x) n += 2;
  if (x !== null && typeof x === 'object') n += 4;
  if (typeof x === 'function') n += 8;
  if (x === y) n += 16;
  if (x == y) n += 32;
  switch (x) {
    case 'div': n += 64; break;
    case 'span': n += 128; break;
    case sym: n += 256; break;
    default: n += 512;
  }
  var t = x !== y, u = x != y;
  n += (t ? 1024 : 0) + (u ? 2048 : 0);
}
out.push('react ' + n);
out.join('\n');
