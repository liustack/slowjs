// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// replace and replaceAll with a function, and match, with a regexp of the realm
// that nothing changed, against the steps of the specification written out
// with exec: every match collected first, then the function called for each
// with the match, the captures (undefined for one that did not match), the
// position, the string and the groups, and lastIndex where exec leaves it.
// Global, sticky and plain regexps, lastIndex at 0 and past it, empty matches
// stepped over one character or one code point with u and v, surrogate pairs
// and lone surrogates, lookbehinds, named groups, the d flag, strings of 8 and
// 16 bits, and the format strings date-fns and dayjs replace. A line gives the
// regexp, how many calls differ from the steps, and a hash of the results and
// the arguments. The file is what Node 24 prints.
var out = [];

function advance(s, i, unicode) {
  if (!unicode || i + 1 >= s.length) return i + 1;
  var c = s.charCodeAt(i);
  if (c < 0xd800 || c > 0xdbff) return i + 1;
  c = s.charCodeAt(i + 1);
  return c >= 0xdc00 && c <= 0xdfff ? i + 2 : i + 1;
}
function show(v) {
  if (v === undefined) return '~';
  if (typeof v === 'string') return JSON.stringify(v);
  if (v === null) return 'null';
  if (typeof v === 'object') {
    if (Array.isArray(v)) {
      var s = '[' + v.map(show).join(',') + ']';
      if ('index' in v) s += '@' + v.index + ' in ' + v.input.length;
      if (v.groups) s += ' g' + show(v.groups);
      if (v.indices) s += ' d' + JSON.stringify(v.indices) + (v.indices.groups ? JSON.stringify(v.indices.groups) : '');
      return s;
    }
    return '{' + Object.keys(v).map(function (k) { return k + ':' + show(v[k]); }).join(',') + '}' +
      (Object.getPrototypeOf(v) === null ? 'n' : '');
  }
  return String(v);
}
var calls;
function fn() {
  calls.push(Array.prototype.map.call(arguments, show).join(','));
  return '<' + arguments[0].length + '>';
}
// RegExp.prototype[Symbol.replace] with a function, step by step
function replaceSteps(rx, S, f) {
  var flags = rx.flags, global = flags.indexOf('g') >= 0;
  var fullUnicode = flags.indexOf('u') >= 0 || flags.indexOf('v') >= 0;
  if (global) rx.lastIndex = 0;
  var results = [], r;
  for (;;) {
    r = rx.exec(S);
    if (r === null) break;
    results.push(r);
    if (!global) break;
    if (String(r[0]) === '') rx.lastIndex = advance(S, rx.lastIndex, fullUnicode);
  }
  var acc = '', next = 0;
  for (var i = 0; i < results.length; i++) {
    r = results[i];
    var matched = String(r[0]), pos = Math.max(Math.min(r.index, S.length), 0), args = [matched];
    for (var n = 1; n < r.length; n++) args.push(r[n] === undefined ? undefined : String(r[n]));
    args.push(pos, S);
    if (r.groups !== undefined) args.push(r.groups);
    var rep = String(f.apply(undefined, args));
    if (pos >= next) {
      acc += S.substring(next, pos) + rep;
      next = pos + matched.length;
    }
  }
  return acc + S.substring(next);
}
// RegExp.prototype[Symbol.match], step by step
function matchSteps(rx, S) {
  var flags = rx.flags;
  if (flags.indexOf('g') < 0) return rx.exec(S);
  var fullUnicode = flags.indexOf('u') >= 0 || flags.indexOf('v') >= 0;
  rx.lastIndex = 0;
  var A = [], r;
  for (;;) {
    r = rx.exec(S);
    if (r === null) return A.length ? A : null;
    var m = String(r[0]);
    A.push(m);
    if (m === '') rx.lastIndex = advance(S, rx.lastIndex, fullUnicode);
  }
}
function hash(h, s) {
  for (var i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0;
  return h;
}

var res = [
  /a/, /a/g, /a/y, /a/gy, /(a)|(b)/g, /(a)?b/g, /a(b)?/, /x*/g, /(?:)/g, /(?:)/gu, /(?:)/gv, /(?:)/y, /(?:)/gy, /$/g,
  /^/gm, /\b/g, /(?=a)/g, /(?<=a)/g, /(?<=a)b/g, /(?<!a)b/gy, /./gu, /./g, /./gs, /\ud83d/g, /\ude00/gu, /😀/gu,
  /(?<=\ud83d)/g, /(?<=\ud83d)/gu, /[\ud800-\udfff]/g, /(?<x>a)(?<y>b)?/g, /(?<x>a)|(?<x>b)/g, /(a)(?:b|(c))/dg,
  /(?<x>a)/d, /A/gi, /é/gi, /中/g, /(\w)\1*/g, /(\w)\1*|''|'(''|[^'])+('|$)|./g,
  /\[([^\]]+)]|Y{1,4}|M{1,4}|D{1,2}|d{1,4}|H{1,2}|h{1,2}|a|A|m{1,2}|s{1,2}|Z{1,2}|SSS/g,
  /(\[[^\]]+])|(MM?M?M?|Do|DD?|ddd?d?|w[o|w]?|YYYY|YY|a|A|hh?|HH?|mm?|ss?|S{1,3}|x|X|ZZ?)/g, /\{\{(.+?)\}\}/g,
  /(a)|b/, /(a)|b/y, /\s+/g, /(\n|\r\n)/g,
];
var strs = [
  '', 'a', 'b', 'ab', 'aab', 'abab', 'ba', 'xaby ab a', 'aaa', 'abcabc', ' a  b ', 'a\nb\r\nc', 'é', 'aÉé中a',
  '😀', 'a😀b', '😀😀', '😀\ud83d', '\ude00😀', 'a\ud800b\udc00', "yyyy-MM-dd'T'HH:mm 'o''clock' a",
  'YYYY-MM-DD HH:mm:ss [at] Do', 'MMMM D, YYYY h:mm A', '{{a}} and {{ b }}', 'xxa', 'aabbcc',
];
var seed = 777;
function next(n) {
  seed = (Math.imul(seed, 1103515245) + 12345) | 0;
  return ((seed >>> 16) & 0x7fff) % n;
}
var alphabet = ['a', 'b', 'c', ' ', '\n', 'é', '中', '😀', '\ud83d', '\ude00', 'A', "'", 'Y', 'M'];
for (var k = 0; k < 100; k++) {
  var n = next(9), s = '';
  for (var j = 0; j < n; j++) s += alphabet[next(alphabet.length)];
  strs.push(s);
}

