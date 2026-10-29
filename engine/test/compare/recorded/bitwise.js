// The bitwise operators on numbers of every range, int and float, with the
// other kinds of operands beside them, as each operator and as a compound
// assignment. The float cases are those the interpreter used to send to
// js_binary_logic_slow and js_shr_slow.
var nums = [0, -0, 1, -1, 5, 31, 32, 33, 0x7fffffff, -0x80000000, 0.5, -0.5, 1.5, -1.5, 2.9, -2.9,
  2 ** 31, -(2 ** 31) - 1, 2 ** 32, 2 ** 32 + 5, -(2 ** 32) - 5, 2 ** 32 - 0.5, 4294967295.9,
  2 ** 52 + 3, 2 ** 53, 2 ** 53 + 2, -(2 ** 53), 2 ** 62 + 2 ** 40, 2 ** 63, 2 ** 64 + 2 ** 33, 2 ** 84 + 2 ** 33,
  2 ** 85, 1e20, -1e20, 1e300, 1.7976931348623157e308, 5e-324, -5e-324, NaN, Infinity, -Infinity,
  0x5bd1e995 * 0x12345, 3432918353 * 0x7fff, 461845907 * 4294967295];
var others = ['7', '1.5', '', 'x', true, false, null, undefined, {}, { valueOf: function () { return 2 ** 33 + 3; } }, [5]];
var big = [0n, 1n, -1n, 5n, 2n ** 70n];
var out = [];
function row(a, b) {
  var r = [];
  try { r.push(a & b, a | b, a ^ b, a << b, a >> b); } catch (e) { r.push(e.name); }
  try { r.push(a >>> b); } catch (e) { r.push(e.name); }
  var c = a; try { c &= b; c |= b; c ^= 3; c <<= b; c >>= 1; } catch (e) { c = e.name; }
  r.push(c);
  return r.map(function (x) { return typeof x === 'number' && Object.is(x, -0) ? '-0' : String(x); }).join(' ');
}
for (var i = 0; i < nums.length; i++)
  for (var j = 0; j < nums.length; j++) out.push(row(nums[i], nums[j]));
for (var i = 0; i < nums.length; i += 3)
  for (var j = 0; j < others.length; j++) out.push(row(nums[i], others[j]), row(others[j], nums[i]));
for (var i = 0; i < big.length; i++) {
  for (var j = 0; j < big.length; j++) out.push(row(big[i], big[j]));
  out.push(row(big[i], 1.5), row(2.5, big[i]));
}
// murmur2 as @emotion/hash writes it: products past int32 become floats.
function murmur2(str) {
  var h = 0, k, i = 0, len = str.length;
  for (; len >= 4; ++i, len -= 4) {
    k = str.charCodeAt(i) & 0xff | (str.charCodeAt(++i) & 0xff) << 8 | (str.charCodeAt(++i) & 0xff) << 16 | (str.charCodeAt(++i) & 0xff) << 24;
    k = (k & 0xffff) * 0x5bd1e995 + ((k >>> 16) * 0xe995 << 16);
    k ^= k >>> 24;
    h = (k & 0xffff) * 0x5bd1e995 + ((k >>> 16) * 0xe995 << 16) ^ (h & 0xffff) * 0x5bd1e995 + ((h >>> 16) * 0xe995 << 16);
  }
  switch (len) {
    case 3: h ^= (str.charCodeAt(i + 2) & 0xff) << 16;
    case 2: h ^= (str.charCodeAt(i + 1) & 0xff) << 8;
    case 1: h ^= str.charCodeAt(i) & 0xff; h = (h & 0xffff) * 0x5bd1e995 + ((h >>> 16) * 0xe995 << 16);
  }
  h ^= h >>> 13;
  h = (h & 0xffff) * 0x5bd1e995 + ((h >>> 16) * 0xe995 << 16);
  return ((h ^ h >>> 15) >>> 0).toString(36);
}
out.push(['', 'a', 'display:flex;', 'color:red;background:blue;padding:8px 16px;', 'é漢字😀'].map(murmur2).join(' '));
out.join('\n');
