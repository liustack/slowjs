// The cold paths of js_strict_eq2 and __JS_AtomToValue, which take a TLS
// frame, and the paths beside them, which do not: ===, Object.is and
// SameValueZero on every kind of value, BigInts short and long among them,
// and the names of integer, string and symbol keys.
var big = [0n, 1n, -1n, 2n ** 40n, -(2n ** 40n), 2n ** 64n, 2n ** 64n + 1n, -(2n ** 64n), 2n ** 200n, BigInt(Number.MAX_SAFE_INTEGER)];
var vals = big.concat([0, -0, 1, 1.5, NaN, Infinity, 'a', 'ab', 'a' + 'b', '', true, false, null, undefined, Symbol.iterator, {}, [], 2 ** 40]);
var out = [];
for (var i = 0; i < vals.length; i++) {
  for (var j = 0; j < vals.length; j++) {
    var a = vals[i], b = vals[j];
    out.push((a === b ? 1 : 0) + '' + (Object.is(a, b) ? 1 : 0) + ([a].includes(b) ? 1 : 0) + (new Map([[a, 1]]).has(b) ? 1 : 0) + (a == b ? 1 : 0));
  }
}
var o = {};
for (var k of [0, 1, 7, 42, 4294967294, 4294967295, 2 ** 53, -1, 'x', '01', '1.5']) o[k] = k;
o[Symbol('s')] = 1;
var keys = [Object.keys(o).join(), Reflect.ownKeys(o).map(String).join(), JSON.stringify(o)];
var arr = [];
for (var n = 0; n < 1000; n += 37) arr[n] = n;
for (var k in arr) keys.push(k);
keys.push(Object.getOwnPropertyNames('héllo').join(), Object.entries([5, 6, 7]).join(';'));
out.join(' ') + '\n' + keys.join('\n');