for (var ri = 0; ri < res.length; ri++) {
  var re = res[ri], differ = 0, h = 0, first = '';
  for (var si = 0; si < strs.length; si++) {
    var str = strs[si];
    for (var start = 0; start <= 2; start += 2) {
      var got, want, gotCalls, wantCalls, gotLast, wantLast;
      re.lastIndex = start;
      calls = [];
      got = re.global ? str.replaceAll(re, fn) : str.replace(re, fn);
      gotCalls = calls.join(';');
      gotLast = re.lastIndex;
      re.lastIndex = start;
      calls = [];
      want = replaceSteps(re, str, fn);
      wantCalls = calls.join(';');
      wantLast = re.lastIndex;
      var a = show(got) + ' ' + gotCalls + ' ' + gotLast, b = show(want) + ' ' + wantCalls + ' ' + wantLast;
      if (a !== b) {
        if (!differ) first = ' first replace ' + JSON.stringify(str) + ' ' + start + ' ' + a + ' want ' + b;
        differ++;
      }
      h = hash(h, a);
      re.lastIndex = start;
      got = show(str.match(re)) + ' ' + re.lastIndex;
      re.lastIndex = start;
      want = show(matchSteps(re, str)) + ' ' + re.lastIndex;
      if (got !== want) {
        if (!differ) first = ' first match ' + JSON.stringify(str) + ' ' + start + ' ' + got + ' want ' + want;
        differ++;
      }
      h = hash(h, got);
    }
  }
  out.push(re + ' differ ' + differ + ' hash ' + h + first);
}

// strings longer than a step of the scan, 8 and 16 bits
var long8 = 'x'.repeat(70000) + 'ab' + 'y'.repeat(70000) + 'a', long16 = '中'.repeat(70000) + 'ab' + 'é'.repeat(70000) + 'a';
[/a(b)?/g, /(?:)/gu, /y+|中+/g, /(?<=x)a/g].forEach(function (re) {
  [long8, long16].forEach(function (s) {
    calls = [];
    var r = s.replace(re, function (m, a, b) { calls.push(typeof a === 'number' ? a : b); return m.length > 3 ? '#' : m; });
    var m = s.match(re);
    out.push('long ' + re + ' ' + r.length + ' ' + calls.length + ' ' + calls.slice(0, 3) + ' ' + calls.slice(-2) +
      ' ' + (m ? m.length + ' ' + m.slice(0, 2).map(function (x) { return x.length; }) : 'null'));
  });
});

// some results written out
calls = [];
out.push(show('2026-11-15'.replace(/(\d+)-(\d+)-(\d+)/, fn)) + ' ' + calls.join(';'));
calls = [];
out.push(show('a😀b'.replace(/(?:)/gu, fn)) + ' ' + calls.join(';'));
calls = [];
out.push(show('a😀b'.replace(/(?:)/g, fn)) + ' ' + calls.join(';'));
calls = [];
out.push(show('ab'.replace(/(?<x>a)|(?<y>b)/g, fn)) + ' ' + calls.join(';'));
out.push(show('aXbX'.replace(/x/gi, function () { return '$&$1'; })));
out.push(show('abc'.replace(/b/g, function () { return { toString: function () { return 'T'; } }; })));
out.push(show('a1b22'.match(/\d/g)) + ' ' + show('a1b22'.match(/(\d)(\d)?/)) + ' ' + show('xyz'.match(/\d/g)));
out.push(show('a😀'.match(/(?:)/gu)) + ' ' + show('a😀'.match(/(?:)/g)));
out.push(out.length + ' lines');
out.join('\n');
