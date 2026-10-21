// Regexps made from the same pattern and flags again and again, through every
// way that compiles one: a literal (through eval, which parses it), new RegExp
// and RegExp of a string, new RegExp of a regexp with new flags,
// RegExp.prototype.compile, and split, which makes a sticky copy of its
// regexp. Each is made three times, the last after enough other regexps that
// a cache of a few hundred has dropped it, and every object is used on its
// own, so that a lastIndex of one shows up as wrong results of another. The
// last value is the transcript: its line count and hash, then one line for
// each result.
var pats = [
  ['a+b', ''], ['a+b', 'g'], ['a+b', 'y'], ['a+b', 'gy'], ['a+b', 'i'], ['a+b', 'gi'], ['a+b', 'ig'], ['a+b', undefined],
  ['(\\d{4})-(\\d\\d)', 'g'], ['(?<year>\\d{4})-(?<month>\\d\\d)', ''], ['(?<year>\\d{4})-(?<month>\\d\\d)', 'gd'],
  ['(?<x>a)|(?<y>b)', 'gd'], ['(a)|(b)', 'd'], ['(?<=\\$)\\d+', 'g'], ['(?<!\\$)\\b\\d+', 'g'], ['\\bfoo\\b', 'gm'],
  ['^\\s+|\\s+$', 'g'], ['[\\p{L}--[a-z]]', 'gv'], ['[[a-z]&&[aeiou]]+', 'v'], ['\\p{Lu}\\p{Ll}*', 'gu'],
  ['\\u{1F600}', 'u'], ['[😀-🙏]', 'gu'], ['.', 'gs'], ['.', 'g'], ['中+', 'g'], ['[一-龥]+', ''], ['é', 'i'],
  ['\ud83d', 'g'], ['\ud83d', 'gu'], ['/', ''], ['a/b', 'g'], ['\n', ''], ['', ''], ['', 'g'], ['(?:)', ''],
  ['[', ''], ['(', 'g'], ['a', 'gg'], ['a', 'uv'], ['a', 'x'], ['\\p{Foo}', 'u'], ['a{2,1}', ''], ['(?<n>a)(?<n>b)', ''],
  [',\\s*', ''], ['\\s+', 'g'], ['(-)', ''], ['x*', 'g'], ['k', 'iu'], ['[^]', 'g'], ['\\cJ', ''], ['\\0', 'u'],
];
var strs = ['', 'aab xaab AAB', '2024-06 and $1999-12', 'foo bar foo\nfoo', '  Hello Wörld ÀB ', 'beautiful', '😀 grin \ud83d', '中中文 é É', 'a/b, c-d'];
var out = [];
function rec(x) { out.push(typeof x === 'string' ? x : JSON.stringify(x)); }
function safe(f) { try { return f(); } catch (e) { return 'ERR ' + e.name; } }
function desc(r) {
  return [r.source, r.flags, r.global, r.ignoreCase, r.multiline, r.dotAll, r.unicode, r.unicodeSets, r.sticky, r.hasIndices, r.lastIndex, String(r)];
}
function match(m) {
  if (m === null) return null;
  return [Array.from(m), m.index, m.groups === undefined ? 'nogroups' : Object.entries(m.groups), m.indices === undefined ? 'noindices' : [Array.from(m.indices), m.indices.groups === undefined ? 'nogroups' : Object.entries(m.indices.groups)]];
}
function literal(src, flags) {
  // A literal cannot hold a line break or an empty pattern.
  if (src === '' || src.indexOf('\n') >= 0) return new RegExp(src, flags);
  return (0, eval)('/' + src.replace(/\//g, '\\/') + '/' + (flags === undefined ? '' : flags));
}
var makers = [
  ['literal', literal],
  ['new', function (s, f) { return new RegExp(s, f); }],
  ['call', function (s, f) { return RegExp(s, f); }],
  ['copy', function (s, f) { return new RegExp(new RegExp(s), f); }],
  ['compile', function (s, f) { return /zz/g.compile(s, f); }],
];
function use(name, r) {
  rec([name, desc(r)]);
  for (var s of strs) {
    var a = safe(() => r.test(s)), li = r.lastIndex;
    r.lastIndex = 0;
    rec([s.length, a, li, safe(() => match(r.exec(s))), r.lastIndex, safe(() => match(r.exec(s))), r.lastIndex]);
    r.lastIndex = 0;
    rec([safe(() => s.search(r)), safe(() => s.match(r)), safe(() => s.replace(r, '<$&>')), safe(() => s.split(r, 3))]);
    if (r.global) rec(safe(() => Array.from(s.matchAll(r), match)));
    r.lastIndex = 0;
  }
}
function round(tag) {
  for (var [src, flags] of pats) {
    for (var [how, make] of makers) {
      var name = tag + ' ' + how + ' ' + JSON.stringify(src) + ' ' + flags;
      var r = safe(() => make(src, flags));
      if (typeof r === 'string') { rec([name, r]); continue; }
      // Two objects of the same pattern and flags, used in turn: each keeps
      // its own lastIndex.
      var r2 = make(src, flags);
      if (r.global || r.sticky) {
        rec([name, 'pair', safe(() => [match(r.exec(strs[1])), r.lastIndex, r2.lastIndex, match(r2.exec(strs[1])), match(r.exec(strs[1])), r.lastIndex, r2.lastIndex])]);
        r.lastIndex = 0;
      }
      use(name, r);
    }
  }
}
round('first');
round('again');
// Enough other regexps that a cache of a few hundred entries has dropped
// every one of pats.
for (var i = 0; i < 1500; i++) new RegExp('filler' + i + '(x|y)+', i % 2 ? 'g' : '');
round('later');
// Flags and patterns that are not strings are converted every time.
var calls = 0;
var flagsObj = { toString() { calls++; return 'g'; } };
var patObj = { toString() { calls++; return 'b+'; } };
for (var i = 0; i < 3; i++) rec([desc(new RegExp('a', flagsObj)), desc(new RegExp(patObj, 'g')), desc(new RegExp(new String('c'), new String('i'))), calls]);
// The same pattern as a string of 8-bit and of 16-bit characters, and as a
// string built at run time.
var wide = 'aé中'.slice(0, 2);
rec([desc(new RegExp('aé')), desc(new RegExp(wide)), new RegExp(wide).test('xaé'), desc(new RegExp(['a', 'é'].join('')))]);
// A regexp that compile changes keeps working, and the regexps made before
// from the same pattern are not changed.
var r1 = new RegExp('q+', 'g'), r2 = new RegExp('q+', 'g');
r2.compile('w+', 'i');
rec([desc(r1), desc(r2), r1.test('qq'), r2.test('WW'), desc(new RegExp('q+', 'g'))]);
// A subclass and a species constructor get the same bytecode.
class R extends RegExp {}
var sub = new R('o+', 'g');
rec([desc(sub), 'foo boo'.replace(sub, '0'), 'foo boo'.split(sub), sub instanceof R]);
var h = 0;
for (var line of out) for (var i = 0; i < line.length; i++) h = (h * 31 + line.charCodeAt(i)) | 0;
out.length + ' lines, hash ' + h + '\n' + out.join('\n');
